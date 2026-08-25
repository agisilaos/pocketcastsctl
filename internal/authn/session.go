package authn

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
)

const ScopeWebPlayer = "webplayer"

// Session is a validated Pocket Casts API session. AccessToken and
// RefreshToken are secret and must never be serialized to the user config.
type Session struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresAt    int64
	AccountID    string
	Email        string
	Method       string
}

func (s Session) normalized() Session {
	s.AccessToken = authutil.NormalizeToken(s.AccessToken)
	s.RefreshToken = strings.TrimSpace(s.RefreshToken)
	s.TokenType = strings.TrimSpace(s.TokenType)
	if s.TokenType == "" {
		s.TokenType = "Bearer"
	}
	s.Scope = strings.TrimSpace(s.Scope)
	s.AccountID = strings.TrimSpace(s.AccountID)
	s.Email = strings.ToLower(strings.TrimSpace(s.Email))
	s.Method = strings.TrimSpace(s.Method)
	accountID, email := authutil.TokenIdentityFromToken(s.AccessToken)
	if s.AccountID == "" {
		s.AccountID = accountID
	}
	if s.Email == "" {
		s.Email = email
	}
	if exp, ok := authutil.TokenExpFromToken(s.AccessToken); ok {
		s.ExpiresAt = exp
	}
	return s
}

func sessionKey(apiBaseURL string, session Session) string {
	session = session.normalized()
	identity := session.AccountID
	if identity == "" {
		identity = session.Email
	}
	if identity == "" {
		identity = "unknown-account"
	}
	scope := session.Scope
	if scope == "" {
		scope = ScopeWebPlayer
	}
	sum := sha256.Sum256([]byte(config.NormalizeAPIBaseURL(apiBaseURL) + "\x00" + identity + "\x00" + scope))
	return hex.EncodeToString(sum[:])
}

// NeedsAccountConfirmation reports whether replacing the active session could
// switch accounts. Unknown identity is intentionally treated as a switch.
func NeedsAccountConfirmation(currentAccountID, currentEmail string, candidate Session) bool {
	candidate = candidate.normalized()
	currentAccountID = strings.TrimSpace(currentAccountID)
	currentEmail = strings.ToLower(strings.TrimSpace(currentEmail))
	if currentAccountID == "" && currentEmail == "" {
		return true
	}
	if currentAccountID != "" && candidate.AccountID != "" {
		return currentAccountID != candidate.AccountID
	}
	if currentEmail != "" && candidate.Email != "" {
		return currentEmail != candidate.Email
	}
	return true
}
