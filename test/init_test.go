package test

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/internal/bootstrap"
	"AtoiTalkAPI/internal/config"
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

	testConfig = config.LoadAppConfig()

	cleanupStorage(true)

	testClient = config.InitEnt(testConfig)

	if err := testClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	validator := config.NewValidator()
	httpClient := config.NewHTTPClient()

	var s3Client *s3.Client
	testRouter = bootstrap.Init(testConfig, testClient, validator, s3Client, httpClient)

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
