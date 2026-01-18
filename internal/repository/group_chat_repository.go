package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/helper"
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

type GroupChatRepository struct {
	client *ent.Client
}

func NewGroupChatRepository(client *ent.Client) *GroupChatRepository {
	return &GroupChatRepository{
		client: client,
	}
}

func (r *GroupChatRepository) SearchPublicGroups(ctx context.Context, queryStr string, cursor string, limit int) ([]*ent.GroupChat, string, bool, error) {
	queryStr = strings.TrimSpace(queryStr)

	query := r.client.GroupChat.Query().
		Where(
			groupchat.IsPublic(true),
			groupchat.HasChatWith(chat.DeletedAtIsNil()),
		)

	if queryStr != "" {
		lowerQuery := strings.ToLower(queryStr)
		query = query.Where(
			groupchat.Or(
				func(s *sql.Selector) {

					s.Where(sql.Contains(sql.Lower(s.C(groupchat.FieldName)), lowerQuery))
				},
				func(s *sql.Selector) {
					s.Where(sql.Contains(sql.Lower(s.C(groupchat.FieldDescription)), lowerQuery))
				},
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
			groupchat.Or(
				groupchat.NameGT(cursorName),
				groupchat.And(
					groupchat.NameEQ(cursorName),
					groupchat.IDGT(cursorID),
				),
			),
		)
	}

	query = query.
		Select(groupchat.FieldID, groupchat.FieldChatID, groupchat.FieldName, groupchat.FieldDescription, groupchat.FieldAvatarID).
		Order(ent.Asc(groupchat.FieldName), ent.Asc(groupchat.FieldID)).
		Limit(limit + 1).
		WithAvatar().
		WithMembers(func(mq *ent.GroupMemberQuery) {
			mq.Select(groupmember.FieldID, groupmember.FieldUserID)
			mq.Where(groupmember.HasUserWith(user.DeletedAtIsNil()))
		})

	groups, err := query.All(ctx)
	if err != nil {
		return nil, "", false, err
	}

	hasNext := false
	var nextCursor string

	if len(groups) > limit {
		hasNext = true
		groups = groups[:limit]
		lastGroup := groups[len(groups)-1]

		nextCursor = helper.EncodeCursor(lastGroup.Name, lastGroup.ID.String(), delimiter)
	}

	return groups, nextCursor, hasNext, nil
}
