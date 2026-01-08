package model

import "github.com/google/uuid"

type SendMessageRequest struct {
	ChatID        uuid.UUID   `json:"chat_id" validate:"required"`
	Content       string      `json:"content" validate:"required_without=AttachmentIDs"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids" validate:"omitempty,dive"`
	ReplyToID     *uuid.UUID  `json:"reply_to_id" validate:"omitempty"`
}

type EditMessageRequest struct {
	Content       string      `json:"content" validate:"required_without=AttachmentIDs"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids" validate:"omitempty,dive"`
}

type GetMessagesRequest struct {
	ChatID    uuid.UUID `json:"chat_id" validate:"required"`
	Cursor    string    `json:"cursor" validate:"omitempty"`
	Limit     int       `json:"limit" validate:"omitempty,gt=0,max=50"`
	Direction string    `json:"direction" validate:"omitempty,oneof=older newer"`
}

type MessageResponse struct {
	ID          uuid.UUID  `json:"id"`
	ChatID      uuid.UUID  `json:"chat_id"`
	SenderID    *uuid.UUID `json:"sender_id,omitempty"`
	SenderName  string     `json:"sender_name,omitempty"`
	Type        string     `json:"type"`
	Content     string     `json:"content,omitempty"`
	// ActionData contains metadata for system messages.
	//
	// Structure depends on Type:
	// - system_create: { "initial_name": string }
	// - system_rename: { "old_name": string, "new_name": string }
	// - system_description: { "old_description": string, "new_description": string }
	// - system_avatar: { "action": "updated" }
	// - system_add: { "target_id": uuid, "actor_id": uuid }
	// - system_kick: { "target_id": uuid, "actor_id": uuid }
	// - system_promote: { "target_id": uuid, "actor_id": uuid, "new_role": string }
	// - system_demote: { "target_id": uuid, "actor_id": uuid, "new_role": string }
	// - system_transfer: { "target_id": uuid, "actor_id": uuid, "new_role": "owner", "action": "ownership_transferred" }
	// - system_leave: (empty, relies on sender_id)
	//
	// Note: "target_name" and "actor_name" are injected dynamically by the backend for display convenience.
	ActionData  map[string]interface{} `json:"action_data,omitempty"`
	Attachments []MediaDTO             `json:"attachments,omitempty"`
	ReplyTo     *ReplyPreviewDTO       `json:"reply_to,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	DeletedAt   *string                `json:"deleted_at,omitempty"`
	EditedAt    *string                `json:"edited_at,omitempty"`
}

type ReplyPreviewDTO struct {
	ID         uuid.UUID              `json:"id"`
	SenderName string                 `json:"sender_name"`
	Type       string                 `json:"type"`
	Content    string                 `json:"content,omitempty"`
	ActionData map[string]interface{} `json:"action_data,omitempty"`
	DeletedAt  *string                `json:"deleted_at,omitempty"`
}
