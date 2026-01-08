package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/otp"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/bootstrap"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/helper"
	"AtoiTalkAPI/internal/middleware"
	"AtoiTalkAPI/internal/repository"
	"AtoiTalkAPI/internal/service"
	"AtoiTalkAPI/internal/websocket"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var (
	testClient *ent.Client
	testConfig *config.AppConfig
	testRouter *chi.Mux
	testHub    *websocket.Hub
)

const (
	cfTurnstileAlwaysPasses      = "1x0000000000000000000000000000000AA"
	cfTurnstileAlwaysFails       = "2x0000000000000000000000000000000AA"
	cfTurnstileTokenAlreadySpent = "3x0000000000000000000000000000000AA"
	dummyTurnstileToken          = "DUMMY_TOKEN_XXXX"
)

func TestMain(m *testing.M) {

	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)
	err := godotenv.Load(filepath.Join(basepath, "../.env.test"))
	if err != nil {
		log.Printf("Warning: Error loading .env.test file: %v", err)
	}

	os.Setenv("APP_PORT", "8080")
	os.Setenv("APP_ENV", "test")
	os.Setenv("APP_URL", "http://localhost:8080")
	os.Setenv("APP_CORS_ALLOWED_ORIGINS", "*")

	os.Setenv("DB_MIGRATE", "true")

	os.Setenv("JWT_SECRET", "secret")
	os.Setenv("JWT_EXP", "86400")
	os.Setenv("OTP_SECRET", "secret")
	os.Setenv("OTP_EXP", "300")
	os.Setenv("OTP_RATE_LIMIT_SECONDS", "2")
	os.Setenv("TURNSTILE_SECRET_KEY", "1x0000000000000000000000000000000AA")

	os.Setenv("STORAGE_MODE", "local")
	os.Setenv("STORAGE_PROFILE", "test_profiles")
	os.Setenv("STORAGE_ATTACHMENT", "test_attachments")

	os.Setenv("STORAGE_CDN_URL", "")
	os.Setenv("S3_BUCKET", "")
	os.Setenv("S3_REGION", "")
	os.Setenv("S3_ACCESS_KEY", "")
	os.Setenv("S3_SECRET_KEY", "")
	os.Setenv("S3_ENDPOINT", "")

	os.Setenv("SMTP_ASYNC", "false")

	testConfig = config.LoadAppConfig()

	cleanupStorage(true)

	testClient = config.InitEnt(testConfig)

	if err := testClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	testHub = websocket.NewHub(testClient)
	go testHub.Run()

	repo := repository.NewRepository(testClient)

	validator := config.NewValidator()
	httpClient := config.NewHTTPClient()
	testRouter = config.NewChi(testConfig)

	emailAdapter := adapter.NewEmailAdapter(testConfig)

	s3Client := &s3.Client{}
	captchaAdapter := adapter.NewCaptchaAdapter(testConfig, httpClient)
	rateLimiter := config.NewRateLimiter(testConfig)
	storageAdapter := adapter.NewStorageAdapter(testConfig, s3Client, httpClient)

	authService := service.NewAuthService(testClient, testConfig, validator, storageAdapter, captchaAdapter)
	authController := controller.NewAuthController(authService)

	otpService := service.NewOTPService(testClient, testConfig, validator, emailAdapter, rateLimiter, captchaAdapter)
	otpController := controller.NewOTPController(otpService)

	userService := service.NewUserService(testClient, repo, testConfig, validator, storageAdapter, testHub)
	userController := controller.NewUserController(userService)

	accountService := service.NewAccountService(testClient, testConfig, validator, testHub)
	accountController := controller.NewAccountController(accountService)

	chatService := service.NewChatService(testClient, repo, testConfig, validator, testHub, storageAdapter)
	privateChatService := service.NewPrivateChatService(testClient, testConfig, validator, testHub)
	groupChatService := service.NewGroupChatService(testClient, repo, testConfig, validator, testHub, storageAdapter)

	chatController := controller.NewChatController(chatService)
	privateChatController := controller.NewPrivateChatController(privateChatService)
	groupChatController := controller.NewGroupChatController(groupChatService)

	messageService := service.NewMessageService(testClient, repo, testConfig, validator, storageAdapter, testHub)
	messageController := controller.NewMessageController(messageService)

	mediaService := service.NewMediaService(testClient, testConfig, validator, storageAdapter)
	mediaController := controller.NewMediaController(mediaService)

	wsController := controller.NewWebSocketController(testHub)

	authMiddleware := middleware.NewAuthMiddleware(authService)

	route := bootstrap.NewRoute(testConfig, testRouter, authController, otpController, userController, accountController, chatController, privateChatController, groupChatController, messageController, mediaController, wsController, authMiddleware)
	route.Register()

	code := m.Run()

	cleanupStorage(false)
	testClient.Close()
	os.Exit(code)
}

func executeRequest(req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)
	return rr
}

func clearDatabase(ctx context.Context) {

	testClient.Message.Delete().Exec(ctx)
	testClient.PrivateChat.Delete().Exec(ctx)
	testClient.GroupMember.Delete().Exec(ctx)
	testClient.GroupChat.Delete().Exec(ctx)
	testClient.Chat.Delete().Exec(ctx)
	testClient.Media.Delete().Exec(ctx)
	testClient.UserIdentity.Delete().Exec(ctx)
	testClient.User.Delete().Exec(ctx)
	testClient.OTP.Delete().Exec(ctx)
}

func cleanupStorage(create bool) {

	_, b, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(b)

	profilePath := filepath.Join(testDir, testConfig.StorageProfile)
	attachmentPath := filepath.Join(testDir, testConfig.StorageAttachment)

	os.RemoveAll(profilePath)
	os.RemoveAll(attachmentPath)

	if create {
		os.MkdirAll(profilePath, 0755)
		os.MkdirAll(attachmentPath, 0755)
	}
}

func createOTP(email, code string, expiresAt time.Time) {
	hashedCode := helper.HashOTP(code, testConfig.OTPSecret)
	testClient.OTP.Create().
		SetEmail(email).
		SetCode(hashedCode).
		SetMode(otp.ModeRegister).
		SetExpiresAt(expiresAt.UTC()).
		Exec(context.Background())
}

func printBody(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Logf("Response Body: %s", rr.Body.String())
}
