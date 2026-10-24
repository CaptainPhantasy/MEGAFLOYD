package server

import (
	"encoding/json"
	"net/http"
)

func (c *controllerV1) registerManagementOperations(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workspaces/{id}/permissions/mode", c.handlePermissionsMode)
	mux.HandleFunc("POST /v1/workspaces/{id}/permissions/mode", c.handlePermissionsMode)
}

func (c *controllerV1) handlePermissionsMode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid permission mode request")
			return
		}
		if req.Mode != "manual" && req.Mode != "yolo" && req.Mode != "auto" {
			jsonError(w, http.StatusBadRequest, "choose manual, yolo or auto")
			return
		}
		if err := c.backend.SetPermissionsMode(r.PathValue("id"), req.Mode); err != nil {
			c.handleError(w, r, err)
			return
		}
	}
	mode, err := c.backend.GetPermissionsMode(r.PathValue("id"))
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, map[string]string{"mode": mode})
}
