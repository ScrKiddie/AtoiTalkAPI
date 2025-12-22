package bootstrap

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

func Init(appConfig *config.AppConfig, client *ent.Client, validator *validator.Validate) *chi.Mux {
	authService := service.NewAuthService(client, appConfig, validator)
	authController := controller.NewAuthController(authService)

	r := SetupRoutes(appConfig, authController)

	return r
}
