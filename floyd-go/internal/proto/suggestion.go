package proto

type SuggestPromptRequest struct {
	Prefix string `json:"prefix"`
}

type SuggestPromptResponse struct {
	Suggestion string `json:"suggestion"`
}
