package message

import (
	"slices"
	"strings"
)

type Attachment struct {
	FilePath string
	FileName string
	MimeType string
	Content  []byte
}

func (a Attachment) IsText() bool     { return IsTextualMIME(a.MimeType) }
func (a Attachment) IsImage() bool    { return strings.HasPrefix(a.MimeType, "image/") }
func (a Attachment) IsMarkdown() bool { return a.MimeType == "text/markdown" }

// IsTextualMIME reports media types whose bytes are text and can be inlined
// into a prompt instead of sent as a binary file part. Providers such as
// DeepSeek reject non-image file parts, so structured text such as JSON must
// travel as prompt text.
func IsTextualMIME(mimeType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml",
		"application/toml", "application/javascript", "application/x-javascript",
		"application/typescript", "application/x-sh":
		return true
	}
	return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
}

// ContainsTextAttachment returns true if any of the attachments is a text attachment.
func ContainsTextAttachment(attachments []Attachment) bool {
	return slices.ContainsFunc(attachments, func(a Attachment) bool {
		return a.IsText()
	})
}
