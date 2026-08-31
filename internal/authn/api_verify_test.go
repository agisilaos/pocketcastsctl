package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIVerifyDoesNotDependOnQueueParsing(t *testing.T) {
	for _, raw := range []string{`{"episodes":[]}`, `{"unexpected":[]}`, `not JSON`} {
		t.Run(raw, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/up_next/list" || r.Header.Get("Authorization") != "Bearer candidate-token" {
					t.Errorf("unexpected verification request: %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(raw))
			}))
			defer srv.Close()
			api := NewAPI(srv.URL, srv.Client())
			if err := api.Verify(context.Background(), Session{AccessToken: "candidate-token"}); err != nil {
				t.Fatalf("successful API request failed auth verification: %v", err)
			}
		})
	}
}
