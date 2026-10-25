package server

import "net/http"

func (c *controllerV1) handleGetWorkspaceTools(w http.ResponseWriter, r *http.Request) {
	tools, err := c.backend.ToolCatalog(r.Context(), r.PathValue("id"))
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, tools)
}
