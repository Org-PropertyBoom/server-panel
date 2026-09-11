package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"ppt/server-panel/services"
)

// newTestSession creates a session in a throwaway store and returns its token.
func newTestSession(t *testing.T) (*services.SessionService, string) {
	t.Helper()
	t.Setenv("SESSION_PATH", filepath.Join(t.TempDir(), "session"))
	sessions := services.NewSessionService()
	session, err := sessions.Create(services.AuthenticatedUser{UID: 1000, Username: "owner"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	return sessions, session.Token
}

func getUpdate(h http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/update", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: services.SessionCookieName, Value: token})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUpdateStatusRequiresSession(t *testing.T) {
	var hits atomic.Int32
	root := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer root.Close()
	sessions, _ := newTestSession(t)

	rec := getUpdate(updateStatusHandler(sessions, newPostClient(root.URL)), "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session: status=%d, want 401", rec.Code)
	}
	if hits.Load() != 0 {
		t.Fatal("an unauthenticated request must not reach the root process")
	}
}

// A logged-in session gets root's check. The forwarded request must be a plain GET
// with no Origin or Referer: postOnly only accepts a source-less request from
// localhost, the same way LoginUser reaches root.
func TestUpdateStatusForwardsRootCheck(t *testing.T) {
	root := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/post/update" {
			t.Errorf("forwarded %s %s, want GET /post/update", r.Method, r.URL.Path)
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Referer") != "" {
			t.Errorf("forwarded request carried Origin=%q Referer=%q; postOnly would reject a foreign source",
				r.Header.Get("Origin"), r.Header.Get("Referer"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(services.UpdateCheckResult{
			UpdateAvailable: true,
			LocalVersion:    "v1",
			RemoteVersion:   "v2",
		})
	}))
	defer root.Close()
	sessions, token := newTestSession(t)

	rec := getUpdate(updateStatusHandler(sessions, newPostClient(root.URL)), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q, want 200", rec.Code, rec.Body.String())
	}
	var got services.UpdateCheckResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.UpdateAvailable || got.RemoteVersion != "v2" || got.LocalVersion != "v1" {
		t.Fatalf("got %+v, want root's answer (available, v1 -> v2)", got)
	}
}

func TestUpdateStatusRootFailureIsBadGateway(t *testing.T) {
	root := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "root process required", http.StatusForbidden)
	}))
	defer root.Close()
	sessions, token := newTestSession(t)

	rec := getUpdate(updateStatusHandler(sessions, newPostClient(root.URL)), token)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("root failure: status=%d, want 502", rec.Code)
	}
}
