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
	"github.com/google/uuid"
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

func (s *GroupChatService) CreateGroupChat(ctx context.Context, creatorID uuid.UUID, req model.CreateGroupChatRequest) (*model.ChatResponse, error) {

	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed for CreateGroupChat", "error", err)
		return nil, helper.NewBadRequestError("")
	}

	users, err := s.client.User.Query().
		Where(user.IDIn(req.MemberIDs...)).
		All(ctx)
	if err != nil {
		slog.Error("Failed to query users for group creation", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if len(users) != len(req.MemberIDs) {
		return nil, helper.NewBadRequestError("One or more members not found.")
	}

	var memberIDs []uuid.UUID
	for _, u := range users {
		if u.ID == creatorID {
			return nil, helper.NewBadRequestError("Cannot add yourself to the member list.")
		}
		memberIDs = append(memberIDs, u.ID)
	}
	allMemberIDs := append(memberIDs, creatorID)

	blocked, err := s.client.UserBlock.Query().Where(
		userblock.Or(
			userblock.And(userblock.BlockerID(creatorID), userblock.BlockedIDIn(memberIDs...)),
			userblock.And(userblock.BlockerIDIn(memberIDs...), userblock.BlockedID(creatorID)),
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

	for _, memberID := range memberIDs {
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

func (s *GroupChatService) UpdateGroupChat(ctx context.Context, requestorID uuid.UUID, groupID uuid.UUID, req model.UpdateGroupChatRequest) (*model.ChatListResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, helper.NewBadRequestError("")
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return nil, helper.NewInternalServerError("")
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
			return nil, helper.NewNotFoundError("Group chat not found")
		}
		slog.Error("Failed to query group chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	requestorMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(requestorID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewForbiddenError("You are not a member of this group")
		}
		slog.Error("Failed to query requestor membership", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if requestorMember.Role != groupmember.RoleOwner && requestorMember.Role != groupmember.RoleAdmin {
		return nil, helper.NewForbiddenError("Only admins or owners can update group info")
	}

	update := tx.GroupChat.UpdateOne(gc)
	var systemMessages []*ent.MessageCreate

	if req.Name != nil && *req.Name != gc.Name {
		update.SetName(*req.Name)
		systemMessages = append(systemMessages, tx.Message.Create().
			SetChatID(gc.ChatID).
			SetSenderID(requestorID).
			SetType(message.TypeSystemRename).
			SetActionData(map[string]interface{}{
				"old_name": gc.Name,
				"new_name": *req.Name,
			}))
	}

	if req.Description != nil {
		oldDesc := ""
		if gc.Description != nil {
			oldDesc = *gc.Description
		}
		if *req.Description != oldDesc {
			update.SetDescription(*req.Description)
			systemMessages = append(systemMessages, tx.Message.Create().
				SetChatID(gc.ChatID).
				SetSenderID(requestorID).
				SetType(message.TypeSystemDescription).
				SetActionData(map[string]interface{}{
					"old_description": oldDesc,
					"new_description": *req.Description,
				}))
		}
	}

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

		media, err := tx.Media.Create().
			SetFileName(fileName).
			SetOriginalName(req.Avatar.Filename).
			SetFileSize(req.Avatar.Size).
			SetMimeType(contentType).
			SetStatus(media.StatusActive).
			SetUploaderID(requestorID).
			Save(ctx)
		if err != nil {
			slog.Error("Failed to create media record for group avatar", "error", err)
			return nil, helper.NewInternalServerError("")
		}

		update.SetAvatar(media)

		systemMessages = append(systemMessages, tx.Message.Create().
			SetChatID(gc.ChatID).
			SetSenderID(requestorID).
			SetType(message.TypeSystemAvatar).
			SetActionData(map[string]interface{}{
				"action": "updated",
			}))
	}

	updatedGroup, err := update.Save(ctx)
	if err != nil {
		slog.Error("Failed to update group chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	var lastSystemMsg *ent.Message
	if len(systemMessages) > 0 {
		msgs, err := tx.Message.CreateBulk(systemMessages...).Save(ctx)
		if err != nil {
			slog.Error("Failed to create system messages", "error", err)
			return nil, helper.NewInternalServerError("")
		}
		lastSystemMsg = msgs[len(msgs)-1]

		err = tx.Chat.UpdateOne(gc.Edges.Chat).
			SetLastMessage(lastSystemMsg).
			SetLastMessageAt(lastSystemMsg.CreatedAt).
			Exec(ctx)
		if err != nil {
			slog.Error("Failed to update chat last message", "error", err)
			return nil, helper.NewInternalServerError("")
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if fileToUpload != nil {
		if err := s.storageAdapter.StoreFromReader(fileToUpload, fileContentType, fileUploadPath); err != nil {
			slog.Error("Failed to store group avatar after db commit", "error", err)

		}
	}

	if s.wsHub != nil && lastSystemMsg != nil {
		go func() {

			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(lastSystemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

			s.wsHub.BroadcastToChat(gc.ChatID, websocket.Event{
				Type:    websocket.EventMessageNew,
				Payload: msgResponse,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    gc.ChatID,
					SenderID:  requestorID,
				},
			})

			updatedGroupWithAvatar, _ := s.client.GroupChat.Query().
				Where(groupchat.ID(updatedGroup.ID)).
				WithAvatar().
				Only(context.Background())

			avatarURL := ""
			if updatedGroupWithAvatar.Edges.Avatar != nil {
				avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, updatedGroupWithAvatar.Edges.Avatar.FileName)
			}

			chatPayload := model.ChatListResponse{
				ID:          gc.Edges.Chat.ID,
				Type:        string(chat.TypeGroup),
				Name:        updatedGroup.Name,
				Avatar:      avatarURL,
				LastMessage: msgResponse,
			}

			s.wsHub.BroadcastToChat(gc.ChatID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: chatPayload,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    gc.ChatID,
					SenderID:  requestorID,
				},
			})
		}()
	}

	avatarURL := ""

	if req.Avatar == nil && gc.Edges.Avatar != nil {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, gc.Edges.Avatar.FileName)
	} else if req.Avatar != nil {

		updatedGroupWithAvatar, _ := s.client.GroupChat.Query().Where(groupchat.ID(updatedGroup.ID)).WithAvatar().Only(context.Background())
		if updatedGroupWithAvatar.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, updatedGroupWithAvatar.Edges.Avatar.FileName)
		}
	}

	myRole := string(requestorMember.Role)

	return &model.ChatListResponse{
		ID:     gc.Edges.Chat.ID,
		Type:   string(chat.TypeGroup),
		Name:   updatedGroup.Name,
		Avatar: avatarURL,
		MyRole: &myRole,
	}, nil
}

func (s *GroupChatService) SearchGroupMembers(ctx context.Context, userID uuid.UUID, req model.SearchGroupMembersRequest) ([]model.GroupMemberDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	gc, err := s.client.GroupChat.Query().
		Where(
			groupchat.ChatID(req.GroupID),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
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

func (s *GroupChatService) AddMember(ctx context.Context, requestorID uuid.UUID, groupID uuid.UUID, req model.AddGroupMemberRequest) error {
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

	targetUsers, err := tx.User.Query().
		Where(user.IDIn(req.UserIDs...)).
		All(ctx)
	if err != nil {
		slog.Error("Failed to query target users", "error", err)
		return helper.NewInternalServerError("")
	}
	if len(targetUsers) != len(req.UserIDs) {
		return helper.NewNotFoundError("One or more users not found")
	}

	var targetUserIDs []uuid.UUID
	for _, u := range targetUsers {
		targetUserIDs = append(targetUserIDs, u.ID)
	}

	existingMembers, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserIDIn(targetUserIDs...),
		).
		All(ctx)
	if err != nil {
		slog.Error("Failed to check existing memberships", "error", err)
		return helper.NewInternalServerError("")
	}

	existingMemberIDs := make(map[uuid.UUID]bool)
	for _, m := range existingMembers {
		existingMemberIDs[m.UserID] = true
	}

	blocked, err := tx.UserBlock.Query().
		Where(
			userblock.Or(
				userblock.And(userblock.BlockerID(requestorID), userblock.BlockedIDIn(targetUserIDs...)),
				userblock.And(userblock.BlockerIDIn(targetUserIDs...), userblock.BlockedID(requestorID)),
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

	var memberCreates []*ent.GroupMemberCreate
	var newMembers []*ent.User

	for _, u := range targetUsers {
		if existingMemberIDs[u.ID] {
			continue
		}
		memberCreates = append(memberCreates, tx.GroupMember.Create().
			SetGroupChat(gc).
			SetUser(u).
			SetRole(groupmember.RoleMember))
		newMembers = append(newMembers, u)
	}

	if len(memberCreates) == 0 {
		return helper.NewConflictError("All users are already members")
	}

	_, err = tx.GroupMember.CreateBulk(memberCreates...).Save(ctx)
	if err != nil {
		slog.Error("Failed to add group members", "error", err)
		return helper.NewInternalServerError("")
	}

	var lastSystemMsg *ent.Message
	for _, u := range newMembers {
		lastSystemMsg, err = tx.Message.Create().
			SetChatID(gc.ChatID).
			SetSenderID(requestorID).
			SetType(message.TypeSystemAdd).
			SetActionData(map[string]interface{}{
				"target_id": u.ID,
				"actor_id":  requestorID,
			}).
			Save(ctx)
		if err != nil {
			slog.Error("Failed to create system message", "error", err)
			return helper.NewInternalServerError("")
		}
	}

	err = tx.Chat.UpdateOne(gc.Edges.Chat).
		SetLastMessage(lastSystemMsg).
		SetLastMessageAt(lastSystemMsg.CreatedAt).
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
			avatarURL := ""
			if gc.Edges.Avatar != nil {
				avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, gc.Edges.Avatar.FileName)
			}

			for _, u := range newMembers {
				fullMsg, _ := s.client.Message.Query().
					Where(message.ID(lastSystemMsg.ID)).
					WithSender().
					Only(context.Background())

				msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

				chatPayload := model.ChatListResponse{
					ID:          gc.Edges.Chat.ID,
					Type:        string(chat.TypeGroup),
					Name:        gc.Name,
					Avatar:      avatarURL,
					LastMessage: msgResponse,
					UnreadCount: 1,
				}

				s.wsHub.BroadcastToUser(u.ID, websocket.Event{
					Type:    websocket.EventChatNew,
					Payload: chatPayload,
					Meta: &websocket.EventMeta{
						Timestamp: time.Now().UTC().UnixMilli(),
						ChatID:    groupID,
						SenderID:  requestorID,
					},
				})
			}

			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(lastSystemMsg.ID)).
				WithSender().
				Only(context.Background())
			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

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

func (s *GroupChatService) LeaveGroup(ctx context.Context, userID uuid.UUID, groupID uuid.UUID) error {
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
		WithChat().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Group chat not found")
		}
		slog.Error("Failed to query group chat", "error", err)
		return helper.NewInternalServerError("")
	}

	member, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewBadRequestError("You are not a member of this group")
		}
		slog.Error("Failed to query membership", "error", err)
		return helper.NewInternalServerError("")
	}

	if member.Role == groupmember.RoleOwner {
		return helper.NewBadRequestError("Owner cannot leave the group. Please transfer ownership first.")
	}

	err = tx.GroupMember.DeleteOne(member).Exec(ctx)
	if err != nil {
		slog.Error("Failed to delete group member", "error", err)
		return helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(userID).
		SetType(message.TypeSystemLeave).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for leave", "error", err)
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
			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

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

func (s *GroupChatService) KickMember(ctx context.Context, requestorID uuid.UUID, groupID uuid.UUID, targetUserID uuid.UUID) error {
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
		return helper.NewForbiddenError("You are not a member of this group")
	}

	if requestorMember.Role == groupmember.RoleMember {
		return helper.NewForbiddenError("Only admins or owners can kick members")
	}

	targetMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(targetUserID),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		return helper.NewNotFoundError("Target user is not a member of this group")
	}

	if targetMember.UserID == requestorID {
		return helper.NewBadRequestError("Cannot kick yourself")
	}

	if requestorMember.Role == groupmember.RoleAdmin {
		if targetMember.Role == groupmember.RoleAdmin || targetMember.Role == groupmember.RoleOwner {
			return helper.NewForbiddenError("Admins cannot kick other admins or the owner")
		}
	}

	err = tx.GroupMember.DeleteOne(targetMember).Exec(ctx)
	if err != nil {
		slog.Error("Failed to delete group member", "error", err)
		return helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(requestorID).
		SetType(message.TypeSystemKick).
		SetActionData(map[string]interface{}{
			"target_id": targetMember.UserID,
			"actor_id":  requestorID,
		}).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for kick", "error", err)
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
			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

			s.wsHub.BroadcastToChat(gc.ChatID, websocket.Event{
				Type:    websocket.EventMessageNew,
				Payload: msgResponse,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    gc.ChatID,
					SenderID:  requestorID,
				},
			})

			s.wsHub.BroadcastToUser(targetUserID, websocket.Event{
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

func (s *GroupChatService) UpdateMemberRole(ctx context.Context, requestorID uuid.UUID, groupID uuid.UUID, targetUserID uuid.UUID, req model.UpdateGroupMemberRoleRequest) error {
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
		Where(
			groupchat.ChatID(groupID),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		).
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
		return helper.NewForbiddenError("You are not a member of this group")
	}

	if requestorMember.Role != groupmember.RoleOwner {
		return helper.NewForbiddenError("Only owner can change member roles")
	}

	targetMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(targetUserID),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		return helper.NewNotFoundError("Target user is not a member of this group")
	}

	if targetMember.UserID == requestorID {
		return helper.NewBadRequestError("Cannot change your own role")
	}

	newRole := groupmember.Role(req.Role)
	if targetMember.Role == newRole {
		return nil
	}

	err = tx.GroupMember.UpdateOne(targetMember).SetRole(newRole).Exec(ctx)
	if err != nil {
		slog.Error("Failed to update member role", "error", err)
		return helper.NewInternalServerError("")
	}

	msgType := message.TypeSystemPromote
	if newRole == groupmember.RoleMember {
		msgType = message.TypeSystemDemote
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(requestorID).
		SetType(msgType).
		SetActionData(map[string]interface{}{
			"target_id": targetMember.UserID,
			"actor_id":  requestorID,
			"new_role":  newRole,
		}).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for role update", "error", err)
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
			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

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

func (s *GroupChatService) TransferOwnership(ctx context.Context, requestorID uuid.UUID, groupID uuid.UUID, req model.TransferGroupOwnershipRequest) error {
	if err := s.validator.Struct(req); err != nil {
		return helper.NewBadRequestError("")
	}

	if requestorID == req.NewOwnerID {
		return helper.NewBadRequestError("Cannot transfer ownership to yourself")
	}

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
		return helper.NewForbiddenError("You are not a member of this group")
	}

	if requestorMember.Role != groupmember.RoleOwner {
		return helper.NewForbiddenError("Only owner can transfer ownership")
	}

	targetMember, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(req.NewOwnerID),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		return helper.NewNotFoundError("Target user is not a member of this group")
	}

	err = tx.GroupMember.UpdateOne(requestorMember).SetRole(groupmember.RoleAdmin).Exec(ctx)
	if err != nil {
		slog.Error("Failed to demote old owner", "error", err)
		return helper.NewInternalServerError("")
	}

	err = tx.GroupMember.UpdateOne(targetMember).SetRole(groupmember.RoleOwner).Exec(ctx)
	if err != nil {
		slog.Error("Failed to promote new owner", "error", err)
		return helper.NewInternalServerError("")
	}

	systemMsg, err := tx.Message.Create().
		SetChatID(gc.ChatID).
		SetSenderID(requestorID).
		SetType(message.TypeSystemPromote).
		SetActionData(map[string]interface{}{
			"target_id": targetMember.UserID,
			"actor_id":  requestorID,
			"new_role":  "owner",
			"action":    "ownership_transferred",
		}).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create system message for ownership transfer", "error", err)
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
			fullMsg, _ := s.client.Message.Query().
				Where(message.ID(systemMsg.ID)).
				WithSender().
				Only(context.Background())

			msgResponse := helper.ToMessageResponse(fullMsg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)

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

func (s *GroupChatService) DeleteGroup(ctx context.Context, userID, groupID uuid.UUID) error {
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
		WithChat().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Group chat not found")
		}
		slog.Error("Failed to query group chat", "error", err)
		return helper.NewInternalServerError("")
	}

	member, err := tx.GroupMember.Query().
		Where(
			groupmember.GroupChatID(gc.ID),
			groupmember.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewForbiddenError("You are not a member of this group")
		}
		slog.Error("Failed to query requestor membership", "error", err)
		return helper.NewInternalServerError("")
	}

	if member.Role != groupmember.RoleOwner {
		return helper.NewForbiddenError("Only owner can delete the group")
	}

	err = tx.Chat.UpdateOneID(groupID).SetDeletedAt(time.Now().UTC()).Exec(ctx)
	if err != nil {
		slog.Error("Failed to soft delete chat", "error", err)
		return helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	members, _ := s.client.GroupMember.Query().
		Where(groupmember.GroupChatID(gc.ID)).
		All(context.Background())

	for _, m := range members {
		if s.wsHub != nil {
			s.wsHub.BroadcastToUser(m.UserID, websocket.Event{
				Type: websocket.EventChatDelete,
				Payload: map[string]interface{}{
					"chat_id": groupID,
				},
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					ChatID:    groupID,
					SenderID:  userID,
				},
			})
		}
	}

	return nil
}
