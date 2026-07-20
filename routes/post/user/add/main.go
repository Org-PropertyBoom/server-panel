package add

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ppt/server-panel/services"
)

type request struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

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
		if strings.ContainsAny(req.Password, ":\r\n") {
			http.Error(w, "password contains unsupported characters", http.StatusBadRequest)
			return
		}

		username := strings.TrimSpace(req.Username)
		if settings.Get("users_auto_username", "false") == "true" {
			generated, err := generateUsername()
			if err != nil {
				http.Error(w, "failed to generate username", http.StatusInternalServerError)
				return
			}
			username = generated
		} else if !usernamePattern.MatchString(username) {
			http.Error(w, "username must use lowercase letters, numbers, underscores, or hyphens", http.StatusBadRequest)
			return
		}

		shell := settings.Get("users_default_shell", "/bin/bash")
		homeBase := settings.Get("users_home_base", "/home")
		home := filepath.Join(homeBase, username)
		createHome := settings.Get("users_create_home", "true") == "true"
		args := []string{"-M", "-d", home, "-s", shell, username}
		if createHome {
			args[0] = "-m"
		}
		cmd := exec.Command("useradd", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			http.Error(w, "failed to create user: "+string(output), http.StatusInternalServerError)
			return
		}

		createdUser, err := user.Lookup(username)
		if err != nil {
			_ = exec.Command("userdel", "-r", username).Run()
			http.Error(w, "failed to look up created user", http.StatusInternalServerError)
			return
		}
		uid, uidErr := strconv.Atoi(createdUser.Uid)
		gid, gidErr := strconv.Atoi(createdUser.Gid)
		if uidErr != nil || gidErr != nil || services.ProvisionUserHome(home, uid, gid) != nil {
			_ = exec.Command("userdel", "-r", username).Run()
			http.Error(w, "failed to provision user home folders", http.StatusInternalServerError)
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
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
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
