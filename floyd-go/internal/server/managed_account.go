package server

import (
	"encoding/json"
	"github.com/legacy-ai/floyd/internal/config"
	"net/http"
)

func (c *controllerV1) registerAccountOperations(mux *http.ServeMux) {
	for _, method := range []string{"GET", "POST", "DELETE"} {
		mux.HandleFunc(method+" /v1/workspaces/{id}/auth/account", c.handleManagedAccount)
	}
	mux.HandleFunc("GET /v1/workspaces/{id}/auth/account/token", c.handleManagedAccountToken)
	mux.HandleFunc("POST /v1/workspaces/{id}/auth/account/refresh", c.handleManagedAccountRefresh)
}
func (c *controllerV1) handleManagedAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	provider := r.URL.Query().Get("provider")
	if r.Method == http.MethodPost {
		var req config.ManagedAccountLogin
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid account request")
			return
		}
		if err := c.backend.SaveManagedAccount(id, req); err != nil {
			c.handleError(w, r, err)
			return
		}
		provider = req.Provider
	} else if r.Method == http.MethodDelete {
		if err := c.backend.LogoutManagedAccount(id, provider); err != nil {
			c.handleError(w, r, err)
			return
		}
	}
	account, err := c.backend.GetManagedAccount(id, provider)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, account)
}
func (c *controllerV1) handleManagedAccountToken(w http.ResponseWriter, r *http.Request) {
	token, err := c.backend.GetManagedAccountToken(r.PathValue("id"), r.URL.Query().Get("provider"))
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, token)
}
func (c *controllerV1) handleManagedAccountRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid account refresh request")
		return
	}
	if err := c.backend.RefreshManagedAccount(r.Context(), r.PathValue("id"), req.Provider); err != nil {
		c.handleError(w, r, err)
		return
	}
	account, err := c.backend.GetManagedAccount(r.PathValue("id"), req.Provider)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, account)
}
