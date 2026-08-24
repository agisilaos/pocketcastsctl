package pocketcasts

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var ErrTokenNotRefreshable = errors.New("API credential cannot be refreshed")

type HTTPError struct {
	Status  int
	Message string
}

func NewHTTPError(status int, message string) *HTTPError {
	return &HTTPError{Status: status, Message: strings.TrimSpace(message)}
}

func (e *HTTPError) Error() string {
	statusText := http.StatusText(e.Status)
	if e.Message == "" || strings.EqualFold(e.Message, statusText) {
		return fmt.Sprintf("http %d: %s", e.Status, statusText)
	}
	return fmt.Sprintf("http %d %s: %s", e.Status, statusText, e.Message)
}

func (e *HTTPError) HTTPStatusCode() int { return e.Status }
