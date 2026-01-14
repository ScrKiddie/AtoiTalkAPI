package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *GroupChatService) SearchPublicGroups(ctx context.Context, userID uuid.UUID, req model.SearchPublicGroupsRequest) ([]model.PublicGroupDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	groups, nextCursor, hasNext, err := s.repo.GroupChat.SearchPublicGroups(ctx, req.Query, req.Cursor, req.Limit)
	if err != nil {
		if strings.Contains(err.Error(), "invalid cursor format") {
			slog.Warn("Invalid cursor format in SearchPublicGroups", "error", err)
			return nil, "", false, helper.NewBadRequestError("")
		}
		slog.Error("Failed to search public groups", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	var groupDTOs []model.PublicGroupDTO
	for _, g := range groups {
		avatarURL := ""
		if g.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, g.Edges.Avatar.FileName)
		}

		isMember := false
		for _, m := range g.Edges.Members {
			if m.UserID == userID {
				isMember = true
				break
			}
		}

		description := ""
		if g.Description != nil {
			description = *g.Description
		}

		groupDTOs = append(groupDTOs, model.PublicGroupDTO{
			ID:          g.ID,
			ChatID:      g.ChatID,
			Name:        g.Name,
			Description: description,
			Avatar:      avatarURL,
			MemberCount: len(g.Edges.Members),
			IsMember:    isMember,
		})
	}

	return groupDTOs, nextCursor, hasNext, nil
}

func (s *GroupChatService) JoinPublicGroup(ctx context.Context, userID uuid.UUID, groupID uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	gc, err := tx.GroupChat.Query().
		Where(
			groupchat.ChatID(groupID),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
		WithAvatar().
		WithChat().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Group chat not found")
		}
		slog.Error("Failed to query group chat", "error", err)
		return helper.NewInternalServerError("")
	}

	if !gc.IsPublic {
		return helper.NewForbiddenError("This group is private. You must be added by an admin.")
	}

	isMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check membership", "error", err)
		return helper.NewInternalServerError("")
	}
	if isMember {
		return helper.NewConflictError("You are already a member of this group")
	}

	_, err = tx.GroupMember.Create().
		SetGroupChat(gc).
		SetUserID(userID).
		SetRole(groupmember.RoleMember).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to add member", "error", err)
		return helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(userID).
		SetType(message.TypeSystemJoin).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for join", "error", err)
		return helper.NewInternalServerError("")
	}

	err = tx.Chat.UpdateOne(gc.Edges.Chat).
		SetLastMessage(systemMsg).
		SetLastMessageAt(systemMsg.CreatedAt).
		Exec(ctx)
	if err != nil {
		slog.Error("Failed to update chat with last message", "error", err)
		return helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	s.redisAdapter.Del(context.Background(), fmt.Sprintf("chat_members:%s", groupID))

	if s.wsHub != nil {
		go func() {
			avatarURL := ""
			if gc.Edges.Avatar != nil {
				avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, gc.Edges.Avatar.FileName)
			}

			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, s.cfg.StorageAttachment, nil)

			chatPayload := model.ChatListResponse{
				ID:          gc.Edges.Chat.ID,
				Type:        string(chat.TypeGroup),
				Name:        gc.Name,
				Avatar:      avatarURL,
				LastMessage: msgResponse,
				UnreadCount: 0,
			}

			s.wsHub.BroadcastToUser(userID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: chatPayload,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    groupID,
					SenderID:  userID,
				},
			})

			s.wsHub.BroadcastToChat(gc.ChatID, websocket.Event{
				Type:    websocket.EventMessageNew,
				Payload: msgResponse,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    gc.ChatID,
					SenderID:  userID,
				},
			})
		}()
	}

	return nil
}
