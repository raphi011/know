package models

// AttachmentType classifies the content kind of a chat attachment.
type AttachmentType string

const (
	AttachmentTypeText  AttachmentType = "text"
	AttachmentTypeImage AttachmentType = "image"
)

// ChatAttachment represents a file attached to a chat message.
type ChatAttachment struct {
	Path     string         `json:"path"`
	Content  string         `json:"content"`
	MimeType string         `json:"mimeType"`
	Language string         `json:"language,omitempty"`
	Type     AttachmentType `json:"type"`
}
