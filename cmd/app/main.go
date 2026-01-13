package main

import (
	"AtoiTalkAPI/internal/bootstrap"
	"AtoiTalkAPI/internal/config"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.LoadAppConfig()

	client := config.InitEnt(cfg)
	defer func() {
		if err := client.Close(); err != nil {
			slog.Error("Error closing database connection", "error", err)
		}
	}()

	s3Client, err := config.NewS3Client(*cfg)
	if err != nil {
		slog.Error("Failed to initialize S3 client", "error", err)
	}

	httpClient := config.NewHTTPClient()
	validate := config.NewValidator()
	chiMux := config.NewChi(cfg)

	bootstrap.Init(cfg, client, validate, s3Client, httpClient, chiMux)

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	slog.Info("Starting AtoiTalkAPI", "port", cfg.AppPort)

	if err := http.ListenAndServe(addr, chiMux); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
