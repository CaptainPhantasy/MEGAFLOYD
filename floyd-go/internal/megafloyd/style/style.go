// Package style injects the owner-mandated output-simplification directive
// into the agent's static protocol kernel.
//
// The owner runs many sessions at once and must read every reply at a glance
// on a phone. The directive in [Directive] forces answer-first, short-line,
// plain-word output in every session. It exists to protect the owner's
// attention, not to compensate for missing technical knowledge.
//
// Set MEGAFLOYD_OUTPUT_STYLE=off to disable the directive for a process;
// [Apply] then returns the kernel unchanged. Any other value, including an
// unset variable, keeps the directive on.
package style

import (
	"os"
	"strings"

	"github.com/legacy-ai/floyd/internal/megafloyd"
)

// EnvVar is the environment variable that controls the directive.
const EnvVar = megafloyd.EnvPrefix + "OUTPUT_STYLE"

// offValue disables the directive when assigned to [EnvVar].
const offValue = "off"

// Directive is the canonical output-simplification rules block. It is
// appended to the protocol kernel as its own section.
const Directive = `## OUTPUT STYLE — SIMPLIFIED (MANDATORY)

Readable at a glance on a phone. Short lines. Plain words. Decision first.
This is a bandwidth rule: the reader is technical and runs many sessions at
once. It protects attention, not comprehension.

- ANSWER FIRST. Line one carries the outcome, finding, or decision. Detail follows.
- ONE IDEA PER LINE. One fact or step per line. Keep lines short.
- NO PREAMBLE. No greetings, no restating the question, no "let me" openers.
- NO RECAP, NO FILLER. Do not repeat what was just said or pad with transitions.
- NO BUZZWORDS. Use the plain word. If a term is unavoidable, define it in one short sentence.
- STATUS IS FACTS. Say what changed, where, and what remains. The diff is the evidence.
- DECISIONS ARE EXPLICIT. When a choice is needed, list the options with a recommendation. Never bury it in prose.
- NO REPEATS. Give every fact the reader needs the first time. Never make them ask again.
- EXEMPT: code, diffs, paths, commands, and error output. Never shorten or reformat them for brevity.`

// Apply returns kernel with [Directive] appended as its own section.
//
// Apply is idempotent: a kernel that already contains the directive is
// returned unchanged, so repeated calls never duplicate it. When [EnvVar] is
// set to "off" (case-insensitive), kernel is returned unchanged.
func Apply(kernel string) string {
	if disabled() {
		return kernel
	}
	if strings.Contains(kernel, Directive) {
		return kernel
	}
	kernel = strings.TrimRight(kernel, "\n")
	if kernel == "" {
		return Directive + "\n"
	}
	return kernel + "\n\n" + Directive + "\n"
}

// disabled reports whether the escape hatch is set.
func disabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvVar)), offValue)
}
