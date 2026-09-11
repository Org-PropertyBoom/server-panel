package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// updateTestRig serves version.json from an httptest server (via the VERSION_URL
// override, so no GitHub call is made) with a controllable clock.
type updateTestRig struct {
	svc  *UpdateService
	hits atomic.Int32
	mu   sync.Mutex
	now  time.Time
}

func newUpdateTestRig(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *updateTestRig {
	t.Helper()
	rig := &updateTestRig{now: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rig.hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	rig.svc = &UpdateService{
		versionOverride: srv.URL + "/version.json",
		httpClient:      srv.Client(),
		localVersion:    "v1",
		localBuildTime:  "2026-09-10T00:00:00Z",
		now: func() time.Time {
			rig.mu.Lock()
			defer rig.mu.Unlock()
			return rig.now
		},
	}
	return rig
}

func (r *updateTestRig) advance(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = r.now.Add(d)
}

func newerBuild(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":"v2","buildTime":"2026-09-11T00:00:00Z"}`))
}

func TestCheckUpdateCachesResultForTTL(t *testing.T) {
	rig := newUpdateTestRig(t, newerBuild)

	res, err := rig.svc.CheckUpdate(context.Background())
	if err != nil || !res.UpdateAvailable || res.RemoteVersion != "v2" {
		t.Fatalf("first check: res=%+v err=%v, want an available v2", res, err)
	}
	rig.advance(updateCheckTTL - time.Second)
	if _, err := rig.svc.CheckUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rig.hits.Load(); got != 1 {
		t.Fatalf("within TTL: remote hits=%d, want 1", got)
	}
	rig.advance(2 * time.Second)
	if _, err := rig.svc.CheckUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rig.hits.Load(); got != 2 {
		t.Fatalf("after TTL: remote hits=%d, want 2", got)
	}
}

// Many sessions polling at once must cost one remote check, not one each.
func TestCheckUpdateSharesOneFetchAcrossConcurrentCallers(t *testing.T) {
	release := make(chan struct{})
	rig := newUpdateTestRig(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
		newerBuild(w, r)
	})

	const callers = 10
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rig.svc.CheckUpdate(context.Background())
			errs <- err
		}()
	}
	time.Sleep(50 * time.Millisecond) // let the callers pile up behind the first fetch
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("caller got %v", err)
		}
	}
	if got := rig.hits.Load(); got != 1 {
		t.Fatalf("remote hits=%d for %d concurrent callers, want 1", got, callers)
	}
}

// A failing remote is retried after a short back-off, not on every poll.
func TestCheckUpdateCachesFailureBriefly(t *testing.T) {
	rig := newUpdateTestRig(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	if _, err := rig.svc.CheckUpdate(context.Background()); err == nil {
		t.Fatal("want an error from a failing remote")
	}
	if _, err := rig.svc.CheckUpdate(context.Background()); err == nil {
		t.Fatal("want the cached error within the back-off")
	}
	if got := rig.hits.Load(); got != 1 {
		t.Fatalf("within back-off: remote hits=%d, want 1", got)
	}
	rig.advance(updateCheckErrorTTL + time.Second)
	_, _ = rig.svc.CheckUpdate(context.Background())
	if got := rig.hits.Load(); got != 2 {
		t.Fatalf("after back-off: remote hits=%d, want 2", got)
	}
}

// A caller that gives up must not poison the cache for everyone else.
func TestCheckUpdateDoesNotCacheCallerCancellation(t *testing.T) {
	rig := newUpdateTestRig(t, newerBuild)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rig.svc.CheckUpdate(ctx); err == nil {
		t.Fatal("want an error for a cancelled caller")
	}
	res, err := rig.svc.CheckUpdate(context.Background())
	if err != nil || !res.UpdateAvailable {
		t.Fatalf("next caller: res=%+v err=%v, want a fresh successful check", res, err)
	}
}
