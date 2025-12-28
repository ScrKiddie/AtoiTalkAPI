package model

type SendMessageRequest struct {
	ChatID        int    `json:"chat_id" validate:"required"`
	Content       string `json:"content" validate:"required_without=AttachmentIDs"`
	Type          string `json:"type" validate:"required,oneof=text image video file audio"`
	AttachmentIDs []int  `json:"attachment_ids" validate:"omitempty"`
	ReplyToID     *int   `json:"reply_to_id" validate:"omitempty"`
}

type MessageResponse struct {
	ID          int              `json:"id"`
	ChatID      int              `json:"chat_id"`
	SenderID    int              `json:"sender_id"`
	Content     string           `json:"content"`
	Attachments []MediaDTO       `json:"attachments,omitempty"`
	ReplyTo     *ReplyPreviewDTO `json:"reply_to,omitempty"`
	CreatedAt   string           `json:"created_at"`
}

type ReplyPreviewDTO struct {
	ID         int    `json:"id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
}
