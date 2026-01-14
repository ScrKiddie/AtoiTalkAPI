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
	"time"

	"github.com/google/uuid"
)

func (s *GroupChatService) JoinGroupByInvite(ctx context.Context, userID uuid.UUID, inviteCode string) (*model.ChatResponse, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	gc, err := tx.GroupChat.Query().
		Where(
			groupchat.InviteCode(inviteCode),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
		WithAvatar().
		WithChat().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("Invalid invite code")
		}
		slog.Error("Failed to query group chat by invite code", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if gc.InviteExpiresAt != nil && time.Now().After(*gc.InviteExpiresAt) {
		return nil, helper.NewBadRequestError("Invite code has expired")
	}

	isMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check membership", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if isMember {
		return nil, helper.NewConflictError("You are already a member of this group")
	}

	_, err = tx.GroupMember.Create().
		SetGroupChat(gc).
		SetUserID(userID).
		SetRole(groupmember.RoleMember).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to add member via invite", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(userID).
		SetType(message.TypeSystemJoin).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for invite join", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	err = tx.Chat.UpdateOne(gc.Edges.Chat).
		SetLastMessage(systemMsg).
		SetLastMessageAt(systemMsg.CreatedAt).
		Exec(ctx)
	if err != nil {
		slog.Error("Failed to update chat with last message", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	s.redisAdapter.Del(context.Background(), fmt.Sprintf("chat_members:%s", gc.ChatID))

	if s.wsHub != nil {
		go func() {
			avatarURL := ""
			if gc.Edges.Avatar != nil {
				avatarURL = s.storageAdapter.GetPublicURL(gc.Edges.Avatar.FileName)
			}

			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.storageAdapter, nil)

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
					ChatID:    gc.ChatID,
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

	return &model.ChatResponse{
		ID:        gc.Edges.Chat.ID,
		Type:      string(chat.TypeGroup),
		CreatedAt: gc.Edges.Chat.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GroupChatService) GetGroupByInviteCode(ctx context.Context, inviteCode string) (*model.GroupPreviewDTO, error) {
	gc, err := s.client.GroupChat.Query().
		Where(
			groupchat.InviteCode(inviteCode),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
		WithAvatar().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("Invalid invite code")
		}
		return nil, helper.NewInternalServerError("")
	}

	if gc.InviteExpiresAt != nil && time.Now().After(*gc.InviteExpiresAt) {
		return nil, helper.NewBadRequestError("Invite code has expired")
	}

	memberCount, err := s.client.GroupMember.Query().
		Where(groupmember.GroupChatID(gc.ID)).
		Count(ctx)
	if err != nil {
		slog.Error("Failed to count group members", "error", err)
	}

	avatarURL := ""
	if gc.Edges.Avatar != nil {
		avatarURL = s.storageAdapter.GetPublicURL(gc.Edges.Avatar.FileName)
	}

	description := ""
	if gc.Description != nil {
		description = *gc.Description
	}

	return &model.GroupPreviewDTO{
		ID:          gc.ID,
		Name:        gc.Name,
		Description: description,
		Avatar:      avatarURL,
		MemberCount: memberCount,
		IsPublic:    gc.IsPublic,
	}, nil
}

func (s *GroupChatService) GetInviteCode(ctx context.Context, userID, groupID uuid.UUID) (*model.GroupInviteResponse, error) {
	gc, err := s.client.GroupChat.Query().
		Where(
			groupchat.ChatID(groupID),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("Group not found")
		}
		return nil, helper.NewInternalServerError("")
	}

	member, err := s.client.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		return nil, helper.NewForbiddenError("You are not a member of this group")
	}

	if member.Role != groupmember.RoleOwner && member.Role != groupmember.RoleAdmin {
		return nil, helper.NewForbiddenError("Only admins can view invite code")
	}

	var expiresAt *string
	if gc.InviteExpiresAt != nil {
		t := gc.InviteExpiresAt.Format(time.RFC3339)
		expiresAt = &t
	}

	return &model.GroupInviteResponse{
		InviteCode: gc.InviteCode,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *GroupChatService) ResetInviteCode(ctx context.Context, userID, groupID uuid.UUID) (*model.GroupInviteResponse, error) {
	gc, err := s.client.GroupChat.Query().
		Where(
			groupchat.ChatID(groupID),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("Group not found")
		}
		return nil, helper.NewInternalServerError("")
	}

	member, err := s.client.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		return nil, helper.NewForbiddenError("You are not a member of this group")
	}

	if member.Role != groupmember.RoleOwner && member.Role != groupmember.RoleAdmin {
		return nil, helper.NewForbiddenError("Only admins can reset invite code")
	}

	newCode, _ := helper.GenerateRandomString(12)

	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	_, err = s.client.GroupChat.UpdateOne(gc).
		SetInviteCode(newCode).
		SetInviteExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to reset invite code", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	expiresAtStr := expiresAt.Format(time.RFC3339)

	return &model.GroupInviteResponse{
		InviteCode: newCode,
		ExpiresAt:  &expiresAtStr,
	}, nil
}
