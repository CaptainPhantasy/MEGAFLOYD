package proto

type SessionArchiveRequest struct {
	Archived *bool `json:"archived"`
}

type SessionMetadataRequest struct {
	Title    *string        `json:"title,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type SessionCloneRequest = SessionMetadataRequest

type SessionUndoRequest struct {
	Count int `json:"count"`
}

type SessionUndoResponse struct {
	Session      Session `json:"session"`
	RemovedTurns int     `json:"removed_turns"`
	UndoRecordID string  `json:"undo_record_id"`
}
