package authutil

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

func HasAuthorizationHeader(headers map[string]string) bool {
	for k, v := range headers {
		if strings.EqualFold(strings.TrimSpace(k), "Authorization") && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func IsUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr interface{ HTTPStatusCode() int }
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatusCode() == 401
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "401") && strings.Contains(s, "unauthorized")
}

func TokenExpiryUnix(headers map[string]string) (int64, bool) {
	for k, v := range headers {
		if !strings.EqualFold(strings.TrimSpace(k), "Authorization") {
			continue
		}
		raw := NormalizeToken(v)
		return TokenExpFromToken(raw)
	}
	return 0, false
}

// TokenMetadata contains unverified JWT claims, not proof of authentication.
type TokenMetadata struct {
	AccountID string
	Email     string
	ExpiresAt int64
	HasExpiry bool
}

// TokenMetadataFromToken decodes the JWT payload once without verifying its
// signature. Malformed and opaque tokens provide no metadata.
func TokenMetadataFromToken(tok string) TokenMetadata {
	claims, ok := tokenClaims(tok)
	if !ok {
		return TokenMetadata{}
	}
	var metadata TokenMetadata
	if exp, ok := claims["exp"].(float64); ok {
		metadata.ExpiresAt, metadata.HasExpiry = int64(exp), true
	}
	for _, key := range []string{"sub", "user_id", "userId", "account_id", "accountId", "uuid"} {
		if value, ok := claims[key].(string); ok && strings.TrimSpace(value) != "" {
			metadata.AccountID = strings.TrimSpace(value)
			break
		}
	}
	if value, ok := claims["email"].(string); ok {
		metadata.Email = strings.ToLower(strings.TrimSpace(value))
	}
	return metadata
}

// TokenExpFromToken extracts unverified expiry metadata from a JWT.
func TokenExpFromToken(tok string) (int64, bool) {
	metadata := TokenMetadataFromToken(tok)
	return metadata.ExpiresAt, metadata.HasExpiry
}

// TokenIdentityFromToken extracts unverified, non-secret account metadata from a
// JWT when the issuer includes it. Missing or opaque claims are ignored.
func TokenIdentityFromToken(tok string) (accountID, email string) {
	metadata := TokenMetadataFromToken(tok)
	return metadata.AccountID, metadata.Email
}

func NormalizeToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	return strings.TrimSpace(token)
}

func decodeJWTPart(s string) ([]byte, error) {
	encoding := base64.RawURLEncoding
	if strings.HasSuffix(s, "=") {
		encoding = base64.URLEncoding
	}
	return encoding.DecodeString(s)
}

func tokenClaims(tok string) (map[string]any, bool) {
	parts := strings.Split(strings.TrimSpace(tok), ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	return claims, true
}
