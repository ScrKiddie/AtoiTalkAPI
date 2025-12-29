package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/message"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

type ChatService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
}

func NewChatService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate) *ChatService {
	return &ChatService{
		client:    client,
		cfg:       cfg,
		validator: validator,
	}
}

func (s *ChatService) CreatePrivateChat(ctx context.Context, userID int, req model.CreatePrivateChatRequest) (*model.ChatResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	if userID == req.TargetUserID {
		return nil, helper.NewBadRequestError("")
	}

	targetUserExists, err := s.client.User.Query().
		Where(user.ID(req.TargetUserID)).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check target user existence", "error", err, "targetUserID", req.TargetUserID)
		return nil, helper.NewInternalServerError("")
	}
	if !targetUserExists {
		return nil, helper.NewNotFoundError("Target user not found")
	}

	existingChat, err := s.client.PrivateChat.Query().
		Where(
			privatechat.Or(
				privatechat.And(
					privatechat.User1ID(userID),
					privatechat.User2ID(req.TargetUserID),
				),
				privatechat.And(
					privatechat.User1ID(req.TargetUserID),
					privatechat.User2ID(userID),
				),
			),
		).
		WithChat().
		Only(ctx)

	if err == nil {

		return &model.ChatResponse{
			ID:        existingChat.Edges.Chat.ID,
			Type:      string(existingChat.Edges.Chat.Type),
			CreatedAt: existingChat.Edges.Chat.CreatedAt.String(),
		}, nil
	} else if !ent.IsNotFound(err) {
		slog.Error("Failed to check existing private chat", "error", err)
		return nil, helper.NewInternalServerError("")
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

	newChat, err := tx.Chat.Create().
		SetType(chat.TypePrivate).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	_, err = tx.PrivateChat.Create().
		SetChat(newChat).
		SetUser1ID(userID).
		SetUser2ID(req.TargetUserID).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to create private chat", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return nil, helper.NewInternalServerError("")
	}

	return &model.ChatResponse{
		ID:        newChat.ID,
		Type:      string(newChat.Type),
		CreatedAt: newChat.CreatedAt.String(),
	}, nil
}

func (s *ChatService) GetChats(ctx context.Context, userID int, req model.GetChatsRequest) ([]model.ChatListResponse, string, bool, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, "", false, helper.NewBadRequestError("")
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	query := s.client.Chat.Query().
		Where(
			chat.Or(
				chat.HasPrivateChatWith(privatechat.Or(
					privatechat.And(privatechat.User1ID(userID), privatechat.User1HiddenAtIsNil()),
					privatechat.And(privatechat.User2ID(userID), privatechat.User2HiddenAtIsNil()),
				)),
				chat.HasGroupChatWith(groupchat.HasMembersWith(groupmember.UserID(userID))),
			),
		)

	if req.Query != "" {
		otherUserPredicate := user.Or(
			user.FullNameContainsFold(req.Query),
			user.EmailContainsFold(req.Query),
		)
		query = query.Where(
			chat.Or(
				chat.HasPrivateChatWith(privatechat.Or(

					privatechat.And(
						privatechat.User1ID(userID),
						privatechat.HasUser2With(otherUserPredicate),
					),
					privatechat.And(
						privatechat.User2ID(userID),
						privatechat.HasUser1With(otherUserPredicate),
					),
				)),
				chat.HasGroupChatWith(groupchat.NameContainsFold(req.Query)),
			),
		)
	}

	if req.Cursor != "" {
		decodedBytes, err := base64.URLEncoding.DecodeString(req.Cursor)
		if err == nil {
			parts := strings.Split(string(decodedBytes), ",")
			if len(parts) == 2 {
				cursorTimeMicro, err1 := strconv.ParseInt(parts[0], 10, 64)
				cursorID, err2 := strconv.Atoi(parts[1])
				if err1 == nil && err2 == nil {
					cursorTime := time.UnixMicro(cursorTimeMicro)
					query = query.Where(
						chat.Or(
							chat.UpdatedAtLT(cursorTime),
							chat.And(
								chat.UpdatedAtEQ(cursorTime),
								chat.IDLT(cursorID),
							),
						),
					)
				}
			}
		}
	}

	chats, err := query.
		Order(ent.Desc(chat.FieldUpdatedAt), ent.Desc(chat.FieldID)).
		Limit(req.Limit + 1).
		WithPrivateChat(func(q *ent.PrivateChatQuery) {
			q.WithUser1(func(uq *ent.UserQuery) { uq.WithAvatar() })
			q.WithUser2(func(uq *ent.UserQuery) { uq.WithAvatar() })
		}).
		WithGroupChat(func(q *ent.GroupChatQuery) {
			q.WithAvatar()

			q.WithMembers(func(mq *ent.GroupMemberQuery) {
				mq.Where(groupmember.UserID(userID))
			})
		}).
		All(ctx)

	if err != nil {
		slog.Error("Failed to get chats", "error", err)
		return nil, "", false, helper.NewInternalServerError("")
	}

	hasNext := false
	var nextCursor string
	if len(chats) > req.Limit {
		hasNext = true
		chats = chats[:req.Limit]
		lastChat := chats[len(chats)-1]
		cursorString := fmt.Sprintf("%d,%d", lastChat.UpdatedAt.UnixMicro(), lastChat.ID)
		nextCursor = base64.URLEncoding.EncodeToString([]byte(cursorString))
	}

	chatIDs := make([]int, len(chats))
	for i, c := range chats {
		chatIDs[i] = c.ID
	}

	lastMessages := make(map[int]*ent.Message)
	if len(chatIDs) > 0 {
		var lastMsgData []struct {
			ChatID int `json:"chat_id"`
			Max    int `json:"max"`
		}
		err := s.client.Message.Query().
			Where(message.ChatIDIn(chatIDs...)).
			GroupBy(message.FieldChatID).
			Aggregate(ent.Max(message.FieldID)).
			Scan(ctx, &lastMsgData)

		if err != nil {
			slog.Error("Failed to get last message IDs", "error", err)
		} else {
			var lastMsgIDs []int
			for _, d := range lastMsgData {
				lastMsgIDs = append(lastMsgIDs, d.Max)
			}

			if len(lastMsgIDs) > 0 {
				msgs, _ := s.client.Message.Query().
					Where(message.IDIn(lastMsgIDs...)).
					WithSender().
					WithAttachments().
					WithReplyTo(func(q *ent.MessageQuery) {
						q.WithSender().WithAttachments()
					}).
					All(ctx)
				for _, msg := range msgs {
					lastMessages[msg.ChatID] = msg
				}
			}
		}
	}

	response := make([]model.ChatListResponse, 0)
	for _, c := range chats {
		var name, avatar string
		var lastReadAt *string
		var isPinned bool
		var unreadCount int

		if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
			pc := c.Edges.PrivateChat
			var otherUser *ent.User
			var myLastRead *time.Time
			if pc.User1ID == userID {
				otherUser = pc.Edges.User2
				myLastRead = pc.User1LastReadAt
				unreadCount = pc.User1UnreadCount
			} else {
				otherUser = pc.Edges.User1
				myLastRead = pc.User2LastReadAt
				unreadCount = pc.User2UnreadCount
			}
			if otherUser != nil {
				name = otherUser.FullName
				if otherUser.Edges.Avatar != nil {
					avatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, otherUser.Edges.Avatar.FileName)
				}
			}
			if myLastRead != nil {
				t := myLastRead.Format(time.RFC3339)
				lastReadAt = &t
			}
		} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
			gc := c.Edges.GroupChat
			name = gc.Name
			if gc.Edges.Avatar != nil {
				avatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, gc.Edges.Avatar.FileName)
			}
			if len(gc.Edges.Members) > 0 {
				unreadCount = gc.Edges.Members[0].UnreadCount
				if gc.Edges.Members[0].LastReadAt != nil {
					t := gc.Edges.Members[0].LastReadAt.Format(time.RFC3339)
					lastReadAt = &t
				}
			}
		}

		var lastMsgResp *model.MessageResponse
		if msg, ok := lastMessages[c.ID]; ok {
			lastMsgResp = helper.ToMessageResponse(msg, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)
		}

		response = append(response, model.ChatListResponse{ID: c.ID, Type: string(c.Type), Name: name, Avatar: avatar, LastMessage: lastMsgResp, UnreadCount: unreadCount, LastReadAt: lastReadAt, IsPinned: isPinned})
	}

	return response, nextCursor, hasNext, nil
}

func (s *ChatService) IncrementUnreadCount(ctx context.Context, groupID *int, chatID int, senderID int) error {
	if groupID != nil {

		return s.client.GroupMember.Update().
			Where(
				groupmember.GroupChatID(*groupID),
				groupmember.UserIDNEQ(senderID),
			).
			AddUnreadCount(1).
			Exec(ctx)
	}

	pc, err := s.client.PrivateChat.Query().Where(privatechat.ChatID(chatID)).Only(ctx)
	if err != nil {
		return err
	}

	update := s.client.PrivateChat.UpdateOne(pc)
	if pc.User1ID == senderID {
		update.AddUser2UnreadCount(1)
	} else {
		update.AddUser1UnreadCount(1)
	}
	return update.Exec(ctx)
}

func (s *ChatService) MarkAsRead(ctx context.Context, userID int, chatID int) error {

	c, err := s.client.Chat.Query().
		Where(chat.ID(chatID)).
		WithPrivateChat().
		WithGroupChat().
		Only(ctx)
	if err != nil {
		return helper.NewNotFoundError("Chat not found")
	}

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		update := s.client.PrivateChat.UpdateOneID(pc.ID)

		if pc.User1ID == userID {
			update.SetUser1UnreadCount(0).SetUser1LastReadAt(time.Now())
		} else if pc.User2ID == userID {
			update.SetUser2UnreadCount(0).SetUser2LastReadAt(time.Now())
		} else {
			return helper.NewForbiddenError("You are not part of this chat")
		}
		return update.Exec(ctx)

	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {

		return s.client.GroupMember.Update().
			Where(
				groupmember.GroupChatID(c.Edges.GroupChat.ID),
				groupmember.UserID(userID),
			).
			SetUnreadCount(0).
			SetLastReadAt(time.Now()).
			Exec(ctx)
	}

	return nil
}
