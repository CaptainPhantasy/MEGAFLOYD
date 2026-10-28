package deletionguard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ConfirmationRequest is the question one confirmation stage puts to the human.
// Stage one names the exact items; stage two states the consequences.
type ConfirmationRequest struct {
	SessionID  string
	ToolCallID string
	Stage      int
	Targets    []string
	Question   string
	Detail     string
}

// Confirmer asks the human and blocks for the answer. A nil Confirmer means no
// human channel is available, so permanent deletion fails closed.
type Confirmer interface {
	Confirm(ctx context.Context, req ConfirmationRequest) (bool, error)
}

// Decision is the gate's answer for one tool call.
type Decision struct {
	// Allowed reports whether the command may run.
	Allowed bool
	// Record reports an authorized permanent deletion that must be recorded
	// after the command completes.
	Record bool
	// Effect echoes the classified effect.
	Effect Effect
	// Targets are the exact targets the decision covers.
	Targets []string
	// Reason explains a denial in plain language.
	Reason string
}

// Guard decides whether a classified removal may run and records the human
// confirmations it obtains.
type Guard struct {
	ledger    *Ledger
	confirmer Confirmer
	now       func() time.Time
}

// NewGuard returns a guard backed by the durable session record in dataDir.
func NewGuard(dataDir string, confirmer Confirmer) *Guard {
	return &Guard{ledger: NewLedger(dataDir), confirmer: confirmer, now: time.Now}
}

// SetConfirmer installs the human confirmation channel.
func (g *Guard) SetConfirmer(confirmer Confirmer) {
	if g != nil {
		g.confirmer = confirmer
	}
}

// Ledger exposes the durable record for diagnostics and tests.
func (g *Guard) Ledger() *Ledger { return g.ledger }

// Check decides whether one classified call may run. Recoverable moves pass
// through; permanent deletion needs both confirmations for its exact target
// set, and a refusal or unreadable record denies it.
func (g *Guard) Check(ctx context.Context, sessionID, toolCallID string, classification Classification) Decision {
	decision := Decision{Allowed: true, Effect: classification.Effect, Targets: classification.Targets}
	if classification.Effect != EffectPermanentDeletion {
		return decision
	}

	decision.Allowed = false
	targetSet := classification.Digest()
	if classification.Unresolved != "" {
		decision.Reason = "Permanent deletion is gated, and " + classification.Unresolved +
			". Pass explicit paths so the exact items can be identified and confirmed."
		return decision
	}
	if len(classification.Targets) == 0 {
		decision.Reason = "Permanent deletion is gated, and the command names no exact target. Name the exact items so they can be confirmed."
		return decision
	}

	record, err := g.ledger.Read(sessionID)
	if err != nil {
		decision.Reason = "Permanent deletion is blocked because the session record cannot be read (" + err.Error() +
			"). Removal means quarantine; move the items to the quarantine directory instead."
		return decision
	}
	if execution, done := record.Executed(targetSet); done {
		decision.Reason = "The confirmed deletion of " + strings.Join(classification.Targets, ", ") +
			" already ran at " + execution.RecordedAt + ". Do not repeat it: inspect the current state and report it." +
			" A new deletion of a different item needs its own two confirmations."
		return decision
	}
	if record.Confirmed(targetSet) {
		decision.Allowed, decision.Record = true, true
		return decision
	}
	if g.confirmer == nil {
		decision.Reason = missingConfirmationReason(record, targetSet, classification)
		return decision
	}

	for stage := 1; stage <= 2; stage++ {
		if _, ok := record.ConfirmationsFor(targetSet)[stage]; ok {
			continue
		}
		request := g.request(sessionID, toolCallID, classification, stage)
		confirmed, confirmErr := g.confirmer.Confirm(ctx, request)
		if confirmErr != nil {
			decision.Reason = "Permanent deletion is blocked because the human confirmation could not be obtained (" +
				confirmErr.Error() + "). Removal means quarantine; move the items instead."
			return decision
		}
		if !confirmed {
			decision.Reason = "Permanent deletion was not confirmed at stage " + strconv.Itoa(stage) + " of 2. Removal means quarantine; move the items instead."
			return decision
		}
		confirmation := Confirmation{
			ID:         uuid.NewString(),
			Stage:      stage,
			TargetSet:  targetSet,
			Targets:    classification.Targets,
			Source:     humanSource,
			Question:   request.Question,
			RecordedAt: g.now().UTC().Format(time.RFC3339Nano),
		}
		if err := g.ledger.AppendConfirmation(sessionID, confirmation); err != nil {
			decision.Reason = "Permanent deletion stays blocked because the human confirmation could not be recorded: " + err.Error()
			return decision
		}
		record.Confirmations = append(record.Confirmations, confirmation)
	}

	decision.Allowed, decision.Record = true, true
	return decision
}

// RecordExecution records that a confirmed deletion ran, so a re-run cannot
// repeat it.
func (g *Guard) RecordExecution(sessionID string, classification Classification, toolCallID string) error {
	if classification.Effect != EffectPermanentDeletion || g == nil {
		return nil
	}
	return g.ledger.AppendExecution(sessionID, Execution{
		ID:         uuid.NewString(),
		TargetSet:  classification.Digest(),
		Targets:    classification.Targets,
		ToolCallID: toolCallID,
		RecordedAt: g.now().UTC().Format(time.RFC3339Nano),
	})
}

// request builds the stage question. The wording follows the project standard:
// stage one identifies the exact items, stage two states the consequences.
func (g *Guard) request(sessionID, toolCallID string, classification Classification, stage int) ConfirmationRequest {
	items := strings.Join(classification.Targets, ", ")
	request := ConfirmationRequest{
		SessionID:  sessionID,
		ToolCallID: toolCallID,
		Stage:      stage,
		Targets:    classification.Targets,
	}
	if stage == 1 {
		request.Question = "I understand you want permanent deletion, not quarantine, of " + items + ". Is that correct?"
		request.Detail = "Removal means quarantine. Confirm the exact items before anything is destroyed. Recovery source: " + classification.Command + "."
		return request
	}
	request.Question = "Knowing these effects, are you certain you want these exact items permanently deleted?"
	request.Detail = "Permanent deletion destroys " + items + ". The content cannot be restored by this harness, and no backup was checked. Quarantine keeps a restore path."
	return request
}

// missingConfirmationReason explains which confirmations are still missing.
func missingConfirmationReason(record Record, targetSet string, classification Classification) string {
	stages := record.ConfirmationsFor(targetSet)
	missing := make([]string, 0, 2)
	for stage := 1; stage <= 2; stage++ {
		if _, ok := stages[stage]; !ok {
			missing = append(missing, strconv.Itoa(stage))
		}
	}
	return fmt.Sprintf(
		"Permanent deletion of %s needs two separate human confirmations for that exact target set; %s of 2 are recorded, and no human confirmation channel is available. Removal means quarantine; move the items instead. (target_set=%s missing_stage=%s)",
		strings.Join(classification.Targets, ", "), strconv.Itoa(2-len(missing)), targetSet, strings.Join(missing, ","),
	)
}
