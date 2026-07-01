package router

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mthan/vps/services"
)

type ClientRuntime struct {
	Env      string `json:"env"`
	IsRoot   bool   `json:"isRoot"`
	Mode     string `json:"mode"`
	UID      int    `json:"uid"`
	Username string `json:"username"`
}

func Register(mux *http.ServeMux, startup services.StartupConfig, clientFS embed.FS) {
	runtime := newClientRuntime(startup)
	subFS, err := fs.Sub(clientFS, "client/build")
	if err != nil {
		subFS = nil
		println("fs.Sub error:", err.Error())
	} else if subFS == nil {
		println("fs.Sub returned nil")
	} else {
		if entries, err := fs.ReadDir(subFS, "."); err == nil {
			println("Successfully resolved subFS. Embedded files:")
			for _, entry := range entries {
				println("  -", entry.Name())
			}
		} else {
			println("Failed to read subFS dir:", err.Error())
		}
	}

	if startup.IsRoot {
		registerRootRoutes(mux, runtime, subFS)
		return
	}

	registerUserRoutes(mux, runtime, subFS)
}

func PostBaseURL(startup services.StartupConfig) string {
	if startup.PostBaseURL != "" {
		return strings.TrimRight(startup.PostBaseURL, "/")
	}

	return internalBaseURL(startup.Addr)
}

func newClientRuntime(startup services.StartupConfig) ClientRuntime {
	return ClientRuntime{
		Env:      startup.Env,
		IsRoot:   startup.IsRoot,
		Mode:     startup.Mode,
		UID:      startup.UID,
		Username: startup.Username,
	}
}

func clientHandler(runtime ClientRuntime, embeddedFS fs.FS, clientDirs ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Try local filesystem directories first (development)
		for _, clientDir := range clientDirs {
			if _, err := os.Stat(clientDir); err == nil {
				fileServer := http.FileServer(http.Dir(clientDir))
				requestedPath, ok := clientPath(clientDir, r.URL.Path)
				if ok && fileExists(requestedPath) {
					if isIndexFile(requestedPath) {
						serveClientIndex(w, requestedPath, runtime)
						return
					}

					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}

		// 2. Fallback to embedded filesystem if available (production / single-binary)
		if embeddedFS != nil {
			cleanPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

			if fileExistsInFS(embeddedFS, cleanPath) {
				if cleanPath == "" || cleanPath == "index.html" {
					serveEmbeddedIndex(w, embeddedFS, runtime)
					return
				}

				http.FileServer(http.FS(embeddedFS)).ServeHTTP(w, r)
				return
			}

			// Fallback to index.html for SPA routing
			if fileExistsInFS(embeddedFS, "index.html") {
				serveEmbeddedIndex(w, embeddedFS, runtime)
				return
			}
		}

		// 3. Fallback to index.html from local filesystem directories if files exist
		for _, clientDir := range clientDirs {
			indexPath := filepath.Join(clientDir, "index.html")
			if fileExists(indexPath) {
				serveClientIndex(w, indexPath, runtime)
				return
			}
		}

		http.Error(w, "react client has not been built", http.StatusServiceUnavailable)
	})
}

func fileExistsInFS(f fs.FS, name string) bool {
	if name == "" || name == "." {
		return true
	}
	file, err := f.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return !stat.IsDir()
}

func serveEmbeddedIndex(w http.ResponseWriter, f fs.FS, runtime ClientRuntime) {
	indexHTML, err := fs.ReadFile(f, "index.html")
	if err != nil {
		http.Error(w, "react client index could not be loaded", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(injectClientRuntime(indexHTML, runtime))
}

func serveClientIndex(w http.ResponseWriter, indexPath string, runtime ClientRuntime) {
	indexHTML, err := os.ReadFile(indexPath)
	if err != nil {
		http.Error(w, "react client index could not be loaded", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(injectClientRuntime(indexHTML, runtime))
}

func injectClientRuntime(indexHTML []byte, runtime ClientRuntime) []byte {
	payload, err := json.Marshal(runtime)
	if err != nil {
		payload = []byte("{}")
	}

	script := []byte("\n<script>window.__VPS_RUNTIME__ = " + string(payload) + ";</script>\n")
	headEnd := []byte("</head>")
	if bytes.Contains(indexHTML, headEnd) {
		return bytes.Replace(indexHTML, headEnd, append(script, headEnd...), 1)
	}

	return append(script, indexHTML...)
}

func clientPath(clientDir string, urlPath string) (string, bool) {
	cleanPath := path.Clean("/" + urlPath)
	requestedPath := filepath.Join(clientDir, strings.TrimPrefix(cleanPath, "/"))

	clientRoot, err := filepath.Abs(clientDir)
	if err != nil {
		return "", false
	}

	requestedAbs, err := filepath.Abs(requestedPath)
	if err != nil {
		return "", false
	}

	relativePath, err := filepath.Rel(clientRoot, requestedAbs)
	if err != nil {
		return "", false
	}

	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
		return "", false
	}

	return requestedAbs, true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir() || isIndexFile(path)
}

func isIndexFile(path string) bool {
	return filepath.Base(path) == "index.html"
}

func internalBaseURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		switch host {
		case "", "0.0.0.0", "::":
			host = "127.0.0.1"
		}

		return "http://" + net.JoinHostPort(host, port)
	}

	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}

	return "http://" + addr
}
