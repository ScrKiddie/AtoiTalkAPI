package service

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/config"
)

type UserService struct {
	client *ent.Client
	cfg    *config.AppConfig
}

func NewUserService(client *ent.Client, cfg *config.AppConfig) *UserService {
	return &UserService{
		client: client,
		cfg:    cfg,
	}
}
