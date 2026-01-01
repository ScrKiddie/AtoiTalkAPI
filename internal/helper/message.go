package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
	"time"
)

func ToMessageResponse(msg *ent.Message, storageMode, appURL, cdnURL, storageAttachment string) *model.MessageResponse {
	if msg == nil {
		return nil
	}

	isDeleted := msg.DeletedAt != nil
	var deletedAtStr *string

	content := ""

	var attachments []model.MediaDTO

	if isDeleted {
		t := msg.DeletedAt.Format(time.RFC3339)
		deletedAtStr = &t
	} else {
		if msg.Content != nil {
			content = *msg.Content
		}

		for _, att := range msg.Edges.Attachments {
			attachments = append(attachments, model.MediaDTO{
				ID:           att.ID,
				FileName:     att.FileName,
				OriginalName: att.OriginalName,
				FileSize:     att.FileSize,
				MimeType:     att.MimeType,
				URL:          BuildImageURL(storageMode, appURL, cdnURL, storageAttachment, att.FileName),
			})
		}
	}

	var replyPreview *model.ReplyPreviewDTO
	if reply := msg.Edges.ReplyTo; reply != nil {
		replyContent := ""
		var replyDeletedAt *string

		if reply.DeletedAt != nil {
			t := reply.DeletedAt.Format(time.RFC3339)
			replyDeletedAt = &t
		} else {
			if reply.Content != nil {
				replyContent = *reply.Content
			}
		}

		replyPreview = &model.ReplyPreviewDTO{
			ID:         reply.ID,
			SenderName: reply.Edges.Sender.FullName,
			Content:    replyContent,
			DeletedAt:  replyDeletedAt,
		}
	}

	return &model.MessageResponse{
		ID:          msg.ID,
		ChatID:      msg.ChatID,
		SenderID:    msg.SenderID,
		Content:     content,
		Attachments: attachments,
		ReplyTo:     replyPreview,
		CreatedAt:   msg.CreatedAt.Format(time.RFC3339),
		DeletedAt:   deletedAtStr,
	}
}
