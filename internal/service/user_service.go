package service

import (
	"AtoiTalkAPI/ent"
)

type UserService struct {
	client *ent.Client
}

func NewUserService(client *ent.Client) *UserService {
	return &UserService{
		client: client,
	}
}
