package server

import (
	"encoding/json"
	"net/http"
)

func (c *controllerV1) registerProviderDirectoryOperations(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/providers/refresh", c.handleRefreshProviderModels)
	mux.HandleFunc("POST /v1/workspaces/{id}/workspace/add-dir", c.handleAddWorkspaceDirectory)
}

func (c *controllerV1) handleRefreshProviderModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID string `json:"provider_id"`
		OAuthOnly  bool   `json:"oauth_only"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid provider refresh request")
		return
	}
	result, err := c.backend.RefreshProviderModels(r.Context(), r.PathValue("id"), req.ProviderID, req.OAuthOnly)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, result)
}

func (c *controllerV1) handleAddWorkspaceDirectory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Persist *bool  `json:"persist"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid additional directory request")
		return
	}
	persist := true
	if req.Persist != nil {
		persist = *req.Persist
	}
	result, err := c.backend.AddWorkspaceDirectory(r.PathValue("id"), req.Path, persist)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, result)
}
