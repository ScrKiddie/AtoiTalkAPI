package service

import (
	"AtoiTalkAPI/ent"
)

type UserService interface {
}

type userService struct {
	client *ent.Client
}

func NewUserService(client *ent.Client) UserService {
	return &userService{
		client: client,
	}
}
