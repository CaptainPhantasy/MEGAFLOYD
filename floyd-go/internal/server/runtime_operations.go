package server

import (
	"encoding/json"
	"net/http"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/goal"
	"github.com/legacy-ai/floyd/internal/pluginruntime"
	"github.com/legacy-ai/floyd/internal/proto"
)

func runtimeBody(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid runtime control request")
		return false
	}
	return true
}

func (c *controllerV1) registerRuntimeOperations(mux *http.ServeMux) {
	c.registerFrontendRuntime(mux)
	c.registerRuntimeOptions(mux)
	base := "/v1/workspaces/{id}"
	mux.HandleFunc("POST "+base+"/config/plugin-reload", func(w http.ResponseWriter, r *http.Request) {
		var in pluginruntime.Input
		if !runtimeBody(w, r, &in) {
			return
		}
		v, e := c.backend.ReloadPlugins(r.Context(), r.PathValue("id"), in)
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, v)
	})
	mux.HandleFunc("GET "+base+"/agent/sessions/{sid}/queue", func(w http.ResponseWriter, r *http.Request) {
		v, e := c.backend.RuntimeQueue(r.Context(), r.PathValue("id"), r.PathValue("sid"))
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, v)
	})
	mux.HandleFunc("PATCH "+base+"/agent/sessions/{sid}/queue/{qid}", func(w http.ResponseWriter, r *http.Request) {
		var in proto.RuntimeQueueEdit
		if !runtimeBody(w, r, &in) {
			return
		}
		e := c.backend.RuntimeEditQueue(r.Context(), r.PathValue("id"), r.PathValue("sid"), r.PathValue("qid"), in.Prompt, false)
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]bool{"updated": true})
	})
	mux.HandleFunc("DELETE "+base+"/agent/sessions/{sid}/queue/{qid}", func(w http.ResponseWriter, r *http.Request) {
		e := c.backend.RuntimeEditQueue(r.Context(), r.PathValue("id"), r.PathValue("sid"), r.PathValue("qid"), "", true)
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]bool{"aborted": true})
	})
	mux.HandleFunc("POST "+base+"/agent/sessions/{sid}/queue/steer", func(w http.ResponseWriter, r *http.Request) {
		var in proto.RuntimeQueueSteer
		if !runtimeBody(w, r, &in) {
			return
		}
		e := c.backend.RuntimeSteerQueue(r.Context(), r.PathValue("id"), r.PathValue("sid"), in.IDs)
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]any{"steered": true, "ids": in.IDs})
	})
	mux.HandleFunc("POST "+base+"/agent/sessions/{sid}/queue/clear", func(w http.ResponseWriter, r *http.Request) {
		if e := c.backend.RuntimeClearQueue(r.Context(), r.PathValue("id"), r.PathValue("sid")); e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]bool{"cleared": true})
	})
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" "+base+"/sessions/{sid}/plan", func(w http.ResponseWriter, r *http.Request) {
			var in *proto.RuntimePlanRequest
			if r.Method == "POST" {
				in = &proto.RuntimePlanRequest{}
				if !runtimeBody(w, r, in) {
					return
				}
			}
			v, e := c.backend.RuntimePlan(r.Context(), r.PathValue("id"), r.PathValue("sid"), in)
			if e != nil {
				c.sessionOperationError(w, r, e)
				return
			}
			jsonEncode(w, v)
		})
		mux.HandleFunc(method+" "+base+"/sessions/{sid}/swarm", func(w http.ResponseWriter, r *http.Request) {
			var in *proto.RuntimeSwarmRequest
			if r.Method == "POST" {
				in = &proto.RuntimeSwarmRequest{}
				if !runtimeBody(w, r, in) {
					return
				}
			}
			v, e := c.backend.RuntimeSwarm(r.Context(), r.PathValue("id"), r.PathValue("sid"), in)
			if e != nil {
				c.sessionOperationError(w, r, e)
				return
			}
			jsonEncode(w, v)
		})
		mux.HandleFunc(method+" "+base+"/sessions/{sid}/goal", func(w http.ResponseWriter, r *http.Request) {
			var in *goal.Input
			if r.Method == "POST" {
				in = &goal.Input{}
				if !runtimeBody(w, r, in) {
					return
				}
			}
			v, e := c.backend.RuntimeGoal(r.Context(), r.PathValue("id"), r.PathValue("sid"), in)
			if e != nil {
				c.sessionOperationError(w, r, e)
				return
			}
			jsonEncode(w, v)
		})
	}
	mux.HandleFunc("GET "+base+"/tasks", func(w http.ResponseWriter, r *http.Request) {
		v, e := c.backend.RuntimeTasks(r.Context(), r.PathValue("id"))
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]any{"items": v})
	})
	mux.HandleFunc("GET "+base+"/tasks/{tid}", c.handleRuntimeTask)
	mux.HandleFunc("POST "+base+"/tasks/{tid}/{action}", c.handleRuntimeTask)
	mux.HandleFunc("GET "+base+"/agent/profiles", func(w http.ResponseWriter, r *http.Request) {
		v, e := c.backend.RuntimeProfiles(r.PathValue("id"))
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]any{"items": v})
	})
	mux.HandleFunc("PUT "+base+"/agent/profiles/{pid}", func(w http.ResponseWriter, r *http.Request) {
		var in agent.RuntimeProfile
		if !runtimeBody(w, r, &in) {
			return
		}
		v, e := c.backend.RuntimeSaveProfile(r.PathValue("id"), r.PathValue("pid"), in)
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, v)
	})
	mux.HandleFunc("GET "+base+"/sessions/{sid}/subagents", func(w http.ResponseWriter, r *http.Request) {
		v, e := c.backend.RuntimeSubagents(r.Context(), r.PathValue("id"), r.PathValue("sid"))
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, map[string]any{"items": v})
	})
	mux.HandleFunc("POST "+base+"/sessions/{sid}/subagents", func(w http.ResponseWriter, r *http.Request) {
		var in proto.RuntimeSubagentRequest
		if !runtimeBody(w, r, &in) {
			return
		}
		v, e := c.backend.RuntimeStartSubagent(r.Context(), r.PathValue("id"), r.PathValue("sid"), in)
		if e != nil {
			c.sessionOperationError(w, r, e)
			return
		}
		jsonEncode(w, v)
	})
}

func (c *controllerV1) handleRuntimeTask(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "" && action != "cancel" && action != "detach" {
		jsonError(w, http.StatusBadRequest, "unknown task action")
		return
	}
	v, e := c.backend.RuntimeTask(r.Context(), r.PathValue("id"), r.PathValue("tid"), action)
	if e != nil {
		c.sessionOperationError(w, r, e)
		return
	}
	jsonEncode(w, v)
}
