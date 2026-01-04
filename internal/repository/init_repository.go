package repository

import "AtoiTalkAPI/ent"

type Repository struct {
	Chat    *ChatRepository
	User    *UserRepository
	Message *MessageRepository
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{
		Chat:    NewChatRepository(client),
		User:    NewUserRepository(client),
		Message: NewMessageRepository(client),
	}
}
