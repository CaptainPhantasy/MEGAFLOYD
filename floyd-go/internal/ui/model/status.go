package model

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

// DefaultStatusTTL is the default time-to-live for status messages.
const DefaultStatusTTL = 5 * time.Second

// Status is the status bar and help model.
type Status struct {
	com        *common.Common
	hideHelp   bool
	help       help.Model
	helpKm     help.KeyMap
	msg        util.InfoMsg
	width      int
	planActive bool
}

// NewStatus creates a new status bar and help model.
func NewStatus(com *common.Common, km help.KeyMap) *Status {
	s := new(Status)
	s.com = com
	s.help = help.New()
	s.help.Styles = com.Styles.Help
	s.helpKm = km
	return s
}

// SetInfoMsg sets the status info message.
func (s *Status) SetInfoMsg(msg util.InfoMsg) {
	s.msg = msg
}

// ClearInfoMsg clears the status info message.
func (s *Status) ClearInfoMsg() {
	s.msg = util.InfoMsg{}
}

// SetWidth sets the width of the status bar and help view.
func (s *Status) SetWidth(width int) {
	s.width = width
	s.applyHelpWidth()
}

// SetPlanActive shows or hides the plan-mode pill and narrows the help
// view so the two do not overlap.
func (s *Status) SetPlanActive(active bool) {
	if s.planActive == active {
		return
	}
	s.planActive = active
	s.applyHelpWidth()
}

func (s *Status) applyHelpWidth() {
	helpStyle := s.com.Styles.Status.Help
	horizontalPadding := helpStyle.GetPaddingLeft() + helpStyle.GetPaddingRight()
	width := s.width - horizontalPadding
	if s.planActive {
		width -= lipgloss.Width(s.planPill()) + 1
	}
	s.help.SetWidth(max(width, 0))
}

func (s *Status) planPill() string {
	// Pills.Focused carries a rounded border, which is three rows tall.
	// The status bar is one row, so use the borderless base pill.
	return s.com.Styles.Pills.Base.Render("plan")
}

func (s *Status) drawPlanPill(scr uv.Screen, area uv.Rectangle) {
	if !s.planActive {
		return
	}
	pill := s.planPill()
	width := lipgloss.Width(pill)
	if width == 0 || width >= area.Dx() {
		return
	}
	pillArea := area
	pillArea.Min.X = area.Max.X - width
	uv.NewStyledString(pill).Draw(scr, pillArea)
}

// ShowingAll returns whether the full help view is shown.
func (s *Status) ShowingAll() bool {
	return s.help.ShowAll
}

// ToggleHelp toggles the full help view.
func (s *Status) ToggleHelp() {
	s.help.ShowAll = !s.help.ShowAll
}

// SetHideHelp sets whether the app is on the onboarding flow.
func (s *Status) SetHideHelp(hideHelp bool) {
	s.hideHelp = hideHelp
}

// Draw draws the status bar onto the screen.
func (s *Status) Draw(scr uv.Screen, area uv.Rectangle) {
	if !s.hideHelp {
		helpView := s.com.Styles.Status.Help.Render(s.help.View(s.helpKm))
		uv.NewStyledString(helpView).Draw(scr, area)
	}

	// Render notifications
	if s.msg.IsEmpty() {
		s.drawPlanPill(scr, area)
		return
	}

	var indStyle lipgloss.Style
	var msgStyle lipgloss.Style
	switch s.msg.Type {
	case util.InfoTypeError:
		indStyle = s.com.Styles.Status.ErrorIndicator
		msgStyle = s.com.Styles.Status.ErrorMessage
	case util.InfoTypeWarn:
		indStyle = s.com.Styles.Status.WarnIndicator
		msgStyle = s.com.Styles.Status.WarnMessage
	case util.InfoTypeUpdate:
		indStyle = s.com.Styles.Status.UpdateIndicator
		msgStyle = s.com.Styles.Status.UpdateMessage
	case util.InfoTypeInfo:
		indStyle = s.com.Styles.Status.InfoIndicator
		msgStyle = s.com.Styles.Status.InfoMessage
	case util.InfoTypeSuccess:
		indStyle = s.com.Styles.Status.SuccessIndicator
		msgStyle = s.com.Styles.Status.SuccessMessage
	}

	ind := indStyle.String()
	indWidth := lipgloss.Width(ind)
	msgPad := msgStyle.GetPaddingLeft() + msgStyle.GetPaddingRight()
	avail := max(0, area.Dx()-indWidth-msgPad)
	msg := strings.Join(strings.Split(s.msg.Msg, "\n"), " ")
	msg = ansi.Truncate(msg, avail, "…")
	if w := lipgloss.Width(msg); w < avail {
		msg += strings.Repeat(" ", avail-w)
	}
	info := msgStyle.Render(msg)

	// Draw the info message over the help view
	uv.NewStyledString(ind+info).Draw(scr, area)

	// The plan pill is drawn last so transient notifications never hide
	// the active mode.
	s.drawPlanPill(scr, area)
}

// clearInfoMsgCmd returns a command that clears the info message after the
// given TTL.
func clearInfoMsgCmd(ttl time.Duration) tea.Cmd {
	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return util.ClearStatusMsg{}
	})
}
