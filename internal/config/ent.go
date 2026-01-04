package config

import (
	"AtoiTalkAPI/ent"
	"AtoiTalkAPI/ent/user"
	"context"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/lib/pq"
)

func InitEnt(cfg *AppConfig) *ent.Client {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName, cfg.DBPassword, cfg.DBSSLMode)

	client, err := ent.Open("postgres", dsn)
	if err != nil {
		slog.Error("failed opening connection to postgres", "error", err)
		os.Exit(1)
	}

	if cfg.DBMigrate {
		if err := client.Schema.Create(context.Background()); err != nil {
			slog.Error("failed creating schema resources", "error", err)
			os.Exit(1)
		}
		slog.Info("Database schema migrated successfully (Ent)")
	} else {
		slog.Info("Database migration skipped (DB_MIGRATE=false)")
	}

	ctx := context.Background()
	count, err := client.User.Update().
		Where(user.IsOnline(true)).
		SetIsOnline(false).
		Save(ctx)
	if err != nil {
		slog.Error("Failed to reset user online status on startup", "error", err)
	} else {
		slog.Info("Successfully reset user online status on startup", "count", count)
	}

	slog.Info("Database connected successfully")
	return client
}
