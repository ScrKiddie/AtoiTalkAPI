package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/report"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/repository"
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
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	wsHub          *websocket.Hub
	repo           *repository.Repository
	storageAdapter *adapter.StorageAdapter
}

func NewAdminService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub, repo *repository.Repository, storageAdapter *adapter.StorageAdapter) *AdminService {
	return &AdminService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		wsHub:          wsHub,
		repo:           repo,
		storageAdapter: storageAdapter,
	}
}

func (s *AdminService) BanUser(ctx context.Context, adminID uuid.UUID, req model.BanUserRequest) error {
	if err := s.validator.Struct(req); err != nil {
		return helper.NewBadRequestError("")
	}

	req.Reason = strings.TrimSpace(req.Reason)

	targetUser, err := s.client.User.Query().
		Where(user.ID(req.TargetUserID)).
		Select(user.FieldID, user.FieldRole, user.FieldIsBanned, user.FieldBannedUntil, user.FieldBanReason).
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

	var newBannedUntil *time.Time
	if req.DurationHours > 0 {
		until := time.Now().UTC().Add(time.Duration(req.DurationHours) * time.Hour)
		newBannedUntil = &until
	}

	if targetUser.IsBanned && targetUser.BanReason != nil && *targetUser.BanReason == req.Reason {
		if (targetUser.BannedUntil == nil && newBannedUntil == nil) ||
			(targetUser.BannedUntil != nil && newBannedUntil != nil && targetUser.BannedUntil.Equal(*newBannedUntil)) {
			return nil
		}
	}

	update := s.client.User.UpdateOneID(req.TargetUserID).
		SetIsBanned(true).
		SetBanReason(req.Reason)

	if newBannedUntil != nil {
		update.SetBannedUntil(*newBannedUntil)
	} else {
		update.ClearBannedUntil()
	}

	if err := update.Exec(ctx); err != nil {
		slog.Error("Failed to ban user", "error", err)
		return helper.NewInternalServerError("")
	}

	if err := s.repo.Session.RevokeAllSessions(ctx, req.TargetUserID); err != nil {
		slog.Error("Failed to revoke sessions for banned user", "error", err)
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
	targetUser, err := s.client.User.Query().
		Where(user.ID(targetUserID)).
		Select(user.FieldID, user.FieldIsBanned).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("User not found")
		}
		return helper.NewInternalServerError("")
	}

	if !targetUser.IsBanned {
		return nil
	}

	err = s.client.User.UpdateOneID(targetUserID).
		SetIsBanned(false).
		ClearBannedUntil().
		ClearBanReason().
		Exec(ctx)

	if err != nil {
		slog.Error("Failed to unban user", "error", err)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		event := websocket.Event{
			Type: websocket.EventUserUnbanned,
			Payload: map[string]interface{}{
				"user_id": targetUserID,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UTC().UnixMilli(),
				SenderID:  adminID,
			},
		}

		s.wsHub.BroadcastToUser(targetUserID, event)
		s.wsHub.BroadcastToContacts(targetUserID, event)
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
			reporterAvatar = s.storageAdapter.GetPublicURL(r.Edges.Reporter.Edges.Avatar.FileName)
		}
	}

	if r.TargetType == report.TargetTypeMessage && r.EvidenceSnapshot != nil {
		if attachments, ok := r.EvidenceSnapshot["attachments"].([]interface{}); ok {
			var refreshedAttachments []string
			for _, att := range attachments {
				if fileName, ok := att.(string); ok {

					url, err := s.storageAdapter.GetPresignedURL(fileName, 15*time.Minute)
					if err == nil {
						refreshedAttachments = append(refreshedAttachments, url)
					} else {
						slog.Error("Failed to refresh presigned URL for report evidence", "error", err, "file", fileName)

						refreshedAttachments = append(refreshedAttachments, fileName)
					}
				}
			}
			r.EvidenceSnapshot["attachments"] = refreshedAttachments
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

	r, err := s.client.Report.Query().
		Where(report.ID(reportID)).
		Select(report.FieldID, report.FieldStatus).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Report not found")
		}
		slog.Error("Failed to query report", "error", err)
		return helper.NewInternalServerError("")
	}

	if r.Status == report.Status(req.Status) {
		return nil
	}

	err = s.client.Report.UpdateOneID(reportID).
		SetStatus(report.Status(req.Status)).
		Exec(ctx)

	if err != nil {
		slog.Error("Failed to resolve report", "error", err)
		return helper.NewInternalServerError("")
	}

	return nil
}
