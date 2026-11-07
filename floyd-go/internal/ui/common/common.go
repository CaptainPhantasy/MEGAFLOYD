package common

import (
	"fmt"
	"image"
	"os"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/clipboard"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
)

// MaxAttachmentSize defines the maximum allowed size for file attachments (5 MB).
const MaxAttachmentSize = int64(5 * 1024 * 1024)

// AllowedImageTypes defines the permitted image file types.
var AllowedImageTypes = []string{".jpg", ".jpeg", ".png"}

// Common defines common UI options and configurations.
type Common struct {
	Workspace workspace.Workspace
	Styles    *styles.Styles
}

// Config returns the pure-data configuration associated with this [Common] instance.
func (c *Common) Config() *config.Config {
	return c.Workspace.Config()
}

// DefaultCommon returns the default common UI configurations. The
// explicitly configured theme wins; otherwise the theme is chosen based
// on the large model's provider, and the default theme is used when no
// model is selected.
func DefaultCommon(ws workspace.Workspace) *Common {
	themeName := ""
	if ws != nil {
		if cfg := ws.Config(); cfg != nil && cfg.Options != nil && cfg.Options.TUI != nil {
			themeName = cfg.Options.TUI.Theme
		}
	}
	s := styles.ThemeForConfig(themeName, largeModelProviderID(ws))
	return &Common{
		Workspace: ws,
		Styles:    &s,
	}
}

// largeModelProviderID returns the provider ID of the currently selected
// large model, or the empty string if none is set or the workspace is nil.
func largeModelProviderID(ws workspace.Workspace) string {
	if ws == nil {
		return ""
	}
	cfg := ws.Config()
	if cfg == nil {
		return ""
	}
	return cfg.Models[config.SelectedModelTypeLarge].Provider
}

// IsHyper reports whether the currently selected large model is provided
// by Hyper.
func (c *Common) IsHyper() bool {
	return largeModelProviderID(c.Workspace) == "hyper"
}

// CenterRect returns a new [Rectangle] centered within the given area with the
// specified width and height.
func CenterRect(area uv.Rectangle, width, height int) uv.Rectangle {
	centerX := area.Min.X + area.Dx()/2
	centerY := area.Min.Y + area.Dy()/2
	minX := centerX - width/2
	minY := centerY - height/2
	maxX := minX + width
	maxY := minY + height
	return image.Rect(minX, minY, maxX, maxY)
}

// BottomLeftRect returns a new [Rectangle] positioned at the bottom-left within the given area with the
// specified width and height.
func BottomLeftRect(area uv.Rectangle, width, height int) uv.Rectangle {
	minX := area.Min.X
	maxX := minX + width
	maxY := area.Max.Y
	minY := maxY - height
	return image.Rect(minX, minY, maxX, maxY)
}

// IsFileTooBig checks if the file at the given path exceeds the specified size
// limit.
func IsFileTooBig(filePath string, sizeLimit int64) (bool, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return false, fmt.Errorf("error getting file info: %w", err)
	}

	if fileInfo.Size() > sizeLimit {
		return true, nil
	}

	return false, nil
}

// CopyToClipboard copies text and reports native delivery, or explicitly
// identifies a terminal request whose delivery cannot be acknowledged.
func CopyToClipboard(text, successMessage string) tea.Cmd {
	return CopyToClipboardWithCallback(text, successMessage, nil)
}

// CopyToClipboardWithCallback copies text to clipboard and executes a callback
// before showing the success message.
// This is useful when you need to perform additional actions like clearing UI state.
func CopyToClipboardWithCallback(text, successMessage string, callback tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteText(text); err != nil {
			return tea.Sequence(tea.SetClipboard(text), callback, util.ReportWarn("Copy requested from the terminal; clipboard delivery is not confirmed."))()
		}
		return tea.Sequence(callback, util.ReportInfo(successMessage))()
	}
}
