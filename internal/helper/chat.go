package helper

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/internal/model"
	"time"
)

type BlockStatus struct {
	BlockedByMe    bool
	BlockedByOther bool
}

func MapChatToResponse(userID int, c *ent.Chat, blockedMap map[int]BlockStatus, storageMode, appURL, cdnURL, storageProfile, storageAttachment string) *model.ChatListResponse {
	var name, avatar string
	var lastReadAt *string
	var otherLastReadAt *string
	var unreadCount int
	var isOnline bool
	var otherUserID *int
	var otherUsername *string
	var isBlockedByMe bool
	var myRole *string

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		var otherUser *ent.User
		var myLastRead *time.Time
		var otherUserLastRead *time.Time

		if pc.User1ID == userID {
			otherUser = pc.Edges.User2
			myLastRead = pc.User1LastReadAt
			otherUserLastRead = pc.User2LastReadAt
			unreadCount = pc.User1UnreadCount
		} else {
			otherUser = pc.Edges.User1
			myLastRead = pc.User2LastReadAt
			otherUserLastRead = pc.User1LastReadAt
			unreadCount = pc.User2UnreadCount
		}

		if otherUser != nil {
			otherUserID = &otherUser.ID
			otherUsername = &otherUser.Username
			name = otherUser.FullName

			status := blockedMap[otherUser.ID]
			isBlockedByMe = status.BlockedByMe

			if status.BlockedByMe || status.BlockedByOther {
				isOnline = false

				if otherUser.Edges.Avatar != nil {
					avatar = BuildImageURL(storageMode, appURL, cdnURL, storageProfile, otherUser.Edges.Avatar.FileName)
				}
			} else {
				isOnline = otherUser.IsOnline
				if otherUser.Edges.Avatar != nil {
					avatar = BuildImageURL(storageMode, appURL, cdnURL, storageProfile, otherUser.Edges.Avatar.FileName)
				}
			}
		}
		if myLastRead != nil {
			t := myLastRead.Format(time.RFC3339)
			lastReadAt = &t
		}
		if otherUserLastRead != nil {
			t := otherUserLastRead.Format(time.RFC3339)
			otherLastReadAt = &t
		}
	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		gc := c.Edges.GroupChat
		name = gc.Name
		if gc.Edges.Avatar != nil {
			avatar = BuildImageURL(storageMode, appURL, cdnURL, storageProfile, gc.Edges.Avatar.FileName)
		}

		if len(gc.Edges.Members) > 0 {
			member := gc.Edges.Members[0]
			unreadCount = member.UnreadCount
			roleStr := string(member.Role)
			myRole = &roleStr
			if member.LastReadAt != nil {
				t := member.LastReadAt.Format(time.RFC3339)
				lastReadAt = &t
			}
		}
	}

	var lastMsgResp *model.MessageResponse
	if c.Edges.LastMessage != nil {
		lastMsgResp = ToMessageResponse(c.Edges.LastMessage, storageMode, appURL, cdnURL, storageAttachment)
	}

	return &model.ChatListResponse{
		ID:              c.ID,
		Type:            string(c.Type),
		Name:            name,
		Avatar:          avatar,
		LastMessage:     lastMsgResp,
		UnreadCount:     unreadCount,
		LastReadAt:      lastReadAt,
		OtherLastReadAt: otherLastReadAt,
		IsOnline:        isOnline,
		OtherUserID:     otherUserID,
		OtherUsername:   otherUsername,
		IsBlockedByMe:   isBlockedByMe,
		MyRole:          myRole,
	}
}
