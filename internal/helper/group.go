package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
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

	return model.GroupMemberDTO{
		ID:       m.ID,
		UserID:   user.ID,
		Username: user.Username,
		FullName: user.FullName,
		Avatar:   avatarURL,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt.String(),
		IsOnline: user.IsOnline,
	}
}
