package model

type CreatePrivateChatRequest struct {
	TargetUserID int `json:"target_user_id" validate:"required"`
}

type ChatResponse struct {
	ID        int    `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
}
