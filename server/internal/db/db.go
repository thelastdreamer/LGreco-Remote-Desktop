package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"github.com/lgreco/remote-desktop/server/internal/config"
)

var DB *sql.DB

func Connect(cfg *config.Config) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)
	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(10)
	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping db: %w", err)
	}
	log.Println("connected to database")
	return nil
}

func RunMigrations() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(64) NOT NULL UNIQUE,
			email VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type VARCHAR(32) NOT NULL DEFAULT 'desktop',
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			container_id VARCHAR(128),
			container_name VARCHAR(128),
			signaling_key VARCHAR(128) NOT NULL,
			resolution VARCHAR(16) NOT NULL DEFAULT '1280x720',
			audio_enabled BOOLEAN NOT NULL DEFAULT true,
			clipboard_sync BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_signaling_key ON sessions(signaling_key)`,
	}
	for _, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nsql: %s", err, m)
		}
	}
	log.Println("migrations completed")
	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
