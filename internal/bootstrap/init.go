package bootstrap

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/config"

	"github.com/go-chi/chi/v5"
)

func Init(appConfig *config.AppConfig, client *ent.Client) *chi.Mux {

	r := SetupRoutes(appConfig)

	return r
}
