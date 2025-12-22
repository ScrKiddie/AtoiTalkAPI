package bootstrap

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/service"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

func Init(appConfig *config.AppConfig, client *ent.Client, validator *validator.Validate, s3Client *s3.Client, httpClient *http.Client) *chi.Mux {
	storageAdapter := adapter.NewStorageAdapter(appConfig, s3Client, httpClient)

	authService := service.NewAuthService(client, appConfig, validator, storageAdapter)
	authController := controller.NewAuthController(authService)

	r := SetupRoutes(appConfig, authController)

	return r
}
