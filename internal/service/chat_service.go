package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/user"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/model"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-playground/validator/v10"
)

type ChatService struct {
	client    *ent.Client
	cfg       *config.AppConfig
	validator *validator.Validate
	wsHub     *websocket.Hub
}

func NewChatService(client *ent.Client, cfg *config.AppConfig, validator *validator.Validate, wsHub *websocket.Hub) *ChatService {
	return &ChatService{
		client:    client,
		cfg:       cfg,
		validator: validator,
		wsHub:     wsHub,
	}
}

func (s *ChatService) CreatePrivateChat(ctx context.Context, userID int, req model.CreatePrivateChatRequest) (*model.ChatResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		slog.Warn("Validation failed", "error", err, "userID", userID)
		return nil, helper.NewBadRequestError("")
	}

	if userID == req.TargetUserID {
		return nil, helper.NewBadRequestError("Cannot create chat with yourself")
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

	isBlocked, err := s.client.UserBlock.Query().
		Where(
			userblock.Or(
				userblock.And(
					userblock.BlockerID(userID),
					userblock.BlockedID(req.TargetUserID),
				),
				userblock.And(
					userblock.BlockerID(req.TargetUserID),
					userblock.BlockedID(userID),
				),
			),
		).
		Exist(ctx)
	if err != nil {
		slog.Error("Failed to check block status", "error", err)
		return nil, helper.NewInternalServerError("")
	}
	if isBlocked {
		return nil, helper.NewForbiddenError("Cannot create chat with a blocked user")
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
			CreatedAt: existingChat.Edges.Chat.CreatedAt.Format(time.RFC3339),
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

	resp := &model.ChatResponse{
		ID:        newChat.ID,
		Type:      string(newChat.Type),
		CreatedAt: newChat.CreatedAt.Format(time.RFC3339),
	}

	if s.wsHub != nil {
		go s.wsHub.BroadcastToUser(req.TargetUserID, websocket.Event{
			Type:    websocket.EventChatNew,
			Payload: resp,
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UnixMilli(),
				ChatID:    newChat.ID,
				SenderID:  userID,
			},
		})
	}

	return resp, nil
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
				chat.HasPrivateChatWith(privatechat.Or(privatechat.User1ID(userID), privatechat.User2ID(userID))),
				chat.HasGroupChatWith(groupchat.HasMembersWith(groupmember.UserID(userID))),
			),
			func(s *sql.Selector) {
				t := sql.Table(privatechat.Table)
				s.Where(
					sql.Not(
						sql.Exists(
							sql.Select(privatechat.FieldID).From(t).Where(
								sql.And(
									sql.ColumnsEQ(t.C(privatechat.FieldChatID), s.C(chat.FieldID)),
									sql.Or(
										sql.And(
											sql.EQ(t.C(privatechat.FieldUser1ID), userID),
											sql.NotNull(t.C(privatechat.FieldUser1HiddenAt)),
											sql.Or(
												sql.ColumnsGTE(t.C(privatechat.FieldUser1HiddenAt), s.C(chat.FieldLastMessageAt)),
												sql.IsNull(s.C(chat.FieldLastMessageAt)),
											),
										),
										sql.And(
											sql.EQ(t.C(privatechat.FieldUser2ID), userID),
											sql.NotNull(t.C(privatechat.FieldUser2HiddenAt)),
											sql.Or(
												sql.ColumnsGTE(t.C(privatechat.FieldUser2HiddenAt), s.C(chat.FieldLastMessageAt)),
												sql.IsNull(s.C(chat.FieldLastMessageAt)),
											),
										),
									),
								),
							),
						),
					),
				)
			},
		)

	if req.Query != "" {
		otherUserPredicate := user.Or(
			user.FullNameContainsFold(req.Query),
			user.UsernameContainsFold(req.Query),
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

	fetchLimit := req.Limit

	chats, err := query.
		Order(ent.Desc(chat.FieldUpdatedAt), ent.Desc(chat.FieldID)).
		Limit(fetchLimit + 1).
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
		WithLastMessage(func(q *ent.MessageQuery) {
			q.WithAttachments(func(aq *ent.MediaQuery) {
				aq.Limit(1)
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

	response := make([]model.ChatListResponse, 0)
	for _, c := range chats {
		var name, avatar string
		var lastReadAt *string
		var otherLastReadAt *string
		var unreadCount int

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
				name = otherUser.FullName
				if otherUser.Edges.Avatar != nil {
					avatar = helper.BuildImageURL(s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageProfile, otherUser.Edges.Avatar.FileName)
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
		if c.Edges.LastMessage != nil {
			lastMsgResp = helper.ToMessageResponse(c.Edges.LastMessage, s.cfg.StorageMode, s.cfg.AppURL, s.cfg.StorageCDNURL, s.cfg.StorageAttachment)
		}

		response = append(response, model.ChatListResponse{
			ID:              c.ID,
			Type:            string(c.Type),
			Name:            name,
			Avatar:          avatar,
			LastMessage:     lastMsgResp,
			UnreadCount:     unreadCount,
			LastReadAt:      lastReadAt,
			OtherLastReadAt: otherLastReadAt,
		})
	}

	return response, nextCursor, hasNext, nil
}

func (s *ChatService) MarkAsRead(ctx context.Context, userID int, chatID int) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		slog.Error("Failed to start transaction", "error", err)
		return helper.NewInternalServerError("")
	}
	defer tx.Rollback()

	c, err := tx.Chat.Query().
		Where(chat.ID(chatID)).
		ForUpdate().
		WithPrivateChat().
		WithGroupChat().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Chat not found")
		}
		slog.Error("Failed to query chat for update", "error", err)
		return helper.NewInternalServerError("")
	}

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		update := tx.PrivateChat.UpdateOneID(pc.ID)

		if pc.User1ID == userID {
			update.SetUser1UnreadCount(0).SetUser1LastReadAt(time.Now())
		} else if pc.User2ID == userID {
			update.SetUser2UnreadCount(0).SetUser2LastReadAt(time.Now())
		} else {
			return helper.NewForbiddenError("You are not part of this chat")
		}
		if err := update.Exec(ctx); err != nil {
			slog.Error("Failed to mark private chat as read", "error", err)
			return helper.NewInternalServerError("")
		}

	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		err := tx.GroupMember.Update().
			Where(
				groupmember.GroupChatID(c.Edges.GroupChat.ID),
				groupmember.UserID(userID),
			).
			SetUnreadCount(0).
			SetLastReadAt(time.Now()).
			Exec(ctx)
		if err != nil {
			slog.Error("Failed to mark group chat as read", "error", err)
			return helper.NewInternalServerError("")
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "error", err)
		return helper.NewInternalServerError("")
	}

	if s.wsHub != nil {
		go s.wsHub.BroadcastToChat(chatID, websocket.Event{
			Type: websocket.EventChatRead,
			Payload: map[string]interface{}{
				"chat_id": chatID,
				"user_id": userID,
			},
			Meta: &websocket.EventMeta{
				Timestamp: time.Now().UnixMilli(),
				ChatID:    chatID,
				SenderID:  userID,
			},
		})
	}

	return nil
}

func (s *ChatService) HideChat(ctx context.Context, userID int, chatID int) error {
	c, err := s.client.Chat.Query().
		Where(chat.ID(chatID)).
		WithPrivateChat().
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return helper.NewNotFoundError("Chat not found")
		}
		slog.Error("Failed to query chat", "error", err, "chatID", chatID)
		return helper.NewInternalServerError("")
	}

	if c.Type != chat.TypePrivate {
		return helper.NewBadRequestError("Only private chats can be hidden")
	}

	if c.Edges.PrivateChat == nil {
		return helper.NewInternalServerError("Private chat data missing")
	}

	pc := c.Edges.PrivateChat
	update := s.client.PrivateChat.UpdateOneID(pc.ID)

	if pc.User1ID == userID {
		update.SetUser1HiddenAt(time.Now()).SetUser1UnreadCount(0)
	} else if pc.User2ID == userID {
		update.SetUser2HiddenAt(time.Now()).SetUser2UnreadCount(0)
	} else {
		return helper.NewForbiddenError("You are not part of this chat")
	}

	if err := update.Exec(ctx); err != nil {
		slog.Error("Failed to hide chat", "error", err, "chatID", chatID)
		return helper.NewInternalServerError("")
	}

	return nil
}
