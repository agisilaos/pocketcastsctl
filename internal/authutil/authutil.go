package authutil

import (
	"encoding/base64"
	"encoding/json"
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

func TokenExpFromToken(tok string) (int64, bool) {
	parts := strings.Split(strings.TrimSpace(tok), ".")
	if len(parts) != 3 {
		return 0, false
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return 0, false
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return 0, false
	}
	switch v := m["exp"].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

func NormalizeToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	return strings.TrimSpace(token)
}

func decodeJWTPart(s string) ([]byte, error) {
	if l := len(s) % 4; l != 0 {
		s += strings.Repeat("=", 4-l)
	}
	return base64.RawURLEncoding.DecodeString(s)
}
