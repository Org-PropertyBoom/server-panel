package delete

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
)

type request struct {
	Username string `json:"username"`
}

type response struct {
	Status string `json:"status"`
}

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		username := strings.TrimSpace(req.Username)
		if username == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}

		// Security constraint: only recognize and delete home users starting with "user-"
		if !strings.HasPrefix(username, "user-") {
			http.Error(w, "permission denied: only accounts starting with 'user-' can be deleted", http.StatusForbidden)
			return
		}

		// Run userdel -r <username>
		cmd := exec.Command("userdel", "-r", username)
		if output, err := cmd.CombinedOutput(); err != nil {
			http.Error(w, "failed to delete user: "+string(output), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, response{
			Status: "ok",
		})
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
