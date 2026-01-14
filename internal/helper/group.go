package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
	"time"

	"github.com/google/uuid"
)

func ToGroupMemberDTO(m *ent.GroupMember, onlineMap map[uuid.UUID]bool, urlGen URLGenerator) model.GroupMemberDTO {
	if m == nil || m.Edges.User == nil {
		return model.GroupMemberDTO{}
	}

	user := m.Edges.User
	avatarURL := ""
	if user.Edges.Avatar != nil {
		avatarURL = urlGen.GetPublicURL(user.Edges.Avatar.FileName)
	}

	fullName := ""
	if user.FullName != nil {
		fullName = *user.FullName
	}

	isBanned := user.IsBanned

	if isBanned && user.BannedUntil != nil && time.Now().After(*user.BannedUntil) {
		isBanned = false
	}

	isOnline := false
	if onlineMap != nil {
		isOnline = onlineMap[user.ID]
	}

	return model.GroupMemberDTO{
		ID:       m.ID,
		UserID:   user.ID,
		FullName: fullName,
		Avatar:   avatarURL,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt.String(),
		IsOnline: isOnline,
		IsBanned: isBanned,
	}
}
