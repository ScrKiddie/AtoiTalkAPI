package model

import (
	"github.com/google/uuid"
)

type CreatePrivateChatRequest struct {
	TargetUserID uuid.UUID `json:"target_user_id" validate:"required"`
}

type ChatResponse struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	CreatedAt string    `json:"created_at"`
}

type ChatListResponse struct {
	ID                 uuid.UUID        `json:"id"`
	Type               string           `json:"type"`
	Name               string           `json:"name"`
	Avatar             string           `json:"avatar"`
	LastMessage        *MessageResponse `json:"last_message"`
	UnreadCount        int              `json:"unread_count,omitempty"`
	LastReadAt         *string          `json:"last_read_at,omitempty"`
	OtherLastReadAt    *string          `json:"other_last_read_at,omitempty"`
	HiddenAt           *string          `json:"hidden_at,omitempty"`
	IsOnline           bool             `json:"is_online"`
	OtherUserID        *uuid.UUID       `json:"other_user_id,omitempty"`
	OtherUserIsDeleted bool             `json:"other_user_is_deleted"`
	OtherUserIsBanned  bool             `json:"other_user_is_banned"`
	IsBlockedByMe      bool             `json:"is_blocked_by_me"`
	IsBlockedByOther   bool             `json:"is_blocked_by_other"`
	MyRole             *string          `json:"my_role,omitempty"`
	MemberCount        int              `json:"member_count,omitempty"`
}

type GetChatsRequest struct {
	Query  string `json:"query" validate:"omitempty"`
	Cursor string `json:"cursor" validate:"omitempty"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
}
