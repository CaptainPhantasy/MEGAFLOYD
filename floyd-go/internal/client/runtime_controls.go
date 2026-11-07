package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/agent/ralph"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/proto"
)

func (c *Client) runtimeControl(ctx context.Context, path string, in, out any) error {
	var response *http.Response
	var err error
	if in == nil {
		response, err = c.get(ctx, path, nil, nil)
	} else {
		response, err = c.post(ctx, path, nil, jsonBody(in), http.Header{"Content-Type": {"application/json"}})
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err = checkStatus(response); err != nil {
		return err
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func (c *Client) RuntimeRalphGoal(ctx context.Context, wid, sid, action, objective string) (*ralph.State, error) {
	var state *ralph.State
	err := c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/ralph", url.PathEscape(wid), url.PathEscape(sid)), proto.RalphGoalRequest{Action: action, Objective: objective}, &state)
	return state, err
}

func (c *Client) RuntimeTasks(ctx context.Context, wid string) ([]agent.RuntimeTask, error) {
	var result struct {
		Items []agent.RuntimeTask `json:"items"`
	}
	err := c.runtimeControl(ctx, "/workspaces/"+url.PathEscape(wid)+"/tasks", nil, &result)
	return result.Items, err
}

func (c *Client) RuntimeTask(ctx context.Context, wid, id, action string) (agent.RuntimeTask, error) {
	var result agent.RuntimeTask
	path := "/workspaces/" + url.PathEscape(wid) + "/tasks/" + url.PathEscape(id)
	var in any
	if action != "" {
		path += "/" + url.PathEscape(action)
		in = struct{}{}
	}
	err := c.runtimeControl(ctx, path, in, &result)
	return result, err
}

func (c *Client) RuntimeStartAgentTask(ctx context.Context, wid, sid, prompt, name string) (agent.RuntimeTask, error) {
	var result agent.RuntimeTask
	err := c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/subagents", url.PathEscape(wid), url.PathEscape(sid)), proto.RuntimeSubagentRequest{Prompt: prompt, Profile: name, Description: "swarm: " + name, NonInteractive: permission.IsNonInteractive(ctx)}, &result)
	return result, err
}

func (c *Client) RuntimePlan(ctx context.Context, wid, sid string, in *proto.RuntimePlanRequest) (proto.RuntimePlan, error) {
	var result proto.RuntimePlan
	var payload any
	if in != nil {
		payload = in
	}
	err := c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/plan", url.PathEscape(wid), url.PathEscape(sid)), payload, &result)
	return result, err
}

func (c *Client) ForkSession(ctx context.Context, wid, sid string) (*proto.Session, error) {
	var result proto.Session
	err := c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/fork", url.PathEscape(wid), url.PathEscape(sid)), proto.SessionCloneRequest{}, &result)
	return &result, err
}

func (c *Client) ReloadConfig(ctx context.Context, wid string) error {
	var result map[string]bool
	return c.runtimeControl(ctx, "/workspaces/"+url.PathEscape(wid)+"/config/reload", struct{}{}, &result)
}

func (c *Client) UndoPrompts(ctx context.Context, wid, sid string, count int) (proto.SessionUndoResponse, error) {
	var result proto.SessionUndoResponse
	err := c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/undo", url.PathEscape(wid), url.PathEscape(sid)), proto.SessionUndoRequest{Count: count}, &result)
	return result, err
}

func (c *Client) CompactSession(ctx context.Context, wid, sid, instruction string) error {
	var result map[string]bool
	return c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/compact", url.PathEscape(wid), url.PathEscape(sid)), proto.CompactRequest{Instruction: instruction}, &result)
}

func (c *Client) FrontendOperation(ctx context.Context, wid string, in proto.FrontendRequest) (proto.FrontendResult, error) {
	var out proto.FrontendResult
	err := c.runtimeControl(ctx, "/workspaces/"+url.PathEscape(wid)+"/frontend-operation", in, &out)
	return out, err
}
