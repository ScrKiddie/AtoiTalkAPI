package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/report"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"encoding/base64"
	"log/slog"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type AdminService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
	wsHub     *websocket.Hub
}

func NewAdminService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub) *AdminService {
	return &AdminService{
		client:    client,
		cfg:       cfg,
		validator: validator,
		wsHub:     wsHub,
	}
}

func (s *AdminService) BanUser(ctx context.Context, adminID uuid.UUID, req model.BanUserRequest) error {
	if err := s.validator.Struct(req); err != nil {
		return helper.NewBadRequestError("")
	}

	req.Reason = strings.TrimSpace(req.Reason)

	targetUser, err := s.client.User.Query().
		Where(user.ID(req.TargetUserID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("User not found")
		}
		return helper.NewInternalServerError("")
	}

	if targetUser.Role == user.RoleAdmin {
		return helper.NewForbiddenError("Cannot ban another admin")
	}

	update := s.client.User.UpdateOneID(req.TargetUserID).
		SetIsBanned(true).
		SetBanReason(req.Reason)

	if req.DurationHours > 0 {
		until := time.Now().UTC().Add(time.Duration(req.DurationHours) * time.Hour)
		update.SetBannedUntil(until)
	} else {
		update.ClearBannedUntil()
	}

	if err := update.Exec(ctx); err != nil {
		slog.Error("Failed to ban user", "error", err)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		event := websocket.Event{
			Type: websocket.EventUserBanned,
			Payload: map[string]interface{}{
				"user_id": req.TargetUserID,
				"reason":  req.Reason,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UTC().UnixMilli(),
				SenderID:  adminID,
			},
		}

		s.wsHub.BroadcastToUser(req.TargetUserID, event)
		s.wsHub.BroadcastToContacts(req.TargetUserID, event)

		s.wsHub.DisconnectUser(req.TargetUserID)
	}

	return nil
}

func (s *AdminService) UnbanUser(ctx context.Context, adminID uuid.UUID, targetUserID uuid.UUID) error {
	err := s.client.User.UpdateOneID(targetUserID).
		SetIsBanned(false).
		ClearBannedUntil().
		ClearBanReason().
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("User not found")
		}
		slog.Error("Failed to unban user", "error", err)
		return helper.NewInternalServerError("")
	}

	return nil
}

func (s *AdminService) GetReports(ctx context.Context, req model.GetReportsRequest) ([]model.ReportListResponse, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	query := s.client.Report.Query()

	if req.Status != "" {
		query = query.Where(report.StatusEQ(report.Status(req.Status)))
	}

	if req.Cursor != "" {
		decodedBytes, err := base64.URLEncoding.DecodeString(req.Cursor)
		if err == nil {
			if cursorID, err := uuid.Parse(string(decodedBytes)); err == nil {
				query = query.Where(report.IDLT(cursorID))
			}
		}
	}

	reports, err := query.
		Order(ent.Desc(report.FieldID)).
		Limit(req.Limit + 1).
		WithReporter().
		All(ctx)

	if err != nil {
		slog.Error("Failed to get reports", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	hasNext := false
	var nextCursor string
	if len(reports) > req.Limit {
		hasNext = true
		reports = reports[:req.Limit]
		lastID := reports[len(reports)-1].ID
		nextCursor = base64.URLEncoding.EncodeToString([]byte(lastID.String()))
	}

	var response []model.ReportListResponse
	for _, r := range reports {
		reporterName := "Unknown"
		if r.Edges.Reporter != nil && r.Edges.Reporter.FullName != nil {
			reporterName = *r.Edges.Reporter.FullName
		}

		response = append(response, model.ReportListResponse{
			ID:           r.ID,
			TargetType:   string(r.TargetType),
			Reason:       r.Reason,
			Status:       string(r.Status),
			ReporterName: reporterName,
			CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		})
	}

	return response, nextCursor, hasNext, nil
}

func (s *AdminService) GetReportDetail(ctx context.Context, reportID uuid.UUID) (*model.ReportDetailResponse, error) {
	r, err := s.client.Report.Query().
		Where(report.ID(reportID)).
		WithReporter(func(q *ent.UserQuery) {
			q.WithAvatar()
		}).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("Report not found")
		}
		slog.Error("Failed to get report detail", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	reporterName := "Unknown"
	reporterAvatar := ""
	if r.Edges.Reporter != nil {
		if r.Edges.Reporter.FullName != nil {
			reporterName = *r.Edges.Reporter.FullName
		}
		if r.Edges.Reporter.Edges.Avatar != nil {
			reporterAvatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, r.Edges.Reporter.Edges.Avatar.FileName)
		}
	}

	var adminNotes *string

	return &model.ReportDetailResponse{
		ID:               r.ID,
		TargetType:       string(r.TargetType),
		Reason:           r.Reason,
		Description:      r.Description,
		Status:           string(r.Status),
		ReporterID:       r.ReporterID,
		ReporterName:     reporterName,
		ReporterAvatar:   reporterAvatar,
		EvidenceSnapshot: r.EvidenceSnapshot,
		AdminNotes:       adminNotes,
		CreatedAt:        r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        r.UpdatedAt.Format(time.RFC3339),
	}, nil
}

func (s *AdminService) ResolveReport(ctx context.Context, reportID uuid.UUID, req model.ResolveReportRequest) error {
	if err := s.validator.Struct(req); err != nil {
		return helper.NewBadRequestError("")
	}

	req.Notes = strings.TrimSpace(req.Notes)

	err := s.client.Report.UpdateOneID(reportID).
		SetStatus(report.Status(req.Status)).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Report not found")
		}
		slog.Error("Failed to resolve report", "error", err)
		return helper.NewInternalServerError("")
	}

	return nil
}
