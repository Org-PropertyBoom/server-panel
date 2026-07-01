package post

import (
	"errors"
	"net/http"
	"os"
	"time"

	"mthan/vps/services"
)

func registerUpdate(mux *http.ServeMux, deps Dependencies) {
	mux.Handle("GET /post/update", postOnly(deps.Startup, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := deps.Update.CheckUpdate(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})))

	mux.Handle("POST /post/update", postOnly(deps.Startup, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := deps.Update.SelfUpdate(r.Context())
		if err != nil {
			writeUpdateError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"update": result,
		})

		scheduleRestart()
	})))
}

func writeUpdateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrUpdateRequiresRoot):
		http.Error(w, "root process required", http.StatusForbidden)
	default:
		http.Error(w, "self update failed", http.StatusInternalServerError)
	}
}

func scheduleRestart() {
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(1)
	}()
}
