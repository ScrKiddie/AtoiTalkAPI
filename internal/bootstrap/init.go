package bootstrap

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/repository"
	"AtoiTalkAPI/internal/service"
	"AtoiTalkAPI/internal/websocket"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

func Init(appConfig *config.AppConfig, client *ent.Client, validator *validator.Validate, s3Client *s3.Client, httpClient *http.Client, chiMux *chi.Mux, rateLimiter *config.RateLimiter) {
	wsHub := websocket.NewHub(client)
	go wsHub.Run()

	repo := repository.NewRepository(client)

	storageAdapter := adapter.NewStorageAdapter(appConfig, s3Client, httpClient)
	emailAdapter := adapter.NewEmailAdapter(appConfig)
	captchaAdapter := adapter.NewCaptchaAdapter(appConfig, httpClient)

	authService := service.NewAuthService(client, appConfig, validator, storageAdapter, captchaAdapter)
	authController := controller.NewAuthController(authService)

	otpService := service.NewOTPService(client, appConfig, validator, emailAdapter, rateLimiter, captchaAdapter)
	otpController := controller.NewOTPController(otpService)

	userService := service.NewUserService(client, repo, appConfig, validator, storageAdapter, wsHub)
	userController := controller.NewUserController(userService)

	accountService := service.NewAccountService(client, appConfig, validator)
	accountController := controller.NewAccountController(accountService)

	chatService := service.NewChatService(client, repo, appConfig, validator, wsHub, storageAdapter)
	privateChatService := service.NewPrivateChatService(client, appConfig, validator, wsHub)
	groupChatService := service.NewGroupChatService(client, appConfig, validator, wsHub, storageAdapter)

	chatController := controller.NewChatController(chatService)
	privateChatController := controller.NewPrivateChatController(privateChatService)
	groupChatController := controller.NewGroupChatController(groupChatService)

	messageService := service.NewMessageService(client, repo, appConfig, validator, storageAdapter, wsHub)
	messageController := controller.NewMessageController(messageService)

	mediaService := service.NewMediaService(client, appConfig, validator, storageAdapter)
	mediaController := controller.NewMediaController(mediaService)

	wsController := controller.NewWebSocketController(wsHub)

	authMiddleware := middleware.NewAuthMiddleware(authService)

	route := NewRoute(appConfig, chiMux, authController, otpController, userController, accountController, chatController, privateChatController, groupChatController, messageController, mediaController, wsController, authMiddleware)
	route.Register()
}
