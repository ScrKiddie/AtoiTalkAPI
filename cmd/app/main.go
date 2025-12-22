package main

import (
	"AtoiTalkAPI/internal/bootstrap"
	"AtoiTalkAPI/internal/config"
	"fmt"
	"log"
	"net/http"
)

func main() {

	cfg := config.LoadAppConfig()

	client := config.InitEnt(cfg)
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	s3Client, err := config.NewS3Client(*cfg)
	if err != nil {
		log.Printf("Failed to initialize S3 client: %v", err)
	}

	httpClient := config.NewHTTPClient()
	validate := config.NewValidator()

	r := bootstrap.Init(cfg, client, validate, s3Client, httpClient)

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	fmt.Printf("Starting AtoiTalkAPI on port %s...\n", cfg.AppPort)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
