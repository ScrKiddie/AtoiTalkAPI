package websocket

import "github.com/google/uuid"

type EventType string

const (
	EventMessageNew    EventType = "message.new"
	EventMessageUpdate EventType = "message.update"
	EventMessageDelete EventType = "message.delete"

	EventChatNew  EventType = "chat.new"
	EventChatRead EventType = "chat.read"
	EventChatHide EventType = "chat.hide"
	EventTyping   EventType = "chat.typing"

	EventUserOnline  EventType = "user.online"
	EventUserOffline EventType = "user.offline"
	EventUserUpdate  EventType = "user.update"
	EventUserBlock   EventType = "user.block"
	EventUserUnblock EventType = "user.unblock"
)

type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
	Meta    *EventMeta  `json:"meta,omitempty"`
}

type EventMeta struct {
	Timestamp   int64     `json:"timestamp"`
	ChatID      uuid.UUID `json:"chat_id,omitempty"`
	SenderID    uuid.UUID `json:"sender_id,omitempty"`
	UnreadCount int       `json:"unread_count"`
}
