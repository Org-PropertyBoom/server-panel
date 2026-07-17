package add

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"mthan/vps/services"
)

type request struct {
	Password string `json:"password"`
}

type response struct {
	Status   string `json:"status"`
	Username string `json:"username"`
}

func Handler(settings *services.SettingsService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		password := strings.TrimSpace(req.Password)
		if password == "" {
			http.Error(w, "password is required", http.StatusBadRequest)
			return
		}

		username, err := generateUsername()
		if err != nil {
			http.Error(w, "failed to generate username", http.StatusInternalServerError)
			return
		}

		shell := settings.Get("users_default_shell", "/bin/bash")
		homeBase := settings.Get("users_home_base", "/home")
		createHome := settings.Get("users_create_home", "true") == "true"
		args := []string{"-M", "-d", filepath.Join(homeBase, username), "-s", shell, username}
		if createHome {
			args[0] = "-m"
		}
		cmd := exec.Command("useradd", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			http.Error(w, "failed to create user: "+string(output), http.StatusInternalServerError)
			return
		}

		// Run chpasswd
		passwdCmd := exec.Command("chpasswd")
		passwdCmd.Stdin = strings.NewReader(username + ":" + password)
		if output, err := passwdCmd.CombinedOutput(); err != nil {
			// Clean up user if password setting fails
			_ = exec.Command("userdel", "-r", username).Run()
			http.Error(w, "failed to set password: "+string(output), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, response{
			Status:   "ok",
			Username: username,
		})
	})
}

func generateUsername() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 8)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return "user-" + string(b), nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
