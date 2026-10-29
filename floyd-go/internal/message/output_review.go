package message

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

// ReviewConfig is trusted host configuration, never provider response data.
// Command optionally proposes a correction over JSON stdin/stdout. It does
// not replace native evidence, exact-artifact or publication checks.
type ReviewConfig struct {
	SecretValues       []string
	Command            []string
	RequireFinalReview bool
}

var ErrOutputReviewPending = errors.New("output review has recoverable unpublished work")

// ReviewRequest identifies the exact private revision under review.
type ReviewRequest struct {
	MessageID    string   `json:"message_id"`
	SessionID    string   `json:"session_id"`
	Revision     int64    `json:"revision"`
	Kind         string   `json:"kind"`
	Text         string   `json:"text"`
	SourceSHA256 string   `json:"source_sha256"`
	Complete     bool     `json:"complete"`
	Protected    []string `json:"protected"`
}

// ReviewClaim binds a declared claim to evidence supplied by the trusted
// review adapter. Text is checked literally; entailment is not inferred.
type ReviewClaim struct {
	Text        string `json:"text"`
	Actor       string `json:"actor"`
	Action      string `json:"action"`
	Result      string `json:"result"`
	Negated     bool   `json:"negated"`
	Uncertainty string `json:"uncertainty"`
	EvidenceID  string `json:"evidence_id"`
}

type ReviewEvidence struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Result string `json:"result"`
}

// ReviewResponse comes only from the configured host review adapter. A
// matching digest binds content; it is not authentication or truth proof.
type ReviewResponse struct {
	SourceSHA256 string           `json:"source_sha256"`
	Text         string           `json:"text"`
	Disposition  string           `json:"disposition"`
	Reviewer     string           `json:"reviewer"`
	Provenance   []string         `json:"provenance"`
	Claims       []ReviewClaim    `json:"claims"`
	Evidence     []ReviewEvidence `json:"evidence"`
}

type ReviewStatus struct {
	MessageID    string          `json:"message_id"`
	Revision     int64           `json:"revision"`
	State        string          `json:"state"`
	SourceSHA256 string          `json:"source_sha256"`
	Receipt      json.RawMessage `json:"receipt"`
}

type ReviewEvent struct {
	Cursor    int64   `json:"cursor"`
	MessageID string  `json:"message_id"`
	Revision  int64   `json:"revision"`
	State     string  `json:"state"`
	Message   Message `json:"message"`
}

type outputReview struct {
	mu   sync.Mutex
	conn *sql.DB
	cfg  ReviewConfig
}

// EnableOutputReview is opt-in before the service is shared with producers.
// The private journal and public projection use the actual workspace DB.
func EnableOutputReview(ctx context.Context, svc Service, cfg ReviewConfig) error {
	s, ok := svc.(*service)
	if !ok || s.db == nil {
		return errors.New("output review requires the actual SQL message service")
	}
	if s.review != nil {
		return nil
	}
	// Seed the exact pre-activation inventory once. A later restart must never
	// grandfather a new row whose producer crashed before journaling it.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := db.New(tx)
	active, err := qtx.CountOutputReviewActivations(ctx)
	if err != nil {
		return err
	}
	if active == 0 {
		err = qtx.SeedLegacyOutputReviewPublic(ctx)
		if err != nil {
			return err
		}
		if err = qtx.CreateOutputReviewActivation(ctx); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	cfg.Command = append([]string(nil), cfg.Command...)
	cfg.SecretValues = append([]string(nil), cfg.SecretValues...)
	sort.Slice(cfg.SecretValues, func(i, j int) bool {
		if len(cfg.SecretValues[i]) != len(cfg.SecretValues[j]) {
			return len(cfg.SecretValues[i]) > len(cfg.SecretValues[j])
		}
		return cfg.SecretValues[i] < cfg.SecretValues[j]
	})
	s.review = &outputReview{conn: s.db, cfg: cfg}
	return nil
}

func OutputReviewEnabled(svc Service) bool { s, ok := svc.(*service); return ok && s.review != nil }
func reviewHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (r *outputReview) redactText(text string) string {
	for _, secret := range r.cfg.SecretValues {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}

func (r *outputReview) redactJSON(raw []byte) ([]byte, error) {
	if len(r.cfg.SecretValues) == 0 {
		return raw, nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(value any) any {
		switch value := value.(type) {
		case string:
			return r.redactText(value)
		case []any:
			for i := range value {
				value[i] = walk(value[i])
			}
			return value
		case map[string]any:
			for key, item := range value {
				value[key] = walk(item)
			}
			return value
		default:
			return value
		}
	}
	return json.Marshal(walk(value))
}

func (r *outputReview) redactMessage(msg Message) (Message, error) {
	raw, err := marshalParts(msg.Parts)
	if err != nil {
		return Message{}, err
	}
	raw, err = r.redactJSON(raw)
	if err != nil {
		return Message{}, err
	}
	out := msg.Clone()
	out.Parts, err = unmarshalParts(raw)
	return out, err
}

func (r *outputReview) validateResponse(request ReviewRequest, response ReviewResponse) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	raw, err = r.redactJSON(raw)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return err
	}
	return validateReviewResponse(request, response)
}

func privateKind(msg Message) string {
	if msg.IsSummaryMessage {
		return "summary"
	}
	return "answer"
}

// record keeps each provider snapshot before review. Cancellation closes
// publication even if a provider sends a late callback. Private late bytes
// remain in the journal without replacing the canceled provider message.
func (r *outputReview) record(ctx context.Context, msg Message, kind string, update bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	parts, err := marshalParts(msg.Parts)
	if err != nil {
		return err
	}
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := db.New(tx)
	prior, err := qtx.GetLatestOutputReviewPrivate(ctx, msg.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	revision := prior.Revision
	hash := reviewHash(string(parts))
	if prior.SourceHash == hash {
		return tx.Commit()
	}
	correction, err := qtx.CountOutputReviewCorrections(ctx, db.CountOutputReviewCorrectionsParams{MessageID: msg.ID, Revision: revision})
	if err != nil {
		return err
	}
	if correction > 0 {
		seen, err := qtx.CountOutputReviewSource(ctx, db.CountOutputReviewSourceParams{MessageID: msg.ID, SourceHash: hash})
		if err != nil {
			return err
		}
		if seen > 0 {
			return tx.Commit()
		}
	}
	revision++
	if err = qtx.CreateOutputReviewPrivate(ctx, db.CreateOutputReviewPrivateParams{MessageID: msg.ID, SessionID: msg.SessionID, Revision: revision, Kind: kind, SourceHash: hash, Parts: string(parts)}); err != nil {
		return err
	}
	public, err := qtx.GetOutputReviewPublic(ctx, msg.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if update && public.State != "cancelled" {
		finish := sql.NullInt64{}
		if f := msg.FinishPart(); f != nil {
			finish = sql.NullInt64{Int64: f.Time, Valid: true}
		}
		if err = qtx.UpdateMessage(ctx, db.UpdateMessageParams{ID: msg.ID, Parts: string(parts), FinishedAt: finish}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// publicSkeleton preserves tool protocol and user material verbatim. All
// generated narrative and signed replay data are separately projected.
func publicSkeleton(msg Message) Message {
	out := msg.Clone()
	out.Parts = nil
	for _, part := range msg.Parts {
		switch part.(type) {
		case TextContent, ReasoningContent, Finish:
		default:
			out.Parts = append(out.Parts, part)
		}
	}
	return out
}

func (r *outputReview) snapshot(ctx context.Context, msg Message) (int64, string, []byte, error) {
	row, err := db.New(r.conn).GetLatestOutputReviewPrivate(ctx, msg.ID)
	return row.Revision, row.SourceHash, []byte(row.Parts), err
}

// evaluate runs outside the serialization lock. The commit checks the same
// exact SQL revision again so cancellation and newer candidates win.
func (r *outputReview) evaluate(ctx context.Context, msg Message, kind string) (Message, bool, error) {
	rev, hash, raw, err := r.snapshot(ctx, msg)
	if err != nil {
		return Message{}, false, err
	}
	parts, err := unmarshalParts(raw)
	if err != nil {
		return Message{}, false, err
	}
	msg.Parts = parts
	q := db.New(r.conn)
	old, oldErr := q.GetOutputReviewPublic(ctx, msg.ID)
	oldParts, oldState := old.Parts, old.State
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return Message{}, false, oldErr
	}
	if oldState == "cancelled" {
		return Message{}, false, nil
	}
	if oldErr == nil {
		if old.Revision == rev && old.SourceHash == hash && oldState != "recovery" {
			return Message{}, false, nil
		}
	}
	out := publicSkeleton(msg)
	state := "pending"
	receipt := map[string]any{"reviewer": "native-deterministic", "judgments_not_enforced": []string{"semantic_entailment", "claim_extraction_completeness", "evidence_authenticity", "natural_language_coherence"}}
	if msg.FinishReason() == FinishReasonCanceled {
		if oldErr == nil {
			out.Parts, err = unmarshalParts([]byte(oldParts))
			if err != nil {
				return Message{}, false, err
			}
		}
		kept := out.Parts[:0]
		for _, p := range out.Parts {
			if _, ok := p.(Finish); !ok {
				kept = append(kept, p)
			}
		}
		out.Parts = kept
		out.AddFinish(FinishReasonCanceled, "User canceled request", "")
		state = "cancelled"
	} else {
		text := msg.Content().Text
		complete := msg.IsFinished()
		failed := complete && msg.FinishReason() != FinishReasonEndTurn && msg.FinishReason() != FinishReasonToolUse
		coherent := coherentPrefix(text, complete && !failed)
		candidate := coherent
		if coherent != "" {
			state = "partial"
			if complete && coherent == text {
				state = "reviewed"
			}
			corrected, correctionErr := q.GetOutputReviewCorrection(ctx, db.GetOutputReviewCorrectionParams{MessageID: msg.ID, Revision: rev})
			if correctionErr == nil {
				var result ReviewResponse
				if err = json.Unmarshal([]byte(corrected), &result); err != nil {
					return Message{}, false, err
				}
				candidate = result.Text
				receipt["adapter"] = result
			} else if !errors.Is(correctionErr, sql.ErrNoRows) {
				return Message{}, false, correctionErr
			} else if len(r.cfg.Command) > 0 {
				redacted := r.redactText(coherent)
				req := ReviewRequest{MessageID: msg.ID, SessionID: msg.SessionID, Revision: rev, Kind: kind, Text: redacted, SourceSHA256: reviewHash(coherent), Complete: complete, Protected: exactArtifacts(redacted)}
				result, reviewErr := r.command(ctx, req)
				if reviewErr != nil {
					if errors.Is(reviewErr, context.Canceled) || errors.Is(reviewErr, context.DeadlineExceeded) {
						return Message{}, false, reviewErr
					}
					state = "recovery"
					receipt["error"] = reviewErr.Error()
					candidate = ""
				} else {
					receipt["adapter"] = result
					if result.Disposition == "wait" || result.Disposition == "revise" {
						state = "withheld"
						candidate = ""
					} else {
						candidate = result.Text
					}
				}
			}
			if r.cfg.RequireFinalReview && (kind == "answer" || kind == "summary") && errors.Is(correctionErr, sql.ErrNoRows) {
				state = "awaiting_model_review"
				candidate = ""
			}
		} else if complete && text == "" {
			state = "reviewed"
		}
		if state == "pending" || state == "recovery" || state == "withheld" || state == "awaiting_model_review" {
			if oldErr == nil {
				prior, e := unmarshalParts([]byte(oldParts))
				if e != nil {
					return Message{}, false, e
				}
				for _, p := range prior {
					if _, ok := p.(TextContent); ok {
						out.Parts = append(out.Parts, p)
					}
				}
			}
		} else if candidate != "" {
			out.Parts = append(out.Parts, TextContent{Text: candidate})
		}
		if failed {
			state = "failed"
		}
		if state == "reviewed" || state == "failed" {
			if f := msg.FinishPart(); f != nil {
				out.Parts = append(out.Parts, *f)
			}
		}
	}
	out, err = r.redactMessage(out)
	if err != nil {
		return Message{}, false, err
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return Message{}, false, err
	}
	receiptJSON, err = r.redactJSON(receiptJSON)
	if err != nil {
		return Message{}, false, err
	}
	encoded, err := marshalParts(out.Parts)
	if err != nil {
		return Message{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, false, err
	}
	defer tx.Rollback()
	qtx := db.New(tx)
	current, err := qtx.GetLatestOutputReviewPrivate(ctx, msg.ID)
	if err != nil {
		return Message{}, false, err
	}
	if current.Revision != rev {
		return Message{}, false, nil
	}
	currentPublic, e := qtx.GetOutputReviewPublic(ctx, msg.ID)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return Message{}, false, e
	}
	if currentPublic.State == "cancelled" {
		return Message{}, false, nil
	}
	err = qtx.UpsertOutputReviewPublic(ctx, db.UpsertOutputReviewPublicParams{MessageID: msg.ID, SessionID: msg.SessionID, Revision: rev, SourceHash: hash, Parts: string(encoded), State: state, Receipt: string(receiptJSON)})
	if err != nil {
		return Message{}, false, err
	}
	committed, priorErr := qtx.GetLatestOutputReviewEvent(ctx, msg.ID)
	if priorErr != nil && !errors.Is(priorErr, sql.ErrNoRows) {
		return Message{}, false, priorErr
	}
	changed := committed.Parts != string(encoded) || committed.State != state
	if changed {
		err = qtx.CreateOutputReviewEvent(ctx, db.CreateOutputReviewEventParams{MessageID: msg.ID, SessionID: msg.SessionID, Revision: rev, Parts: string(encoded), State: state})
		if err != nil {
			return Message{}, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Message{}, false, err
	}
	return out, changed, nil
}

var artifactRE = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`|\"[^\"\n]*\"|(?m)^>[^\n]*(?:\n|$)")

func exactArtifacts(text string) []string { return artifactRE.FindAllString(text, -1) }

// coherentPrefix uses explicit paragraph/sentence boundaries. It does not
// claim that punctuation proves semantic independence from later context.
func coherentPrefix(text string, complete bool) string {
	if complete {
		return text
	}
	end, codeRun := 0, 0
	var codeByte byte
	quoted := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '`' || c == '~' && i+2 < len(text) && text[i:i+3] == "~~~" {
			n := 1
			for i+n < len(text) && text[i+n] == c {
				n++
			}
			if codeRun == 0 {
				codeRun = n
				codeByte = c
			} else if c == codeByte && n == codeRun {
				codeRun = 0
			}
			i += n - 1
			continue
		}
		if codeRun > 0 {
			continue
		}
		if c == '"' && (i == 0 || text[i-1] != '\\') {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if c == '\n' && i+1 < len(text) && text[i+1] == '\n' {
			end = i + 2
		}
		if (c == '.' || c == '!' || c == '?') && i+1 < len(text) && (text[i+1] == ' ' || text[i+1] == '\n') {
			end = i + 2
		}
	}
	return text[:end]
}

func (r *outputReview) command(ctx context.Context, request ReviewRequest) (ReviewResponse, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return ReviewResponse{}, err
	}
	cmd := exec.CommandContext(ctx, r.cfg.Command[0], r.cfg.Command[1:]...)
	cmd.Stdin = bytes.NewReader(data)
	var output bytes.Buffer
	cmd.Stdout = &output
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ReviewResponse{}, ctx.Err()
		}
		return ReviewResponse{}, fmt.Errorf("review adapter: %w", err)
	}
	var response ReviewResponse
	decoder := json.NewDecoder(&output)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&response); err != nil {
		return response, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return response, errors.New("review adapter returned trailing data")
	}
	return response, r.validateResponse(request, response)
}

func validateReviewResponse(request ReviewRequest, response ReviewResponse) error {
	if response.SourceSHA256 != request.SourceSHA256 {
		return errors.New("review adapter used a stale candidate")
	}
	if response.Disposition != "publish" && response.Disposition != "wait" && response.Disposition != "revise" {
		return errors.New("unknown review disposition")
	}
	if response.Reviewer == "" || len(response.Provenance) == 0 {
		return errors.New("review adapter omitted review provenance")
	}
	if response.Disposition != "publish" {
		return nil
	}
	for _, exact := range request.Protected {
		if strings.Count(response.Text, exact) < strings.Count(request.Text, exact) {
			return errors.New("review changed exact protected material")
		}
	}
	evidence := map[string]ReviewEvidence{}
	for _, fact := range response.Evidence {
		if fact.ID == "" || fact.Source == "" {
			return errors.New("review evidence lacks identity or source")
		}
		if prior, ok := evidence[fact.ID]; ok && prior != fact {
			return errors.New("conflicting evidence identity")
		}
		evidence[fact.ID] = fact
	}
	for _, claim := range response.Claims {
		fact, ok := evidence[claim.EvidenceID]
		if !ok || claim.Text == "" || !strings.Contains(response.Text, claim.Text) || claim.Actor == "" || claim.Action == "" || claim.Result != fact.Result || claim.Uncertainty == "" {
			return errors.New("review claim does not bind declared evidence")
		}
	}
	return nil
}

func PublicGet(ctx context.Context, svc Service, id string) (Message, error) {
	msg, err := reviewStoredMessage(ctx, svc, id)
	if err != nil {
		return Message{}, err
	}
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return msg, nil
	}
	if msg.Role != Assistant {
		return s.review.redactMessage(msg)
	}
	projection, err := db.New(s.db).GetOutputReviewPublic(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.review.redactMessage(publicSkeleton(msg))
	}
	if err != nil {
		return Message{}, err
	}
	msg.Parts, err = unmarshalParts([]byte(projection.Parts))
	if err != nil {
		return Message{}, err
	}
	return s.review.redactMessage(msg)
}

func PublicList(ctx context.Context, svc Service, sessionID string) ([]Message, error) {
	messages, err := svc.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		messages[i], err = PublicGet(ctx, svc, messages[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func OutputReviewStatus(ctx context.Context, svc Service, id string) (ReviewStatus, error) {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return ReviewStatus{}, errors.New("output review not enabled")
	}
	result := ReviewStatus{MessageID: id}
	row, err := db.New(s.db).GetOutputReviewPublic(ctx, id)
	if err != nil {
		return result, err
	}
	result.Revision, result.State, result.SourceSHA256 = row.Revision, row.State, row.SourceHash
	result.Receipt, err = s.review.redactJSON([]byte(row.Receipt))
	return result, err
}

// FinalPublicText is used by RunComplete and return-value paths. A partial
// reviewed stream is not presented as a completed final answer.
func FinalPublicText(ctx context.Context, svc Service, id string) (string, error) {
	msg, err := PublicGet(ctx, svc, id)
	if err != nil {
		return "", err
	}
	if OutputReviewEnabled(svc) {
		status, e := OutputReviewStatus(ctx, svc, id)
		if e != nil {
			return "", e
		}
		if status.State != "reviewed" && status.State != "legacy_visible" && status.State != "imported_visible" {
			return "", ErrOutputReviewPending
		}
	}
	return msg.Content().Text, nil
}

// ReviewText routes suggestions and other standalone displays through the
// same SQL journal and reviewer without adding them to provider history.
func ReviewText(ctx context.Context, svc Service, sessionID, kind, text string) (string, error) {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return text, nil
	}
	msg := Message{ID: uuid.NewString(), SessionID: sessionID, Role: Assistant, Parts: []ContentPart{TextContent{Text: text}}}
	msg.AddFinish(FinishReasonEndTurn, "", "")
	if err := s.review.record(ctx, msg, kind, false); err != nil {
		return "", err
	}
	projected, _, err := s.review.evaluate(ctx, msg, kind)
	if err != nil {
		return "", err
	}
	status, err := OutputReviewStatus(ctx, svc, msg.ID)
	if err != nil {
		return "", err
	}
	if status.State != "reviewed" {
		return "", ErrOutputReviewPending
	}
	return projected.Content().Text, nil
}

// ReplayPublic reads durable reviewed revisions after a cursor. Message IDs
// and revision numbers allow clients to suppress reconnect duplicates.
func ReplayPublic(ctx context.Context, svc Service, sessionID string, after int64) ([]ReviewEvent, error) {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return nil, errors.New("output review not enabled")
	}
	rows, err := db.New(s.db).ListOutputReviewEvents(ctx, db.ListOutputReviewEventsParams{SessionID: sessionID, Cursor: after})
	if err != nil {
		return nil, err
	}
	var events []ReviewEvent
	for _, row := range rows {
		event := ReviewEvent{Cursor: row.Cursor, MessageID: row.MessageID, Revision: row.Revision, State: row.State}
		event.Message = Message{ID: event.MessageID, SessionID: sessionID, Role: Assistant}
		event.Message.Parts, err = unmarshalParts([]byte(row.Parts))
		if err != nil {
			return nil, err
		}
		event.Message, err = s.review.redactMessage(event.Message)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

// ExportPublic serializes only the same reviewed view clients receive.
func ExportPublic(ctx context.Context, svc Service, sessionID string, w io.Writer) error {
	messages, err := PublicList(ctx, svc, sessionID)
	if err != nil {
		return err
	}
	type exported struct {
		ID    string          `json:"id"`
		Role  MessageRole     `json:"role"`
		Parts json.RawMessage `json:"parts"`
	}
	out := make([]exported, 0, len(messages))
	for _, msg := range messages {
		parts, e := marshalParts(msg.Parts)
		if e != nil {
			return e
		}
		out = append(out, exported{msg.ID, msg.Role, parts})
	}
	return json.NewEncoder(w).Encode(out)
}

func (s *service) publishOutput(ctx context.Context, kind pubsub.EventType, msg Message, terminal bool) error {
	if s.review != nil && msg.Role == Assistant {
		projected, changed, err := s.review.evaluate(ctx, msg, privateKind(msg))
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		msg = projected
	}
	if s.review != nil {
		var err error
		msg, err = s.review.redactMessage(msg)
		if err != nil {
			return err
		}
	}
	if terminal {
		s.PublishMustDeliver(ctx, kind, msg.Clone())
	} else {
		s.Publish(kind, msg.Clone())
	}
	return nil
}

// OutputReviewRequest reads the latest immutable candidate, including a prior
// correction. Provider replay still reads the original messages table.
func OutputReviewRequest(ctx context.Context, svc Service, id string) (ReviewRequest, error) {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return ReviewRequest{}, errors.New("output review not enabled")
	}
	msg, err := reviewStoredMessage(ctx, svc, id)
	if err != nil {
		return ReviewRequest{}, err
	}
	rev, _, raw, err := s.review.snapshot(ctx, msg)
	if err != nil {
		return ReviewRequest{}, err
	}
	msg.Parts, err = unmarshalParts(raw)
	if err != nil {
		return ReviewRequest{}, err
	}
	text := msg.Content().Text
	redacted := s.review.redactText(text)
	return ReviewRequest{MessageID: id, SessionID: msg.SessionID, Revision: rev, Kind: privateKind(msg), Text: redacted, SourceSHA256: reviewHash(text), Complete: msg.IsFinished(), Protected: exactArtifacts(redacted)}, nil
}

// ReviewCandidate is the trusted selected-model return boundary. The host
// supplies actual call provenance, not a generated approval flag. The native
// checks run again; original provider rows are not replaced by corrections.
func ReviewCandidate(ctx context.Context, svc Service, id string, response ReviewResponse) error {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return errors.New("output review not enabled")
	}
	request, err := OutputReviewRequest(ctx, svc, id)
	if err != nil {
		return err
	}
	encodedReceipt, err := json.Marshal(response)
	if err != nil {
		return err
	}
	priorReceipt, e := db.New(s.db).GetLatestOutputReviewCorrection(ctx, id)
	if e == nil && priorReceipt == string(encodedReceipt) {
		return nil
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if err = s.review.validateResponse(request, response); err != nil {
		return err
	}
	if response.Disposition != "publish" {
		return ErrOutputReviewPending
	}
	msg, err := reviewStoredMessage(ctx, svc, id)
	if err != nil {
		return err
	}
	_, _, raw, err := s.review.snapshot(ctx, msg)
	if err != nil {
		return err
	}
	msg.Parts, err = unmarshalParts(raw)
	if err != nil {
		return err
	}
	for i, part := range msg.Parts {
		if _, ok := part.(TextContent); ok {
			msg.Parts[i] = TextContent{Text: response.Text}
		}
	}
	raw, err = marshalParts(msg.Parts)
	if err != nil {
		return err
	}
	r := s.review
	r.mu.Lock()
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	defer tx.Rollback()
	qtx := db.New(tx)
	current, err := qtx.GetLatestOutputReviewPrivate(ctx, id)
	var public db.OutputReviewPublic
	if err == nil {
		public, e = qtx.GetOutputReviewPublic(ctx, id)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			err = e
		}
	}
	if err == nil && (current.Revision != request.Revision || public.State == "cancelled") {
		err = errors.New("review candidate is stale or cancelled")
	}
	if err == nil {
		err = qtx.CreateOutputReviewPrivate(ctx, db.CreateOutputReviewPrivateParams{MessageID: id, SessionID: msg.SessionID, Revision: current.Revision + 1, Kind: current.Kind, SourceHash: reviewHash(string(raw)), Parts: string(raw)})
	}
	if err == nil {
		err = qtx.CreateOutputReviewCorrection(ctx, db.CreateOutputReviewCorrectionParams{MessageID: id, Revision: current.Revision + 1, Receipt: string(encodedReceipt)})
	}
	if err == nil {
		err = tx.Commit()
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	if _, originalErr := svc.Get(ctx, id); errors.Is(originalErr, sql.ErrNoRows) {
		_, _, err = s.review.evaluate(ctx, msg, request.Kind)
		return err
	} else if originalErr != nil {
		return originalErr
	}
	return s.publishOutput(ctx, pubsub.UpdatedEvent, msg, msg.IsFinished())
}

// RecordReviewAttempt preserves rejected selected-model proposals for repair.
// It is private evidence, never a publication or an approval signal.
func RecordReviewAttempt(ctx context.Context, svc Service, id, source, candidate, issue string) error {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return errors.New("output review not enabled")
	}
	return db.New(s.db).CreateOutputReviewAttempt(ctx, db.CreateOutputReviewAttemptParams{MessageID: id, SourceHash: source, Candidate: candidate, Issue: issue})
}

// BeginStandaloneReview journals generated suggestions/titles without changing
// the conversation's provider history or emitting an extra chat message.
func BeginStandaloneReview(ctx context.Context, svc Service, sessionID, kind, text string) (Message, error) {
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return Message{}, errors.New("output review not enabled")
	}
	msg := Message{ID: uuid.NewString(), SessionID: sessionID, Role: Assistant, Parts: []ContentPart{TextContent{Text: text}}}
	msg.AddFinish(FinishReasonEndTurn, "", "")
	if err := s.review.record(ctx, msg, kind, false); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func reviewStoredMessage(ctx context.Context, svc Service, id string) (Message, error) {
	msg, err := svc.Get(ctx, id)
	if !errors.Is(err, sql.ErrNoRows) {
		return msg, err
	}
	s, ok := svc.(*service)
	if !ok || s.review == nil {
		return msg, err
	}
	row, err := db.New(s.db).GetLatestOutputReviewPrivate(ctx, id)
	if err != nil {
		return Message{}, err
	}
	msg = Message{ID: id, SessionID: row.SessionID, Role: Assistant, IsSummaryMessage: row.Kind == "summary"}
	msg.Parts, err = unmarshalParts([]byte(row.Parts))
	return msg, err
}
