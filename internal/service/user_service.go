package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"log/slog"
	"path/filepath"

	"github.com/go-playground/validator/v10"
)

type UserService struct {
	client         *ent.Client
	cfg            *config.AppConfig
	validator      *validator.Validate
	storageAdapter *adapter.StorageAdapter
}

func NewUserService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, storageAdapter *adapter.StorageAdapter) *UserService {
	return &UserService{
		client:         client,
		cfg:            cfg,
		validator:      validator,
		storageAdapter: storageAdapter,
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
		FullName:    u.FullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: u.PasswordHash != nil,
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

	u, err := s.client.User.Query().Where(user.ID(userID)).WithAvatar().Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, helper.NewNotFoundError("")
		}

		slog.Error("Failed to query user", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
	}

	update := s.client.User.UpdateOneID(userID).SetFullName(req.FullName)
	if req.Bio != "" {
		update.SetBio(req.Bio)
	} else {
		update.ClearBio()
	}

	var newAvatarFileName string
	var isAvatarUpdated bool

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

		fileName := helper.GenerateUniqueFileName(req.Avatar.Filename)
		filePath := filepath.Join(s.cfg.StorageProfile, fileName)
		contentType := req.Avatar.Header.Get("Content-Type")

		if err := s.storageAdapter.StoreFromReader(file, contentType, filePath); err != nil {

			slog.Error("Failed to store avatar to storage", "error", err, "userID", userID)
			return nil, helper.NewInternalServerError("")
		}

		media, err := s.client.Media.Create().
			SetFileName(fileName).SetOriginalName(req.Avatar.Filename).
			SetFileSize(req.Avatar.Size).SetMimeType(contentType).
			Save(ctx)

		if err != nil {

			slog.Error("Failed to create media record", "error", err, "userID", userID)
			return nil, helper.NewInternalServerError("")
		}

		update.SetAvatar(media)
		newAvatarFileName = fileName
		isAvatarUpdated = true
	}

	updatedUser, err := update.Save(ctx)
	if err != nil {

		slog.Error("Failed to save user profile update", "error", err, "userID", userID)
		return nil, helper.NewInternalServerError("")
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

	return &model.UserDTO{
		ID:          updatedUser.ID,
		Email:       updatedUser.Email,
		FullName:    updatedUser.FullName,
		Avatar:      avatarURL,
		Bio:         bio,
		HasPassword: updatedUser.PasswordHash != nil,
	}, nil
}

func (s *UserService) SearchUsers(ctx context.Context, req model.SearchUserRequest) ([]model.UserDTO, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	query := s.client.User.Query()

	if req.Query != "" {
		query = query.Where(
			user.Or(
				user.FullNameContainsFold(req.Query),
				user.EmailContainsFold(req.Query),
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

	query = query.Order(ent.Asc(user.FieldFullName), ent.Asc(user.FieldID)).
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
			ID:          u.ID,
			Email:       u.Email,
			FullName:    u.FullName,
			Avatar:      avatarURL,
			Bio:         bio,
			HasPassword: u.PasswordHash != nil,
		}
	}

	return userDTOs, nextCursor, hasNext, nil
}
