package apps

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os/user"
	"strconv"
	"strings"

	"ppt/server-panel/services"
)

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := strings.TrimSpace(r.URL.Query().Get("username"))
		if username == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}

		linuxUser, exists, err := services.HomeUser(username)
		if err != nil {
			http.Error(w, "linux users could not be loaded", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		if r.Method == http.MethodPost {
			account, lookupErr := user.Lookup(username)
			if lookupErr != nil {
				http.Error(w, "system user not found", http.StatusBadRequest)
				return
			}
			uid, _ := strconv.Atoi(account.Uid)
			gid, _ := strconv.Atoi(account.Gid)
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				if err := r.ParseMultipartForm(128 << 20); err != nil {
					http.Error(w, "invalid upload", http.StatusBadRequest)
					return
				}
				file, header, err := r.FormFile("file")
				if err != nil {
					http.Error(w, "ZIP file is required", http.StatusBadRequest)
					return
				}
				defer file.Close()
				readerAt, ok := file.(io.ReaderAt)
				if !ok {
					http.Error(w, "invalid ZIP upload", http.StatusBadRequest)
					return
				}
				archive, err := zip.NewReader(readerAt, header.Size)
				if err != nil {
					http.Error(w, "invalid ZIP file", http.StatusBadRequest)
					return
				}
				err = services.UploadUserAppZIP(linuxUser.Home, strings.TrimSpace(r.FormValue("name")), archive, uid, gid)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				var input struct {
					Name       string `json:"name"`
					Repository string `json:"repository"`
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				if err := services.CloneUserApp(linuxUser.Home, strings.TrimSpace(input.Name), strings.TrimSpace(input.Repository), uid, gid); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		userApps, err := services.UserApps(linuxUser.Home)
		if err != nil {
			http.Error(w, "user apps could not be loaded", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"apps":   userApps,
		})
	})
}
