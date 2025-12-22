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

	validate := config.NewValidator()

	r := bootstrap.Init(cfg, client, validate)

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	fmt.Printf("Starting AtoiTalkAPI on port %s...\n", cfg.AppPort)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
