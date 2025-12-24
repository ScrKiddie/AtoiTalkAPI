package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/adapter"
	"AtoiTalkAPI/internal/bootstrap"
	"AtoiTalkAPI/internal/config"
	"AtoiTalkAPI/internal/controller"
	"AtoiTalkAPI/internal/service"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var (
	testClient *ent.Client
	testConfig *config.AppConfig
	testRouter *chi.Mux
)

func TestMain(m *testing.M) {

	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)
	err := godotenv.Load(filepath.Join(basepath, "../.env.test"))
	if err != nil {
		log.Printf("Warning: Error loading .env.test file: %v", err)
	}

	os.Setenv("STORAGE_MODE", "local")
	os.Setenv("STORAGE_PROFILE", "test_profiles")
	os.Setenv("STORAGE_ATTACHMENT", "test_attachments")
	os.Setenv("APP_CORS_ALLOWED_ORIGINS", "*")
	os.Setenv("TEMP_CODE_EXP", "300")
	os.Setenv("TEMP_CODE_RATE_LIMIT_SECONDS", "2")
	os.Setenv("TURNSTILE_SECRET_KEY", "1x0000000000000000000000000000000AA")
	os.Setenv("SMTP_ASYNC", "false")

	testConfig = config.LoadAppConfig()

	cleanupStorage(true)

	testClient = config.InitEnt(testConfig)

	if err := testClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	validator := config.NewValidator()
	httpClient := config.NewHTTPClient()
	testRouter = config.NewChi(testConfig)

	emailAdapter := adapter.NewEmailAdapter(testConfig)

	var s3Client *s3.Client
	captchaAdapter := adapter.NewCaptchaAdapter(testConfig, httpClient)
	rateLimiter := config.NewRateLimiter(testConfig)
	storageAdapter := adapter.NewStorageAdapter(testConfig, s3Client, httpClient)

	authService := service.NewAuthService(testClient, testConfig, validator, storageAdapter)
	authController := controller.NewAuthController(authService)

	otpService := service.NewOTPService(testClient, testConfig, validator, emailAdapter, rateLimiter, captchaAdapter)
	otpController := controller.NewOTPController(otpService)

	route := bootstrap.NewRoute(testConfig, testRouter, authController, otpController)
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
	testClient.User.Delete().Exec(ctx)
	testClient.TempCodes.Delete().Exec(ctx)
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
