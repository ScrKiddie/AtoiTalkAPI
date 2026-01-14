package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
	"time"
)

func ToMessageResponse(msg *ent.Message, urlGen URLGenerator, hiddenAt *time.Time) *model.MessageResponse {
	if msg == nil {
		return nil
	}

	isDeleted := msg.DeletedAt != nil
	var deletedAtStr *string
	var editedAtStr *string
	var content string
	var attachments []model.MediaDTO
	var actionData map[string]interface{}

	if isDeleted {
		t := msg.DeletedAt.Format(time.RFC3339)
		deletedAtStr = &t
	} else {
		if msg.Content != nil {
			content = *msg.Content
		}
		if msg.EditedAt != nil {
			t := msg.EditedAt.Format(time.RFC3339)
			editedAtStr = &t
		}
		if msg.ActionData != nil {
			actionData = msg.ActionData
		}
		for _, att := range msg.Edges.Attachments {

			url, _ := urlGen.GetPresignedURL(att.FileName, 15*time.Minute)
			attachments = append(attachments, model.MediaDTO{
				ID:           att.ID,
				FileName:     att.FileName,
				OriginalName: att.OriginalName,
				FileSize:     att.FileSize,
				MimeType:     att.MimeType,
				URL:          url,
			})
		}
	}

	var senderName string
	var senderAvatar string

	if msg.Edges.Sender != nil {
		if msg.Edges.Sender.FullName != nil {
			senderName = *msg.Edges.Sender.FullName
		}
		if msg.Edges.Sender.Edges.Avatar != nil {
			senderAvatar = urlGen.GetPublicURL(msg.Edges.Sender.Edges.Avatar.FileName)
		}
	}

	var replyPreview *model.ReplyPreviewDTO
	if reply := msg.Edges.ReplyTo; reply != nil {
		replyContent := ""
		var replyDeletedAt *string
		var replyActionData map[string]interface{}
		replySenderName := ""

		if reply.Edges.Sender != nil && reply.Edges.Sender.FullName != nil {
			replySenderName = *reply.Edges.Sender.FullName
		}

		if reply.DeletedAt != nil {
			t := reply.DeletedAt.Format(time.RFC3339)
			replyDeletedAt = &t
		} else {
			if reply.Content != nil {
				replyContent = *reply.Content
			}
			if reply.ActionData != nil {
				replyActionData = reply.ActionData
			}
		}

		replyPreview = &model.ReplyPreviewDTO{
			ID:         reply.ID,
			SenderName: replySenderName,
			Type:       string(reply.Type),
			Content:    replyContent,
			ActionData: replyActionData,
			DeletedAt:  replyDeletedAt,
			CreatedAt:  reply.CreatedAt.Format(time.RFC3339),
		}
	}

	return &model.MessageResponse{
		ID:           msg.ID,
		ChatID:       msg.ChatID,
		SenderID:     msg.SenderID,
		SenderName:   senderName,
		SenderAvatar: senderAvatar,
		Type:         string(msg.Type),
		Content:      content,
		ActionData:   actionData,
		Attachments:  attachments,
		ReplyTo:      replyPreview,
		CreatedAt:    msg.CreatedAt.Format(time.RFC3339),
		DeletedAt:    deletedAtStr,
		EditedAt:     editedAtStr,
	}
}
