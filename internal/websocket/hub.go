package websocket

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/groupchat"
	"AtoiTalkAPI/ent/groupmember"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/userblock"
	"AtoiTalkAPI/internal/adapter"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

const cacheTTL = 5 * time.Minute
const typingThrottle = 2 * time.Second
const pubSubChannel = "events:broadcast"
const onlineUserTTL = 70 * time.Second

type Hub struct {
	clients     map[*Client]bool
	userClients map[uuid.UUID]map[*Client]bool
	mu          sync.RWMutex

	Register   chan *Client
	Unregister chan *Client

	db    *ent.Client
	redis *adapter.RedisAdapter
}

type redisPayload struct {
	TargetUserID uuid.UUID `json:"target_user_id"`
	EventData    []byte    `json:"event_data"`
}

func NewHub(db *ent.Client, redis *adapter.RedisAdapter) *Hub {
	hub := &Hub{
		clients:     make(map[*Client]bool),
		userClients: make(map[uuid.UUID]map[*Client]bool),
		Register:    make(chan *Client, 256),
		Unregister:  make(chan *Client, 256),
		db:          db,
		redis:       redis,
	}

	go hub.listenToRedis()
	return hub
}

func (h *Hub) listenToRedis() {
	ctx := context.Background()
	pubsub := h.redis.Client().Subscribe(ctx, pubSubChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()

	for msg := range ch {
		var payload redisPayload
		if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
			slog.Error("Failed to unmarshal redis pubsub payload", "error", err)
			continue
		}

		h.deliverToLocalClients(payload.TargetUserID, payload.EventData)
	}
}

func (h *Hub) deliverToLocalClients(userID uuid.UUID, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.userClients[userID]; ok {
		for client := range clients {
			select {
			case client.Send <- data:
			default:
				select {
				case h.Unregister <- client:
				default:
				}
			}
		}
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.clients[client] = true
			if _, ok := h.userClients[client.UserID]; !ok {
				h.userClients[client.UserID] = make(map[*Client]bool)
				go h.broadcastUserStatus(client.UserID, true)
			}
			h.userClients[client.UserID][client] = true

			go h.KeepAlive(client.UserID)

			h.mu.Unlock()

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)

				if userSet, ok := h.userClients[client.UserID]; ok {
					delete(userSet, client)
					if len(userSet) == 0 {
						delete(h.userClients, client.UserID)
						go h.broadcastUserStatus(client.UserID, false)
					}
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) BroadcastToUser(userID uuid.UUID, event Event) {
	eventData, err := json.Marshal(event)
	if err != nil {
		slog.Error("Failed to marshal event for redis publish", "error", err)
		return
	}

	payload := redisPayload{
		TargetUserID: userID,
		EventData:    eventData,
	}

	payloadData, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to marshal redis payload", "error", err)
		return
	}

	h.redis.Client().Publish(context.Background(), pubSubChannel, payloadData)
}

func (h *Hub) BroadcastToChat(chatID uuid.UUID, event Event) {
	switch event.Type {
	case EventTyping:
		h.broadcastTyping(chatID, event)
	case EventChatRead:
		h.broadcastRead(chatID, event)
	default:
		h.broadcastHeavy(chatID, event)
	}
}

func (h *Hub) broadcastTyping(chatID uuid.UUID, event Event) {
	senderID := uuid.Nil
	if event.Meta != nil {
		senderID = event.Meta.SenderID
	}

	if senderID != uuid.Nil {
		key := fmt.Sprintf("typing:%s:%s", senderID, chatID)
		ok, err := h.redis.Client().SetNX(context.Background(), key, 1, typingThrottle).Result()
		if err != nil || !ok {
			return
		}
	}

	targetUserIDs := h.getChatMembers(chatID)

	for _, uid := range targetUserIDs {
		if uid == senderID {
			continue
		}
		h.BroadcastToUser(uid, event)
	}
}

func (h *Hub) getChatMembers(chatID uuid.UUID) []uuid.UUID {
	key := fmt.Sprintf("chat_members:%s", chatID)
	ctx := context.Background()

	membersStr, err := h.redis.Get(ctx, key)
	if err == nil {
		var userIDs []uuid.UUID
		if json.Unmarshal([]byte(membersStr), &userIDs) == nil {
			return userIDs
		}
	}

	return h.fetchAndCacheMembers(chatID)
}

func (h *Hub) fetchAndCacheMembers(chatID uuid.UUID) []uuid.UUID {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := h.db.Chat.Query().
		Where(chat.ID(chatID)).
		Select(chat.FieldType).
		Only(ctx)

	if err != nil {
		return nil
	}

	var userIDs []uuid.UUID

	if c.Type == chat.TypePrivate {
		pc, err := h.db.PrivateChat.Query().
			Where(privatechat.ChatID(chatID)).
			Select(privatechat.FieldUser1ID, privatechat.FieldUser2ID).
			Only(ctx)
		if err == nil {
			userIDs = []uuid.UUID{pc.User1ID, pc.User2ID}
		}
	} else if c.Type == chat.TypeGroup {
		gcID, err := h.db.GroupChat.Query().
			Where(groupchat.ChatID(chatID)).
			OnlyID(ctx)
		if err == nil {
			members, err := h.db.GroupMember.Query().
				Where(groupmember.GroupChatID(gcID)).
				Select(groupmember.FieldUserID).
				All(ctx)
			if err == nil {
				for _, m := range members {
					userIDs = append(userIDs, m.UserID)
				}
			}
		}
	}

	if len(userIDs) > 0 {
		key := fmt.Sprintf("chat_members:%s", chatID)
		data, _ := json.Marshal(userIDs)
		h.redis.Set(ctx, key, data, cacheTTL)
	}

	return userIDs
}

func (h *Hub) broadcastRead(chatID uuid.UUID, event Event) {
	targetUserIDs := h.getChatMembers(chatID)
	for _, uid := range targetUserIDs {
		h.BroadcastToUser(uid, event)
	}
}

func (h *Hub) broadcastHeavy(chatID uuid.UUID, event Event) {
	ctx := context.Background()

	c, err := h.db.Chat.Query().
		Where(chat.ID(chatID)).
		WithPrivateChat().
		WithGroupChat(func(q *ent.GroupChatQuery) {
			q.WithMembers()
		}).
		Only(ctx)

	if err != nil {
		slog.Error("Failed to fetch chat members for broadcast", "error", err, "chatID", chatID)
		return
	}

	memberUnreadMap := make(map[uuid.UUID]int)

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		memberUnreadMap[pc.User1ID] = pc.User1UnreadCount
		memberUnreadMap[pc.User2ID] = pc.User2UnreadCount
	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		for _, m := range c.Edges.GroupChat.Edges.Members {
			memberUnreadMap[m.UserID] = m.UnreadCount
		}
	}

	blockedUserIDs := make(map[uuid.UUID]bool)
	if event.Meta != nil && event.Meta.SenderID != uuid.Nil {
		senderID := event.Meta.SenderID

		blocks, err := h.db.UserBlock.Query().
			Where(
				userblock.Or(
					userblock.BlockerID(senderID),
					userblock.BlockedID(senderID),
				),
			).
			All(ctx)

		if err == nil {
			for _, b := range blocks {
				if b.BlockerID == senderID {
					blockedUserIDs[b.BlockedID] = true
				} else {
					blockedUserIDs[b.BlockerID] = true
				}
			}
		} else {
			slog.Error("Failed to fetch block list for heavy broadcast", "error", err)
		}
	}

	for uid, unreadCount := range memberUnreadMap {
		if blockedUserIDs[uid] {
			continue
		}

		personalEvent := event

		if event.Meta != nil {
			newMeta := *event.Meta
			newMeta.UnreadCount = unreadCount
			personalEvent.Meta = &newMeta
		} else {
			personalEvent.Meta = &EventMeta{
				UnreadCount: unreadCount,
			}
		}

		h.BroadcastToUser(uid, personalEvent)
	}
}

func (h *Hub) getContacts(userID uuid.UUID) []uuid.UUID {
	key := fmt.Sprintf("contacts:%s", userID)
	ctx := context.Background()

	contactsStr, err := h.redis.Get(ctx, key)
	if err == nil {
		var userIDs []uuid.UUID
		if json.Unmarshal([]byte(contactsStr), &userIDs) == nil {
			return userIDs
		}
	}

	return h.fetchAndCacheContacts(userID)
}

func (h *Hub) fetchAndCacheContacts(userID uuid.UUID) []uuid.UUID {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chats, err := h.db.PrivateChat.Query().
		Where(
			privatechat.Or(
				privatechat.User1ID(userID),
				privatechat.User2ID(userID),
			),
		).
		Select(privatechat.FieldUser1ID, privatechat.FieldUser2ID).
		All(ctx)

	if err != nil {
		slog.Error("Failed to fetch contacts for cache", "error", err)
		return nil
	}

	targetUserIDs := make([]uuid.UUID, 0, len(chats))
	for _, pc := range chats {
		if pc.User1ID == userID {
			targetUserIDs = append(targetUserIDs, pc.User2ID)
		} else {
			targetUserIDs = append(targetUserIDs, pc.User1ID)
		}
	}

	key := fmt.Sprintf("contacts:%s", userID)
	data, _ := json.Marshal(targetUserIDs)
	h.redis.Set(ctx, key, data, cacheTTL)

	return targetUserIDs
}

func (h *Hub) BroadcastToContacts(userID uuid.UUID, event Event) {
	ctx := context.Background()

	targetUserIDs := h.getContacts(userID)

	blockedUserIDs := make(map[uuid.UUID]bool)

	if event.Type == EventUserOnline || event.Type == EventUserOffline {
		blocks, err := h.db.UserBlock.Query().
			Where(
				userblock.Or(
					userblock.BlockerID(userID),
					userblock.BlockedID(userID),
				),
			).
			All(ctx)

		if err == nil {
			for _, b := range blocks {
				if b.BlockerID == userID {
					blockedUserIDs[b.BlockedID] = true
				} else {
					blockedUserIDs[b.BlockerID] = true
				}
			}
		}
	}

	for _, targetID := range targetUserIDs {
		if (event.Type == EventUserOnline || event.Type == EventUserOffline) && blockedUserIDs[targetID] {
			continue
		}
		h.BroadcastToUser(targetID, event)
	}
}

func (h *Hub) broadcastUserStatus(userID uuid.UUID, isOnline bool) {
	ctx := context.Background()
	key := fmt.Sprintf("online:%s", userID)

	if isOnline {

		h.redis.Set(ctx, key, "true", onlineUserTTL)
	} else {
		h.redis.Del(ctx, key)

		if err := h.db.User.UpdateOneID(userID).SetLastSeenAt(time.Now().UTC()).Exec(ctx); err != nil {
			slog.Error("Failed to update user last_seen_at in DB", "error", err)
		}
	}

	eventType := EventUserOffline
	if isOnline {
		eventType = EventUserOnline
	}

	event := Event{
		Type: eventType,
		Payload: map[string]interface{}{
			"user_id":      userID,
			"is_online":    isOnline,
			"last_seen_at": time.Now().UTC().UnixMilli(),
		},
		Meta: &EventMeta{
			Timestamp: time.Now().UTC().UnixMilli(),
			SenderID:  userID,
		},
	}

	h.BroadcastToContacts(userID, event)
}

func (h *Hub) KeepAlive(userID uuid.UUID) {
	ctx := context.Background()
	key := fmt.Sprintf("online:%s", userID)
	h.redis.Set(ctx, key, "true", onlineUserTTL)
}

func (h *Hub) DisconnectUser(userID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.userClients[userID]; ok {
		for client := range clients {
			delete(h.clients, client)
			close(client.Send)
			client.Conn.Close()
		}
		delete(h.userClients, userID)
		go h.broadcastUserStatus(userID, false)
	}
}
