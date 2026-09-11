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

// A manual refresh is forwarded to root as exactly ?refresh=1. The query is built
// by UpdateStatus, not copied, so anything else a caller appends is dropped.
func TestUpdateStatusForwardsRefreshOnly(t *testing.T) {
	var gotQuery atomic.Value
	gotQuery.Store("<unset>")
	root := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery.Store(r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(services.UpdateCheckResult{})
	}))
	defer root.Close()
	sessions, token := newTestSession(t)
	h := updateStatusHandler(sessions, newPostClient(root.URL))

	cases := []struct{ path, want string }{
		{"/api/update", ""},                                // background poll: no refresh
		{"/api/update?refresh=1", "refresh=1"},             // manual click
		{"/api/update?refresh=1&x=../../etc", "refresh=1"}, // extras never reach root
		{"/api/update?refresh=true", ""},                   // only the exact flag counts
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: services.SessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d, want 200", tc.path, rec.Code)
		}
		if got := gotQuery.Load().(string); got != tc.want {
			t.Errorf("%s: forwarded query %q, want %q", tc.path, got, tc.want)
		}
	}
}
