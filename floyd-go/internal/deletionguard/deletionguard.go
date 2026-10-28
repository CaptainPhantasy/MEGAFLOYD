// Package deletionguard enforces the removal rule from the project standard
// before a shell command runs. Removal means quarantine: a command that moves
// data is recoverable and needs no confirmation, while permanent deletion
// requires two separate human confirmations for the exact target set. The
// confirmations are read from the durable per-session record, never from model
// text, and an unreadable record refuses permanent deletion.
package deletionguard

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"

	"github.com/legacy-ai/floyd/internal/filepathext"
)

// Effect is the classified effect of a shell command on stored data.
type Effect int

const (
	// EffectNone means the command does not remove stored data.
	EffectNone Effect = iota
	// EffectQuarantine means the command moves data, which a later move can
	// restore.
	EffectQuarantine
	// EffectPermanentDeletion means the command destroys stored data.
	EffectPermanentDeletion
)

// String returns the effect name used in messages and records.
func (e Effect) String() string {
	switch e {
	case EffectQuarantine:
		return "quarantine"
	case EffectPermanentDeletion:
		return "permanent_deletion"
	default:
		return "none"
	}
}

// Classification is the effect of one tool call and the exact targets it acts
// on.
type Classification struct {
	// Effect is the strongest effect found in the command.
	Effect Effect
	// Command is the command that produced the effect, such as "rm".
	Command string
	// Targets are the absolute, cleaned, sorted and de-duplicated paths the
	// command removes.
	Targets []string
	// Recursive reports a directory-tree removal.
	Recursive bool
	// Unresolved explains why the exact target set cannot be proven. A
	// permanent deletion with an unresolved target set stays blocked until
	// the caller names the exact items.
	Unresolved string
	// Reason explains the classification in one plain-language sentence.
	Reason string
}

// IsDestructive reports whether the classification needs the deletion gate.
func (c Classification) IsDestructive() bool {
	return c.Effect == EffectPermanentDeletion
}

// Digest is the stable identity of this classification's exact target set.
func (c Classification) Digest() string { return Digest(c.Targets) }

// Digest returns the stable identity of an exact target set.
func Digest(targets []string) string {
	hash := sha256.New()
	for _, target := range CanonicalTargets(targets) {
		hash.Write([]byte(target))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// CanonicalTargets cleans, sorts and de-duplicates a target set so the same
// removal always has the same identity.
func CanonicalTargets(targets []string) []string {
	seen := make(map[string]struct{}, len(targets))
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
	}
	slices.Sort(out)
	return out
}

// ClassifyAll merges the effects of several commands from one tool call.
func ClassifyAll(commands []string, workingDir string) Classification {
	merged := Classification{Effect: EffectNone}
	for _, command := range commands {
		classification := Classify(command, workingDir)
		merged.Targets = append(merged.Targets, classification.Targets...)
		merged.Recursive = merged.Recursive || classification.Recursive
		if classification.Effect > merged.Effect {
			merged.Effect = classification.Effect
			merged.Command = classification.Command
			merged.Reason = classification.Reason
		}
		if merged.Unresolved == "" {
			merged.Unresolved = classification.Unresolved
		}
	}
	merged.Targets = CanonicalTargets(merged.Targets)
	return merged
}

// ResolveTarget turns a shell operand into the absolute path a command acts
// on. The working directory only applies to relative operands.
func ResolveTarget(workingDir, name string) (string, error) {
	path := filepathext.SmartJoin(workingDir, name)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
