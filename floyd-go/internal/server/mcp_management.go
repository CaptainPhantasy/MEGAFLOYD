package server

import (
	"encoding/json"
	"net/http"

	"github.com/legacy-ai/floyd/internal/config"
)

func (c *controllerV1) registerMCPManagementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/mcp/test", c.handleMCPConnectionTest)
	mux.HandleFunc("POST /v1/workspaces/{id}/mcp/auth-reset", c.handleMCPAuthenticationReset)
}

func (c *controllerV1) handleMCPConnectionTest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name   string            `json:"name"`
		Config *config.MCPConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Name == "" && input.Config == nil {
		jsonError(w, http.StatusBadRequest, "Supply an MCP server name or configuration")
		return
	}
	result, err := c.backend.TestMCPConnection(r.Context(), r.PathValue("id"), input.Name, input.Config)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, result)
}

func (c *controllerV1) handleMCPAuthenticationReset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Name == "" {
		jsonError(w, http.StatusBadRequest, "MCP name is required")
		return
	}
	if err := c.backend.ResetMCPAuthentication(r.PathValue("id"), input.Name); err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, map[string]bool{"reset": true})
}
