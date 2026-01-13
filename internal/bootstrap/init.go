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

	storageAdapter := adapter.NewStorageAdapter(appConfig, s3Client, httpClient)
	emailAdapter := adapter.NewEmailAdapter(appConfig)
	captchaAdapter := adapter.NewCaptchaAdapter(appConfig, httpClient)
	redisAdapter := adapter.NewRedisAdapter(appConfig)

	wsHub := websocket.NewHub(client, redisAdapter)
	go wsHub.Run()

	repo := repository.NewRepository(client, redisAdapter, appConfig)

	otpService := service.NewOTPService(client, appConfig, validator, emailAdapter, rateLimiter, captchaAdapter, redisAdapter)

	authService := service.NewAuthService(client, appConfig, validator, storageAdapter, captchaAdapter, otpService, repo, wsHub)

	accountService := service.NewAccountService(client, appConfig, validator, wsHub, otpService, redisAdapter, repo)

	userService := service.NewUserService(client, repo, appConfig, validator, storageAdapter, wsHub, redisAdapter)
	chatService := service.NewChatService(client, repo, appConfig, validator, wsHub, storageAdapter, redisAdapter)
	privateChatService := service.NewPrivateChatService(client, appConfig, validator, wsHub, redisAdapter)
	groupChatService := service.NewGroupChatService(client, repo, appConfig, validator, wsHub, storageAdapter, redisAdapter)
	messageService := service.NewMessageService(client, repo, appConfig, validator, storageAdapter, wsHub)
	mediaService := service.NewMediaService(client, appConfig, validator, storageAdapter)
	reportService := service.NewReportService(client, appConfig, validator)

	adminService := service.NewAdminService(client, appConfig, validator, wsHub, repo)

	authController := controller.NewAuthController(authService)
	otpController := controller.NewOTPController(otpService)
	userController := controller.NewUserController(userService)
	accountController := controller.NewAccountController(accountService)
	chatController := controller.NewChatController(chatService)
	privateChatController := controller.NewPrivateChatController(privateChatService)
	groupChatController := controller.NewGroupChatController(groupChatService)
	messageController := controller.NewMessageController(messageService)
	mediaController := controller.NewMediaController(mediaService)
	reportController := controller.NewReportController(reportService)
	adminController := controller.NewAdminController(adminService)
	wsController := controller.NewWebSocketController(wsHub)

	authMiddleware := middleware.NewAuthMiddleware(authService, repo.Session)

	route := NewRoute(appConfig, chiMux, authController, otpController, userController, accountController, chatController, privateChatController, groupChatController, messageController, mediaController, wsController, reportController, adminController, authMiddleware)
	route.Register()
}
