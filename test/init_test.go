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
		log.Fatalf("Error loading .env.test file: %v", err)
	}

	testConfig = config.LoadAppConfig()

	testClient = config.InitEnt(testConfig)

	if err := testClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	validator := config.NewValidator()
	testRouter = bootstrap.Init(testConfig, testClient, validator)

	code := m.Run()

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
