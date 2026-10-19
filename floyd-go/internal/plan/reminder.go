package plan

import (
	"os"
	"strings"
)

// FullReminder is injected on the first turn after plan mode is entered.
const FullReminder = `Plan mode is active. You MUST NOT make any edits (with the exception of the current plan file) or otherwise make changes to the system unless a tool request is explicitly approved. Prefer read-only tools. Use Bash only when needed; Bash follows the normal permission mode and rules. This supersedes any other instructions you have received. job_kill, cron_create, and cron_delete are also blocked in plan mode — call exit_plan_mode first if you need them.

Workflow:
  1. Understand — explore the codebase with Glob, Grep, Read.
  2. Design — converge on the best approach; consider trade-offs but aim for a single recommendation.
  3. Review — re-read key files to verify understanding.
  4. Write Plan — modify the plan file with Write or Edit. Use Write if the plan file does not exist yet.
  5. Exit — call exit_plan_mode for user approval.

## Handling multiple approaches
Keep it focused: at most 2-3 meaningfully different approaches. Do NOT pad with minor variations — if one approach is clearly superior, just propose that one.
When the best approach depends on user preferences, constraints, or context you don't have, use the question tool to clarify first. This helps you write a better, more targeted plan rather than dumping multiple options for the user to sort through.
When you do include multiple approaches in the plan, present them clearly and recommend one.

Never ask about plan approval via text or the question tool.
Your turn must end with either a clarifying question or exit_plan_mode (to request plan approval). Do NOT end your turn any other way.
Do NOT use the question tool to ask about plan approval or reference "the plan" — the user cannot see the plan until you call exit_plan_mode.`

// SparseReminder is injected on later turns while plan mode stays active.
const SparseReminder = `Plan mode still active (see full instructions earlier). Prefer read-only tools except the current plan file. Use Write or Edit to modify the plan file. If it does not exist yet, create it with Write first. Use Bash only when needed; Bash follows the normal permission mode and rules. If the plan has multiple approaches, present them clearly in the plan. End turns with a clarifying question or exit_plan_mode (for approval). Never ask about plan approval via text or the question tool.`

// ReentryReminder is injected when plan mode is already active but this
// context has not seen the plan file yet (a restarted process or a reset
// conversation).
const ReentryReminder = `Plan mode is active. You MUST NOT make any edits (with the exception of the current plan file) or otherwise make changes to the system unless a tool request is explicitly approved. Prefer read-only tools. Use Bash only when needed; Bash follows the normal permission mode and rules. This supersedes any other instructions you have received.

## Re-entering Plan Mode
A plan file from a previous planning session already exists.
Before proceeding:
  1. Read the existing plan file to understand what was previously planned.
  2. Evaluate the user's current request against that plan.
  3. If different task: replace the old plan with a fresh one. If same task: update the existing plan.
  4. You may use Write or Edit to modify the plan file. If the file does not exist yet, create it with Write first.
  5. Use the question tool to clarify missing requirements or user preferences that affect the plan.
  6. Always edit the plan file before calling exit_plan_mode.

Your turn must end with either a clarifying question or exit_plan_mode (to request plan approval).`

// ExitReminder is injected on the first turn after plan mode ends.
const ExitReminder = `Plan mode is no longer active. The read-only and plan-file-only restrictions from plan mode no longer apply. Continue with the approved plan using the normal tool and permission rules.`

// Reminder returns the reminder to inject for the session's next turn, or
// an empty string when none is due. The first call after entering returns
// the full reminder; later calls return the sparse reminder. If the
// service sees an active plan it has not reminded about yet (a restarted
// process or a reset context), it returns the reentry reminder when the
// plan file already has content.
func (s *Service) Reminder(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState(sessionID)
	if err != nil {
		return ""
	}
	if !state.Active || state.ID == "" {
		if s.reminded[sessionID] {
			delete(s.reminded, sessionID)
			return ExitReminder
		}
		return ""
	}

	path := s.PlanPath(sessionID, state.ID)
	content, _ := os.ReadFile(path)
	hasContent := strings.TrimSpace(string(content)) != ""

	if s.reset[sessionID] {
		delete(s.reset, sessionID)
		s.reminded[sessionID] = true
		if hasContent {
			return withPlanFile(ReentryReminder, path)
		}
		return withPlanFile(FullReminder, path)
	}

	if !s.reminded[sessionID] {
		s.reminded[sessionID] = true
		if hasContent {
			return withPlanFile(ReentryReminder, path)
		}
		return withPlanFile(FullReminder, path)
	}

	return withPlanFile(SparseReminder, path)
}

// MarkContextReset makes the next Reminder a reentry reminder, used after
// the conversation context is compacted or replaced.
func (s *Service) MarkContextReset(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset[sessionID] = true
}

func withPlanFile(body, path string) string {
	if path == "" {
		return body
	}
	return body + "\n\nPlan file: " + path
}
