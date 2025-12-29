package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
	"strings"
)

func ToMessageResponse(msg *ent.Message, storageMode, appURL, cdnURL, storageAttachment string) *model.MessageResponse {
	if msg == nil {
		return nil
	}

	content := ""
	if msg.DeletedAt != nil {
		content = "Pesan telah dihapus"
	} else if msg.Content != nil && *msg.Content != "" {
		content = *msg.Content
	} else if len(msg.Edges.Attachments) > 0 {

		firstAtt := msg.Edges.Attachments[0]
		switch {
		case strings.HasPrefix(firstAtt.MimeType, "image/"):
			content = "📷 Foto"
		case strings.HasPrefix(firstAtt.MimeType, "video/"):
			content = "🎥 Video"
		case strings.HasPrefix(firstAtt.MimeType, "audio/"):
			content = "🎵 Audio"
		default:
			content = "📄 Lampiran"
		}
	}

	var attachments []model.MediaDTO
	for _, att := range msg.Edges.Attachments {
		url := BuildImageURL(storageMode, appURL, cdnURL, storageAttachment, att.FileName)
		attachments = append(attachments, model.MediaDTO{
			ID:           att.ID,
			FileName:     att.FileName,
			OriginalName: att.OriginalName,
			FileSize:     att.FileSize,
			MimeType:     att.MimeType,
			URL:          url,
		})
	}

	var replyPreview *model.ReplyPreviewDTO
	if msg.Edges.ReplyTo != nil {
		replyContent := "Pesan telah dihapus"
		if msg.Edges.ReplyTo.DeletedAt == nil {
			if msg.Edges.ReplyTo.Content != nil && *msg.Edges.ReplyTo.Content != "" {
				replyContent = *msg.Edges.ReplyTo.Content
			} else if len(msg.Edges.ReplyTo.Edges.Attachments) > 0 {

				firstAtt := msg.Edges.ReplyTo.Edges.Attachments[0]
				switch {
				case strings.HasPrefix(firstAtt.MimeType, "image/"):
					replyContent = "📷 Foto"
				case strings.HasPrefix(firstAtt.MimeType, "video/"):
					replyContent = "🎥 Video"
				case strings.HasPrefix(firstAtt.MimeType, "audio/"):
					replyContent = "🎵 Audio"
				default:
					replyContent = "📄 Lampiran"
				}
			}
		}
		replyPreview = &model.ReplyPreviewDTO{
			ID:         msg.Edges.ReplyTo.ID,
			SenderName: msg.Edges.ReplyTo.Edges.Sender.FullName,
			Content:    replyContent,
		}
	}

	return &model.MessageResponse{
		ID:          msg.ID,
		ChatID:      msg.ChatID,
		SenderID:    msg.SenderID,
		Content:     content,
		Attachments: attachments,
		ReplyTo:     replyPreview,
		CreatedAt:   msg.CreatedAt.String(),
	}
}
