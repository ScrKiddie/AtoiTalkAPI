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
	ID             uuid.UUID `json:"id"`
	TargetType     string    `json:"target_type"`
	Reason         string    `json:"reason"`
	Description    *string   `json:"description,omitempty"`
	Status         string    `json:"status"`
	ReporterID     uuid.UUID `json:"reporter_id"`
	ReporterName   string    `json:"reporter_name"`
	ReporterAvatar string    `json:"reporter_avatar"`
	// EvidenceSnapshot contains a snapshot of the reported entity at the time of reporting.
	//
	// Structure depends on TargetType:
	// - message: { "content": string, "sender_id": uuid, "sent_at": time, "attachments": []string, "is_edited": bool }
	// - group: { "name": string, "description": string, "avatar": string, "created_by": uuid }
	// - user: { "username": string, "full_name": string, "bio": string, "avatar": string }
	EvidenceSnapshot map[string]interface{} `json:"evidence_snapshot"`
	AdminNotes       *string                `json:"admin_notes,omitempty"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

type GetReportsRequest struct {
	Status string `json:"status" validate:"omitempty,oneof=pending reviewed resolved rejected"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
	Cursor string `json:"cursor" validate:"omitempty"`
}
