package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type PrivateChatService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	wsHub          *websocket.Hub
	redisAdapter   *adapter.RedisAdapter
	storageAdapter *adapter.StorageAdapter
}

func NewPrivateChatService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub, redisAdapter *adapter.RedisAdapter, storageAdapter *adapter.StorageAdapter) *PrivateChatService {
	return &PrivateChatService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		wsHub:          wsHub,
		redisAdapter:   redisAdapter,
		storageAdapter: storageAdapter,
	}
}

func (s *PrivateChatService) CreatePrivateChat(ctx context.Context, userID uuid.UUID, req model.CreatePrivateChatRequest) (*model.ChatResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	if userID == req.TargetUserID {
		return nil, helper.NewBadRequestError("Cannot chat with yourself")
	}

	users, err := s.client.User.Query().
		Where(
			user.IDIn(userID, req.TargetUserID),
			user.DeletedAtIsNil(),
		).
		WithAvatar().
		Select(user.FieldID, user.FieldFullName, user.FieldIsBanned, user.FieldBannedUntil, user.FieldAvatarID).
		All(ctx)
	if err != nil {
		slog.Error("Failed to query users for private chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if len(users) != 2 {
		return nil, helper.NewNotFoundError("One or both users not found")
	}

	var creator, targetUser *ent.User
	for _, u := range users {
		if u.ID == userID {
			creator = u
		} else if u.ID == req.TargetUserID {
			targetUser = u
		}
	}

	if creator == nil || targetUser == nil {
		return nil, helper.NewNotFoundError("User not found")
	}

	if targetUser.IsBanned {
		if targetUser.BannedUntil == nil || time.Now().Before(*targetUser.BannedUntil) {
			return nil, helper.NewForbiddenError("Cannot create chat with suspended/banned user")
		}
	}

	isBlocked, err := s.client.UserBlock.Query().
		Where(
			userblock.Or(
				userblock.And(
					userblock.BlockerID(userID),
					userblock.BlockedID(req.TargetUserID),
				),
				userblock.And(
					userblock.BlockerID(req.TargetUserID),
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
					privatechat.User2ID(req.TargetUserID),
				),
				privatechat.And(
					privatechat.User1ID(req.TargetUserID),
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
		SetUser2ID(req.TargetUserID).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create private chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	s.redisAdapter.Del(context.Background(), fmt.Sprintf("contacts:%s", userID))
	s.redisAdapter.Del(context.Background(), fmt.Sprintf("contacts:%s", req.TargetUserID))

	if s.wsHub != nil {
		go func() {
			creatorAvatarURL := ""
			if creator.Edges.Avatar != nil {
				creatorAvatarURL = s.storageAdapter.GetPublicURL(creator.Edges.Avatar.FileName)
			}

			creatorName := ""
			if creator.FullName != nil {
				creatorName = *creator.FullName
			}

			keyCreator := fmt.Sprintf("online:%s", creator.ID)
			existsCreator, _ := s.redisAdapter.Client().Exists(context.Background(), keyCreator).Result()
			creatorIsOnline := existsCreator > 0

			payloadForTarget := model.ChatListResponse{
				ID:          newChat.ID,
				Type:        string(newChat.Type),
				Name:        creatorName,
				Avatar:      creatorAvatarURL,
				LastMessage: nil,
				UnreadCount: 0,
				IsOnline:    creatorIsOnline,
				OtherUserID: &creator.ID,
			}
			s.wsHub.BroadcastToUser(targetUser.ID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payloadForTarget,
				Meta:    &websocket.EventMeta{Timestamp: time.Now().UTC().UnixMilli(), ChatID: newChat.ID, SenderID: userID},
			})

			targetAvatarURL := ""
			if targetUser.Edges.Avatar != nil {
				targetAvatarURL = s.storageAdapter.GetPublicURL(targetUser.Edges.Avatar.FileName)
			}

			targetName := ""
			if targetUser.FullName != nil {
				targetName = *targetUser.FullName
			}

			keyTarget := fmt.Sprintf("online:%s", targetUser.ID)
			existsTarget, _ := s.redisAdapter.Client().Exists(context.Background(), keyTarget).Result()
			targetUserIsOnline := existsTarget > 0

			payloadForCreator := model.ChatListResponse{
				ID:          newChat.ID,
				Type:        string(newChat.Type),
				Name:        targetName,
				Avatar:      targetAvatarURL,
				LastMessage: nil,
				UnreadCount: 0,
				IsOnline:    targetUserIsOnline,
				OtherUserID: &targetUser.ID,
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
