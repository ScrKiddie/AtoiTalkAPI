package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"log/slog"
	"time"

	"github.com/go-playground/validator/v10"
)

type PrivateChatService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
	wsHub     *websocket.Hub
}

func NewPrivateChatService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub) *PrivateChatService {
	return &PrivateChatService{
		client:    client,
		cfg:       cfg,
		validator: validator,
		wsHub:     wsHub,
	}
}

func (s *PrivateChatService) CreatePrivateChat(ctx context.Context, userID int, req model.CreatePrivateChatRequest) (*model.ChatResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	targetUser, err := s.client.User.Query().
		Where(user.UsernameEQ(req.Username)).
		WithAvatar().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("User not found")
		}
		slog.Error("Failed to resolve username", "error", err, "username", req.Username)
		return nil, helper.NewInternalServerError("")
	}

	targetUserID := targetUser.ID

	if userID == targetUserID {
		return nil, helper.NewBadRequestError("Cannot chat with yourself")
	}

	creator, err := s.client.User.Query().Where(user.ID(userID)).WithAvatar().Only(ctx)
	if err != nil {
		slog.Error("Failed to query creator", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	isBlocked, err := s.client.UserBlock.Query().
		Where(
			userblock.Or(
				userblock.And(
					userblock.BlockerID(userID),
					userblock.BlockedID(targetUserID),
				),
				userblock.And(
					userblock.BlockerID(targetUserID),
					userblock.BlockedID(userID),
				),
			),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check block status", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if isBlocked {
		return nil, helper.NewForbiddenError("Cannot create chat with blocked user")
	}

	existingChat, err := s.client.PrivateChat.Query().
		Where(
			privatechat.Or(
				privatechat.And(
					privatechat.User1ID(userID),
					privatechat.User2ID(targetUserID),
				),
				privatechat.And(
					privatechat.User1ID(targetUserID),
					privatechat.User2ID(userID),
				),
			),
		).
		WithChat().
		Only(ctx)

	if err == nil {
		return &model.ChatResponse{
			ID:        existingChat.Edges.Chat.ID,
			Type:      string(existingChat.Edges.Chat.Type),
			CreatedAt: existingChat.Edges.Chat.CreatedAt.Format(time.RFC3339),
		}, nil
	} else if !ent.IsNotFound(err) {
		slog.Error("Failed to check existing private chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	defer func() {
		_ = tx.Rollback()
		if v := recover(); v != nil {
			panic(v)
		}
	}()

	newChat, err := tx.Chat.Create().
		SetType(chat.TypePrivate).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	_, err = tx.PrivateChat.Create().
		SetChat(newChat).
		SetUser1ID(userID).
		SetUser2ID(targetUserID).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create private chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		go func() {
			creatorAvatarURL := ""
			if creator.Edges.Avatar != nil {
				creatorAvatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, creator.Edges.Avatar.FileName)
			}
			payloadForTarget := model.ChatListResponse{
				ID:            newChat.ID,
				Type:          string(newChat.Type),
				Name:          creator.FullName,
				Avatar:        creatorAvatarURL,
				LastMessage:   nil,
				UnreadCount:   0,
				IsOnline:      creator.IsOnline,
				OtherUserID:   &creator.ID,
				OtherUsername: &creator.Username,
			}
			s.wsHub.BroadcastToUser(targetUser.ID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payloadForTarget,
				Meta:    &websocket.EventMeta{Timestamp: time.Now().UTC().UnixMilli(), ChatID: newChat.ID, SenderID: userID},
			})

			targetAvatarURL := ""
			if targetUser.Edges.Avatar != nil {
				targetAvatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, targetUser.Edges.Avatar.FileName)
			}
			payloadForCreator := model.ChatListResponse{
				ID:            newChat.ID,
				Type:          string(newChat.Type),
				Name:          targetUser.FullName,
				Avatar:        targetAvatarURL,
				LastMessage:   nil,
				UnreadCount:   0,
				IsOnline:      targetUser.IsOnline,
				OtherUserID:   &targetUser.ID,
				OtherUsername: &targetUser.Username,
			}
			s.wsHub.BroadcastToUser(creator.ID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payloadForCreator,
				Meta:    &websocket.EventMeta{Timestamp: time.Now().UTC().UnixMilli(), ChatID: newChat.ID, SenderID: userID},
			})
		}()
	}

	return &model.ChatResponse{
		ID:        newChat.ID,
		Type:      string(newChat.Type),
		CreatedAt: newChat.CreatedAt.Format(time.RFC3339),
	}, nil
}
