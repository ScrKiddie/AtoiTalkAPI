package bootstrap

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/service"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

func Init(appConfig *config.AppConfig, client *ent.Client, validator *validator.Validate, s3Client *s3.Client, httpClient *http.Client, chiMux *chi.Mux, rateLimiter *config.RateLimiter) {
	storageAdapter := adapter.NewStorageAdapter(appConfig, s3Client, httpClient)
	emailAdapter := adapter.NewEmailAdapter(appConfig)
	captchaAdapter := adapter.NewCaptchaAdapter(appConfig, httpClient)

	authService := service.NewAuthService(client, appConfig, validator, storageAdapter, captchaAdapter)
	authController := controller.NewAuthController(authService)

	otpService := service.NewOTPService(client, appConfig, validator, emailAdapter, rateLimiter, captchaAdapter)
	otpController := controller.NewOTPController(otpService)

	userService := service.NewUserService(client, appConfig)
	userController := controller.NewUserController(userService)

	authMiddleware := middleware.NewAuthMiddleware(authService)

	route := NewRoute(appConfig, chiMux, authController, otpController, userController, authMiddleware)
	route.Register()
}
