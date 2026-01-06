package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/media"
	"AtoiTalkAPI/ent/privatechat"
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

type ChatService struct {
	client         *ent.Client
	repo           *repository.Repository
	cfg            *config.AppConfig
	validator      *validator.Validate
	wsHub          *websocket.Hub
	storageAdapter *adapter.StorageAdapter
}

func NewChatService(client *ent.Client, repo *repository.Repository, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub, storageAdapter *adapter.StorageAdapter) *ChatService {
	return &ChatService{
		client:         client,
		repo:           repo,
		cfg:            cfg,
		validator:      validator,
		wsHub:          wsHub,
		storageAdapter: storageAdapter,
	}
}

func (s *ChatService) CreateGroupChat(ctx context.Context, creatorID int, req model.CreateGroupChatRequest) (*model.ChatResponse, error) {

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

		fileName := helper.GenerateUniqueFileName(req.Avatar.Filename)
		filePath := filepath.Join(s.cfg.StorageProfile, fileName)
		contentType := req.Avatar.Header.Get("Content-Type")

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
		SetRole(groupmember.RoleAdmin))

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

			payload := model.ChatListResponse{
				ID:          newChat.ID,
				Type:        string(newChat.Type),
				Name:        newGroupChat.Name,
				Avatar:      avatarURL,
				LastMessage: nil,
				UnreadCount: 0,
			}

			event := websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payload,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UnixMilli(),
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

func (s *ChatService) CreatePrivateChat(ctx context.Context, userID int, req model.CreatePrivateChatRequest) (*model.ChatResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	if userID == req.TargetUserID {
		return nil, helper.NewBadRequestError("")
	}

	users, err := s.client.User.Query().Where(user.IDIn(userID, req.TargetUserID)).WithAvatar().All(ctx)
	if err != nil {
		slog.Error("Failed to query users for private chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if len(users) != 2 {
		return nil, helper.NewNotFoundError("")
	}

	var creator, targetUser *ent.User
	for _, u := range users {
		if u.ID == userID {
			creator = u
		} else {
			targetUser = u
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
		return nil, helper.NewForbiddenError("")
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

	if s.wsHub != nil {
		go func() {

			creatorAvatarURL := ""
			if creator.Edges.Avatar != nil {
				creatorAvatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, creator.Edges.Avatar.FileName)
			}
			payloadForTarget := model.ChatListResponse{
				ID:          newChat.ID,
				Type:        string(newChat.Type),
				Name:        creator.FullName,
				Avatar:      creatorAvatarURL,
				LastMessage: nil,
				UnreadCount: 0,
				IsOnline:    creator.IsOnline,
				OtherUserID: &creator.ID,
			}
			s.wsHub.BroadcastToUser(targetUser.ID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payloadForTarget,
				Meta:    &websocket.EventMeta{Timestamp: time.Now().UnixMilli(), ChatID: newChat.ID, SenderID: userID},
			})

			targetAvatarURL := ""
			if targetUser.Edges.Avatar != nil {
				targetAvatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, targetUser.Edges.Avatar.FileName)
			}
			payloadForCreator := model.ChatListResponse{
				ID:          newChat.ID,
				Type:        string(newChat.Type),
				Name:        targetUser.FullName,
				Avatar:      targetAvatarURL,
				LastMessage: nil,
				UnreadCount: 0,
				IsOnline:    targetUser.IsOnline,
				OtherUserID: &targetUser.ID,
			}
			s.wsHub.BroadcastToUser(creator.ID, websocket.Event{
				Type:    websocket.EventChatNew,
				Payload: payloadForCreator,
				Meta:    &websocket.EventMeta{Timestamp: time.Now().UnixMilli(), ChatID: newChat.ID, SenderID: userID},
			})
		}()
	}

	return &model.ChatResponse{
		ID:        newChat.ID,
		Type:      string(newChat.Type),
		CreatedAt: newChat.CreatedAt.Format(time.RFC3339),
	}, nil
}

type blockStatus struct {
	blockedByMe    bool
	blockedByOther bool
}

func (s *ChatService) GetChatByID(ctx context.Context, userID, chatID int) (*model.ChatListResponse, error) {
	c, err := s.repo.Chat.GetChatByID(ctx, userID, chatID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}
		slog.Error("Failed to get chat by ID", "error", err, "chatID", chatID)
		return nil, helper.NewInternalServerError("")
	}

	blockedMap := make(map[int]blockStatus)
	var otherUserID int

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
					status.blockedByMe = true
				} else {
					status.blockedByOther = true
				}
				blockedMap[otherUserID] = status
			}
		}
	}

	return s.mapChatToResponse(ctx, userID, c, blockedMap)
}

func (s *ChatService) GetChats(ctx context.Context, userID int, req model.GetChatsRequest) ([]model.ChatListResponse, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	chats, nextCursor, hasNext, err := s.repo.Chat.GetChats(ctx, userID, req.Query, req.Cursor, req.Limit)
	if err != nil {
		slog.Error("Failed to get chats", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	otherUserIDs := make([]int, 0)
	for _, c := range chats {
		if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
			if c.Edges.PrivateChat.User1ID == userID {
				otherUserIDs = append(otherUserIDs, c.Edges.PrivateChat.User2ID)
			} else {
				otherUserIDs = append(otherUserIDs, c.Edges.PrivateChat.User1ID)
			}
		}
	}

	blockedMap := make(map[int]blockStatus)
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
				status := blockedMap[0]
				if b.BlockerID == userID {
					status = blockedMap[b.BlockedID]
					status.blockedByMe = true
					blockedMap[b.BlockedID] = status
				} else {
					status = blockedMap[b.BlockerID]
					status.blockedByOther = true
					blockedMap[b.BlockerID] = status
				}
			}
		}
	}

	response := make([]model.ChatListResponse, 0)
	for _, c := range chats {
		resp, err := s.mapChatToResponse(ctx, userID, c, blockedMap)
		if err == nil {
			response = append(response, *resp)
		}
	}

	return response, nextCursor, hasNext, nil
}

func (s *ChatService) mapChatToResponse(ctx context.Context, userID int, c *ent.Chat, blockedMap map[int]blockStatus) (*model.ChatListResponse, error) {
	var name, avatar string
	var lastReadAt *string
	var otherLastReadAt *string
	var unreadCount int
	var isOnline bool
	var otherUserID *int
	var isBlockedByMe bool

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		var otherUser *ent.User
		var myLastRead *time.Time
		var otherUserLastRead *time.Time

		if pc.User1ID == userID {
			otherUser = pc.Edges.User2
			myLastRead = pc.User1LastReadAt
			otherUserLastRead = pc.User2LastReadAt
			unreadCount = pc.User1UnreadCount
		} else {
			otherUser = pc.Edges.User1
			myLastRead = pc.User2LastReadAt
			otherUserLastRead = pc.User1LastReadAt
			unreadCount = pc.User2UnreadCount
		}

		if otherUser != nil {
			otherUserID = &otherUser.ID
			name = otherUser.FullName

			status := blockedMap[otherUser.ID]
			isBlockedByMe = status.blockedByMe

			if status.blockedByMe || status.blockedByOther {
				isOnline = false

				if otherUser.Edges.Avatar != nil {
					avatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, otherUser.Edges.Avatar.FileName)
				}
			} else {
				isOnline = otherUser.IsOnline
				if otherUser.Edges.Avatar != nil {
					avatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, otherUser.Edges.Avatar.FileName)
				}
			}
		}
		if myLastRead != nil {
			t := myLastRead.Format(time.RFC3339)
			lastReadAt = &t
		}
		if otherUserLastRead != nil {
			t := otherUserLastRead.Format(time.RFC3339)
			otherLastReadAt = &t
		}
	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		gc := c.Edges.GroupChat
		name = gc.Name
		if gc.Edges.Avatar != nil {
			avatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, gc.Edges.Avatar.FileName)
		}
		if len(gc.Edges.Members) > 0 {
			unreadCount = gc.Edges.Members[0].UnreadCount
			if gc.Edges.Members[0].LastReadAt != nil {
				t := gc.Edges.Members[0].LastReadAt.Format(time.RFC3339)
				lastReadAt = &t
			}
		}
	}

	var lastMsgResp *model.MessageResponse
	if c.Edges.LastMessage != nil {
		lastMsgResp = helper.ToMessageResponse(c.Edges.LastMessage, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)
	}

	return &model.ChatListResponse{
		ID:              c.ID,
		Type:            string(c.Type),
		Name:            name,
		Avatar:          avatar,
		LastMessage:     lastMsgResp,
		UnreadCount:     unreadCount,
		LastReadAt:      lastReadAt,
		OtherLastReadAt: otherLastReadAt,
		IsOnline:        isOnline,
		OtherUserID:     otherUserID,
		IsBlockedByMe:   isBlockedByMe,
	}, nil
}

func (s *ChatService) MarkAsRead(ctx context.Context, userID int, chatID int) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	c, err := tx.Chat.Query().
		Where(chat.ID(chatID)).
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
	var otherUserID int

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		update := tx.PrivateChat.UpdateOneID(pc.ID)

		if pc.User1ID == userID {
			otherUserID = pc.User2ID
			update.SetUser1UnreadCount(0)
		} else if pc.User2ID == userID {
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
				update.SetUser1LastReadAt(time.Now())
			} else {
				update.SetUser2LastReadAt(time.Now())
			}
		}

		if err := update.Exec(ctx); err != nil {
			slog.Error("Failed to mark private chat as read", "error", err)
			return helper.NewInternalServerError("")
		}

	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {

		err := tx.GroupMember.Update().
			Where(
				groupmember.GroupChatID(c.Edges.GroupChat.ID),
				groupmember.UserID(userID),
			).
			SetUnreadCount(0).
			SetLastReadAt(time.Now()).
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
				Timestamp: time.Now().UnixMilli(),
				ChatID:    chatID,
				SenderID:  userID,
			},
		})
	}

	return nil
}

func (s *ChatService) HideChat(ctx context.Context, userID int, chatID int) error {
	c, err := s.client.Chat.Query().
		Where(chat.ID(chatID)).
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
		update.SetUser1HiddenAt(time.Now()).SetUser1UnreadCount(0)
	} else if pc.User2ID == userID {
		update.SetUser2HiddenAt(time.Now()).SetUser2UnreadCount(0)
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
				Timestamp: time.Now().UnixMilli(),
				ChatID:    chatID,
				SenderID:  userID,
			},
		})
	}

	return nil
}
