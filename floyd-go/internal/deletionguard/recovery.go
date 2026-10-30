package deletionguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Approval binds the original human answers to the operation and target bytes.
type Approval struct {
	TargetSet  string            `json:"target_set"`
	Targets    []string          `json:"targets"`
	Command    string            `json:"command"`
	Recursive  bool              `json:"recursive"`
	Snapshots  map[string]string `json:"snapshots"`
	Operations map[string]string `json:"operations,omitempty"`
}

// Attempt records both launch and outcome so a restart cannot assume success.
type Attempt struct {
	ToolCallID  string   `json:"tool_call_id"`
	ApprovalSet string   `json:"approval_set"`
	Targets     []string `json:"targets"`
	State       string   `json:"state"`
	RecordedAt  string   `json:"recorded_at"`
}

func snapshotTargets(targets []string) (map[string]string, error) {
	out := make(map[string]string, len(targets))
	for _, target := range targets {
		value, err := snapshot(target)
		if err != nil {
			return nil, err
		}
		out[target] = value
	}
	return out, nil
}

func snapshot(target string) (string, error) {
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return "absent", nil
	} else if err != nil {
		return "", err
	}
	hash := sha256.New()
	parent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return "", err
	}
	fmt.Fprintln(hash, parent)
	err = filepath.Walk(target, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		// Device and inode distinguish replacement even when bytes are identical.
		identity := reflect.Indirect(reflect.ValueOf(info.Sys()))
		var dev, ino any
		if identity.IsValid() && identity.Kind() == reflect.Struct {
			if v := identity.FieldByName("Dev"); v.IsValid() {
				dev = v.Interface()
			}
			if v := identity.FieldByName("Ino"); v.IsValid() {
				ino = v.Interface()
			}
		}
		metadata, _ := json.Marshal([]any{rel, info.Mode().String(), info.Size(), info.ModTime().UnixNano(), dev, ino})
		hash.Write(metadata)
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(hash, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintln(hash, link)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameOperation(a Approval, c Classification) bool {
	if len(a.Operations) > 0 {
		for _, target := range c.Targets {
			if a.Operations[target] == "" || a.Operations[target] != c.Operations[target] {
				return false
			}
		}
		return len(c.Targets) > 0
	}
	return a.Command == c.Command && a.Recursive == c.Recursive
}

func subset(targets, whole []string) bool {
	for _, target := range targets {
		if !slices.Contains(whole, target) {
			return false
		}
	}
	return len(targets) > 0
}

func overlaps(a, b []string) bool {
	for _, one := range a {
		for _, two := range b {
			rel, err := filepath.Rel(one, two)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func approvalForSubset(record Record, c Classification) (Approval, bool) {
	for _, a := range record.Approvals {
		if record.Confirmed(a.TargetSet) && sameOperation(a, c) && subset(c.Targets, a.Targets) {
			return a, true
		}
	}
	// Existing v1 exact-set confirmations remain valid; they confer no subset authority.
	if record.Confirmed(c.Digest()) {
		return Approval{TargetSet: c.Digest(), Targets: c.Targets, Command: c.Command, Recursive: c.Recursive}, true
	}
	return Approval{}, false
}

func completedFor(record Record, a Approval) []string {
	var out []string
	for _, e := range record.Executions {
		if e.ApprovalSet == a.TargetSet || e.TargetSet == a.TargetSet {
			out = append(out, e.Targets...)
		}
	}
	return CanonicalTargets(out)
}

func reusableApproval(record Record, c Classification) (Approval, bool) {
	for _, a := range record.Approvals {
		if !record.Confirmed(a.TargetSet) || !sameOperation(a, c) {
			continue
		}
		completed := completedFor(record, a)
		var remaining []string
		for _, target := range a.Targets {
			if !slices.Contains(completed, target) {
				remaining = append(remaining, target)
			}
		}
		if !slices.Equal(CanonicalTargets(remaining), CanonicalTargets(c.Targets)) {
			continue
		}
		valid := true
		for _, target := range remaining {
			now, err := snapshot(target)
			if err != nil || a.Snapshots[target] == "" || now != a.Snapshots[target] {
				valid = false
				break
			}
		}
		if valid {
			return a, true
		}
	}
	return Approval{}, false
}

func recoveryDenial(record Record, c Classification) string {
	for i, a := range record.Attempts {
		if a.State != "started" && a.State != "uncertain" {
			continue
		}
		finished := false
		for _, later := range record.Attempts[i+1:] {
			if later.ToolCallID == a.ToolCallID && later.State == "finished" {
				finished = true
			}
		}
		if !finished && (overlaps(a.Targets, c.Targets) || overlaps(c.Targets, a.Targets)) {
			return "Permanent deletion is blocked because the previous execution is still running or its completion is uncertain. Inspect the operation before retrying."
		}
	}
	for _, e := range record.Executions {
		if overlaps(e.Targets, c.Targets) || overlaps(c.Targets, e.Targets) {
			return "The confirmed deletion already ran for a requested target. Do not repeat completed items; retry only the verified remainder."
		}
	}
	for _, a := range record.Approvals {
		if a.TargetSet == c.Digest() || subset(c.Targets, a.Targets) {
			if !record.Confirmed(a.TargetSet) && a.TargetSet == c.Digest() && sameOperation(a, c) {
				valid := true
				for target, before := range a.Snapshots {
					now, err := snapshot(target)
					if err != nil || now != before {
						valid = false
					}
				}
				if valid {
					continue
				}
			}
			if _, ok := reusableApproval(record, c); !ok {
				return "Permanent deletion is blocked because the requested items are not the verified unchanged remainder of the approved operation."
			}
		}
	}
	return ""
}

func (g *Guard) saveApproval(sessionID string, c Classification, snapshots map[string]string) error {
	for target, before := range snapshots {
		after, err := snapshot(target)
		if err != nil {
			return err
		}
		if before != after {
			return errors.New("target changed while obtaining confirmations")
		}
	}
	g.ledger.mu.Lock()
	defer g.ledger.mu.Unlock()
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return err
	}
	for _, a := range record.Approvals {
		if a.TargetSet == c.Digest() {
			if !sameOperation(a, c) {
				return errors.New("operation changed after approval")
			}
			for target, before := range a.Snapshots {
				if snapshots[target] != before {
					return errors.New("target changed after approval")
				}
			}
			return nil
		}
	}
	record.Approvals = append(record.Approvals, Approval{TargetSet: c.Digest(), Targets: CanonicalTargets(c.Targets), Command: c.Command, Recursive: c.Recursive, Snapshots: snapshots, Operations: c.Operations})
	return g.ledger.write(sessionID, record)
}

// BeginExecution durably marks an authorized operation pending before launch.
func (g *Guard) BeginExecution(sessionID string, c Classification, callID string) error {
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return err
	}
	if reason := recoveryDenial(record, c); reason != "" {
		return errors.New(reason)
	}
	a, ok := approvalForSubset(record, c)
	if !ok {
		return ErrNotConfirmed
	}
	if len(a.Snapshots) == 0 {
		snapshots, err := snapshotTargets(c.Targets)
		if err != nil {
			return err
		}
		if err := g.saveApproval(sessionID, c, snapshots); err != nil {
			return err
		}
	}
	g.ledger.mu.Lock()
	defer g.ledger.mu.Unlock()
	record, err = g.ledger.Read(sessionID)
	if err != nil {
		return err
	}
	if reason := recoveryDenial(record, c); reason != "" {
		return errors.New(reason)
	}
	record.Attempts = append(record.Attempts, Attempt{ToolCallID: callID, ApprovalSet: a.TargetSet, Targets: c.Targets, State: "started", RecordedAt: g.now().UTC().Format(time.RFC3339Nano)})
	return g.ledger.write(sessionID, record)
}

// RecordOutcome records proven target successes, retaining uncertain launches.
func (g *Guard) RecordOutcome(sessionID string, c Classification, callID string, successfulTargets []string, uncertain bool) error {
	g.ledger.mu.Lock()
	defer g.ledger.mu.Unlock()
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return err
	}
	a, ok := approvalForSubset(record, c)
	if !ok {
		return ErrNotConfirmed
	}
	started := false
	for _, attempt := range record.Attempts {
		if attempt.ToolCallID != callID {
			continue
		}
		if attempt.State == "finished" {
			return errors.New("execution outcome already recorded")
		}
		if attempt.State == "started" && attempt.ApprovalSet == a.TargetSet && slices.Equal(CanonicalTargets(attempt.Targets), CanonicalTargets(c.Targets)) {
			started = true
		}
	}
	if !started {
		return errors.New("execution outcome has no matching durable launch")
	}
	if len(successfulTargets) > 0 && !subset(successfulTargets, c.Targets) {
		return errors.New("completion includes an unauthorized target")
	}
	var completed []string
	for _, target := range successfulTargets {
		if a.Snapshots[target] != "" && a.Snapshots[target] != "absent" {
			now, err := snapshot(target)
			if err != nil {
				return err
			}
			if now == a.Snapshots[target] || (c.RemovesTarget(target) && now != "absent") {
				continue
			}
		}
		completed = append(completed, target)
	}
	if !uncertain {
		for _, target := range c.Targets {
			if c.RemovesTarget(target) && a.Snapshots[target] != "" && a.Snapshots[target] != "absent" {
				now, err := snapshot(target)
				if err != nil {
					return err
				}
				if now == "absent" {
					completed = append(completed, target)
				}
			}
		}
	}
	completed = CanonicalTargets(completed)
	if len(completed) > 0 {
		record.Executions = append(record.Executions, Execution{ID: uuid.NewString(), TargetSet: Digest(completed), ApprovalSet: a.TargetSet, Targets: completed, ToolCallID: callID, RecordedAt: g.now().UTC().Format(time.RFC3339Nano)})
	}
	state := "finished"
	if uncertain {
		state = "uncertain"
	}
	record.Attempts = append(record.Attempts, Attempt{ToolCallID: callID, ApprovalSet: a.TargetSet, Targets: c.Targets, State: state, RecordedAt: g.now().UTC().Format(time.RFC3339Nano)})
	return g.ledger.write(sessionID, record)
}
