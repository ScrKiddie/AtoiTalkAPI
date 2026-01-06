package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/message"
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
	"mime/multipart"
	"path/filepath"
	"time"

	"github.com/go-playground/validator/v10"
)

type GroupChatService struct {
	client         *ent.Client
	repo           *repository.Repository
	cfg            *config.AppConfig
	validator      *validator.Validate
	wsHub          *websocket.Hub
	storageAdapter *adapter.StorageAdapter
}

func NewGroupChatService(client *ent.Client, repo *repository.Repository, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub, storageAdapter *adapter.StorageAdapter) *GroupChatService {
	return &GroupChatService{
		client:         client,
		repo:           repo,
		cfg:            cfg,
		validator:      validator,
		wsHub:          wsHub,
		storageAdapter: storageAdapter,
	}
}

func (s *GroupChatService) CreateGroupChat(ctx context.Context, creatorID int, req model.CreateGroupChatRequest) (*model.ChatResponse, error) {

	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed for CreateGroupChat", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	memberIDs := make(map[int]bool)
	for _, id := range req.MemberIDs {
		if id == creatorID {
			return nil, helper.NewBadRequestError("Cannot add yourself to the member list.")
		}
		memberIDs[id] = true
	}
	allMemberIDs := append(req.MemberIDs, creatorID)

	users, err := s.client.User.Query().Where(user.IDIn(allMemberIDs...)).All(ctx)
	if err != nil {
		slog.Error("Failed to query users for group creation", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if len(users) != len(allMemberIDs) {
		return nil, helper.NewBadRequestError("One or more members not found.")
	}

	blocked, err := s.client.UserBlock.Query().Where(
		userblock.Or(
			userblock.And(userblock.BlockerID(creatorID), userblock.BlockedIDIn(req.MemberIDs...)),
			userblock.And(userblock.BlockerIDIn(req.MemberIDs...), userblock.BlockedID(creatorID)),
		),
	).Exist(ctx)
	if err != nil {
		slog.Error("Failed to check block status for group creation", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if blocked {
		return nil, helper.NewForbiddenError("Cannot create a group with a blocked user.")
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	var avatarMedia *ent.Media
	var fileToUpload multipart.File
	var fileUploadPath string
	var fileContentType string

	if req.Avatar != nil {
		file, err := req.Avatar.Open()
		if err != nil {
			slog.Error("Failed to open avatar file", "error", err)
			return nil, helper.NewInternalServerError("")
		}
		defer file.Close()
		fileToUpload = file

		contentType, err := helper.DetectFileContentType(file)
		if err != nil {
			slog.Error("Failed to detect file content type", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		fileName := helper.GenerateUniqueFileName(req.Avatar.Filename)
		filePath := filepath.Join(s.cfg.StorageProfile, fileName)

		fileUploadPath = filePath
		fileContentType = contentType

		avatarMedia, err = tx.Media.Create().
			SetFileName(fileName).
			SetOriginalName(req.Avatar.Filename).
			SetFileSize(req.Avatar.Size).
			SetMimeType(contentType).
			SetStatus(media.StatusActive).
			SetUploaderID(creatorID).
			Save(ctx)
		if err != nil {
			slog.Error("Failed to create media record for group avatar", "error", err)
			return nil, helper.NewInternalServerError("")
		}
	}

	newChat, err := tx.Chat.Create().SetType(chat.TypeGroup).Save(ctx)
	if err != nil {
		slog.Error("Failed to create chat entity for group", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	groupCreate := tx.GroupChat.Create().
		SetChat(newChat).
		SetCreatorID(creatorID).
		SetName(req.Name).
		SetNillableDescription(&req.Description)

	if avatarMedia != nil {
		groupCreate.SetAvatar(avatarMedia)
	}

	newGroupChat, err := groupCreate.Save(ctx)
	if err != nil {
		slog.Error("Failed to create group chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	var memberCreates []*ent.GroupMemberCreate

	memberCreates = append(memberCreates, tx.GroupMember.Create().
		SetGroupChat(newGroupChat).
		SetUserID(creatorID).
		SetRole(groupmember.RoleOwner))

	for _, memberID := range req.MemberIDs {
		memberCreates = append(memberCreates, tx.GroupMember.Create().
			SetGroupChat(newGroupChat).
			SetUserID(memberID).
			SetRole(groupmember.RoleMember))
	}
	if err := tx.GroupMember.CreateBulk(memberCreates...).Exec(ctx); err != nil {
		slog.Error("Failed to create group members", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(newChat.ID).
		SetSenderID(creatorID).
		SetType(message.TypeSystemCreate).
		SetActionData(map[string]interface{}{
			"initial_name": req.Name,
		}).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for group creation", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	err = tx.Chat.UpdateOne(newChat).
		SetLastMessage(systemMsg).
		SetLastMessageAt(systemMsg.CreatedAt).
		Exec(ctx)
	if err != nil {
		slog.Error("Failed to update chat with last message", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction for group creation", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if fileToUpload != nil {
		if err := s.storageAdapter.StoreFromReader(fileToUpload, fileContentType, fileUploadPath); err != nil {
			slog.Error("Failed to store group avatar after db commit", "error", err)

		}
	}

	if s.wsHub != nil {
		go func() {
			avatarURL := ""
			if avatarMedia != nil {
				avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, avatarMedia.FileName)
			}

			fullMsg, err := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			var lastMsgResp *model.MessageResponse
			if err == nil {
				lastMsgResp = helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)
			}

			payload := model.ChatListResponse{
				ID:          newChat.ID,
				Type:        string(newChat.Type),
				Name:        newGroupChat.Name,
				Avatar:      avatarURL,
				LastMessage: lastMsgResp,
				UnreadCount: 0,
			}

			event := websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payload,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    newChat.ID,
					SenderID:  creatorID,
				},
			}

			for _, memberID := range allMemberIDs {
				s.wsHub.BroadcastToUser(memberID, event)
			}
		}()
	}

	return &model.ChatResponse{
		ID:        newChat.ID,
		Type:      string(newChat.Type),
		CreatedAt: newChat.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GroupChatService) SearchGroupMembers(ctx context.Context, userID int, req model.SearchGroupMembersRequest) ([]model.GroupMemberDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	gc, err := s.client.GroupChat.Query().
		Where(groupchat.ChatID(req.GroupID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, "", false, helper.NewNotFoundError("Group chat not found")
		}
		slog.Error("Failed to query group chat", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	isMember, err := s.client.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check group membership", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}
	if !isMember {
		return nil, "", false, helper.NewForbiddenError("You are not a member of this group")
	}

	members, nextCursor, hasNext, err := s.repo.GroupMember.SearchGroupMembers(ctx, gc.ID, req.Query, req.Cursor, req.Limit)
	if err != nil {
		slog.Error("Failed to search group members", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	var memberDTOs []model.GroupMemberDTO
	for _, m := range members {
		memberDTOs = append(memberDTOs, helper.ToGroupMemberDTO(m, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile))
	}

	return memberDTOs, nextCursor, hasNext, nil
}

func (s *GroupChatService) AddMember(ctx context.Context, requestorID int, groupID int, req model.AddGroupMemberRequest) error {
	if err := s.validator.Struct(req); err != nil {
		return helper.NewBadRequestError("")
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	gc, err := tx.GroupChat.Query().
		Where(groupchat.ChatID(groupID)).
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

	requestorMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(requestorID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewForbiddenError("You are not a member of this group")
		}
		slog.Error("Failed to query requestor membership", "error", err)
		return helper.NewInternalServerError("")
	}

	if requestorMember.Role != groupmember.RoleOwner && requestorMember.Role != groupmember.RoleAdmin {
		return helper.NewForbiddenError("Only admins or owners can add members")
	}

	targetUser, err := tx.User.Query().
		Where(user.ID(req.UserID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("User not found")
		}
		slog.Error("Failed to query target user", "error", err)
		return helper.NewInternalServerError("")
	}

	exists, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(req.UserID),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check existing membership", "error", err)
		return helper.NewInternalServerError("")
	}
	if exists {
		return helper.NewConflictError("User is already a member of this group")
	}

	blocked, err := tx.UserBlock.Query().
		Where(
			userblock.Or(
				userblock.And(userblock.BlockerID(requestorID), userblock.BlockedID(req.UserID)),
				userblock.And(userblock.BlockerID(req.UserID), userblock.BlockedID(requestorID)),
			),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check block status", "error", err)
		return helper.NewInternalServerError("")
	}
	if blocked {
		return helper.NewForbiddenError("Cannot add a blocked user")
	}

	_, err = tx.GroupMember.Create().
		SetGroupChat(gc).
		SetUser(targetUser).
		SetRole(groupmember.RoleMember).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to add group member", "error", err)
		return helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(requestorID).
		SetType(message.TypeSystemAdd).
		SetActionData(map[string]interface{}{
			"target_id":   targetUser.ID,
			"target_name": targetUser.FullName,
		}).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for adding member", "error", err)
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

	if s.wsHub != nil {
		go func() {

			fullMsg, err := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			if err != nil {
				slog.Error("Failed to fetch full system message for broadcast", "error", err)
				return
			}

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

			avatarURL := ""
			if gc.Edges.Avatar != nil {
				avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, gc.Edges.Avatar.FileName)
			}

			chatPayload := model.ChatListResponse{
				ID:          gc.Edges.Chat.ID,
				Type:        string(chat.TypeGroup),
				Name:        gc.Name,
				Avatar:      avatarURL,
				LastMessage: msgResponse,
				UnreadCount: 1,
			}

			s.wsHub.BroadcastToUser(req.UserID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: chatPayload,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    groupID,
					SenderID:  requestorID,
				},
			})

			s.wsHub.BroadcastToChat(gc.ChatID, websocket.Event{
				Type:    websocket.EventMessageNew,
				Payload: msgResponse,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    gc.ChatID,
					SenderID:  requestorID,
				},
			})
		}()
	}

	return nil
}
