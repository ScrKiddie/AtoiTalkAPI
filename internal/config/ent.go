package config

import (
	"AtoiTalkAPI/ent"
	"context"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func InitEnt(cfg *AppConfig) *ent.Client {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName, cfg.DBPassword, cfg.DBSSLMode)

	client, err := ent.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed opening connection to postgres: %v", err)
	}

	if cfg.DBMigrate {
		if err := client.Schema.Create(context.Background()); err != nil {
			log.Fatalf("failed creating schema resources: %v", err)
		}
		fmt.Println("Database schema migrated successfully (Ent)")
	} else {
		fmt.Println("Database migration skipped (DB_MIGRATE=false)")
	}

	fmt.Println("Database connected successfully")
	return client
}
