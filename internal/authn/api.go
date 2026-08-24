package authn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/pocketcasts"
)

type API struct {
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

func NewAPI(baseURL string, httpClient *http.Client) *API {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.pocketcasts.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &API{BaseURL: baseURL, HTTP: httpClient, Now: time.Now}
}

func (a *API) Login(ctx context.Context, email, password string) (Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return Session{}, errors.New("email and password are required")
	}
	body := map[string]any{"email": email, "password": password, "scope": ScopeWebPlayer}
	session, err := a.exchange(ctx, "/user/login_pocket_casts", body)
	if err != nil {
		return Session{}, err
	}
	session.Email = email
	session.Method = "password"
	session.Scope = ScopeWebPlayer
	return session.normalized(), nil
}

func (a *API) Refresh(ctx context.Context, current Session) (Session, error) {
	current = current.normalized()
	if current.RefreshToken == "" {
		return Session{}, errors.New("active API session has no refresh token")
	}
	scope := current.Scope
	if scope == "" {
		scope = ScopeWebPlayer
	}
	body := map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": current.RefreshToken,
		"scope":         scope,
	}
	refreshed, err := a.exchange(ctx, "/user/token", body)
	if err != nil {
		return Session{}, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = current.RefreshToken
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = current.AccountID
	}
	if refreshed.Email == "" {
		refreshed.Email = current.Email
	}
	refreshed.Method = current.Method
	refreshed.Scope = scope
	return refreshed.normalized(), nil
}

// Verify exercises the read-only route shared by every current API workflow.
func (a *API) Verify(ctx context.Context, session Session) error {
	session = session.normalized()
	if session.AccessToken == "" {
		return errors.New("API session has no access token")
	}
	client := pocketcasts.New(pocketcasts.Options{
		BaseURL: a.BaseURL,
		Headers: map[string]string{"Authorization": "Bearer " + session.AccessToken},
		HTTP:    a.HTTP,
	})
	_, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          ScopeWebPlayer,
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	return err
}

// Validate verifies a candidate and uses its refresh token once when the
// access token is stale or rejected. The returned session contains any token
// rotation and is the only value callers should persist.
func (a *API) Validate(ctx context.Context, session Session) (Session, error) {
	session = session.normalized()
	refreshed := false
	if session.RefreshToken != "" && session.ExpiresAt > 0 && session.ExpiresAt <= a.Now().Add(time.Minute).Unix() {
		var err error
		session, err = a.Refresh(ctx, session)
		if err != nil {
			return Session{}, err
		}
		refreshed = true
	}
	if err := a.Verify(ctx, session); err == nil {
		return session, nil
	} else if !authutil.IsUnauthorizedError(err) || session.RefreshToken == "" || refreshed {
		return Session{}, err
	}
	rotated, refreshErr := a.Refresh(ctx, session)
	if refreshErr != nil {
		return Session{}, refreshErr
	}
	if err := a.Verify(ctx, rotated); err != nil {
		return Session{}, err
	}
	return rotated, nil
}

func (a *API) exchange(ctx context.Context, path string, payload map[string]any) (Session, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Session{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Session{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return Session{}, apiResponseError(resp.StatusCode, body)
	}
	session, err := decodeTokenResponse(body, a.Now())
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func apiResponseError(status int, body []byte) error {
	message := ""
	var object map[string]any
	if json.Unmarshal(body, &object) == nil {
		message = firstString(object, "error_description", "error", "message")
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return pocketcasts.NewHTTPError(status, message)
}

func decodeTokenResponse(raw []byte, now time.Time) (Session, error) {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return Session{}, fmt.Errorf("decode token response: %w", err)
	}
	session := Session{
		AccessToken:  firstString(object, "accessToken", "access_token"),
		RefreshToken: firstString(object, "refreshToken", "refresh_token"),
		TokenType:    firstString(object, "tokenType", "token_type"),
		Scope:        firstString(object, "scope"),
		AccountID:    firstString(object, "userId", "user_id", "accountId", "account_id", "uuid", "sub"),
		Email:        firstString(object, "email"),
	}
	if session.AccessToken == "" {
		return Session{}, errors.New("token response did not contain an access token")
	}
	if expiresIn, ok := firstInt64(object, "expiresIn", "expires_in"); ok && expiresIn > 0 {
		session.ExpiresAt = now.Add(time.Duration(expiresIn) * time.Second).Unix()
	}
	return session.normalized(), nil
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			continue
		}
		switch value := value.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func firstInt64(object map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			continue
		}
		switch value := value.(type) {
		case float64:
			return int64(value), true
		case string:
			parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}
