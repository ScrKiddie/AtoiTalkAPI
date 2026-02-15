package model

import (
	"github.com/google/uuid"
)

type CreateReportRequest struct {
	TargetType   string     `json:"target_type" validate:"required,oneof=message group user"`
	Reason       string     `json:"reason" validate:"required"`
	Description  string     `json:"description"`
	MessageID    *uuid.UUID `json:"message_id,omitempty"`
	GroupID      *uuid.UUID `json:"group_id,omitempty"`
	TargetUserID *uuid.UUID `json:"target_user_id,omitempty"`
}

type BanUserRequest struct {
	TargetUserID  uuid.UUID `json:"target_user_id" validate:"required"`
	Reason        string    `json:"reason" validate:"required"`
	DurationHours int       `json:"duration_hours" validate:"omitempty,min=0"`
}

type ResolveReportRequest struct {
	Status string `json:"status" validate:"required,oneof=resolved rejected"`
	Notes  string `json:"notes"`
}

type ReportListResponse struct {
	ID           uuid.UUID `json:"id"`
	TargetType   string    `json:"target_type"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	ReporterName string    `json:"reporter_name"`
	CreatedAt    string    `json:"created_at"`
}

type ReportDetailResponse struct {
	ID                uuid.UUID              `json:"id"`
	TargetType        string                 `json:"target_type"`
	TargetID          *uuid.UUID             `json:"target_id,omitempty"` // ID of the reported entity (User ID, Group ID, or Message ID)
	TargetIsDeleted   bool                   `json:"target_is_deleted"`   // Indicates if the target entity has been soft-deleted
	TargetIsBanned    bool                   `json:"target_is_banned"`    // Indicates if the target user (or sender of message) is banned
	Reason            string                 `json:"reason"`
	Description       *string                `json:"description,omitempty"`
	Status            string                 `json:"status"`
	ReporterID        uuid.UUID              `json:"reporter_id"`
	ReporterName      string                 `json:"reporter_name"`
	ReporterAvatar    string                 `json:"reporter_avatar"`
	ReporterIsDeleted bool                   `json:"reporter_is_deleted"` // Indicates if the reporter user has been soft-deleted
	ReporterIsBanned  bool                   `json:"reporter_is_banned"`  // Indicates if the reporter user is banned
	// EvidenceSnapshot contains a snapshot of the reported entity at the time of reporting.
	//
	// Structure depends on TargetType:
	// - message: { "content": string, "sender_id": uuid, "sender_username": string, "sender_name": string, "sent_at": time, "attachments": []object{id, file_name, original_name, file_size, mime_type, url}, "is_edited": bool, "chat_id": uuid, "chat_type": string, "group_id": uuid (optional) }
	// - group: { "group_id": uuid, "chat_id": uuid, "name": string, "description": string, "avatar": string, "created_by": uuid }
	// - user: { "user_id": uuid, "username": string, "full_name": string, "bio": string, "avatar": string }
	EvidenceSnapshot map[string]interface{} `json:"evidence_snapshot"`
	AdminNotes       *string                `json:"admin_notes,omitempty"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

type GetReportsRequest struct {
	Query  string `json:"query" validate:"omitempty"`
	Status string `json:"status" validate:"omitempty,oneof=pending reviewed resolved rejected"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
	Cursor string `json:"cursor" validate:"omitempty"`
}

type DashboardStatsResponse struct {
	TotalUsers    int `json:"total_users"`
	TotalGroups   int `json:"total_groups"`
	TotalMessages int `json:"total_messages"`
	ActiveReports int `json:"active_reports"`
}

type AdminGetUserListRequest struct {
	Query  string `json:"query" validate:"omitempty"`
	Role   string `json:"role" validate:"omitempty,oneof=user admin"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
	Cursor string `json:"cursor" validate:"omitempty"`
}

type AdminUserListResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     *string   `json:"email"`
	FullName  *string   `json:"full_name"`
	Role      string    `json:"role"`
	IsBanned  bool      `json:"is_banned"`
	CreatedAt string    `json:"created_at"`
}

type AdminUserDetailResponse struct {
	ID            uuid.UUID `json:"id"`
	Username      string    `json:"username"`
	Email         *string   `json:"email"`
	FullName      *string   `json:"full_name"`
	Bio           *string   `json:"bio"`
	Avatar        string    `json:"avatar"`
	Role          string    `json:"role"`
	IsBanned      bool      `json:"is_banned"`
	BanReason     *string   `json:"ban_reason,omitempty"`
	BannedUntil   *string   `json:"banned_until,omitempty"`
	CreatedAt     string    `json:"created_at"`
	LastSeenAt    *string   `json:"last_seen_at"`
	TotalMessages int       `json:"total_messages"`
	TotalGroups   int       `json:"total_groups"`
}

type ResetUserInfoRequest struct {
	TargetUserID uuid.UUID `json:"target_user_id" validate:"required"`
	ResetAvatar  bool      `json:"reset_avatar"`
	ResetBio     bool      `json:"reset_bio"`
	ResetName    bool      `json:"reset_name"`
}

type AdminGetGroupListRequest struct {
	Query  string `json:"query" validate:"omitempty"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
	Cursor string `json:"cursor" validate:"omitempty"`
}

type AdminGroupListResponse struct {
	ID          uuid.UUID `json:"id"`
	ChatID      uuid.UUID `json:"chat_id"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   string    `json:"created_at"`
}

type AdminGroupDetailResponse struct {
	ID            uuid.UUID  `json:"id"`
	ChatID        uuid.UUID  `json:"chat_id"`
	Name          string     `json:"name"`
	Description   *string    `json:"description"`
	Avatar        string     `json:"avatar"`
	IsPublic      bool       `json:"is_public"`
	CreatorID     *uuid.UUID `json:"creator_id"`
	CreatorName   *string    `json:"creator_name"`
	MemberCount   int        `json:"member_count"`
	TotalMessages int        `json:"total_messages"`
	CreatedAt     string     `json:"created_at"`
}

type ResetGroupInfoRequest struct {
	ResetAvatar      bool `json:"reset_avatar"`
	ResetDescription bool `json:"reset_description"`
	ResetName        bool `json:"reset_name"`
}
