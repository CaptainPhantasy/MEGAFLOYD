package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/intelligence"
)

const SymbolIndexToolName = "symbol_index"

// SymbolIndexParams defines parameters for the symbol_index tool
type SymbolIndexParams struct {
	Action string `json:"action"` // "index", "query", "query_kind", "query_file", "stats", "clear"
	// For indexing
	Path  string `json:"path,omitempty"`  // File or directory to index
	Depth int    `json:"depth,omitempty"` // Max depth for directory indexing (default: 10)
	// For querying
	Name  string `json:"name,omitempty"`  // Symbol name to search (fuzzy match)
	Kind  string `json:"kind,omitempty"`  // Symbol kind: function, method, struct, interface, class, etc.
	Limit int    `json:"limit,omitempty"` // Max results (default: 20)
}

// SymbolIndexResponse is the structured response
type SymbolIndexResponse struct {
	Action  string                `json:"action"`
	Symbols []intelligence.Symbol `json:"symbols,omitempty"`
	Stats   *IndexStatsResponse   `json:"stats,omitempty"`
	Indexed int                   `json:"indexed,omitempty"` // Files indexed
	Error   string                `json:"error,omitempty"`
}

// IndexStatsResponse mirrors intelligence.IndexStats for JSON export
type IndexStatsResponse struct {
	Files        int            `json:"files"`
	TotalSymbols int            `json:"total_symbols"`
	ByKind       map[string]int `json:"by_kind"`
}

// Global symbol index instance (lazy-initialized)
var globalSymbolIndex *intelligence.SymbolIndex

// getSymbolIndex returns the global symbol index, initializing if needed
func getSymbolIndex(workDir string) *intelligence.SymbolIndex {
	if globalSymbolIndex == nil {
		globalSymbolIndex = intelligence.NewSymbolIndex(workDir)
	}
	return globalSymbolIndex
}

// NewSymbolIndexTool creates a tool for code symbol extraction and search
func NewSymbolIndexTool(workDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SymbolIndexToolName,
		"Extract and search code symbols (functions, classes, structs, interfaces, methods). "+
			"Actions: 'index' (file/dir), 'query' (fuzzy search by name), 'query_kind' (filter by kind), "+
			"'query_file' (symbols in file), 'stats' (index statistics), 'clear' (reset index). "+
			"Useful for understanding codebase structure, finding definitions, and navigation.",
		func(ctx context.Context, params SymbolIndexParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			si := getSymbolIndex(workDir)
			action := strings.ToLower(params.Action)

			// Set defaults
			if params.Limit == 0 {
				params.Limit = 20
			}
			if params.Depth == 0 {
				params.Depth = 10
			}

			switch action {
			case "index":
				return handleIndexAction(ctx, si, params, workDir)
			case "query":
				return handleQueryAction(si, params)
			case "query_kind":
				return handleQueryKindAction(si, params)
			case "query_file":
				return handleQueryFileAction(si, params, workDir)
			case "stats":
				return handleStatsAction(si)
			case "clear":
				return handleClearAction(si)
			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action: %s. Valid: index, query, query_kind, query_file, stats, clear", action)), nil
			}
		})
}

func handleIndexAction(ctx context.Context, si *intelligence.SymbolIndex, params SymbolIndexParams, workDir string) (fantasy.ToolResponse, error) {
	if params.Path == "" {
		return fantasy.NewTextErrorResponse("path required for index action"), nil
	}

	// Resolve path
	path := params.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}

	// Determine if file or directory
	var err error
	var indexed int

	// Try as directory first
	err = si.IndexDir(ctx, path, params.Depth)
	if err != nil {
		// Try as single file
		err = si.IndexFile(ctx, path)
		if err != nil {
			return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to index %s: %v", path, err)), nil
		}
		indexed = 1
	} else {
		stats := si.Stats()
		indexed = stats.Files
	}

	response := SymbolIndexResponse{
		Action:  "index",
		Indexed: indexed,
	}

	summary := fmt.Sprintf("Indexed %d files. Use 'stats' action to see symbol counts.", indexed)
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(summary), response), nil
}

func handleQueryAction(si *intelligence.SymbolIndex, params SymbolIndexParams) (fantasy.ToolResponse, error) {
	if params.Name == "" {
		return fantasy.NewTextErrorResponse("name required for query action"), nil
	}

	symbols := si.Query(params.Name, params.Limit)
	response := SymbolIndexResponse{
		Action:  "query",
		Symbols: symbols,
	}

	if len(symbols) == 0 {
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf("No symbols found matching '%s'", params.Name)), response), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d symbol(s) matching '%s':\n", len(symbols), params.Name))
	for _, sym := range symbols {
		exported := ""
		if sym.Exported {
			exported = " [exported]"
		}
		parent := ""
		if sym.Parent != "" {
			parent = fmt.Sprintf(" (%s)", sym.Parent)
		}
		sb.WriteString(fmt.Sprintf("  %s %s%s%s\n  → %s:%d\n", sym.Kind, sym.Name, parent, exported, sym.File, sym.Line))
		if sym.Signature != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", sym.Signature))
		}
	}

	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(sb.String()), response), nil
}

func handleQueryKindAction(si *intelligence.SymbolIndex, params SymbolIndexParams) (fantasy.ToolResponse, error) {
	if params.Kind == "" {
		return fantasy.NewTextErrorResponse("kind required for query_kind action. Valid: function, method, struct, interface, class, variable, constant, type, enum, field"), nil
	}

	symbols := si.QueryByKind(intelligence.SymbolKind(params.Kind), params.Limit)
	response := SymbolIndexResponse{
		Action:  "query_kind",
		Symbols: symbols,
	}

	if len(symbols) == 0 {
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf("No symbols of kind '%s' found", params.Kind)), response), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d %s(s):\n", len(symbols), params.Kind))
	for _, sym := range symbols {
		sb.WriteString(fmt.Sprintf("  %s → %s:%d\n", sym.Name, sym.File, sym.Line))
	}

	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(sb.String()), response), nil
}

func handleQueryFileAction(si *intelligence.SymbolIndex, params SymbolIndexParams, workDir string) (fantasy.ToolResponse, error) {
	if params.Path == "" {
		return fantasy.NewTextErrorResponse("path required for query_file action"), nil
	}

	// Resolve path
	path := params.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}

	symbols := si.QueryByFile(path)
	response := SymbolIndexResponse{
		Action:  "query_file",
		Symbols: symbols,
	}

	if len(symbols) == 0 {
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf("No symbols found in %s (index it first?)", path)), response), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d symbol(s) in %s:\n", len(symbols), path))
	for _, sym := range symbols {
		exported := ""
		if sym.Exported {
			exported = " [exported]"
		}
		sb.WriteString(fmt.Sprintf("  L%d: %s %s%s\n", sym.Line, sym.Kind, sym.Name, exported))
	}

	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(sb.String()), response), nil
}

func handleStatsAction(si *intelligence.SymbolIndex) (fantasy.ToolResponse, error) {
	stats := si.Stats()

	byKindStr := make(map[string]int)
	for k, v := range stats.ByKind {
		byKindStr[string(k)] = v
	}

	response := SymbolIndexResponse{
		Action: "stats",
		Stats: &IndexStatsResponse{
			Files:        stats.Files,
			TotalSymbols: stats.TotalSymbols,
			ByKind:       byKindStr,
		},
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Symbol Index Statistics:\n"))
	sb.WriteString(fmt.Sprintf("  Files indexed: %d\n", stats.Files))
	sb.WriteString(fmt.Sprintf("  Total symbols: %d\n", stats.TotalSymbols))
	if len(stats.ByKind) > 0 {
		sb.WriteString("  By kind:\n")
		for kind, count := range stats.ByKind {
			sb.WriteString(fmt.Sprintf("    %s: %d\n", kind, count))
		}
	}

	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(sb.String()), response), nil
}

func handleClearAction(si *intelligence.SymbolIndex) (fantasy.ToolResponse, error) {
	si.Clear()
	response := SymbolIndexResponse{
		Action: "clear",
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse("Symbol index cleared"), response), nil
}
