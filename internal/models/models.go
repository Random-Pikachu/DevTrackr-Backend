package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID      `json:"id"`
	Username         sql.NullString `json:"username,omitempty"`
	GithubHandle     sql.NullString `json:"github_handle,omitempty"`
	LeetcodeHandle   sql.NullString `json:"leetcode_handle,omitempty"`
	CodeforcesHandle sql.NullString `json:"codeforces_handle,omitempty"`
	Email            string         `json:"email"`
	EmailFrequency   string         `json:"email_frequency,omitempty"`
	Timezone         string         `json:"timezone,omitempty"`
	DigestTime       string         `json:"digest_time,omitempty"`
	EmailOptIn       bool           `json:"email_opt_in,omitempty"`
	ProfilePublic    bool           `json:"profile_public,omitempty"`
	PublicSlug       sql.NullString `json:"public_slug,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type Integration struct {
	ID           uuid.UUID      `json:"id"`
	UserID       uuid.UUID      `json:"user_id"`
	Platform     string         `json:"platform"`
	Handle       string         `json:"handle"`
	AccessToken  sql.NullString `json:"access_token,omitempty"`
	IsActive     bool           `json:"is_active,omitempty"`
	LastSyncedAt sql.NullString `json:"last_synced_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type Activity struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	IntegrationID uuid.UUID       `json:"integration_id"`
	Platform      string          `json:"platform"`
	ActivityDate  time.Time       `json:"activity_date"`
	ActivityType  string          `json:"activity_type"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	FetchedAt     time.Time       `json:"fetched_at,omitempty"`
}

type DailyMetric struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	MetricDate       time.Time `json:"metric_date"`
	GithubCommits    int       `json:"github_commits"`
	LcEasySolved     int       `json:"lc_easy_solved"`
	LcMediumSolved   int       `json:"lc_medium_solved"`
	LcHardSolved     int       `json:"lc_hard_solved"`
	CfProblemsSolved int       `json:"cf_problems_solved"`
	StreakDays       int       `json:"streak_days"`
	ComputedAt       time.Time `json:"computed_at,omitempty"`
}

type EmailLog struct {
	ID                uuid.UUID      `json:"id"`
	UserID            uuid.UUID      `json:"user_id"`
	DigestDate        time.Time      `json:"digest_date"`
	Status            string         `json:"status"`
	ProviderMessageID sql.NullString `json:"provider_message_id"`
	SentAt            time.Time      `json:"sent_at"`
}
