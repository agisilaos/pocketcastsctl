package authn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

type Source string

const (
	SourceNone        Source = "none"
	SourceEnvironment Source = "environment"
	SourceKeychain    Source = "keychain"
	SourceLegacy      Source = "legacy_config"
)

var (
	ErrNotConfigured         = errors.New("API authentication is not configured")
	ErrCredentialUnavailable = errors.New("active Keychain session is unavailable")
)

type ManagerOptions struct {
	Store Store
	HTTP  *http.Client
	Now   func() time.Time
}

// Manager resolves credentials in deterministic precedence order and owns
// refresh-token rotation for one command process.
type Manager struct {
	mu      sync.Mutex
	cfg     config.Config
	store   Store
	api     *API
	now     func() time.Time
	loaded  bool
	loadErr error
	session Session
	source  Source
	warning string
}

func NewManager(cfg config.Config, opts ManagerOptions) *Manager {
	store := opts.Store
	if store == nil {
		store = NewKeyringStore()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	api := NewAPI(cfg.APIBaseURL, opts.HTTP)
	api.Now = now
	return &Manager{cfg: cfg, store: store, api: api, now: now}
}

func NewPocketCastsClient(cfg config.Config, opts ManagerOptions) (*pocketcasts.Client, *Manager) {
	manager := NewManager(cfg, opts)
	return pocketcasts.New(pocketcasts.Options{
		BaseURL:     cfg.APIBaseURL,
		Headers:     withoutAuthorization(cfg.APIHeaders),
		HTTP:        opts.HTTP,
		TokenSource: manager,
	}), manager
}

func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadLocked(ctx); err != nil {
		return "", err
	}
	if m.source == SourceKeychain && m.shouldRefreshLocked() {
		if err := m.refreshLocked(ctx); err != nil {
			return "", err
		}
	}
	if m.session.AccessToken == "" {
		return "", ErrNotConfigured
	}
	return m.session.AccessToken, nil
}

func (m *Manager) ForceRefresh(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadLocked(ctx); err != nil {
		return "", err
	}
	if m.source != SourceKeychain {
		return "", fmt.Errorf("%w: %s credentials", pocketcasts.ErrTokenNotRefreshable, m.source)
	}
	if err := m.refreshLocked(ctx); err != nil {
		return "", err
	}
	return m.session.AccessToken, nil
}

// Snapshot reads local credential state without performing network I/O or
// refreshing an expired access token.
func (m *Manager) Snapshot(ctx context.Context) (Session, Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadLocked(ctx); err != nil {
		return Session{}, SourceNone, err
	}
	return m.session, m.source, nil
}

func (m *Manager) Warning() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.warning
}

func (m *Manager) loadLocked(ctx context.Context) error {
	if m.loaded {
		return m.loadErr
	}
	m.loaded = true

	if token := authutil.NormalizeToken(os.Getenv(config.EnvAccessToken)); token != "" {
		m.session = Session{AccessToken: token, Method: "environment"}.normalized()
		m.source = SourceEnvironment
		return nil
	}

	if key := strings.TrimSpace(m.cfg.Auth.SessionKey); key != "" {
		session, err := m.store.Load(ctx, key)
		if err == nil {
			m.session = mergeMetadata(session, m.cfg.Auth)
			m.source = SourceKeychain
			return nil
		}
		m.loadErr = fmt.Errorf("%w: %v", ErrCredentialUnavailable, err)
		return m.loadErr
	}

	if token := legacyAuthorization(m.cfg.APIHeaders); token != "" {
		m.session = Session{AccessToken: token, Method: "legacy-config"}.normalized()
		m.source = SourceLegacy
		m.warning = "legacy plaintext Authorization config is deprecated; run `pocketcastsctl auth login` or `pocketcastsctl auth import-browser`"
		return nil
	}

	m.loadErr = ErrNotConfigured
	return m.loadErr
}

func (m *Manager) shouldRefreshLocked() bool {
	if m.session.RefreshToken == "" || m.session.ExpiresAt == 0 {
		return false
	}
	return m.session.ExpiresAt <= m.now().Add(time.Minute).Unix()
}

func (m *Manager) refreshLocked(ctx context.Context) error {
	refreshed, err := m.api.Refresh(ctx, m.session)
	if err != nil {
		return fmt.Errorf("refresh API session: %w", err)
	}
	key := strings.TrimSpace(m.cfg.Auth.SessionKey)
	if key == "" {
		return errors.New("active API session has no credential-store key")
	}
	if err := m.store.Save(ctx, key, refreshed); err != nil {
		return err
	}
	m.session = refreshed
	m.cfg.Auth = metadataFor(key, refreshed)
	if err := config.Save(m.cfg); err != nil {
		return fmt.Errorf("refreshed tokens were saved, but session metadata could not be updated: %w", err)
	}
	return nil
}

func mergeMetadata(session Session, metadata config.AuthConfig) Session {
	if session.AccountID == "" {
		session.AccountID = metadata.AccountID
	}
	if session.Email == "" {
		session.Email = metadata.Email
	}
	if session.Method == "" {
		session.Method = metadata.Method
	}
	if session.Scope == "" {
		session.Scope = metadata.Scope
	}
	if session.ExpiresAt == 0 {
		session.ExpiresAt = metadata.ExpiresAt
	}
	return session.normalized()
}

func metadataFor(key string, session Session) config.AuthConfig {
	session = session.normalized()
	return config.AuthConfig{
		SessionKey: key,
		AccountID:  session.AccountID,
		Email:      session.Email,
		Method:     session.Method,
		Scope:      session.Scope,
		ExpiresAt:  session.ExpiresAt,
	}
}

func legacyAuthorization(headers map[string]string) string {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "Authorization") {
			return authutil.NormalizeToken(value)
		}
	}
	return ""
}

func withoutAuthorization(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "Authorization") {
			continue
		}
		out[key] = value
	}
	return out
}
