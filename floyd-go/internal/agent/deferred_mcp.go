package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"charm.land/fantasy"
)

const deferredMCPSearchName = "mcp_tool_search"
const deferredMCPCallName = "mcp_tool_call"

// deferredMCPTool keeps target schemas out of the initial model palette. Each
// per-call copy contains only targets allowed by that call's captured settings.
// Targets are the normal installed MCP wrappers, including hooks and permission.
type deferredMCPTool struct {
	fantasy.AgentTool
	targets []*installedMCPTool
	wrap    func(fantasy.AgentTool) fantasy.AgentTool
}

type deferredMCPSearchParams struct {
	Query  string `json:"query,omitempty" description:"Search words from the tool name or description; empty lists available tools"`
	Server string `json:"server,omitempty" description:"Optional exact MCP server name"`
	Limit  int    `json:"limit,omitempty" description:"Maximum results, default 10, maximum 25"`
}

type deferredMCPCallParams struct {
	Name      string         `json:"name" description:"Exact tool name returned by mcp_tool_search"`
	Arguments map[string]any `json:"arguments" description:"Arguments matching the discovered input_schema"`
}

func newDeferredMCPTool(name string, targets []*installedMCPTool, wrap func(fantasy.AgentTool) fantasy.AgentTool) *deferredMCPTool {
	targets = slices.Clone(targets)
	slices.SortFunc(targets, func(a, b *installedMCPTool) int { return strings.Compare(a.Info().Name, b.Info().Name) })
	var tool fantasy.AgentTool
	if name == deferredMCPSearchName {
		tool = fantasy.NewAgentTool(name, "Discover deferred MCP tools and their input schemas. Use mcp_tool_call to execute an exact discovered name.", func(_ context.Context, params deferredMCPSearchParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			limit := params.Limit
			if limit <= 0 {
				limit = 10
			}
			if limit > 25 {
				limit = 25
			}
			terms := strings.Fields(strings.ToLower(params.Query))
			matches := make([]map[string]any, 0)
			total := 0
			for _, target := range targets {
				if params.Server != "" && target.server != params.Server {
					continue
				}
				info := target.Info()
				text := strings.ToLower(info.Name + " " + info.Description)
				if !allSearchTerms(text, terms) {
					continue
				}
				total++
				if len(matches) >= limit {
					continue
				}
				matches = append(matches, map[string]any{
					"name": info.Name, "server": target.server, "description": info.Description,
					"input_schema": map[string]any{"type": "object", "properties": info.Parameters, "required": info.Required},
				})
			}
			encoded, err := json.Marshal(map[string]any{"tools": matches, "total": total})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			return fantasy.NewTextResponse(string(encoded)), nil
		})
	} else {
		tool = fantasy.NewAgentTool(name, "Execute a deferred MCP tool by its exact discovered name. The target uses normal permissions, hooks and guards.", func(ctx context.Context, params deferredMCPCallParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			for _, target := range targets {
				if target.Info().Name != params.Name {
					continue
				}
				if params.Arguments == nil {
					params.Arguments = map[string]any{}
				}
				encoded, err := json.Marshal(params.Arguments)
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				call.Name, call.Input = target.Info().Name, string(encoded)
				return target.Run(ctx, call)
			}
			return fantasy.NewTextErrorResponse("Deferred MCP tool is unavailable or excluded by this turn's settings: " + params.Name), nil
		})
	}
	if wrap != nil {
		tool = wrap(tool)
	}
	return &deferredMCPTool{AgentTool: tool, targets: targets, wrap: wrap}
}

func allSearchTerms(text string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

func (d *deferredMCPTool) forSettings(settings SessionRunSettings) fantasy.AgentTool {
	if slices.Contains(settings.DisabledTools, d.Info().Name) {
		return nil
	}
	allows := func(allowlist *[]string, target string) bool {
		return allowlist == nil || slices.Contains(*allowlist, target) || slices.Contains(*allowlist, d.Info().Name)
	}
	var selected []*installedMCPTool
	for _, target := range d.targets {
		if !allows(settings.Tools, target.Info().Name) || !allows(settings.profileTools, target.Info().Name) {
			continue
		}
		if slices.Contains(settings.DisabledTools, target.Info().Name) {
			continue
		}
		if settings.MCPServers != nil && !slices.Contains(*settings.MCPServers, target.server) {
			continue
		}
		selected = append(selected, target)
	}
	if len(selected) == 0 {
		return nil
	}
	return newDeferredMCPTool(d.Info().Name, selected, d.wrap)
}

func (c *coordinator) deferMCPTools(palette []fantasy.AgentTool, isSubAgent bool) []fantasy.AgentTool {
	var visible []fantasy.AgentTool
	var deferred []*installedMCPTool
	for _, tool := range palette {
		target := tool.(*installedMCPTool)
		if c.cfg.Config().MCP[target.server].Deferred {
			deferred = append(deferred, target)
		} else {
			visible = append(visible, target)
		}
	}
	if len(deferred) > 0 {
		wrap := func(tool fantasy.AgentTool) fantasy.AgentTool {
			return c.wrapAgentTools([]fantasy.AgentTool{tool}, isSubAgent)[0]
		}
		visible = append(visible, newDeferredMCPTool(deferredMCPSearchName, deferred, wrap), newDeferredMCPTool(deferredMCPCallName, deferred, wrap))
	}
	return visible
}
