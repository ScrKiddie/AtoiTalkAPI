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
	"AtoiTalkAPI/internal/websocket"
	"context"
	"log/slog"
	"mime/multipart"
	"path/filepath"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-playground/validator/v10"
)

type UserService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
	wsHub          *websocket.Hub
}

func NewUserService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter, wsHub *websocket.Hub) *UserService {
	return &UserService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
		wsHub:          wsHub,
	}
}

func (s *UserService) GetCurrentUser(ctx context.Context, userID int) (*model.UserDTO, error) {
	u, err := s.client.User.Query().
		Where(user.ID(userID)).
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

	return &model.UserDTO{
		ID:          u.ID,
		Email:       u.Email,
		Username:    u.Username,
		FullName:    u.FullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: u.PasswordHash != nil,
	}, nil
}

func (s *UserService) GetUserProfile(ctx context.Context, currentUserID int, targetUserID int) (*model.UserDTO, error) {

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
		Where(user.ID(targetUserID)).
		Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldBio, user.FieldIsOnline, user.FieldLastSeenAt, user.FieldAvatarID).
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
	username := u.Username

	if isBlockedByMe || isBlockedByOther {
		isOnline = false
		lastSeenAt = nil

	} else {
		if u.LastSeenAt != nil {
			t := u.LastSeenAt.Format(time.RFC3339)
			lastSeenAt = &t
		}
	}

	return &model.UserDTO{
		ID:               u.ID,
		Username:         username,
		FullName:         u.FullName,
		Avatar:           avatarURL,
		Bio:              bio,
		HasPassword:      false,
		IsBlockedByMe:    isBlockedByMe,
		IsBlockedByOther: isBlockedByOther,
		IsOnline:         isOnline,
		LastSeenAt:       lastSeenAt,
	}, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID int, req model.UpdateProfileRequest) (*model.UserDTO, error) {
	if req.DeleteAvatar && req.Avatar != nil {
		return nil, helper.NewBadRequestError("")
	}

	if err := s.validator.Struct(&req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
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

	u, err := tx.User.Query().Where(user.ID(userID)).WithAvatar().Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}

		slog.Error("Failed to query user", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	update := tx.User.UpdateOneID(userID).SetFullName(req.FullName)
	if req.Bio != "" {
		update.SetBio(req.Bio)
	} else {
		update.ClearBio()
	}

	if req.Username != "" {
		normalizedUsername := helper.NormalizeUsername(req.Username)
		if normalizedUsername != u.Username {
			exists, err := tx.User.Query().Where(user.UsernameEQ(normalizedUsername)).Exist(ctx)
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
	var mediaID int

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

		fileName := helper.GenerateUniqueFileName(req.Avatar.Filename)
		filePath := filepath.Join(s.cfg.StorageProfile, fileName)
		contentType := req.Avatar.Header.Get("Content-Type")

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

			if mediaID != 0 {
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

	httpResp := &model.UserDTO{
		ID:          updatedUser.ID,
		Email:       updatedUser.Email,
		Username:    updatedUser.Username,
		FullName:    updatedUser.FullName,
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
		wsPayload := &model.UserUpdateEventPayload{
			ID:       updatedUser.ID,
			Username: updatedUser.Username,
			FullName: updatedUser.FullName,
			Avatar:   avatarURL,
			Bio:      bio,
		}
		if updatedUser.LastSeenAt != nil {
			t := updatedUser.LastSeenAt.Format(time.RFC3339)
			wsPayload.LastSeenAt = &t
		}

		go s.wsHub.BroadcastToContacts(userID, websocket.Event{
			Type:    websocket.EventUserUpdate,
			Payload: wsPayload,
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UnixMilli(),
				SenderID:  userID,
			},
		})
	}

	return httpResp, nil
}

func (s *UserService) SearchUsers(ctx context.Context, currentUserID int, req model.SearchUserRequest) ([]model.UserDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	query := s.client.User.Query().
		Where(
			user.IDNEQ(currentUserID),
			func(s *sql.Selector) {
				t := sql.Table(userblock.Table)
				s.Where(
					sql.Not(
						sql.Exists(
							sql.Select(userblock.FieldID).From(t).Where(
								sql.Or(
									sql.And(
										sql.EQ(t.C(userblock.FieldBlockerID), currentUserID),
										sql.ColumnsEQ(t.C(userblock.FieldBlockedID), s.C(user.FieldID)),
									),
									sql.And(
										sql.ColumnsEQ(t.C(userblock.FieldBlockerID), s.C(user.FieldID)),
										sql.EQ(t.C(userblock.FieldBlockedID), currentUserID),
									),
								),
							),
						),
					),
				)
			},
		)

	if req.Query != "" {
		query = query.Where(
			user.Or(
				user.FullNameContainsFold(req.Query),
				user.EmailEQ(req.Query),
				user.UsernameContainsFold(req.Query),
			),
		)
	}

	delimiter := "|||"

	if req.Cursor != "" {
		cursorName, cursorID, err := helper.DecodeCursor(req.Cursor, delimiter)
		if err != nil {
			return nil, "", false, helper.NewBadRequestError("")
		}

		query = query.Where(
			user.Or(
				user.FullNameGT(cursorName),
				user.And(
					user.FullNameEQ(cursorName),
					user.IDGT(cursorID),
				),
			),
		)
	}

	query = query.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldBio, user.FieldAvatarID).
		Order(ent.Asc(user.FieldFullName), ent.Asc(user.FieldID)).
		Limit(req.Limit + 1).
		WithAvatar()

	users, err := query.All(ctx)
	if err != nil {
		slog.Error("Failed to search users", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	hasNext := false
	var nextCursor string

	if len(users) > req.Limit {
		hasNext = true
		users = users[:req.Limit]
		lastUser := users[len(users)-1]

		nextCursor = helper.EncodeCursor(lastUser.FullName, lastUser.ID, delimiter)
	}

	privateChatMap := make(map[int]int)

	if req.IncludeChatID && len(users) > 0 {
		userIDs := make([]int, len(users))
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
				var targetID int
				if pc.User1ID == currentUserID {
					targetID = pc.User2ID
				} else {
					targetID = pc.User1ID
				}
				privateChatMap[targetID] = pc.ChatID
			}
		}
	}

	userDTOs := make([]model.UserDTO, len(users))
	for i, u := range users {
		avatarURL := ""
		if u.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
		}
		bio := ""
		if u.Bio != nil {
			bio = *u.Bio
		}

		dto := model.UserDTO{
			ID:          u.ID,
			Username:    u.Username,
			FullName:    u.FullName,
			Avatar:      avatarURL,
			Bio:         bio,
			HasPassword: false,
		}

		if chatID, exists := privateChatMap[u.ID]; exists {
			dto.PrivateChatID = &chatID
		}

		userDTOs[i] = dto
	}

	return userDTOs, nextCursor, hasNext, nil
}

func (s *UserService) GetBlockedUsers(ctx context.Context, currentUserID int, req model.GetBlockedUsersRequest) ([]model.UserDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	query := s.client.User.Query().
		Where(
			user.HasBlockedByRelWith(userblock.BlockerID(currentUserID)),
		)

	if req.Query != "" {
		query = query.Where(
			user.Or(
				user.FullNameContainsFold(req.Query),
				user.UsernameContainsFold(req.Query),
			),
		)
	}

	delimiter := "|||"

	if req.Cursor != "" {
		cursorName, cursorID, err := helper.DecodeCursor(req.Cursor, delimiter)
		if err != nil {
			return nil, "", false, helper.NewBadRequestError("")
		}

		query = query.Where(
			user.Or(
				user.FullNameGT(cursorName),
				user.And(
					user.FullNameEQ(cursorName),
					user.IDGT(cursorID),
				),
			),
		)
	}

	query = query.Select(user.FieldID, user.FieldUsername, user.FieldFullName, user.FieldBio, user.FieldAvatarID).
		Order(ent.Asc(user.FieldFullName), ent.Asc(user.FieldID)).
		Limit(req.Limit + 1).
		WithAvatar()

	users, err := query.All(ctx)
	if err != nil {
		slog.Error("Failed to get blocked users", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	hasNext := false
	var nextCursor string

	if len(users) > req.Limit {
		hasNext = true
		users = users[:req.Limit]
		lastUser := users[len(users)-1]
		nextCursor = helper.EncodeCursor(lastUser.FullName, lastUser.ID, delimiter)
	}

	userDTOs := make([]model.UserDTO, len(users))
	for i, u := range users {
		avatarURL := ""
		if u.Edges.Avatar != nil {
			avatarURL = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, u.Edges.Avatar.FileName)
		}
		bio := ""
		if u.Bio != nil {
			bio = *u.Bio
		}

		userDTOs[i] = model.UserDTO{
			ID:            u.ID,
			Username:      u.Username,
			FullName:      u.FullName,
			Avatar:        avatarURL,
			Bio:           bio,
			IsBlockedByMe: true,
		}
	}

	return userDTOs, nextCursor, hasNext, nil
}

func (s *UserService) BlockUser(ctx context.Context, blockerID int, blockedID int) error {
	if blockerID == blockedID {
		return helper.NewBadRequestError("")
	}

	exists, err := s.client.User.Query().Where(user.ID(blockedID)).Exist(ctx)
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
		go s.wsHub.BroadcastToUser(blockedID, websocket.Event{
			Type: websocket.EventUserBlock,
			Payload: map[string]int{
				"blocker_id": blockerID,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UnixMilli(),
				SenderID:  blockerID,
			},
		})
	}

	return nil
}

func (s *UserService) UnblockUser(ctx context.Context, blockerID int, blockedID int) error {
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
		go s.wsHub.BroadcastToUser(blockedID, websocket.Event{
			Type: websocket.EventUserUnblock,
			Payload: map[string]int{
				"blocker_id": blockerID,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UnixMilli(),
				SenderID:  blockerID,
			},
		})
	}

	return nil
}
