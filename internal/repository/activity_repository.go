package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type ActivityRepository struct {
	db *sql.DB
}

func NewActivityRepository(db *sql.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

func (r *ActivityRepository) BulkInsertActivities(ctx context.Context, activities []models.Activity) error {
	if len(activities) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO activities (
			user_id,
			integration_id,
			platform,
			activity_date,
			activity_type,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, activity := range activities {
		_, err := stmt.ExecContext(
			ctx,
			activity.UserID,
			activity.IntegrationID,
			activity.Platform,
			activity.ActivityDate,
			activity.ActivityType,
			activity.Metadata,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ActivityRepository) GetActivitiesByUserAndDate(
	ctx context.Context,
	userID string,
	activityDate time.Time,
) ([]models.Activity, error) {
	query := `
		SELECT id, user_id, integration_id, platform, activity_date, activity_type, metadata, fetched_at
		FROM activities
		WHERE user_id = $1 AND activity_date = $2
		ORDER BY fetched_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID, activityDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []models.Activity
	for rows.Next() {
		var activity models.Activity
		err := rows.Scan(
			&activity.ID,
			&activity.UserID,
			&activity.IntegrationID,
			&activity.Platform,
			&activity.ActivityDate,
			&activity.ActivityType,
			&activity.Metadata,
			&activity.FetchedAt,
		)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return activities, nil
}

func (r *ActivityRepository) DeleteActivitiesByIntegration(ctx context.Context, integrationID string) error {
	query := `
		DELETE FROM activities
		WHERE integration_id = $1
	`

	result, err := r.db.ExecContext(ctx, query, integrationID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("no activities found for integration")
	}

	return nil
}
