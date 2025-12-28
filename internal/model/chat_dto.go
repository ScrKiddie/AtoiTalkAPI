package model

type CreatePrivateChatRequest struct {
	TargetUserID int `json:"target_user_id" validate:"required"`
}

type ChatResponse struct {
	ID        int    `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
}

type GetChatsRequest struct {
	Query  string `json:"query"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit" validate:"min=1,max=50"`
}

type ChatListResponse struct {
	ID          int              `json:"id"`
	Type        string           `json:"type"`
	Name        string           `json:"name"`
	Avatar      string           `json:"avatar"`
	LastMessage *MessageResponse `json:"last_message,omitempty"`
	UnreadCount int              `json:"unread_count"`
	LastReadAt  *string          `json:"last_read_at,omitempty"`
	IsPinned    bool             `json:"is_pinned"`
}
