package router

import (
	"io/fs"
	"net/http"
)

func registerUserRoutes(mux *http.ServeMux, runtime ClientRuntime, embeddedFS fs.FS) {
	mux.Handle("GET /", clientHandler(
		runtime,
		embeddedFS,
		"client/build/user",
		"bin/client/user",
		"client/build",
		"bin/client",
	))
}
