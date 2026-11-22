package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"charm.land/fantasy"
)

// Reasons a media part is replaced before a request is sent. Each names what
// was established about that one artifact, never about the session.
const (
	reasonRejectedByProvider   = "rejected_by_provider"
	reasonUnidentifiedThisTurn = "unidentified_this_request"
	reasonNonImageAttachment   = "non_image_attachment"
	reasonModelUnsupported     = "model_unsupported"
)

// maxMediaRequestBodyBytes bounds the recorded provider request body before it
// is parsed. A body larger than this is treated as unattributable.
const maxMediaRequestBodyBytes = 32 << 20

// mediaCoordinatePattern matches the coordinate a provider names in its error,
// for example messages[140].content[1].
var mediaCoordinatePattern = regexp.MustCompile(`messages\[(\d+)\]\.content\[(\d+)\]`)

// mediaPartID identifies one media payload independently of the request that
// carried it: the decoded bytes plus the declared media type. Two turns that
// attach the same artifact share an ID; different artifacts never do.
func mediaPartID(data []byte, mediaType string) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + ":" + strings.ToLower(strings.TrimSpace(mediaType))
}

// mediaIDForStoredData hashes a media payload after repairing the historical
// double-encoding. Rows written by the old vision tool hold base64 of base64;
// the identity must describe the image, not its storage accident.
func mediaIDForStoredData(data, mediaType string) (string, bool) {
	decoded, err := decodeMediaData(data)
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	return mediaPartID(decoded, mediaType), true
}

// mediaVerdict is the recorded reason one artifact cannot be sent to one
// provider model.
type mediaVerdict struct {
	Reason string
	Origin string
}

// mediaVerdictKey scopes a verdict to one session, provider model, and artifact.
func mediaVerdictKey(sessionID, provider, model, mediaID string) string {
	return sessionID + "\x00" + provider + "\x00" + model + "\x00" + mediaID
}

// mediaCoordinate extracts the message/part coordinate named by a provider
// rejection. It reads the message first, then the response body.
func mediaCoordinate(err *fantasy.ProviderError) (int, int, bool) {
	if err == nil {
		return 0, 0, false
	}
	texts := []string{err.Message, string(err.ResponseBody)}
	if err.Cause != nil {
		texts = append(texts, err.Cause.Error())
	}
	for _, text := range texts {
		match := mediaCoordinatePattern.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		messageIndex, msgErr := strconv.Atoi(match[1])
		partIndex, partErr := strconv.Atoi(match[2])
		if msgErr == nil && partErr == nil {
			return messageIndex, partIndex, true
		}
	}
	return 0, 0, false
}

// mediaIDAtCoordinate reads the recorded request body and returns the identity
// of the media payload at the named coordinate. It fails closed: an absent,
// oversized, unparsable, or non-media part yields ok=false.
func mediaIDAtCoordinate(body []byte, messageIndex, partIndex int) (string, bool) {
	if len(body) == 0 || len(body) > maxMediaRequestBodyBytes {
		return "", false
	}
	var payload struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return "", false
	}
	if messageIndex < 0 || messageIndex >= len(payload.Messages) {
		return "", false
	}
	var parts []map[string]any
	if json.Unmarshal(payload.Messages[messageIndex].Content, &parts) != nil {
		return "", false
	}
	if partIndex < 0 || partIndex >= len(parts) {
		return "", false
	}
	mediaType, data, ok := mediaFromPart(parts[partIndex])
	if !ok {
		return "", false
	}
	return mediaPartID(data, mediaType), true
}

// mediaFromPart extracts media from the OpenAI image_url form, the Responses
// image_url string form, or the Anthropic base64 image form.
func mediaFromPart(part map[string]any) (string, []byte, bool) {
	if raw, ok := part["image_url"].(map[string]any); ok {
		url, _ := raw["url"].(string)
		return decodeDataURI(url)
	}
	if url, ok := part["image_url"].(string); ok {
		return decodeDataURI(url)
	}
	if source, ok := part["source"].(map[string]any); ok {
		if kind, _ := source["type"].(string); kind != "base64" {
			return "", nil, false
		}
		mediaType, _ := source["media_type"].(string)
		encoded, _ := source["data"].(string)
		if mediaType == "" || encoded == "" {
			return "", nil, false
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", nil, false
		}
		return mediaType, data, true
	}
	return "", nil, false
}

// decodeDataURI decodes a data: URI into its media type and bytes.
func decodeDataURI(uri string) (string, []byte, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(uri, prefix) {
		return "", nil, false
	}
	header, encoded, found := strings.Cut(strings.TrimPrefix(uri, prefix), ",")
	if !found || !strings.HasSuffix(header, ";base64") {
		return "", nil, false
	}
	mediaType := strings.TrimSuffix(header, ";base64")
	if mediaType == "" {
		return "", nil, false
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return "", nil, false
	}
	return mediaType, data, true
}

// attributeRejectedMedia derives the identity of the media a provider rejected.
// ok=false means the rejection cannot be attributed and the caller must fall
// back to the media introduced by the failing run.
func attributeRejectedMedia(err *fantasy.ProviderError) (string, bool) {
	messageIndex, partIndex, ok := mediaCoordinate(err)
	if !ok {
		return "", false
	}
	if err == nil {
		return "", false
	}
	return mediaIDAtCoordinate(err.RequestBody, messageIndex, partIndex)
}

// recordMediaVerdict stores the reason one artifact cannot be sent to one
// provider model. Only the attributed artifact is recorded.
func (a *sessionAgent) recordMediaVerdict(sessionID string, model Model, mediaID, reason, origin string) {
	if a.mediaVerdicts == nil || sessionID == "" || mediaID == "" {
		return
	}
	a.mediaVerdicts.Set(
		mediaVerdictKey(sessionID, model.ModelCfg.Provider, model.ModelCfg.Model, mediaID),
		mediaVerdict{Reason: reason, Origin: origin},
	)
}

// mediaVerdictFor reports the recorded verdict for one artifact, if any.
func (a *sessionAgent) mediaVerdictFor(sessionID string, model Model, mediaID string) (mediaVerdict, bool) {
	if a.mediaVerdicts == nil || sessionID == "" || mediaID == "" {
		return mediaVerdict{}, false
	}
	return a.mediaVerdicts.Get(
		mediaVerdictKey(sessionID, model.ModelCfg.Provider, model.ModelCfg.Model, mediaID),
	)
}

// withheldReason reports why one artifact must be replaced in this request.
// A request-scoped omission is used by the bounded scan that identifies a
// culprit when the provider supplies no coordinate.
func (a *sessionAgent) withheldReason(sessionID string, model Model, mediaID string, requestOmit map[string]struct{}) string {
	if _, ok := requestOmit[mediaID]; ok {
		return reasonUnidentifiedThisTurn
	}
	if verdict, ok := a.mediaVerdictFor(sessionID, model, mediaID); ok {
		return verdict.Reason
	}
	return ""
}

// handleMediaRejection records only the artifact the provider rejected. When
// the error has no usable coordinate it records the media introduced by this
// run, never the session. The bool reports whether a verdict was recorded.
func (a *sessionAgent) handleMediaRejection(sessionID string, model Model, err error, runMedia []string) bool {
	var providerErr *fantasy.ProviderError
	if !errors.As(err, &providerErr) || !isUnsupportedImageError(providerErr) {
		return false
	}
	if mediaID, ok := attributeRejectedMedia(providerErr); ok {
		a.recordMediaVerdict(sessionID, model, mediaID, reasonRejectedByProvider, "provider_error")
		logMediaRejection(sessionID, model, mediaID, reasonRejectedByProvider, "provider_error")
		return true
	}
	for _, mediaID := range runMedia {
		a.recordMediaVerdict(sessionID, model, mediaID, reasonUnidentifiedThisTurn, "provider_error")
		logMediaRejection(sessionID, model, mediaID, reasonUnidentifiedThisTurn, "provider_error")
	}
	return len(runMedia) > 0
}

// applyMediaRetryToolPolicy makes a retry safe after tool results were stored:
// the retry reuses the stored tool results and cannot start another tool. The
// model can answer, but no tool executes twice.
func applyMediaRetryToolPolicy(prepared *fantasy.PrepareStepResult, retryWithoutTools bool) {
	if !retryWithoutTools {
		return
	}
	prepared.Tools = nil
	prepared.DisableAllTools = true
}

// logMediaRejection records the identity and reason, never the payload.
func logMediaRejection(sessionID string, model Model, mediaID, reason, origin string) {
	slog.Warn("Provider rejected media; withholding one artifact",
		"session_id", sessionID,
		"provider", model.ModelCfg.Provider,
		"model", model.ModelCfg.Model,
		"media_id", hashPrefix(mediaID),
		"reason", reason,
		"origin", origin)
}

// mediaNoteText is the human-readable replacement for a withheld artifact.
func mediaNoteText(reason string) string {
	switch reason {
	case reasonRejectedByProvider:
		return "[Media withheld: rejected_by_provider]"
	case reasonUnidentifiedThisTurn:
		return "[Media withheld: unidentified_this_request]"
	case reasonNonImageAttachment:
		return "[Attachment withheld: non_image_attachment]"
	case reasonModelUnsupported:
		return "[Media withheld: model_unsupported]"
	default:
		return "[Media withheld]"
	}
}

// hashPrefix returns the short digest used in logs, never the payload.
func hashPrefix(mediaID string) string {
	if i := strings.IndexByte(mediaID, ':'); i == 12 {
		return mediaID[:12]
	}
	if len(mediaID) > 12 {
		return mediaID[:12]
	}
	return mediaID
}

// mediaIDsInMessages lists the identities of every media payload a request
// would carry, in tool results and in file parts.
func mediaIDsInMessages(messages []fantasy.Message) []string {
	seen := make(map[string]struct{})
	var ids []string
	add := func(id string, ok bool) {
		if !ok || id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, msg := range messages {
		for _, part := range msg.Content {
			if toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
				if media, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](toolResult.Output); ok {
					add(mediaIDForStoredData(media.Data, media.MediaType))
				}
				continue
			}
			if file, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
				add(mediaPartID(file.Data, file.MediaType), true)
			}
		}
	}
	return ids
}

// newMediaIDs returns the artifacts in this step that were not present in an
// earlier step of the same run. It updates the caller's seen set.
func newMediaIDs(messages []fantasy.Message, seen map[string]struct{}) []string {
	var introduced []string
	for _, id := range mediaIDsInMessages(messages) {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		introduced = append(introduced, id)
	}
	return introduced
}

// filterProviderMedia withholds only files with a verdict and adds one typed
// note per withheld file to the prompt. Every other file is delivered.
func (a *sessionAgent) filterProviderMedia(prompt string, files []fantasy.FilePart, sessionID string, model Model, supportsImages bool, requestOmit map[string]struct{}) (string, []fantasy.FilePart) {
	if len(files) == 0 {
		return prompt, files
	}
	kept := make([]fantasy.FilePart, 0, len(files))
	var notes []string
	for _, file := range files {
		mediaID := mediaPartID(file.Data, file.MediaType)
		reason := ""
		switch {
		case !supportsImages && strings.HasPrefix(strings.ToLower(file.MediaType), "image/"):
			reason = reasonModelUnsupported
		case !strings.HasPrefix(strings.ToLower(file.MediaType), "image/"):
			reason = reasonNonImageAttachment
		default:
			reason = a.withheldReason(sessionID, model, mediaID, requestOmit)
		}
		if reason == "" {
			kept = append(kept, file)
			continue
		}
		name := file.Filename
		if name == "" {
			name = "attachment"
		}
		notes = append(notes, mediaNoteText(reason)+" ("+name+")")
	}
	if len(notes) > 0 {
		prompt = strings.TrimRight(prompt, "\n") + "\n\n" + strings.Join(notes, "\n")
	}
	return prompt, kept
}
