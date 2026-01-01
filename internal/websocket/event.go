package websocket

type EventType string

const (
	EventMessageNew    EventType = "message.new"
	EventMessageDelete EventType = "message.delete"

	EventChatNew  EventType = "chat.new"
	EventChatRead EventType = "chat.read"
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
	Timestamp   int64 `json:"timestamp"`
	ChatID      int   `json:"chat_id,omitempty"`
	SenderID    int   `json:"sender_id,omitempty"`
	UnreadCount int   `json:"unread_count"`
}
