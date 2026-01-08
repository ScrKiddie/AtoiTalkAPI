package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/helper"
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

type UserRepository struct {
	client *ent.Client
}

func NewUserRepository(client *ent.Client) *UserRepository {
	return &UserRepository{
		client: client,
	}
}

func (r *UserRepository) SearchUsers(ctx context.Context, currentUserID uuid.UUID, queryStr string, cursor string, limit int) ([]*ent.User, string, bool, error) {
	query := r.client.User.Query().
		Where(
			user.IDNEQ(currentUserID),
			user.DeletedAtIsNil(),
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

	if queryStr != "" {
		query = query.Where(
			user.Or(
				user.FullNameEqualFold(queryStr),
				user.EmailEqualFold(queryStr),
				user.UsernameEqualFold(queryStr),
			),
		)
	}

	delimiter := "|||"

	if cursor != "" {
		cursorName, cursorIDStr, err := helper.DecodeCursor(cursor, delimiter)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor format: %w", err)
		}

		cursorID, err := uuid.Parse(cursorIDStr)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor id format: %w", err)
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
		Limit(limit + 1).
		WithAvatar()

	users, err := query.All(ctx)
	if err != nil {
		return nil, "", false, err
	}

	hasNext := false
	var nextCursor string

	if len(users) > limit {
		hasNext = true
		users = users[:limit]
		lastUser := users[len(users)-1]
		
		fullName := ""
		if lastUser.FullName != nil {
			fullName = *lastUser.FullName
		}
		nextCursor = helper.EncodeCursor(fullName, lastUser.ID.String(), delimiter)
	}

	return users, nextCursor, hasNext, nil
}

func (r *UserRepository) GetBlockedUsers(ctx context.Context, currentUserID uuid.UUID, queryStr string, cursor string, limit int) ([]*ent.User, string, bool, error) {
	query := r.client.User.Query().
		Where(
			user.HasBlockedByRelWith(userblock.BlockerID(currentUserID)),
			user.DeletedAtIsNil(),
		)

	if queryStr != "" {
		query = query.Where(
			user.Or(
				user.FullNameContainsFold(queryStr),
				user.UsernameContainsFold(queryStr),
			),
		)
	}

	delimiter := "|||"

	if cursor != "" {
		cursorName, cursorIDStr, err := helper.DecodeCursor(cursor, delimiter)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor format: %w", err)
		}

		cursorID, err := uuid.Parse(cursorIDStr)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor id format: %w", err)
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
		Limit(limit + 1).
		WithAvatar()

	users, err := query.All(ctx)
	if err != nil {
		return nil, "", false, err
	}

	hasNext := false
	var nextCursor string

	if len(users) > limit {
		hasNext = true
		users = users[:limit]
		lastUser := users[len(users)-1]
		
		fullName := ""
		if lastUser.FullName != nil {
			fullName = *lastUser.FullName
		}
		nextCursor = helper.EncodeCursor(fullName, lastUser.ID.String(), delimiter)
	}

	return users, nextCursor, hasNext, nil
}
