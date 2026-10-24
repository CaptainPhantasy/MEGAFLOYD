package proto

type PendingInteractions struct {
	Permissions []PermissionRequest `json:"permissions"`
	Questions   []QuestionRequest   `json:"questions"`
}
