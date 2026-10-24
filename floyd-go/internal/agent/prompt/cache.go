package prompt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/megafloyd"
)

// CacheablePrompt contains the separated static and dynamic parts of a prompt.
type CacheablePrompt struct {
	// StaticPrompt is the system prompt without dynamic values (cacheable)
	StaticPrompt string
	// DynamicContext is the environment-specific context (changes per request)
	DynamicContext string
}

// BuildCacheablePrompts splits a prompt into static (cacheable) and dynamic parts.
// It extracts content within <env>...</env> tags as dynamic context.
func BuildCacheablePrompts(ctx context.Context, fullPrompt string, data PromptDat) CacheablePrompt {
	// Find the <env> block
	startTag := "<env>"
	endTag := "</env>"

	startIdx := strings.Index(fullPrompt, startTag)
	endIdx := strings.Index(fullPrompt, endTag)

	if startIdx == -1 || endIdx == -1 {
		// No <env> block found, return as-is with empty dynamic context
		return CacheablePrompt{
			StaticPrompt:   fullPrompt,
			DynamicContext: buildDynamicContext(data),
		}
	}

	// Extract static parts (before <env> and after </env>)
	staticPrompt := fullPrompt[:startIdx] + fullPrompt[endIdx+len(endTag):]

	// Build dynamic context from the PromptDat
	dynamicContext := buildDynamicContext(data)

	return CacheablePrompt{
		StaticPrompt:   strings.TrimSpace(staticPrompt),
		DynamicContext: dynamicContext,
	}
}

// DynamicContext captures the current date, Git state, and project memory.
// Call it once when a run starts, then retain that snapshot for its tool loop.
func DynamicContext(ctx context.Context, workingDir string, cfg config.Config) string {
	return buildDynamicContext(PromptDataForDynamic(ctx, workingDir, cfg))
}

// buildDynamicContext constructs the dynamic environment context string.
// This content changes per-request and should NOT be cached.
func buildDynamicContext(data PromptDat) string {
	var sb strings.Builder

	sb.WriteString("<env>\n")
	fmt.Fprintf(&sb, "Working directory: %s\n", data.WorkingDir)

	if data.IsGitRepo {
		sb.WriteString("Is directory a git repo: Yes\n")
		// Add git status if available
		if data.GitStatus != "" {
			// Add git status information
			lines := strings.Split(strings.TrimSpace(data.GitStatus), "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Fprintf(&sb, "%s\n", line)
				}
			}
		}
	} else {
		sb.WriteString("Is directory a git repo: No\n")
	}

	fmt.Fprintf(&sb, "Platform: %s\n", data.Platform)
	fmt.Fprintf(&sb, "Today's date: %s\n", data.Date)

	// Auto-inject .supercache content so the agent doesn't waste a tool call
	// reading it on every boot. The supercache is a small JSON file with
	// project identity and last-known state.
	if supercache := readSupercache(data.WorkingDir, personalPromptDataDir(data.Config)); supercache != "" {
		fmt.Fprintf(&sb, "\nProject state (.supercache): %s\n", supercache)
	}

	sb.WriteString("</env>")

	return sb.String()
}

// readSupercache reads only this distribution's project memory file.
// Returns empty string if not found or on any error.
func readSupercache(workingDir string, selected ...string) string {
	var explicit string
	if len(selected) > 0 {
		explicit = selected[0]
	}
	dataDir, err := megafloyd.PersonalDataDir(workingDir, explicit)
	if err != nil {
		slog.Error("Cannot resolve personal prompt memory", "error", err)
		return "[personal memory unavailable: " + err.Error() + "]"
	}
	path := filepath.Join(dataDir, ".supercache")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("Cannot read personal prompt memory", "error", err)
			return "[personal memory unavailable: " + err.Error() + "]"
		}
		return ""
	}
	content := strings.TrimSpace(string(data))
	// Cap at 2KB to prevent bloated supercaches from poisoning context
	if len(content) > 2048 {
		content = content[:2048] + "... [truncated]"
	}
	return content
}

// PromptDataForDynamic creates a PromptDat for building dynamic context only.
// This is useful when you need to update dynamic context without rebuilding
// the entire system prompt.
func PromptDataForDynamic(ctx context.Context, workingDir string, cfg config.Config) PromptDat {
	isGit := isGitRepo(workingDir)
	data := PromptDat{
		WorkingDir: workingDir,
		Config:     cfg,
		IsGitRepo:  isGit,
		Platform:   runtime.GOOS,
		Date:       time.Now().Format("1/2/2006"),
	}

	if isGit {
		if status, err := getGitStatus(ctx, workingDir); err == nil {
			data.GitStatus = status
		}
	}

	return data
}

func personalPromptDataDir(cfg config.Config) string {
	if cfg.Options == nil {
		return ""
	}
	return cfg.Options.DataDirectory
}
