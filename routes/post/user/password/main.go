package password

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"ppt/server-panel/services"
)

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Password string `json:"password"`
			Username string `json:"username"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		username := strings.TrimSpace(input.Username)
		if input.Password == "" {
			http.Error(w, "password is required", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(input.Password, ":\r\n") {
			http.Error(w, "password contains unsupported characters", http.StatusBadRequest)
			return
		}
		linuxUser, exists, err := services.HomeUser(username)
		if err != nil {
			http.Error(w, "linux users could not be loaded", http.StatusInternalServerError)
			return
		}
		if !exists || linuxUser.UID < 0 || linuxUser.Username != username {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		command := exec.Command("chpasswd")
		command.Stdin = strings.NewReader(username + ":" + input.Password + "\n")
		if output, err := command.CombinedOutput(); err != nil {
			http.Error(w, "failed to set password: "+strings.TrimSpace(string(output)), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}
