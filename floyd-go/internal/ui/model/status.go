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
	com         *common.Common
	hideHelp    bool
	help        help.Model
	helpKm      help.KeyMap
	msg         util.InfoMsg
	width       int
	planActive  bool
	swarmActive bool
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

// SetSwarmActive shows or hides the swarm-mode pill. Swarm state is per
// session and durable, so the pill reflects stored state rather than a
// transient toggle.
func (s *Status) SetSwarmActive(active bool) {
	if s.swarmActive == active {
		return
	}
	s.swarmActive = active
	s.applyHelpWidth()
}

func (s *Status) applyHelpWidth() {
	helpStyle := s.com.Styles.Status.Help
	horizontalPadding := helpStyle.GetPaddingLeft() + helpStyle.GetPaddingRight()
	width := s.width - horizontalPadding
	width -= s.pillsWidth()
	s.help.SetWidth(max(width, 0))
}

// pills returns the active mode pills in draw order, left to right.
func (s *Status) pills() []string {
	// Pills.Focused carries a rounded border, which is three rows tall.
	// The status bar is one row, so use the borderless base pill.
	var out []string
	if s.planActive {
		out = append(out, s.com.Styles.Pills.Base.Render("plan"))
	}
	if s.swarmActive {
		out = append(out, s.com.Styles.Pills.Base.Render("swarm"))
	}
	return out
}

// pillsWidth is the horizontal space the mode pills occupy, including
// the one-column gap before each.
func (s *Status) pillsWidth() int {
	width := 0
	for _, pill := range s.pills() {
		width += lipgloss.Width(pill) + 1
	}
	return width
}

func (s *Status) drawPills(scr uv.Screen, area uv.Rectangle) {
	pills := s.pills()
	if len(pills) == 0 {
		return
	}
	x := area.Max.X
	// Draw right to left so the first pill in the list stays leftmost.
	for i := len(pills) - 1; i >= 0; i-- {
		width := lipgloss.Width(pills[i])
		if width == 0 || width >= area.Dx() {
			continue
		}
		x -= width
		pillArea := area
		pillArea.Min.X = x
		uv.NewStyledString(pills[i]).Draw(scr, pillArea)
		x -= 1
	}
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
		s.drawPills(scr, area)
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
	// Mode pills are drawn last so notifications cannot hide them.
	// Reserve their width here or the message tail is painted over.
	avail = max(0, avail-s.pillsWidth())
	msg := strings.Join(strings.Split(s.msg.Msg, "\n"), " ")
	msg = ansi.Truncate(msg, avail, "…")
	if w := lipgloss.Width(msg); w < avail {
		msg += strings.Repeat(" ", avail-w)
	}
	info := msgStyle.Render(msg)

	// Draw the info message over the help view
	uv.NewStyledString(ind+info).Draw(scr, area)

	// The mode pills are drawn last so transient notifications never hide
	// an active mode.
	s.drawPills(scr, area)
}

// clearInfoMsgCmd returns a command that clears the info message after the
// given TTL.
func clearInfoMsgCmd(ttl time.Duration) tea.Cmd {
	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return util.ClearStatusMsg{}
	})
}
