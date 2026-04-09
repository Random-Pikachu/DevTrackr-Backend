package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/collectors"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/config"
	_ "github.com/lib/pq"
)

type integrationRow struct {
	Platform    string
	Handle      string
	AccessToken sql.NullString
	IsActive    bool
}

func main() {
	userID := os.Getenv("DIAG_USER_ID")
	if userID == "" {
		userID = "2b105110-ba8b-4d7a-8b82-4beccf39425a"
	}

	targetDate := time.Now().UTC().Truncate(24 * time.Hour)
	if raw := os.Getenv("DIAG_DATE"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			log.Fatalf("invalid DIAG_DATE %q: %v", raw, err)
		}
		targetDate = parsed.UTC().Truncate(24 * time.Hour)
	}

	if err := config.LoadLocalEnv(".env", "backend/.env"); err != nil {
		log.Fatalf("failed to load env: %v", err)
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSL_MODE"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("failed to ping db: %v", err)
	}

	fmt.Printf("diagnosing user=%s date=%s\n", userID, targetDate.Format("2006-01-02"))

	printStoredState(ctx, db, userID, targetDate)

	rows, err := db.QueryContext(ctx, `
		SELECT platform, handle, access_token, is_active
		FROM integrations
		WHERE user_id = $1
		ORDER BY platform
	`, userID)
	if err != nil {
		log.Fatalf("failed to query integrations: %v", err)
	}
	defer rows.Close()

	var integrations []integrationRow
	for rows.Next() {
		var row integrationRow
		if err := rows.Scan(&row.Platform, &row.Handle, &row.AccessToken, &row.IsActive); err != nil {
			log.Fatalf("failed to scan integration: %v", err)
		}
		integrations = append(integrations, row)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("failed iterating integrations: %v", err)
	}

	if len(integrations) == 0 {
		fmt.Println("no integrations found for user")
		return
	}

	fmt.Printf("found %d integrations\n", len(integrations))
	for _, integration := range integrations {
		token := ""
		if integration.AccessToken.Valid {
			token = integration.AccessToken.String
		}

		fmt.Printf("\nplatform=%s handle=%s active=%t token=%t\n",
			integration.Platform,
			integration.Handle,
			integration.IsActive,
			token != "",
		)

		if !integration.IsActive {
			fmt.Println("skipping inactive integration")
			continue
		}

		collector, err := collectors.GetCollector(integration.Platform, token)
		if err != nil {
			fmt.Printf("collector init failed: %v\n", err)
			continue
		}

		valid, err := collector.ValidateHandle(integration.Handle)
		if err != nil {
			fmt.Printf("handle validation failed: %v\n", err)
		} else {
			fmt.Printf("handle_valid=%t\n", valid)
		}

		activities, err := collector.FetchDailyActivity(integration.Handle, targetDate)
		if err != nil {
			fmt.Printf("collector fetch failed: %v\n", err)
			continue
		}

		fmt.Printf("activities=%d\n", len(activities))
		for i, activity := range activities {
			fmt.Printf("  %d. platform=%s type=%s metadata=%s\n",
				i+1,
				activity.Platform,
				activity.ActivityType,
				formatMetadata(activity.Metadata),
			)
		}
	}
}

func formatMetadata(metadata map[string]interface{}) string {
	parts := make([]string, 0, len(metadata))
	for key, value := range metadata {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ", ")
}

func printStoredState(ctx context.Context, db *sql.DB, userID string, targetDate time.Time) {
	var activityCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM activities
		WHERE user_id = $1 AND activity_date = $2
	`, userID, targetDate).Scan(&activityCount); err != nil {
		log.Fatalf("failed to count activities: %v", err)
	}

	var githubCommits, lcEasy, lcMedium, lcHard, cfSolved, streak int
	err := db.QueryRowContext(ctx, `
		SELECT github_commits, lc_easy_solved, lc_medium_solved, lc_hard_solved, cf_problems_solved, streak_days
		FROM daily_metrics
		WHERE user_id = $1 AND metric_date = $2
	`, userID, targetDate).Scan(&githubCommits, &lcEasy, &lcMedium, &lcHard, &cfSolved, &streak)
	switch {
	case err == sql.ErrNoRows:
		fmt.Printf("stored_state activities=%d daily_metric=missing\n", activityCount)
	case err != nil:
		log.Fatalf("failed to query daily metrics: %v", err)
	default:
		fmt.Printf(
			"stored_state activities=%d daily_metric github=%d lc_easy=%d lc_medium=%d lc_hard=%d cf=%d streak=%d\n",
			activityCount,
			githubCommits,
			lcEasy,
			lcMedium,
			lcHard,
			cfSolved,
			streak,
		)
	}
}
