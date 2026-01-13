package repository

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
)

type Repository struct {
	Chat        *ChatRepository
	User        *UserRepository
	Message     *MessageRepository
	GroupMember *GroupMemberRepository
	Session     *SessionRepository
	RateLimit   *RateLimitRepository
}

func NewRepository(client *ent.Client, redisAdapter *adapter.RedisAdapter, cfg *config.AppConfig) *Repository {
	return &Repository{
		Chat:        NewChatRepository(client),
		User:        NewUserRepository(client),
		Message:     NewMessageRepository(client),
		GroupMember: NewGroupMemberRepository(client),
		Session:     NewSessionRepository(redisAdapter, cfg),
		RateLimit:   NewRateLimitRepository(redisAdapter),
	}
}
