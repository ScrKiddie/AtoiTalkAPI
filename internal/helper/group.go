package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/model"
)

func ToGroupMemberDTO(m *ent.GroupMember, urlGen URLGenerator) model.GroupMemberDTO {
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

	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	return model.GroupMemberDTO{
		ID:       m.ID,
		UserID:   user.ID,
		Username: username,
		FullName: fullName,
		Avatar:   avatarURL,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt.String(),
	}
}
