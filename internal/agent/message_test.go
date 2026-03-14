package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/models"
)

func TestBuildUserMessage(t *testing.T) {
	t.Run("no images returns plain text message", func(t *testing.T) {
		msg := buildUserMessage("hello", nil)
		if msg.Role != schema.User {
			t.Errorf("Role = %v, want %v", msg.Role, schema.User)
		}
		if msg.Content != "hello" {
			t.Errorf("Content = %q, want %q", msg.Content, "hello")
		}
		if msg.UserInputMultiContent != nil {
			t.Error("expected nil UserInputMultiContent for text-only message")
		}
	})

	t.Run("with images returns multimodal message", func(t *testing.T) {
		images := []models.ChatAttachment{
			{Path: "a.png", Content: "base64data1", MimeType: "image/png", Type: models.AttachmentTypeImage},
			{Path: "b.jpg", Content: "base64data2", MimeType: "image/jpeg", Type: models.AttachmentTypeImage},
		}
		msg := buildUserMessage("describe these", images)
		if msg.Role != schema.User {
			t.Errorf("Role = %v, want %v", msg.Role, schema.User)
		}
		if msg.Content != "" {
			t.Errorf("Content = %q, want empty for multimodal message", msg.Content)
		}
		parts := msg.UserInputMultiContent
		if len(parts) != 3 {
			t.Fatalf("got %d parts, want 3 (1 text + 2 images)", len(parts))
		}

		// First part is text
		if parts[0].Type != schema.ChatMessagePartTypeText {
			t.Errorf("parts[0].Type = %v, want Text", parts[0].Type)
		}
		if parts[0].Text != "describe these" {
			t.Errorf("parts[0].Text = %q, want %q", parts[0].Text, "describe these")
		}

		// Image parts
		for i, want := range []struct {
			mime string
			b64  string
		}{
			{"image/png", "base64data1"},
			{"image/jpeg", "base64data2"},
		} {
			p := parts[i+1]
			if p.Type != schema.ChatMessagePartTypeImageURL {
				t.Errorf("parts[%d].Type = %v, want ImageURL", i+1, p.Type)
			}
			if p.Image == nil {
				t.Fatalf("parts[%d].Image is nil", i+1)
			}
			if p.Image.MIMEType != want.mime {
				t.Errorf("parts[%d].MIMEType = %q, want %q", i+1, p.Image.MIMEType, want.mime)
			}
			if p.Image.Base64Data == nil || *p.Image.Base64Data != want.b64 {
				t.Errorf("parts[%d].Base64Data = %v, want %q", i+1, p.Image.Base64Data, want.b64)
			}
		}
	})

	t.Run("each image gets its own base64 pointer", func(t *testing.T) {
		images := []models.ChatAttachment{
			{Path: "a.png", Content: "AAA", MimeType: "image/png", Type: models.AttachmentTypeImage},
			{Path: "b.png", Content: "BBB", MimeType: "image/png", Type: models.AttachmentTypeImage},
		}
		msg := buildUserMessage("test", images)
		parts := msg.UserInputMultiContent
		if len(parts) != 3 {
			t.Fatalf("got %d parts, want 3", len(parts))
		}
		// Verify distinct pointers with correct values (not sharing the last loop iteration)
		if *parts[1].Image.Base64Data != "AAA" {
			t.Errorf("parts[1] base64 = %q, want %q", *parts[1].Image.Base64Data, "AAA")
		}
		if *parts[2].Image.Base64Data != "BBB" {
			t.Errorf("parts[2] base64 = %q, want %q", *parts[2].Image.Base64Data, "BBB")
		}
		if parts[1].Image.Base64Data == parts[2].Image.Base64Data {
			t.Error("image parts share the same Base64Data pointer — loop variable capture bug")
		}
	})
}
