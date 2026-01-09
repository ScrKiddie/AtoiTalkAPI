package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
	"time"
)

func ToGroupMemberDTO(m *ent.GroupMember, storageMode, appURL, cdnURL, storageProfile string) model.GroupMemberDTO {
	if m == nil || m.Edges.User == nil {
		return model.GroupMemberDTO{}
	}

	user := m.Edges.User
	avatarURL := ""
	if user.Edges.Avatar != nil {
		avatarURL = BuildImageURL(storageMode, appURL, cdnURL, storageProfile, user.Edges.Avatar.FileName)
	}

	fullName := ""
	if user.FullName != nil {
		fullName = *user.FullName
	}

	isBanned := user.IsBanned

	if isBanned && user.BannedUntil != nil && time.Now().After(*user.BannedUntil) {
		isBanned = false
	}

	return model.GroupMemberDTO{
		ID:       m.ID,
		UserID:   user.ID,
		FullName: fullName,
		Avatar:   avatarURL,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt.String(),
		IsOnline: user.IsOnline,
		IsBanned: isBanned,
	}
}
