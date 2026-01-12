package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/repository"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type ChatService struct {
	client         *ent.Client
	repo           *repository.Repository
	cfg            *config.AppConfig
	validator      *validator.Validate
	wsHub          *websocket.Hub
	storageAdapter *adapter.StorageAdapter
	redisAdapter   *adapter.RedisAdapter
}

func NewChatService(client *ent.Client, repo *repository.Repository, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub, storageAdapter *adapter.StorageAdapter, redisAdapter *adapter.RedisAdapter) *ChatService {
	return &ChatService{
		client:         client,
		repo:           repo,
		cfg:            cfg,
		validator:      validator,
		wsHub:          wsHub,
		storageAdapter: storageAdapter,
		redisAdapter:   redisAdapter,
	}
}

func (s *ChatService) GetChatByID(ctx context.Context, userID, chatID uuid.UUID) (*model.ChatListResponse, error) {
	c, err := s.repo.Chat.GetChatByID(ctx, userID, chatID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}
		slog.Error("Failed to get chat by ID", "error", err, "chatID", chatID)
		return nil, helper.NewInternalServerError("")
	}

	blockedMap := make(map[uuid.UUID]helper.BlockStatus)
	var otherUserID uuid.UUID

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		if c.Edges.PrivateChat.User1ID == userID {
			otherUserID = c.Edges.PrivateChat.User2ID
		} else {
			otherUserID = c.Edges.PrivateChat.User1ID
		}

		blocks, err := s.client.UserBlock.Query().
			Where(
				userblock.Or(
					userblock.And(userblock.BlockerID(userID), userblock.BlockedID(otherUserID)),
					userblock.And(userblock.BlockerID(otherUserID), userblock.BlockedID(userID)),
				),
			).
			All(ctx)

		if err == nil {
			for _, b := range blocks {
				status := blockedMap[otherUserID]
				if b.BlockerID == userID {
					status.BlockedByMe = true
				} else {
					status.BlockedByOther = true
				}
				blockedMap[otherUserID] = status
			}
		}
	}

	onlineMap := make(map[uuid.UUID]bool)
	if otherUserID != uuid.Nil {
		isOnline, _ := s.redisAdapter.Client().SIsMember(ctx, "online_users", otherUserID.String()).Result()
		onlineMap[otherUserID] = isOnline
	}

	resp := helper.MapChatToResponse(userID, c, blockedMap, onlineMap, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, s.cfg.StorageAttachment)

	if resp != nil {

		if resp.LastMessage != nil && resp.LastMessage.ActionData != nil {
			if targetIDStr, ok := resp.LastMessage.ActionData["target_id"].(string); ok {
				if id, err := uuid.Parse(targetIDStr); err == nil {
					u, err := s.client.User.Query().Where(user.ID(id)).Select(user.FieldFullName).Only(ctx)
					if err == nil && u.FullName != nil {
						resp.LastMessage.ActionData["target_name"] = *u.FullName
					}
				}
			}
		}

		if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
			count, err := s.client.GroupMember.Query().
				Where(
					groupmember.GroupChatID(c.Edges.GroupChat.ID),
					groupmember.HasUserWith(user.DeletedAtIsNil()),
				).
				Count(ctx)
			if err == nil {
				resp.MemberCount = count
			} else {
				slog.Error("Failed to count group members", "error", err)
			}
		}
	}

	return resp, nil
}

func (s *ChatService) GetChats(ctx context.Context, userID uuid.UUID, req model.GetChatsRequest) ([]model.ChatListResponse, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	req.Query = strings.TrimSpace(req.Query)

	chats, nextCursor, hasNext, err := s.repo.Chat.GetChats(ctx, userID, req.Query, req.Cursor, req.Limit)
	if err != nil {
		slog.Error("Failed to get chats", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	otherUserIDs := make([]uuid.UUID, 0)
	for _, c := range chats {
		if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
			if c.Edges.PrivateChat.User1ID == userID {
				otherUserIDs = append(otherUserIDs, c.Edges.PrivateChat.User2ID)
			} else {
				otherUserIDs = append(otherUserIDs, c.Edges.PrivateChat.User1ID)
			}
		}
	}

	blockedMap := make(map[uuid.UUID]helper.BlockStatus)
	if len(otherUserIDs) > 0 {
		blocks, err := s.client.UserBlock.Query().
			Where(
				userblock.Or(
					userblock.And(userblock.BlockerID(userID), userblock.BlockedIDIn(otherUserIDs...)),
					userblock.And(userblock.BlockerIDIn(otherUserIDs...), userblock.BlockedID(userID)),
				),
			).
			All(ctx)
		if err == nil {
			for _, b := range blocks {
				status := blockedMap[uuid.Nil]
				if b.BlockerID == userID {
					status = blockedMap[b.BlockedID]
					status.BlockedByMe = true
					blockedMap[b.BlockedID] = status
				} else {
					status = blockedMap[b.BlockerID]
					status.BlockedByOther = true
					blockedMap[b.BlockerID] = status
				}
			}
		}
	}

	onlineMap := make(map[uuid.UUID]bool)
	if len(otherUserIDs) > 0 {
		results, err := s.redisAdapter.Client().Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, id := range otherUserIDs {
				pipe.SIsMember(ctx, "online_users", id.String())
			}
			return nil
		})
		if err == nil {
			for i, res := range results {
				if boolCmd, ok := res.(*redis.BoolCmd); ok {
					onlineMap[otherUserIDs[i]] = boolCmd.Val()
				}
			}
		}
	}

	userIDsToResolve := make(map[uuid.UUID]bool)
	for _, c := range chats {
		if c.Edges.LastMessage != nil && c.Edges.LastMessage.ActionData != nil {
			if targetIDStr, ok := c.Edges.LastMessage.ActionData["target_id"].(string); ok {
				if id, err := uuid.Parse(targetIDStr); err == nil {
					userIDsToResolve[id] = true
				}
			}
		}
	}

	userMap := make(map[uuid.UUID]string)
	if len(userIDsToResolve) > 0 {
		ids := make([]uuid.UUID, 0, len(userIDsToResolve))
		for id := range userIDsToResolve {
			ids = append(ids, id)
		}
		users, err := s.client.User.Query().
			Where(user.IDIn(ids...)).
			Select(user.FieldID, user.FieldFullName).
			All(ctx)
		if err == nil {
			for _, u := range users {
				if u.FullName != nil {
					userMap[u.ID] = *u.FullName
				}
			}
		}
	}

	response := make([]model.ChatListResponse, 0)
	for _, c := range chats {
		resp := helper.MapChatToResponse(userID, c, blockedMap, onlineMap, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, s.cfg.StorageAttachment)
		if resp != nil {

			if resp.LastMessage != nil && resp.LastMessage.ActionData != nil {
				if targetIDStr, ok := resp.LastMessage.ActionData["target_id"].(string); ok {
					if id, err := uuid.Parse(targetIDStr); err == nil {
						if name, exists := userMap[id]; exists {
							resp.LastMessage.ActionData["target_name"] = name
						}
					}
				}
			}
			response = append(response, *resp)
		}
	}

	return response, nextCursor, hasNext, nil
}

func (s *ChatService) MarkAsRead(ctx context.Context, userID uuid.UUID, chatID uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	c, err := tx.Chat.Query().
		Where(
			chat.ID(chatID),
			chat.DeletedAtIsNil(),
		).
		ForUpdate().
		WithPrivateChat().
		WithGroupChat().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("")
		}
		slog.Error("Failed to query chat for update", "error", err)
		return helper.NewInternalServerError("")
	}

	var isBlocked bool
	var otherUserID uuid.UUID

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		update := tx.PrivateChat.UpdateOneID(pc.ID)

		if pc.User1ID == userID {
			if pc.User1UnreadCount == 0 {
				return nil
			}
			otherUserID = pc.User2ID
			update.SetUser1UnreadCount(0)
		} else if pc.User2ID == userID {
			if pc.User2UnreadCount == 0 {
				return nil
			}
			otherUserID = pc.User1ID
			update.SetUser2UnreadCount(0)
		} else {
			return helper.NewForbiddenError("")
		}

		blockExists, err := tx.UserBlock.Query().
			Where(
				userblock.Or(
					userblock.And(userblock.BlockerID(userID), userblock.BlockedID(otherUserID)),
					userblock.And(userblock.BlockerID(otherUserID), userblock.BlockedID(userID)),
				),
			).
			Exist(ctx)

		if err != nil {
			slog.Error("Failed to check block status in MarkAsRead", "error", err)

		}
		isBlocked = blockExists

		if !isBlocked {
			if pc.User1ID == userID {
				update.SetUser1LastReadAt(time.Now().UTC())
			} else {
				update.SetUser2LastReadAt(time.Now().UTC())
			}
		}

		if err := update.Exec(ctx); err != nil {
			slog.Error("Failed to mark private chat as read", "error", err)
			return helper.NewInternalServerError("")
		}

	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		member, err := tx.GroupMember.Query().
			Where(
				groupmember.GroupChatID(c.Edges.GroupChat.ID),
				groupmember.UserID(userID),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return helper.NewForbiddenError("Not a member of this group")
			}
			slog.Error("Failed to query group member", "error", err)
			return helper.NewInternalServerError("")
		}

		if member.UnreadCount == 0 {
			return nil
		}

		err = tx.GroupMember.UpdateOne(member).
			SetUnreadCount(0).
			SetLastReadAt(time.Now().UTC()).
			Exec(ctx)
		if err != nil {
			slog.Error("Failed to mark group chat as read", "error", err)
			return helper.NewInternalServerError("")
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil && !isBlocked {
		go s.wsHub.BroadcastToChat(chatID, websocket.Event{
			Type: websocket.EventChatRead,
			Payload: map[string]interface{}{
				"chat_id": chatID,
				"user_id": userID,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UTC().UnixMilli(),
				ChatID:    chatID,
				SenderID:  userID,
			},
		})
	}

	return nil
}

func (s *ChatService) HideChat(ctx context.Context, userID uuid.UUID, chatID uuid.UUID) error {
	c, err := s.client.Chat.Query().
		Where(
			chat.ID(chatID),
			chat.DeletedAtIsNil(),
		).
		WithPrivateChat().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("")
		}
		slog.Error("Failed to query chat", "error", err, "chatID", chatID)
		return helper.NewInternalServerError("")
	}

	if c.Type != chat.TypePrivate {
		return helper.NewBadRequestError("")
	}

	if c.Edges.PrivateChat == nil {
		return helper.NewInternalServerError("")
	}

	pc := c.Edges.PrivateChat
	update := s.client.PrivateChat.UpdateOneID(pc.ID)

	if pc.User1ID == userID {
		update.SetUser1HiddenAt(time.Now().UTC()).SetUser1UnreadCount(0)
	} else if pc.User2ID == userID {
		update.SetUser2HiddenAt(time.Now().UTC()).SetUser2UnreadCount(0)
	} else {
		return helper.NewForbiddenError("")
	}

	if err := update.Exec(ctx); err != nil {
		slog.Error("Failed to hide chat", "error", err, "chatID", chatID)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		go s.wsHub.BroadcastToUser(userID, websocket.Event{
			Type: websocket.EventChatHide,
			Payload: map[string]interface{}{
				"chat_id": chatID,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UTC().UnixMilli(),
				ChatID:    chatID,
				SenderID:  userID,
			},
		})
	}

	return nil
}
