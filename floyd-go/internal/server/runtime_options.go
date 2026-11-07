package server

import (
	"net/http"

	"github.com/legacy-ai/floyd/internal/config"
)

func (c *controllerV1) registerRuntimeOptions(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/config/runtime-options", func(w http.ResponseWriter, r *http.Request) {
		var options config.RuntimeOptions
		if !runtimeBody(w, r, &options) {
			return
		}
		if err := c.backend.ApplyRuntimeOptions(r.PathValue("id"), options); err != nil {
			c.sessionOperationError(w, r, err)
			return
		}
		jsonEncode(w, map[string]bool{"applied": true})
	})
}
