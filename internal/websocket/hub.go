package websocket

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/chat"
	"AtoiTalkAPI/ent/privatechat"
	"AtoiTalkAPI/ent/userblock"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

type Hub struct {
	clients     map[*Client]bool
	userClients map[int]map[*Client]bool
	Register    chan *Client
	Unregister  chan *Client
	broadcast   chan []byte

	db *ent.Client
	mu sync.RWMutex
}

func NewHub(db *ent.Client) *Hub {
	return &Hub{
		broadcast:   make(chan []byte),
		Register:    make(chan *Client),
		Unregister:  make(chan *Client),
		clients:     make(map[*Client]bool),
		userClients: make(map[int]map[*Client]bool),
		db:          db,
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

func (h *Hub) BroadcastToUser(userID int, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.userClients[userID]; ok {
		data, err := json.Marshal(event)
		if err != nil {
			slog.Error("Failed to marshal event", "error", err)
			return
		}
		for client := range clients {
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				delete(h.clients, client)
			}
		}
	}
}

func (h *Hub) BroadcastToChat(chatID int, event Event) {
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

	memberUnreadMap := make(map[int]int)

	if c.Type == chat.TypePrivate && c.Edges.PrivateChat != nil {
		pc := c.Edges.PrivateChat
		memberUnreadMap[pc.User1ID] = pc.User1UnreadCount
		memberUnreadMap[pc.User2ID] = pc.User2UnreadCount
	} else if c.Type == chat.TypeGroup && c.Edges.GroupChat != nil {
		for _, m := range c.Edges.GroupChat.Edges.Members {
			memberUnreadMap[m.UserID] = m.UnreadCount
		}
	}

	blockedUserIDs := make(map[int]bool)
	if event.Type == EventTyping && event.Meta != nil && event.Meta.SenderID != 0 {
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
			slog.Error("Failed to fetch block list for typing event", "error", err)
		}
	}

	for uid, unreadCount := range memberUnreadMap {

		if event.Type == EventTyping && blockedUserIDs[uid] {
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

func (h *Hub) BroadcastToContacts(userID int, event Event) {
	ctx := context.Background()

	chats, err := h.db.PrivateChat.Query().
		Where(
			privatechat.Or(
				privatechat.User1ID(userID),
				privatechat.User2ID(userID),
			),
		).
		All(ctx)

	if err != nil {
		slog.Error("Failed to fetch contacts for broadcast", "error", err)
		return
	}

	targetUserIDs := make([]int, 0, len(chats))
	for _, pc := range chats {
		if pc.User1ID == userID {
			targetUserIDs = append(targetUserIDs, pc.User2ID)
		} else {
			targetUserIDs = append(targetUserIDs, pc.User1ID)
		}
	}

	blockedUserIDs := make(map[int]bool)
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

func (h *Hub) broadcastUserStatus(userID int, isOnline bool) {
	ctx := context.Background()
	update := h.db.User.UpdateOneID(userID).SetIsOnline(isOnline)
	if !isOnline {
		update.SetLastSeenAt(time.Now())
	}
	if err := update.Exec(ctx); err != nil {
		slog.Error("Failed to update user online status in DB", "error", err)
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
			"last_seen_at": time.Now().UnixMilli(),
		},
		Meta: &EventMeta{
			Timestamp: time.Now().UnixMilli(),
			SenderID:  userID,
		},
	}

	h.BroadcastToContacts(userID, event)
}
