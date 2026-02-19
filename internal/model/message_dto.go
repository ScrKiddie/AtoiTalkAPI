package model

import "github.com/google/uuid"

type SendMessageRequest struct {
	ChatID        uuid.UUID   `json:"chat_id" validate:"required"`
	Content       string      `json:"content" validate:"required_without=AttachmentIDs,max=4000"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids" validate:"omitempty,dive"`
	ReplyToID     *uuid.UUID  `json:"reply_to_id" validate:"omitempty"`
}

type EditMessageRequest struct {
	Content       string      `json:"content" validate:"required_without=AttachmentIDs,max=4000"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids" validate:"omitempty,dive"`
}

type GetMessagesRequest struct {
	ChatID          uuid.UUID  `json:"chat_id" validate:"required"`
	Cursor          string     `json:"cursor" validate:"omitempty"`
	AroundMessageID *uuid.UUID `json:"around_message_id" validate:"omitempty"`
	Limit           int        `json:"limit" validate:"omitempty,gt=0,max=50"`
	Direction       string     `json:"direction" validate:"omitempty,oneof=older newer"`
}

type MessageResponse struct {
	ID     uuid.UUID `json:"id"`
	ChatID uuid.UUID `json:"chat_id"`

	// ID of the sender, null if it is a system message or deleted user
	SenderID *uuid.UUID `json:"sender_id,omitempty"`

	// Name of the sender or variable system message name
	SenderName string `json:"sender_name,omitempty"`

	// Avatar URL of the sender
	SenderAvatar string `json:"sender_avatar,omitempty"`

	// Role of the sender in the group (owner, admin, member)
	SenderRole string `json:"sender_role,omitempty"`

	Type    string `json:"type"`
	Content string `json:"content,omitempty"`

	// Metadata for system messages, structure depends on the message type (e.g., renamed group, added member)
	ActionData map[string]interface{} `json:"action_data,omitempty"`

	Attachments []MediaDTO `json:"attachments,omitempty"`

	// Preview of the message this message is replying to
	ReplyTo *ReplyPreviewDTO `json:"reply_to,omitempty"`

	CreatedAt string  `json:"created_at"`
	DeletedAt *string `json:"deleted_at,omitempty"`
	EditedAt  *string `json:"edited_at,omitempty"`

	// Total number of members in the group, only for group chats
	MemberCount *int `json:"member_count,omitempty"`
}

type ReplyPreviewDTO struct {
	ID         uuid.UUID              `json:"id"`
	SenderID   *uuid.UUID             `json:"sender_id,omitempty"`
	SenderName string                 `json:"sender_name"`
	Type       string                 `json:"type"`
	Content    string                 `json:"content,omitempty"`
	ActionData map[string]interface{} `json:"action_data,omitempty"`
	DeletedAt  *string                `json:"deleted_at,omitempty"`
	CreatedAt  string                 `json:"created_at"`
}
