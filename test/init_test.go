package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/user"
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
	"fmt"
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
	testClient   *ent.Client
	testConfig   *config.AppConfig
	testRouter   *chi.Mux
	testHub      *websocket.Hub
	redisAdapter *adapter.RedisAdapter
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

	if os.Getenv("APP_PORT") == "" {
		os.Setenv("APP_PORT", "8080")
	}
	if os.Getenv("APP_ENV") == "" {
		os.Setenv("APP_ENV", "test")
	}
	if os.Getenv("APP_URL") == "" {
		os.Setenv("APP_URL", "http://localhost:8080")
	}
	if os.Getenv("APP_CORS_ALLOWED_ORIGINS") == "" {
		os.Setenv("APP_CORS_ALLOWED_ORIGINS", "*")
	}

	os.Setenv("DB_MIGRATE", "true")

	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "secret")
	}
	if os.Getenv("JWT_EXP") == "" {
		os.Setenv("JWT_EXP", "86400")
	}
	if os.Getenv("OTP_SECRET") == "" {
		os.Setenv("OTP_SECRET", "secret")
	}
	if os.Getenv("OTP_EXP") == "" {
		os.Setenv("OTP_EXP", "300")
	}
	if os.Getenv("OTP_RATE_LIMIT_SECONDS") == "" {
		os.Setenv("OTP_RATE_LIMIT_SECONDS", "2")
	}
	if os.Getenv("TURNSTILE_SECRET_KEY") == "" {
		os.Setenv("TURNSTILE_SECRET_KEY", "1x0000000000000000000000000000000AA")
	}

	os.Setenv("STORAGE_MODE", "local")
	os.Setenv("STORAGE_PROFILE", "test_profiles")
	os.Setenv("STORAGE_ATTACHMENT", "test_attachments")

	os.Setenv("SMTP_ASYNC", "false")

	testConfig = config.LoadAppConfig()

	cleanupStorage(true)

	testClient = config.InitEnt(testConfig)

	if err := testClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	redisAdapter = adapter.NewRedisAdapter(testConfig)
	testHub = websocket.NewHub(testClient, redisAdapter)
	go testHub.Run()

	repo := repository.NewRepository(testClient, redisAdapter, testConfig)

	validator := config.NewValidator()
	httpClient := config.NewHTTPClient()
	testRouter = config.NewChi(testConfig)

	emailAdapter := adapter.NewEmailAdapter(testConfig)

	s3Client := &s3.Client{}
	captchaAdapter := adapter.NewCaptchaAdapter(testConfig, httpClient)
	storageAdapter := adapter.NewStorageAdapter(testConfig, s3Client, httpClient)

	otpService := service.NewOTPService(testClient, testConfig, validator, emailAdapter, captchaAdapter, redisAdapter, repo.RateLimit)
	otpController := controller.NewOTPController(otpService)

	authService := service.NewAuthService(testClient, testConfig, validator, storageAdapter, captchaAdapter, otpService, repo, testHub)
	authController := controller.NewAuthController(authService)

	userService := service.NewUserService(testClient, repo, testConfig, validator, storageAdapter, testHub, redisAdapter)
	userController := controller.NewUserController(userService)

	accountService := service.NewAccountService(testClient, testConfig, validator, testHub, otpService, redisAdapter, repo)
	accountController := controller.NewAccountController(accountService)

	chatService := service.NewChatService(testClient, repo, testConfig, validator, testHub, storageAdapter, redisAdapter)
	privateChatService := service.NewPrivateChatService(testClient, testConfig, validator, testHub, redisAdapter)
	groupChatService := service.NewGroupChatService(testClient, repo, testConfig, validator, testHub, storageAdapter, redisAdapter)

	chatController := controller.NewChatController(chatService)
	privateChatController := controller.NewPrivateChatController(privateChatService)
	groupChatController := controller.NewGroupChatController(groupChatService)

	messageService := service.NewMessageService(testClient, repo, testConfig, validator, storageAdapter, testHub)
	messageController := controller.NewMessageController(messageService)

	mediaService := service.NewMediaService(testClient, testConfig, validator, storageAdapter)
	mediaController := controller.NewMediaController(mediaService)

	reportService := service.NewReportService(testClient, testConfig, validator)
	reportController := controller.NewReportController(reportService)

	adminService := service.NewAdminService(testClient, testConfig, validator, testHub, repo)
	adminController := controller.NewAdminController(adminService)

	wsController := controller.NewWebSocketController(testHub)

	authMiddleware := middleware.NewAuthMiddleware(authService, repo.Session)
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(repo.RateLimit)

	route := bootstrap.NewRoute(testConfig, testRouter, authController, otpController, userController, accountController, chatController, privateChatController, groupChatController, messageController, mediaController, wsController, reportController, adminController, authMiddleware, rateLimitMiddleware)
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

	testClient.Report.Delete().Exec(ctx)
	testClient.Message.Delete().Exec(ctx)
	testClient.PrivateChat.Delete().Exec(ctx)
	testClient.GroupMember.Delete().Exec(ctx)
	testClient.GroupChat.Delete().Exec(ctx)
	testClient.Chat.Delete().Exec(ctx)
	testClient.Media.Delete().Exec(ctx)
	testClient.UserIdentity.Delete().Exec(ctx)
	testClient.User.Delete().Exec(ctx)

	if redisAdapter != nil {
		redisAdapter.Client().FlushDB(ctx)
	}
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
	duration := time.Until(expiresAt)
	if duration <= 0 {

		return
	}

	hashedCode := helper.HashOTP(code, testConfig.OTPSecret)

	key := fmt.Sprintf("otp:%s:%s", "register", email)
	redisAdapter.Set(context.Background(), key, hashedCode, duration)
}

func printBody(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Logf("Response Body: %s", rr.Body.String())
}

func createTestUser(t *testing.T, prefix string) *ent.User {
	email := fmt.Sprintf("%s%d@test.com", prefix, time.Now().UnixNano())
	username := fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	hashedPassword, _ := helper.HashPassword("Password123!")

	u, err := testClient.User.Create().
		SetEmail(email).
		SetUsername(username).
		SetFullName(prefix + " User").
		SetPasswordHash(hashedPassword).
		SetRole(user.RoleUser).
		Save(context.Background())
	if err != nil {
		t.Fatalf("Failed to create user %s: %v", prefix, err)
	}
	return u
}
