package router

import (
	"io/fs"
	"net/http"
)

func registerRootRoutes(mux *http.ServeMux, runtime ClientRuntime, embeddedFS fs.FS) {
	mux.Handle("GET /", clientHandler(
		runtime,
		embeddedFS,
		"client/build/root",
		"bin/client/root",
		"client/build",
		"bin/client",
	))
}
