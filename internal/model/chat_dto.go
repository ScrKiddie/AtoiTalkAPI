package model

type CreatePrivateChatRequest struct {
	TargetUserID int `json:"target_user_id" validate:"required"`
}

type GetChatsRequest struct {
	Query  string `json:"query" validate:"omitempty,min=1"`
	Cursor string `json:"cursor" validate:"omitempty"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
}

type ChatResponse struct {
	ID        int    `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
}

type ChatListResponse struct {
	ID              int              `json:"id"`
	Type            string           `json:"type"`
	Name            string           `json:"name"`
	Avatar          string           `json:"avatar"`
	LastMessage     *MessageResponse `json:"last_message"`
	UnreadCount     int              `json:"unread_count"`
	LastReadAt      *string          `json:"last_read_at"`
	OtherLastReadAt *string          `json:"other_last_read_at"`
}
