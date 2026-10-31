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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Approval binds the original human answers to the operation and target bytes.
type Approval struct {
	TargetSet  string                `json:"target_set"`
	Targets    []string              `json:"targets"`
	Command    string                `json:"command"`
	Recursive  bool                  `json:"recursive"`
	Snapshots  map[string]string     `json:"snapshots"`
	Operations map[string]string     `json:"operations,omitempty"`
	Moves      map[string]MoveSource `json:"moves,omitempty"`
	Copies     map[string]CopySource `json:"copies,omitempty"`
}

// MoveSource binds the incoming entry before approval and launch. Relocated
// omits location, inode and timestamps so copied entries can also be verified.
// Identity still binds the original source entry before the tool may launch.
type MoveSource struct {
	Path      string `json:"path"`
	Identity  string `json:"identity"`
	Relocated string `json:"relocated"`
}

// CopySource binds retained source identity and expected destination bytes.
// Parent identities prevent a directory alias change from substituting paths.
type CopySource struct {
	Path                       string `json:"path"`
	Identity                   string `json:"identity"`
	Content                    string `json:"content"`
	OriginalDestinationContent string `json:"original_destination_content"`
	SourceParent               string `json:"source_parent"`
	DestinationParent          string `json:"destination_parent"`
	PreserveMetadata           string `json:"preserve_metadata,omitempty"`
}

// Attempt records both launch and outcome so a restart cannot assume success.
type Attempt struct {
	ToolCallID        string                `json:"tool_call_id"`
	ApprovalSet       string                `json:"approval_set"`
	Targets           []string              `json:"targets"`
	State             string                `json:"state"`
	RecordedAt        string                `json:"recorded_at"`
	Moves             map[string]MoveSource `json:"moves,omitempty"`
	Copies            map[string]CopySource `json:"copies,omitempty"`
	BackgroundOutcome *BackgroundResult     `json:"background_outcome,omitempty"`
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
	return snapshotEntry(target, true)
}

func snapshotEntry(target string, bindParent bool) (string, error) {
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
	if bindParent {
		fmt.Fprintln(hash, parent)
	}
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
		metadataFields := []any{rel, info.Mode().String(), info.Size(), info.ModTime().UnixNano(), dev, ino}
		if !bindParent {
			// mv across filesystems copies payload and type, then unlinks the source.
			// Inodes and directory allocation sizes necessarily change in that case.
			var size int64
			if info.Mode().IsRegular() {
				size = info.Size()
			}
			metadataFields = []any{rel, info.Mode().String(), size}
		}
		metadata, _ := json.Marshal(metadataFields)
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
	if a.TargetSet != Digest(a.Targets) || len(a.Targets) == 0 {
		return false
	}
	for _, target := range a.Targets {
		if a.Snapshots[target] == "" || a.Operations[target] == "" {
			return false
		}
	}
	for _, target := range c.Targets {
		if source, move := moveSource(target, c.Operations[target]); move && (a.Moves[target].Path != source || a.Moves[target].Identity == "" || a.Moves[target].Relocated == "") {
			return false
		}
		if source, copy := copySource(target, c.Operations[target]); copy && (a.Copies[target].Path != source || a.Copies[target].Identity == "" || a.Copies[target].Content == "" || a.Copies[target].OriginalDestinationContent == "" || a.Copies[target].SourceParent == "" || a.Copies[target].DestinationParent == "" || copyPreservesMetadata(c.Operations[target]) != (a.Copies[target].PreserveMetadata != "")) {
			return false
		}
		if a.Operations[target] == "" || a.Operations[target] != c.Operations[target] {
			return false
		}
	}
	return len(c.Targets) > 0
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
			if err != nil || a.Snapshots[target] == "" || now != a.Snapshots[target] || (!unchangedMoveSource(a, target) || !unchangedCopySource(a, target)) {
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
	moves, err := captureMoveSources(c)
	if err != nil {
		return err
	}
	copies, err := captureCopySources(c)
	if err != nil {
		return err
	}
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
				if snapshots[target] != before || (!reflect.DeepEqual(moves[target], a.Moves[target]) || !reflect.DeepEqual(copies[target], a.Copies[target])) {
					return errors.New("target changed after approval")
				}
			}
			return nil
		}
	}
	record.Approvals = append(record.Approvals, Approval{TargetSet: c.Digest(), Targets: CanonicalTargets(c.Targets), Command: c.Command, Recursive: c.Recursive, Snapshots: snapshots, Operations: c.Operations, Moves: moves, Copies: copies})
	return g.ledger.write(sessionID, record)
}

// BeginExecution durably marks an authorized operation pending before launch.
func (g *Guard) BeginExecution(sessionID string, c Classification, callID string) error {
	if !c.IsDestructive() || c.Unresolved != "" || len(c.Targets) == 0 {
		return ErrNotConfirmed
	}
	// Obtain metadata before the final identity check. No callback or stale
	// pre-lock approval may substitute new snapshots after that check.
	recordedAt := g.now().UTC().Format(time.RFC3339Nano)
	g.ledger.mu.Lock()
	defer g.ledger.mu.Unlock()
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return err
	}
	if err := inPlaceTargetIdentity(c); err != nil {
		return err
	}
	if reason := recoveryDenial(record, c); reason != "" {
		return errors.New(reason)
	}
	approval, ok := reusableApproval(record, c)
	if !ok {
		return fmt.Errorf("%w: the original target identity and operation are not verified", ErrNotConfirmed)
	}
	if err := g.ledger.background.reserve(sessionID, callID); err != nil {
		return err
	}
	record.Attempts = append(record.Attempts, Attempt{ToolCallID: callID, ApprovalSet: approval.TargetSet, Targets: c.Targets, State: "started", RecordedAt: recordedAt, Moves: approval.Moves, Copies: approval.Copies})
	if err := g.ledger.write(sessionID, record); err != nil {
		g.ledger.background.release(sessionID, callID)
		return err
	}
	return nil
}

// RecordOutcome records proven target successes, retaining uncertain launches.
func (g *Guard) RecordOutcome(sessionID string, c Classification, callID string, successfulTargets []string, uncertain bool) error {
	return g.recordOutcome(sessionID, c, callID, successfulTargets, uncertain, nil)
}

func (g *Guard) recordOutcome(sessionID string, c Classification, callID string, successfulTargets []string, uncertain bool, background *BackgroundResult) error {
	g.ledger.mu.Lock()
	defer g.ledger.mu.Unlock()
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return err
	}
	if background != nil {
		matched := false
		for _, job := range record.BackgroundJobs {
			if reflect.DeepEqual(job, background.Job) {
				matched = true
				break
			}
		}
		if !matched || background.Job.SessionID != sessionID || background.Job.ToolCallID != callID || !reflect.DeepEqual(background.Job.Classification, c) {
			return errors.New("background outcome does not match the durable execution identity")
		}
	}
	a, ok := approvalForSubset(record, c)
	if !ok {
		return ErrNotConfirmed
	}
	started := false
	var launchedMoves map[string]MoveSource
	var launchedCopies map[string]CopySource
	for _, attempt := range record.Attempts {
		if attempt.ToolCallID != callID {
			continue
		}
		if background != nil && attempt.BackgroundOutcome != nil {
			if reflect.DeepEqual(*attempt.BackgroundOutcome, *background) {
				return nil
			}
			return errors.New("conflicting terminal background outcome")
		}
		if attempt.State == "finished" {
			if background != nil {
				return nil // repeated terminal observations are idempotent
			}
			return errors.New("execution outcome already recorded")
		}
		if attempt.State == "started" && attempt.ApprovalSet == a.TargetSet && slices.Equal(CanonicalTargets(attempt.Targets), CanonicalTargets(c.Targets)) {
			started = true
			launchedMoves = attempt.Moves
			launchedCopies = attempt.Copies
		}
	}
	if !started {
		return errors.New("execution outcome has no matching durable launch")
	}
	if len(successfulTargets) > 0 && !subset(successfulTargets, c.Targets) {
		return errors.New("completion includes an unauthorized target")
	}
	toolFinished := !uncertain
	var completed []string
	for _, target := range successfulTargets {
		before := a.Snapshots[target]
		current, err := snapshot(target)
		if err != nil {
			return err
		}
		// Absence proves an unlink outcome, not an in-place transformation.
		// An already absent unlink target is a successful harmless no-op.
		if current == "absent" && c.RemovesTarget(target) {
			completed = append(completed, target)
			continue
		}
		if !c.RemovesTarget(target) {
			proven, err := copyPostcondition(target, c.Operations[target], launchedCopies[target])
			if err == nil && !proven {
				proven, err = movePostcondition(target, c.Operations[target], launchedMoves[target])
			}
			if err != nil {
				return err
			}
			if !proven {
				proven, err = inPlacePostcondition(target, c.Operations[target])
			}
			if err != nil {
				return err
			}
			if proven {
				completed = append(completed, target)
				continue
			}
		}
		// An unrelated metadata/content change cannot prove the requested
		// destructive effect. Keep that outcome uncertain, never successful.
		if current != before {
			uncertain = true
		}
		if info, err := os.Lstat(target); err == nil && !info.Mode().IsRegular() && !c.RemovesTarget(target) {
			uncertain = true
		}
	}
	if toolFinished {
		for _, target := range c.Targets {
			if _, copy := copySource(target, c.Operations[target]); copy {
				proven, err := copyPostcondition(target, c.Operations[target], launchedCopies[target])
				if err != nil {
					return err
				}
				current, err := snapshot(target)
				if err != nil {
					return err
				}
				if proven {
					// A preexisting payload or unrelated timestamp change cannot
					// prove that a failed/skipped copy ran. Inferred completion
					// requires a bound transition from different destination bytes;
					// explicit command success can also prove a successful no-op.
					transition := launchedCopies[target].OriginalDestinationContent != launchedCopies[target].Content
					if slices.Contains(successfulTargets, target) || (transition && current != a.Snapshots[target]) {
						completed = append(completed, target)
					} else if current != a.Snapshots[target] {
						uncertain = true
					}
				} else if current != a.Snapshots[target] || !unchangedCopySource(a, target) {
					uncertain = true
				}
			}
			if _, move := moveSource(target, c.Operations[target]); move {
				proven, err := movePostcondition(target, c.Operations[target], launchedMoves[target])
				if err != nil {
					return err
				}
				if proven {
					completed = append(completed, target)
				} else {
					current, err := snapshot(target)
					if err != nil {
						return err
					}
					if current != a.Snapshots[target] || !unchangedMoveSource(a, target) {
						uncertain = true
					}
				}
			}
			if c.RemovesTarget(target) && a.Snapshots[target] != "" && a.Snapshots[target] != "absent" {
				current, err := snapshot(target)
				if err != nil {
					return err
				}
				if current == "absent" {
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
	record.Attempts = append(record.Attempts, Attempt{ToolCallID: callID, ApprovalSet: a.TargetSet, Targets: c.Targets, State: state, RecordedAt: g.now().UTC().Format(time.RFC3339Nano), BackgroundOutcome: background})
	if err := g.ledger.write(sessionID, record); err != nil {
		return err
	}
	if background == nil {
		g.ledger.background.release(sessionID, callID)
	}
	return nil
}

// inPlacePostcondition proves the final size for an empty redirect or one
// absolute numeric truncate. Other producers, relative sizes, reference files,
// block units and compound effects never become success by default.
func inPlacePostcondition(target, signature string) (bool, error) {
	var operations [][]string
	if json.Unmarshal([]byte(signature), &operations) != nil || len(operations) != 1 || len(operations[0]) == 0 {
		return false, nil
	}
	operation := operations[0]
	if (operation[0] == ">" || operation[0] == ">|" || operation[0] == "&>") && len(operation) == 2 && strings.TrimSpace(operation[1]) == "" {
		info, err := os.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return info.Mode().IsRegular() && info.Size() == 0, nil
	}
	if filepath.Base(operation[0]) != "truncate" {
		return false, nil
	}
	args := operation[1:]
	value := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "--no-create" || arg == "--":
		case arg == "-s" || arg == "--size":
			if value != "" || i+1 >= len(args) {
				return false, nil
			}
			i++
			value = args[i]
		case strings.HasPrefix(arg, "--size="):
			if value != "" {
				return false, nil
			}
			value = strings.TrimPrefix(arg, "--size=")
		case strings.HasPrefix(arg, "-s") && len(arg) > 2:
			if value != "" {
				return false, nil
			}
			value = arg[2:]
		default:
			return false, nil
		}
	}
	if value == "" {
		return false, nil
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false, nil
		}
	}
	expected, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return false, nil
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular() && info.Size() == expected, nil
}

// In-place operations follow a final symlink, whereas an unlink removes the
// link itself. A link-only fingerprint cannot approve changing referent data.
func inPlaceTargetIdentity(c Classification) error {
	for _, target := range c.Targets {
		if _, move := moveSource(target, c.Operations[target]); c.RemovesTarget(target) || move {
			continue
		}
		info, err := os.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("permanent deletion is blocked because the in-place target cannot be inspected: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("permanent deletion is blocked because an in-place operation on a symbolic link cannot verify the referenced item; name the explicit regular-file target instead")
		}
	}
	return nil
}

// moveSource accepts one classifier-owned move descriptor, whose final operands
// are the canonical source and effective destination (including directory moves).
func moveSource(target, signature string) (string, bool) {
	var operations [][]string
	if json.Unmarshal([]byte(signature), &operations) != nil || len(operations) != 1 {
		return "", false
	}
	operation := operations[0]
	if len(operation) < 3 || filepath.Base(operation[0]) != "mv" || operation[len(operation)-1] != target {
		return "", false
	}
	source := operation[len(operation)-2]
	if !filepath.IsAbs(source) || !filepath.IsAbs(target) || source == target {
		return "", false
	}
	return source, true
}

func captureMoveSources(c Classification) (map[string]MoveSource, error) {
	var moves map[string]MoveSource
	for _, target := range c.Targets {
		source, move := moveSource(target, c.Operations[target])
		if !move {
			continue
		}
		identity, err := snapshot(source)
		if err != nil {
			return nil, err
		}
		if identity == "absent" {
			return nil, errors.New("move source is absent; its replacement cannot be verified")
		}
		relocated, err := snapshotEntry(source, false)
		if err != nil {
			return nil, err
		}
		if moves == nil {
			moves = make(map[string]MoveSource)
		}
		moves[target] = MoveSource{Path: source, Identity: identity, Relocated: relocated}
	}
	return moves, nil
}

func unchangedMoveSource(a Approval, target string) bool {
	source, move := moveSource(target, a.Operations[target])
	if !move {
		return true
	}
	before := a.Moves[target]
	if before.Path != source || before.Identity == "" || before.Relocated == "" {
		return false
	}
	now, err := snapshot(source)
	if err != nil || now != before.Identity {
		return false
	}
	relocated, err := snapshotEntry(source, false)
	return err == nil && relocated == before.Relocated
}

func movePostcondition(target, signature string, before MoveSource) (bool, error) {
	source, move := moveSource(target, signature)
	if !move || before.Path != source || before.Identity == "" || before.Relocated == "" {
		return false, nil
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return false, err
		}
		return false, nil
	}
	after, err := snapshotEntry(target, false)
	return err == nil && after != "absent" && after == before.Relocated, err
}

// copySource accepts a classifier-owned descriptor whose last two operands are
// the canonical source and effective destination. Options stay in the operation
// signature, so a retry cannot silently change copying semantics.
func copySource(target, signature string) (string, bool) {
	var operations [][]string
	if json.Unmarshal([]byte(signature), &operations) != nil || len(operations) != 1 {
		return "", false
	}
	op := operations[0]
	if len(op) < 3 || filepath.Base(op[0]) != "cp" || op[len(op)-1] != target {
		return "", false
	}
	source := op[len(op)-2]
	return source, filepath.IsAbs(source) && filepath.IsAbs(target) && source != target
}

func captureCopySources(c Classification) (map[string]CopySource, error) {
	var copies map[string]CopySource
	for _, target := range c.Targets {
		source, copy := copySource(target, c.Operations[target])
		if !copy {
			continue
		}
		identity, err := snapshot(source)
		if err != nil {
			return nil, err
		}
		content, err := copyPayload(source)
		if err != nil {
			return nil, err
		}
		if identity == "absent" || content == "" {
			return nil, errors.New("copy source must be a verified regular file; symbolic links and directory copies cannot establish this overwrite")
		}
		sourceInfo, err := os.Lstat(source)
		if err != nil {
			return nil, err
		}
		destinationInfo, err := os.Lstat(target)
		if err != nil {
			return nil, err
		}
		if !destinationInfo.Mode().IsRegular() || os.SameFile(sourceInfo, destinationInfo) {
			return nil, errors.New("copy overwrite requires a distinct regular-file destination")
		}
		originalContent, err := copyPayload(target)
		if err != nil {
			return nil, err
		}
		if originalContent == "" {
			return nil, errors.New("copy destination contents cannot be bound")
		}

		sourceParent, err := copyParentIdentity(source)
		if err != nil {
			return nil, err
		}
		destinationParent, err := copyParentIdentity(target)
		if err != nil {
			return nil, err
		}
		after, err := snapshot(source)
		if err != nil {
			return nil, err
		}
		if identity != after {
			return nil, errors.New("copy source changed while binding expected contents")
		}
		if copies == nil {
			copies = make(map[string]CopySource)
		}
		preserveMetadata := ""
		if copyPreservesMetadata(c.Operations[target]) {
			preserveMetadata, err = copyMetadata(sourceInfo)
			if err != nil {
				return nil, err
			}
		}
		copies[target] = CopySource{Path: source, Identity: identity, Content: content, OriginalDestinationContent: originalContent, SourceParent: sourceParent, DestinationParent: destinationParent, PreserveMetadata: preserveMetadata}
	}
	return copies, nil
}

func unchangedCopySource(a Approval, target string) bool {
	source, copy := copySource(target, a.Operations[target])
	if !copy {
		return true
	}
	before := a.Copies[target]
	if before.Path != source || before.Identity == "" || before.Content == "" || before.OriginalDestinationContent == "" || copyPreservesMetadata(a.Operations[target]) != (before.PreserveMetadata != "") {
		return false
	}
	identity, err := snapshot(source)
	if err != nil || identity != before.Identity {
		return false
	}
	if before.PreserveMetadata != "" {
		info, err := os.Lstat(source)
		if err != nil {
			return false
		}
		metadata, err := copyMetadata(info)
		if err != nil || metadata != before.PreserveMetadata {
			return false
		}
	}
	sourceParent, err := copyParentIdentity(source)
	if err != nil || sourceParent != before.SourceParent {
		return false
	}
	destinationParent, err := copyParentIdentity(target)
	return err == nil && destinationParent == before.DestinationParent
}

func copyPostcondition(target, signature string, before CopySource) (bool, error) {
	source, copy := copySource(target, signature)
	if !copy || before.Path != source || before.Identity == "" || before.Content == "" || before.OriginalDestinationContent == "" || copyPreservesMetadata(signature) != (before.PreserveMetadata != "") {
		return false, nil
	}
	identity, err := snapshot(source)
	if err != nil {
		return false, err
	}
	if identity != before.Identity {
		return false, nil
	}
	sourceParent, err := copyParentIdentity(source)
	if err != nil {
		return false, err
	}
	destinationParent, err := copyParentIdentity(target)
	if err != nil {
		return false, err
	}
	if sourceParent != before.SourceParent || destinationParent != before.DestinationParent {
		return false, nil
	}
	content, err := copyPayload(target)
	if err != nil {
		return false, err
	}
	after, err := snapshot(source)
	if err != nil {
		return false, err
	}
	if before.PreserveMetadata != "" {
		for _, file := range []string{source, target} {
			info, err := os.Lstat(file)
			if err != nil {
				return false, err
			}
			metadata, err := copyMetadata(info)
			if err != nil {
				return false, err
			}
			if metadata != before.PreserveMetadata {
				return false, nil
			}
		}
	}
	return content != "" && content == before.Content && after == before.Identity, nil
}

// copyParentIdentity tolerates stable system aliases such as /tmp, but detects
// retargeted aliases or replaced parent directories without binding mutable
// directory timestamps that copying itself changes.
func copyParentIdentity(file string) (string, error) {
	parent, err := filepath.EvalSymlinks(filepath.Dir(file))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return "", errors.New("copy parent identity is unavailable")
	}
	dev, ino := stat.FieldByName("Dev"), stat.FieldByName("Ino")
	if !dev.IsValid() || !ino.IsValid() {
		return "", errors.New("copy parent identity is unavailable")
	}
	bytes, err := json.Marshal([]any{parent, dev.Interface(), ino.Interface()})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:]), nil
}

// copyPayload reads one stable regular-file entry. Comparing the descriptor's
// identity with Lstat before/after rejects final symlinks and path substitution;
// source and destination directory identities are checked separately.
func copyPayload(file string) (string, error) {
	before, err := os.Lstat(file)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() {
		return "", nil
	}
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !sameCopyEntry(before, opened) {
		return "", errors.New("copy file changed while opening")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	after, err := f.Stat()
	if err != nil {
		return "", err
	}
	current, err := os.Lstat(file)
	if err != nil {
		return "", err
	}
	if !sameCopyEntry(before, after) || !sameCopyEntry(before, current) {
		return "", errors.New("copy file changed while reading")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameCopyEntry(a, b os.FileInfo) bool {
	return b.Mode().IsRegular() && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// Preserve-mode copies require the source's ordinary POSIX metadata as well as
// its bytes. Plain copying into an existing file may keep destination mode.
func copyPreservesMetadata(signature string) bool {
	var operations [][]string
	if json.Unmarshal([]byte(signature), &operations) != nil || len(operations) != 1 {
		return false
	}
	op := operations[0]
	for i := 1; i < len(op)-2; i++ {
		arg := op[i]
		if arg == "--" {
			break
		}
		if arg == "-t" || arg == "--target-directory" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-t") || strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.HasPrefix(arg, "-") && strings.ContainsAny(arg[1:], "pa") {
			return true
		}
	}
	return false
}

func copyMetadata(info os.FileInfo) (string, error) {
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return "", errors.New("copy ownership metadata is unavailable")
	}
	uid, gid := stat.FieldByName("Uid"), stat.FieldByName("Gid")
	if !uid.IsValid() || !gid.IsValid() {
		return "", errors.New("copy ownership metadata is unavailable")
	}
	bytes, err := json.Marshal([]any{info.Mode().String(), info.ModTime().UnixNano(), uid.Interface(), gid.Interface()})
	return string(bytes), err
}
