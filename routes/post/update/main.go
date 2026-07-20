package update

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"ppt/server-panel/services"
)

func CheckHandler(updateService *services.UpdateService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := updateService.CheckUpdate(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func SelfUpdateHandler(updateService *services.UpdateService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := updateService.SelfUpdate(r.Context())
		if err != nil {
			writeUpdateError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"update": result,
		})

		scheduleRestart()
	})
}

func writeUpdateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrUpdateRequiresRoot):
		http.Error(w, "root process required", http.StatusForbidden)
	default:
		http.Error(w, "self update failed: "+err.Error(), http.StatusInternalServerError)
	}
}

func scheduleRestart() {
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(1)
	}()
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
