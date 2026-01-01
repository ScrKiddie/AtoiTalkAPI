package model

type SendMessageRequest struct {
	ChatID        int    `json:"chat_id" validate:"required"`
	Content       string `json:"content" validate:"required_without=AttachmentIDs"`
	AttachmentIDs []int  `json:"attachment_ids" validate:"omitempty,dive,gt=0"`
	ReplyToID     *int   `json:"reply_to_id" validate:"omitempty,gt=0"`
}

type GetMessagesRequest struct {
	ChatID int `json:"chat_id" validate:"required"`
	Cursor int `json:"cursor" validate:"omitempty,gt=0"`
	Limit  int `json:"limit" validate:"omitempty,gt=0,max=50"`
}

type MessageResponse struct {
	ID          int              `json:"id"`
	ChatID      int              `json:"chat_id"`
	SenderID    int              `json:"sender_id"`
	Content     string           `json:"content"`
	Attachments []MediaDTO       `json:"attachments,omitempty"`
	ReplyTo     *ReplyPreviewDTO `json:"reply_to,omitempty"`
	CreatedAt   string           `json:"created_at"`
	DeletedAt   *string          `json:"deleted_at,omitempty"`
}

type ReplyPreviewDTO struct {
	ID         int    `json:"id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
}
