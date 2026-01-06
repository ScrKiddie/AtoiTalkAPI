package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/helper"
	"context"
	"fmt"
	"time"
)

type GroupMemberRepository struct {
	client *ent.Client
}

func NewGroupMemberRepository(client *ent.Client) *GroupMemberRepository {
	return &GroupMemberRepository{
		client: client,
	}
}

func (r *GroupMemberRepository) SearchGroupMembers(ctx context.Context, groupID int, query, cursor string, limit int) ([]*ent.GroupMember, string, bool, error) {
	queryBuilder := r.client.GroupMember.Query().
		Where(groupmember.GroupChatID(groupID)).
		Order(ent.Asc(groupmember.FieldJoinedAt), ent.Asc(groupmember.FieldID)).
		Limit(limit + 1).
		WithUser(func(uq *ent.UserQuery) {
			uq.WithAvatar()
		})

	if query != "" {
		queryBuilder = queryBuilder.Where(groupmember.HasUserWith(
			user.Or(
				user.UsernameContainsFold(query),
				user.FullNameContainsFold(query),
			),
		))
	}

	if cursor != "" {
		joinedAtStr, id, err := helper.DecodeCursor(cursor, "|")
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor format: %w", err)
		}

		joinedAt, err := time.Parse(time.RFC3339Nano, joinedAtStr)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor time format: %w", err)
		}

		queryBuilder = queryBuilder.Where(
			groupmember.Or(
				groupmember.JoinedAtGT(joinedAt),
				groupmember.And(
					groupmember.JoinedAtEQ(joinedAt),
					groupmember.IDGT(id),
				),
			),
		)
	}

	members, err := queryBuilder.All(ctx)
	if err != nil {
		return nil, "", false, err
	}

	hasNext := false
	if len(members) > limit {
		hasNext = true
		members = members[:limit]
	}

	var nextCursor string
	if hasNext && len(members) > 0 {
		lastMember := members[len(members)-1]
		nextCursor = helper.EncodeCursor(lastMember.JoinedAt.Format(time.RFC3339Nano), lastMember.ID, "|")
	}

	return members, nextCursor, hasNext, nil
}
