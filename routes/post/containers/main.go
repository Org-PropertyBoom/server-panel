package containers

import (
	"encoding/json"
	"errors"
	"net/http"

	"ppt/server-panel/services"
)

func Handler(sessions *services.SessionService, containers *services.ContainerService, vhost *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		list := containers.ListAll()
		if vhost != nil {
			list = vhost.AnnotateContainers(r.Context(), list) // reverse route view: which hostnames point here
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"containers": list}); err != nil {
			http.Error(w, "could not read containers", http.StatusInternalServerError)
		}
	})
}

func DockerfileHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		engine, owner, id := r.URL.Query().Get("engine"), r.URL.Query().Get("owner"), r.URL.Query().Get("id")
		var result services.ContainerDockerfile
		var err error
		if r.Method == http.MethodGet {
			result, err = containers.DockerfileAll(engine, owner, id)
		} else {
			var input struct {
				Content string `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			result, err = containers.WriteDockerfileAll(engine, owner, id, input.Content)
		}
		if err != nil {
			if errors.Is(err, services.ErrContainerDockerfileMissing) {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusForbidden)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}

func InspectHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		details, err := containers.InspectAll(r.URL.Query().Get("engine"), r.URL.Query().Get("owner"), r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(details)
	})
}

// RebuildHandler rebuilds + recreates a compose-managed container from its
// Dockerfile. Returns 200 with {output, error} so the build log is always shown,
// even on failure. Long-running (up to 10m).
func RebuildHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var input struct {
			Engine string `json:"engine"`
			ID     string `json:"id"`
			Owner  string `json:"owner"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		output, err := containers.RebuildAll(input.Engine, input.Owner, input.ID)
		w.Header().Set("Content-Type", "application/json")
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"output": output, "error": errText})
	})
}

// CreateHandler runs a new Docker container (`docker run -d`). Returns 200 with
// {output, error} so the run output is shown either way. Long-running (image pull).
func CreateHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var spec services.ContainerCreateSpec
		if json.NewDecoder(r.Body).Decode(&spec) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		output, err := containers.CreateContainer(spec)
		w.Header().Set("Content-Type", "application/json")
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"output": output, "error": errText})
	})
}

func ActionHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var input struct {
			Action string `json:"action"`
			Engine string `json:"engine"`
			ID     string `json:"id"`
			Owner  string `json:"owner"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := containers.ActionAll(input.Engine, input.Owner, input.ID, input.Action); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

func LogsHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		logs, err := containers.LogsAll(r.URL.Query().Get("engine"), r.URL.Query().Get("owner"), r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"logs": logs})
	})
}

func validSession(r *http.Request, sessions *services.SessionService) bool {
	cookie, err := r.Cookie(services.SessionCookieName)
	if err != nil {
		return false
	}
	_, ok := sessions.Get(cookie.Value)
	return ok
}
