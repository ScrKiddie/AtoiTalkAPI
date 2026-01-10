package service

import (
	"AtoiTalkAPI/ent"
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
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type UserService struct {
	client         *ent.Client
	repo           *repository.Repository
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
	wsHub          *websocket.Hub
}

func NewUserService(client *ent.Client, repo *repository.Repository, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter, wsHub *websocket.Hub) *UserService {
	return &UserService{
		client:         client,
		repo:           repo,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
		wsHub:          wsHub,
	}
}

func (s *UserService) GetCurrentUser(ctx context.Context, userID uuid.UUID) (*model.UserDTO, error) {
	u, err := s.client.User.Query().
		Where(
			user.ID(userID),
			user.DeletedAtIsNil(),
		).
		WithAvatar().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}
		slog.Error("Failed to query user", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	avatarURL := ""
	if u.Edges.Avatar != nil {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
	}

	bio := ""
	if u.Bio != nil {
		bio = *u.Bio
	}

	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	username := ""
	if u.Username != nil {
		username = *u.Username
	}

	fullName := ""
	if u.FullName != nil {
		fullName = *u.FullName
	}

	return &model.UserDTO{
		ID:          u.ID,
		Email:       email,
		Username:    username,
		FullName:    fullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: u.PasswordHash != nil,
	}, nil
}

func (s *UserService) GetUserProfile(ctx context.Context, currentUserID uuid.UUID, targetUserID uuid.UUID) (*model.UserDTO, error) {
	blocks, err := s.client.UserBlock.Query().
		Where(
			userblock.Or(
				userblock.And(
					userblock.BlockerID(currentUserID),
					userblock.BlockedID(targetUserID),
				),
				userblock.And(
					userblock.BlockerID(targetUserID),
					userblock.BlockedID(currentUserID),
				),
			),
		).
		All(ctx)

	if err != nil {
		slog.Error("Failed to check block status", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	isBlockedByMe := false
	isBlockedByOther := false

	for _, b := range blocks {
		if b.BlockerID == currentUserID {
			isBlockedByMe = true
		}
		if b.BlockerID == targetUserID {
			isBlockedByOther = true
		}
	}

	u, err := s.client.User.Query().
		Where(
			user.ID(targetUserID),
			user.DeletedAtIsNil(),
		).
		Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldBio, user.FieldIsOnline, user.FieldLastSeenAt, user.FieldAvatarID, user.FieldIsBanned, user.FieldBannedUntil).
		WithAvatar().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("User not found")
		}
		slog.Error("Failed to query user profile", "error", err, "targetUserID", targetUserID)
		return nil, helper.NewInternalServerError("")
	}

	avatarURL := ""
	if u.Edges.Avatar != nil {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
	}

	bio := ""
	if u.Bio != nil {
		bio = *u.Bio
	}

	var lastSeenAt *string
	isOnline := u.IsOnline
	username := ""
	if u.Username != nil {
		username = *u.Username
	}

	isBanned := u.IsBanned
	if isBanned && u.BannedUntil != nil && time.Now().After(*u.BannedUntil) {
		isBanned = false
	}

	if isBlockedByMe || isBlockedByOther || isBanned {
		isOnline = false
		lastSeenAt = nil
	} else {
		if u.LastSeenAt != nil {
			t := u.LastSeenAt.Format(time.RFC3339)
			lastSeenAt = &t
		}
	}

	fullName := ""
	if u.FullName != nil {
		fullName = *u.FullName
	}

	return &model.UserDTO{
		ID:               u.ID,
		Username:         username,
		FullName:         fullName,
		Avatar:           avatarURL,
		Bio:              bio,
		HasPassword:      false,
		IsBlockedByMe:    isBlockedByMe,
		IsBlockedByOther: isBlockedByOther,
		IsOnline:         isOnline,
		LastSeenAt:       lastSeenAt,
	}, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.UserDTO, error) {
	if req.DeleteAvatar && req.Avatar != nil {
		return nil, helper.NewBadRequestError("")
	}

	if err := s.validator.Struct(&req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	req.FullName = strings.TrimSpace(req.FullName)
	req.Bio = strings.TrimSpace(req.Bio)

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

	u, err := tx.User.Query().
		Where(
			user.ID(userID),
			user.DeletedAtIsNil(),
		).
		WithAvatar().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}

		slog.Error("Failed to query user", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	update := tx.User.UpdateOneID(userID).SetNillableFullName(&req.FullName)
	if req.Bio != "" {
		update.SetBio(req.Bio)
	} else {
		update.ClearBio()
	}

	if req.Username != "" {
		normalizedUsername := helper.NormalizeUsername(req.Username)
		currentUsername := ""
		if u.Username != nil {
			currentUsername = *u.Username
		}

		if normalizedUsername != currentUsername {
			exists, err := tx.User.Query().
				Where(
					user.UsernameEQ(normalizedUsername),
					user.DeletedAtIsNil(),
				).
				Exist(ctx)
			if err != nil {
				slog.Error("Failed to check username existence", "error", err)
				return nil, helper.NewInternalServerError("")
			}
			if exists {
				return nil, helper.NewConflictError("Username already taken")
			}
			update.SetUsername(normalizedUsername)
		}
	}

	var newAvatarFileName string
	var isAvatarUpdated bool

	var fileToUpload multipart.File
	var fileUploadPath string
	var fileContentType string
	var mediaID uuid.UUID

	if req.DeleteAvatar {
		update.ClearAvatar().ClearAvatarID()
		isAvatarUpdated = true
	} else if req.Avatar != nil {
		file, err := req.Avatar.Open()
		if err != nil {
			slog.Error("Failed to open avatar file", "error", err, "userID", userID)
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
			SetFileName(fileName).SetOriginalName(req.Avatar.Filename).
			SetFileSize(req.Avatar.Size).SetMimeType(contentType).
			SetStatus(media.StatusActive).
			SetUploaderID(userID).
			Save(ctx)

		if err != nil {
			slog.Error("Failed to create media record", "error", err, "userID", userID)
			return nil, helper.NewInternalServerError("")
		}
		mediaID = media.ID

		update.SetAvatar(media)
		newAvatarFileName = fileName
		isAvatarUpdated = true
	}

	updatedUser, err := update.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, helper.NewConflictError("Username already taken")
		}
		slog.Error("Failed to save user profile update", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if fileToUpload != nil {
		if err := s.storageAdapter.StoreFromReader(fileToUpload, fileContentType, fileUploadPath); err != nil {
			slog.Error("Failed to store avatar to storage after db commit", "error", err, "userID", userID)

			if mediaID != uuid.Nil {
				cleanupCtx := context.Background()

				_, unlinkErr := s.client.User.UpdateOneID(userID).ClearAvatar().Save(cleanupCtx)
				if unlinkErr != nil {
					slog.Error("Failed to unlink avatar after file upload failure", "error", unlinkErr, "userID", userID)
				} else {

					if delErr := s.client.Media.DeleteOneID(mediaID).Exec(cleanupCtx); delErr != nil {
						slog.Error("Failed to delete orphan media record after file upload failure", "error", delErr, "mediaID", mediaID)
					}
				}
			}
		}
	}

	var avatarFileName string
	if isAvatarUpdated {
		avatarFileName = newAvatarFileName
	} else if u.Edges.Avatar != nil {
		avatarFileName = u.Edges.Avatar.FileName
	}

	avatarURL := ""
	if avatarFileName != "" {
		avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, avatarFileName)
	}

	bio := ""
	if updatedUser.Bio != nil {
		bio = *updatedUser.Bio
	}

	email := ""
	if updatedUser.Email != nil {
		email = *updatedUser.Email
	}
	username := ""
	if updatedUser.Username != nil {
		username = *updatedUser.Username
	}

	fullName := ""
	if updatedUser.FullName != nil {
		fullName = *updatedUser.FullName
	}

	httpResp := &model.UserDTO{
		ID:          updatedUser.ID,
		Email:       email,
		Username:    username,
		FullName:    fullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: updatedUser.PasswordHash != nil,
		IsOnline:    updatedUser.IsOnline,
	}
	if updatedUser.LastSeenAt != nil {
		t := updatedUser.LastSeenAt.Format(time.RFC3339)
		httpResp.LastSeenAt = &t
	}

	if s.wsHub != nil {
		go func() {
			wsPayload := &model.UserUpdateEventPayload{
				ID:       updatedUser.ID,
				Username: username,
				FullName: fullName,
				Avatar:   avatarURL,
				Bio:      bio,
			}
			if updatedUser.LastSeenAt != nil {
				t := updatedUser.LastSeenAt.Format(time.RFC3339)
				wsPayload.LastSeenAt = &t
			}

			event := websocket.Event{
				Type:    websocket.EventUserUpdate,
				Payload: wsPayload,
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					SenderID:  userID,
				},
			}

			s.wsHub.BroadcastToUser(userID, event)

			s.wsHub.BroadcastToContacts(userID, event)
		}()
	}

	return httpResp, nil
}

func (s *UserService) SearchUsers(ctx context.Context, currentUserID uuid.UUID, req model.SearchUserRequest) ([]model.UserDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	users, nextCursor, hasNext, err := s.repo.User.SearchUsers(ctx, currentUserID, req.Query, req.Cursor, req.Limit)
	if err != nil {
		if strings.Contains(err.Error(), "invalid cursor format") {
			slog.Warn("Invalid cursor format in SearchUsers", "error", err)
			return nil, "", false, helper.NewBadRequestError("")
		}
		slog.Error("Failed to search users", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	privateChatMap := make(map[uuid.UUID]uuid.UUID)

	if req.IncludeChatID && len(users) > 0 {
		userIDs := make([]uuid.UUID, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
		}

		chats, err := s.client.PrivateChat.Query().
			Where(
				privatechat.Or(
					privatechat.And(
						privatechat.User1ID(currentUserID),
						privatechat.User2IDIn(userIDs...),
					),
					privatechat.And(
						privatechat.User1IDIn(userIDs...),
						privatechat.User2ID(currentUserID),
					),
				),
			).
			All(ctx)

		if err != nil {
			slog.Error("Failed to fetch private chats for search results", "error", err)
		} else {
			for _, pc := range chats {
				var targetID uuid.UUID
				if pc.User1ID == currentUserID {
					targetID = pc.User2ID
				} else {
					targetID = pc.User1ID
				}
				privateChatMap[targetID] = pc.ChatID
			}
		}
	}

	userDTOs := make([]model.UserDTO, 0)
	for _, u := range users {

		if u.DeletedAt != nil {
			continue
		}

		avatarURL := ""
		if u.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
		}
		bio := ""
		if u.Bio != nil {
			bio = *u.Bio
		}
		username := ""
		if u.Username != nil {
			username = *u.Username
		}

		fullName := ""
		if u.FullName != nil {
			fullName = *u.FullName
		}

		dto := model.UserDTO{
			ID:          u.ID,
			Username:    username,
			FullName:    fullName,
			Avatar:      avatarURL,
			Bio:         bio,
			HasPassword: false,
		}

		if chatID, exists := privateChatMap[u.ID]; exists {
			dto.PrivateChatID = &chatID
		}

		userDTOs = append(userDTOs, dto)
	}

	return userDTOs, nextCursor, hasNext, nil
}

func (s *UserService) GetBlockedUsers(ctx context.Context, currentUserID uuid.UUID, req model.GetBlockedUsersRequest) ([]model.UserDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	users, nextCursor, hasNext, err := s.repo.User.GetBlockedUsers(ctx, currentUserID, req.Query, req.Cursor, req.Limit)
	if err != nil {
		if strings.Contains(err.Error(), "invalid cursor format") {
			slog.Warn("Invalid cursor format in GetBlockedUsers", "error", err)
			return nil, "", false, helper.NewBadRequestError("")
		}
		slog.Error("Failed to get blocked users", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	userDTOs := make([]model.UserDTO, 0)
	for _, u := range users {

		if u.DeletedAt != nil {
			continue
		}

		avatarURL := ""
		if u.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
		}
		bio := ""
		if u.Bio != nil {
			bio = *u.Bio
		}
		username := ""
		if u.Username != nil {
			username = *u.Username
		}

		fullName := ""
		if u.FullName != nil {
			fullName = *u.FullName
		}

		userDTOs = append(userDTOs, model.UserDTO{
			ID:            u.ID,
			Username:      username,
			FullName:      fullName,
			Avatar:        avatarURL,
			Bio:           bio,
			IsBlockedByMe: true,
		})
	}

	return userDTOs, nextCursor, hasNext, nil
}

func (s *UserService) BlockUser(ctx context.Context, blockerID uuid.UUID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return helper.NewBadRequestError("Cannot block yourself")
	}

	exists, err := s.client.User.Query().
		Where(
			user.ID(blockedID),
			user.DeletedAtIsNil(),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check user existence", "error", err)
		return helper.NewInternalServerError("")
	}
	if !exists {
		return helper.NewNotFoundError("")
	}

	_, err = s.client.UserBlock.Create().
		SetBlockerID(blockerID).
		SetBlockedID(blockedID).
		Save(ctx)

	if err != nil {
		if ent.IsConstraintError(err) {
			return nil
		}
		slog.Error("Failed to block user", "error", err)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		go func() {
			event := websocket.Event{
				Type: websocket.EventUserBlock,
				Payload: map[string]uuid.UUID{
					"blocker_id": blockerID,
					"blocked_id": blockedID,
				},
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					SenderID:  blockerID,
				},
			}

			s.wsHub.BroadcastToUser(blockedID, event)

			s.wsHub.BroadcastToUser(blockerID, event)
		}()
	}

	return nil
}

func (s *UserService) UnblockUser(ctx context.Context, blockerID uuid.UUID, blockedID uuid.UUID) error {
	_, err := s.client.UserBlock.Delete().
		Where(
			userblock.BlockerID(blockerID),
			userblock.BlockedID(blockedID),
		).
		Exec(ctx)

	if err != nil {
		slog.Error("Failed to unblock user", "error", err)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		go func() {
			event := websocket.Event{
				Type: websocket.EventUserUnblock,
				Payload: map[string]uuid.UUID{
					"blocker_id": blockerID,
					"blocked_id": blockedID,
				},
				Meta: &websocket.EventMeta{
					Timestamp: time.Now().UTC().UnixMilli(),
					SenderID:  blockerID,
				},
			}

			s.wsHub.BroadcastToUser(blockedID, event)

			s.wsHub.BroadcastToUser(blockerID, event)
		}()
	}

	return nil
}
