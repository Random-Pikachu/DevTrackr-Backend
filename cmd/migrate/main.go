package main

import (
	"fmt"
	"log"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/config"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/database"
)

func main() {
	if err := config.LoadLocalEnv(".env", "backend/.env"); err != nil {
		log.Fatalf("failed to load local env: %v", err)
	}

	database.InitDB()
	db := database.DB

	fmt.Println("Running database migrations...")

	schema := `
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		username VARCHAR(255) UNIQUE,
		github_handle VARCHAR(255),
		leetcode_handle VARCHAR(255),
		codeforces_handle VARCHAR(255),
		email VARCHAR(255) UNIQUE NOT NULL,
		email_frequency VARCHAR(50) DEFAULT 'daily',
		timezone VARCHAR(100) DEFAULT 'UTC',
		digest_time VARCHAR(5) DEFAULT '23:30',
		email_opt_in BOOLEAN DEFAULT TRUE,
		profile_public BOOLEAN DEFAULT FALSE,
		public_slug VARCHAR(255) UNIQUE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	ALTER TABLE users
	ADD COLUMN IF NOT EXISTS digest_time VARCHAR(5) DEFAULT '23:30';

	ALTER TABLE users
	ALTER COLUMN digest_time SET DEFAULT '23:30';

	ALTER TABLE users
	ADD COLUMN IF NOT EXISTS username VARCHAR(255);

	ALTER TABLE users
	ADD COLUMN IF NOT EXISTS password_hash TEXT;

	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_unique
		ON users(username);

	CREATE TABLE IF NOT EXISTS integrations (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID REFERENCES users(id) ON DELETE CASCADE,
		platform VARCHAR(50) NOT NULL,
		handle VARCHAR(255) NOT NULL,
		access_token VARCHAR(255),
		is_active BOOLEAN DEFAULT TRUE,
		last_synced_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, platform)
	);

	CREATE TABLE IF NOT EXISTS activities (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID REFERENCES users(id) ON DELETE CASCADE,
		integration_id UUID REFERENCES integrations(id) ON DELETE CASCADE,
		platform VARCHAR(50) NOT NULL,
		activity_date DATE NOT NULL,
		activity_type VARCHAR(100) NOT NULL,
		metadata JSONB,
		fetched_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS daily_metrics (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID REFERENCES users(id) ON DELETE CASCADE,
		metric_date DATE NOT NULL,
		github_commits INT DEFAULT 0,
		lc_easy_solved INT DEFAULT 0,
		lc_medium_solved INT DEFAULT 0,
		lc_hard_solved INT DEFAULT 0,
		cf_problems_solved INT DEFAULT 0,
		streak_days INT DEFAULT 0,
		computed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, metric_date)
	);

	CREATE TABLE IF NOT EXISTS email_logs (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID REFERENCES users(id) ON DELETE CASCADE,
		digest_date DATE NOT NULL,
		status VARCHAR(50) NOT NULL,
		provider_message_id VARCHAR(255),
		sent_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS auth_codes (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID REFERENCES users(id) ON DELETE CASCADE,
		email VARCHAR(255) NOT NULL,
		purpose VARCHAR(50) NOT NULL,
		code_hash VARCHAR(255) NOT NULL,
		attempts INT NOT NULL DEFAULT 0,
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		used_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_activities_user_date
    ON activities(user_id, activity_date);

	CREATE INDEX IF NOT EXISTS idx_daily_metrics_user_date
		ON daily_metrics(user_id, metric_date);

	CREATE INDEX IF NOT EXISTS idx_integrations_user
		ON integrations(user_id);

	CREATE INDEX IF NOT EXISTS idx_auth_codes_email_purpose
		ON auth_codes(email, purpose, created_at DESC);

	CREATE INDEX IF NOT EXISTS idx_auth_codes_user
		ON auth_codes(user_id);
	`

	_, err := db.Exec(schema)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	fmt.Println("All tables created successfully!")
}
