package model

type CreatePrivateChatRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
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
	LastMessage     *MessageResponse `json:"last_message,omitempty"`
	UnreadCount     int              `json:"unread_count,omitempty"`
	LastReadAt      *string          `json:"last_read_at,omitempty"`
	OtherLastReadAt *string          `json:"other_last_read_at,omitempty"`
	IsOnline        bool             `json:"is_online"`
	OtherUserID     *int             `json:"other_user_id,omitempty"`
	OtherUsername   *string          `json:"other_username,omitempty"`
	IsBlockedByMe   bool             `json:"is_blocked_by_me,omitempty"`
}
