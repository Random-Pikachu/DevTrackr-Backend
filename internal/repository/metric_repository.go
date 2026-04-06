package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type MetricRepository struct {
	db *sql.DB
}

func NewMetricRepository(db *sql.DB) *MetricRepository {
	return &MetricRepository{db: db}
}

func (r *MetricRepository) UpsertDailyMetric(
	ctx context.Context,
	metric models.DailyMetric,
) (models.DailyMetric, error) {
	query := `
		INSERT INTO daily_metrics (
			user_id,
			metric_date,
			github_commits,
			lc_easy_solved,
			lc_medium_solved,
			lc_hard_solved,
			cf_problems_solved,
			streak_days
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, metric_date)
		DO UPDATE SET
			github_commits = EXCLUDED.github_commits,
			lc_easy_solved = EXCLUDED.lc_easy_solved,
			lc_medium_solved = EXCLUDED.lc_medium_solved,
			lc_hard_solved = EXCLUDED.lc_hard_solved,
			cf_problems_solved = EXCLUDED.cf_problems_solved,
			streak_days = EXCLUDED.streak_days,
			computed_at = NOW()
		RETURNING id, computed_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		metric.UserID,
		metric.MetricDate,
		metric.GithubCommits,
		metric.LcEasySolved,
		metric.LcMediumSolved,
		metric.LcHardSolved,
		metric.CfProblemsSolved,
		metric.StreakDays,
	).Scan(&metric.ID, &metric.ComputedAt)
	if err != nil {
		return models.DailyMetric{}, err
	}

	return metric, nil
}

func (r *MetricRepository) GetDailyMetric(
	ctx context.Context,
	userID string,
	metricDate time.Time,
) (models.DailyMetric, error) {
	query := `
		SELECT id, user_id, metric_date, github_commits, lc_easy_solved, lc_medium_solved, lc_hard_solved, cf_problems_solved, streak_days, computed_at
		FROM daily_metrics
		WHERE user_id = $1 AND metric_date = $2
	`

	var metric models.DailyMetric
	err := r.db.QueryRowContext(ctx, query, userID, metricDate).Scan(
		&metric.ID,
		&metric.UserID,
		&metric.MetricDate,
		&metric.GithubCommits,
		&metric.LcEasySolved,
		&metric.LcMediumSolved,
		&metric.LcHardSolved,
		&metric.CfProblemsSolved,
		&metric.StreakDays,
		&metric.ComputedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.DailyMetric{}, errors.New("daily metric not found")
		}
		return models.DailyMetric{}, err
	}

	return metric, nil
}

func (r *MetricRepository) ListDailyMetricsByRange(
	ctx context.Context,
	userID string,
	startDate time.Time,
	endDate time.Time,
) ([]models.DailyMetric, error) {
	query := `
		SELECT id, user_id, metric_date, github_commits, lc_easy_solved, lc_medium_solved, lc_hard_solved, cf_problems_solved, streak_days, computed_at
		FROM daily_metrics
		WHERE user_id = $1 AND metric_date >= $2 AND metric_date <= $3
		ORDER BY metric_date DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.DailyMetric
	for rows.Next() {
		var metric models.DailyMetric
		err := rows.Scan(
			&metric.ID,
			&metric.UserID,
			&metric.MetricDate,
			&metric.GithubCommits,
			&metric.LcEasySolved,
			&metric.LcMediumSolved,
			&metric.LcHardSolved,
			&metric.CfProblemsSolved,
			&metric.StreakDays,
			&metric.ComputedAt,
		)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return metrics, nil
}

func (r *MetricRepository) GetMostRecentMetricBeforeDate(
	ctx context.Context,
	userID string,
	beforeDate time.Time,
) (models.DailyMetric, error) {
	query := `
		SELECT id, user_id, metric_date, github_commits, lc_easy_solved, lc_medium_solved, lc_hard_solved, cf_problems_solved, streak_days, computed_at
		FROM daily_metrics
		WHERE user_id = $1 AND metric_date < $2
		ORDER BY metric_date DESC
		LIMIT 1
	`

	var metric models.DailyMetric
	err := r.db.QueryRowContext(ctx, query, userID, beforeDate).Scan(
		&metric.ID,
		&metric.UserID,
		&metric.MetricDate,
		&metric.GithubCommits,
		&metric.LcEasySolved,
		&metric.LcMediumSolved,
		&metric.LcHardSolved,
		&metric.CfProblemsSolved,
		&metric.StreakDays,
		&metric.ComputedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.DailyMetric{}, errors.New("daily metric not found")
		}
		return models.DailyMetric{}, err
	}

	return metric, nil
}
