package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/ent/report"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"log/slog"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type ReportService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
}

func NewReportService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate) *ReportService {
	return &ReportService{
		client:    client,
		cfg:       cfg,
		validator: validator,
	}
}

func (s *ReportService) CreateReport(ctx context.Context, reporterID uuid.UUID, req model.CreateReportRequest) error {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err)
		return helper.NewBadRequestError("")
	}

	req.Reason = strings.TrimSpace(req.Reason)
	req.Description = strings.TrimSpace(req.Description)

	var evidenceData map[string]interface{}
	var targetMessageID *uuid.UUID
	var targetGroupID *uuid.UUID
	var targetUserID *uuid.UUID
	var mediaIDsToProtect []uuid.UUID

	switch req.TargetType {
	case "message":
		if req.MessageID == nil {
			return helper.NewBadRequestError("Message ID is required")
		}
		targetMessageID = req.MessageID

		msg, err := s.client.Message.Query().
			Where(message.ID(*req.MessageID)).
			WithSender().
			WithAttachments().
			Only(ctx)

		if err != nil {
			if ent.IsNotFound(err) {
				return helper.NewNotFoundError("Message not found")
			}
			return helper.NewInternalServerError("")
		}

		if msg.SenderID != nil && *msg.SenderID == reporterID {
			return helper.NewBadRequestError("You cannot report your own message")
		}

		chatInfo, err := s.client.Chat.Query().
			Where(chat.ID(msg.ChatID)).
			WithPrivateChat().
			WithGroupChat().
			Only(ctx)

		if err != nil {
			slog.Error("Failed to query chat info for report validation", "error", err)
			return helper.NewInternalServerError("")
		}

		if chatInfo.Type == chat.TypePrivate {
			if chatInfo.Edges.PrivateChat == nil {
				return helper.NewInternalServerError("")
			}
			pc := chatInfo.Edges.PrivateChat
			if pc.User1ID != reporterID && pc.User2ID != reporterID {
				return helper.NewForbiddenError("You cannot report a message from a chat you are not part of")
			}
		} else if chatInfo.Type == chat.TypeGroup {
			if chatInfo.Edges.GroupChat == nil {
				return helper.NewInternalServerError("")
			}
			isMember, err := s.client.GroupMember.Query().
				Where(
					groupmember.GroupChatID(chatInfo.Edges.GroupChat.ID),
					groupmember.UserID(reporterID),
				).Exist(ctx)
			if err != nil {
				slog.Error("Failed to check group membership for report", "error", err)
				return helper.NewInternalServerError("")
			}
			if !isMember {
				return helper.NewForbiddenError("You cannot report a message from a group you are not part of")
			}
		}

		attachments := make([]string, 0)
		for _, att := range msg.Edges.Attachments {
			url := helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment, att.FileName)
			attachments = append(attachments, url)
			mediaIDsToProtect = append(mediaIDsToProtect, att.ID)
		}

		senderID := ""
		if msg.SenderID != nil {
			senderID = msg.SenderID.String()
		}

		evidenceData = map[string]interface{}{
			"content":     msg.Content,
			"sender_id":   senderID,
			"sent_at":     msg.CreatedAt,
			"attachments": attachments,
			"is_edited":   msg.EditedAt != nil,
		}

	case "group":
		if req.GroupID == nil {
			return helper.NewBadRequestError("Group ID is required")
		}
		targetGroupID = req.GroupID

		group, err := s.client.GroupChat.Query().
			Where(groupchat.ChatID(*req.GroupID)).
			WithAvatar().
			Only(ctx)

		if err != nil {
			if ent.IsNotFound(err) {
				return helper.NewNotFoundError("Group not found")
			}
			return helper.NewInternalServerError("")
		}

		isMember, _ := s.client.GroupMember.Query().
			Where(groupmember.GroupChatID(group.ID), groupmember.UserID(reporterID)).
			Exist(ctx)
		if !isMember {
			return helper.NewForbiddenError("You must be a member of the group to report it")
		}

		avatarURL := ""
		if group.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, group.Edges.Avatar.FileName)
			mediaIDsToProtect = append(mediaIDsToProtect, group.Edges.Avatar.ID)
		}

		evidenceData = map[string]interface{}{
			"name":        group.Name,
			"description": group.Description,
			"avatar":      avatarURL,
			"created_by":  group.CreatedBy,
		}

	case "user":
		if req.TargetUserID == nil {
			return helper.NewBadRequestError("Target User ID is required")
		}
		if *req.TargetUserID == reporterID {
			return helper.NewBadRequestError("You cannot report yourself")
		}
		targetUserID = req.TargetUserID

		u, err := s.client.User.Query().
			Where(user.ID(*req.TargetUserID)).
			WithAvatar().
			Only(ctx)

		if err != nil {
			if ent.IsNotFound(err) {
				return helper.NewNotFoundError("User not found")
			}
			return helper.NewInternalServerError("")
		}

		avatarURL := ""
		if u.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
			mediaIDsToProtect = append(mediaIDsToProtect, u.Edges.Avatar.ID)
		}

		evidenceData = map[string]interface{}{
			"username":  u.Username,
			"full_name": u.FullName,
			"bio":       u.Bio,
			"avatar":    avatarURL,
		}
	}

	create := s.client.Report.Create().
		SetReporterID(reporterID).
		SetTargetType(report.TargetType(req.TargetType)).
		SetReason(req.Reason).
		SetNillableDescription(&req.Description).
		SetEvidenceSnapshot(evidenceData)

	if targetMessageID != nil {
		create.SetMessageID(*targetMessageID)
	}
	if targetGroupID != nil {
		gc, err := s.client.GroupChat.Query().Where(groupchat.ChatID(*targetGroupID)).Only(ctx)
		if err == nil {
			create.SetGroupID(gc.ID)
		} else {
			slog.Error("Failed to resolve GroupChat ID for report", "error", err)
			return helper.NewInternalServerError("")
		}
	}
	if targetUserID != nil {
		create.SetTargetUserID(*targetUserID)
	}

	if len(mediaIDsToProtect) > 0 {
		create.AddEvidenceMediumIDs(mediaIDsToProtect...)
	}

	_, err := create.Save(ctx)
	if err != nil {
		slog.Error("Failed to create report", "error", err)
		return helper.NewInternalServerError("")
	}

	return nil
}
