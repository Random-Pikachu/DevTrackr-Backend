package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type IntegrationRepository struct {
	db *sql.DB
}

func NewIntegrationRepository(db *sql.DB) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

func (r *IntegrationRepository) AddIntegration(ctx context.Context, integration models.Integration) (models.Integration, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Integration{}, err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO integrations (user_id, platform, handle, access_token, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	err = tx.QueryRowContext(
		ctx,
		query,
		integration.UserID,
		integration.Platform,
		integration.Handle,
		integration.AccessToken,
		integration.IsActive,
	).Scan(
		&integration.ID,
		&integration.CreatedAt,
	)
	if err != nil {
		return models.Integration{}, err
	}

	if err := syncUserIntegrationHandleTx(ctx, tx, integration.UserID.String(), integration.Platform, integration.Handle, integration.IsActive); err != nil {
		return models.Integration{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Integration{}, err
	}

	return integration, nil
}

func (r *IntegrationRepository) UpsertIntegration(ctx context.Context, integration models.Integration) (models.Integration, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Integration{}, err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO integrations (user_id, platform, handle, access_token, is_active)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, platform)
		DO UPDATE SET
			handle = EXCLUDED.handle,
			access_token = EXCLUDED.access_token,
			is_active = EXCLUDED.is_active,
			last_synced_at = NULL
		RETURNING id, created_at, last_synced_at
	`

	err = tx.QueryRowContext(
		ctx,
		query,
		integration.UserID,
		integration.Platform,
		integration.Handle,
		integration.AccessToken,
		integration.IsActive,
	).Scan(
		&integration.ID,
		&integration.CreatedAt,
		&integration.LastSyncedAt,
	)
	if err != nil {
		return models.Integration{}, err
	}

	if err := syncUserIntegrationHandleTx(ctx, tx, integration.UserID.String(), integration.Platform, integration.Handle, integration.IsActive); err != nil {
		return models.Integration{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Integration{}, err
	}

	return integration, nil
}

func (r *IntegrationRepository) GetActiveIntegrations(ctx context.Context, userID string) ([]models.Integration, error) {
	query := `
		SELECT id, user_id, platform, handle, access_token, is_active, last_synced_at, created_at
		FROM integrations
		WHERE user_id = $1 AND is_active = true
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var integrations []models.Integration

	for rows.Next() {
		var integration models.Integration

		err := rows.Scan(
			&integration.ID,
			&integration.UserID,
			&integration.Platform,
			&integration.Handle,
			&integration.AccessToken,
			&integration.IsActive,
			&integration.LastSyncedAt,
			&integration.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		integrations = append(integrations, integration)
	}

	return integrations, nil
}

func (r *IntegrationRepository) DeactivateIntegration(ctx context.Context, integrationID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		UPDATE integrations
		SET is_active = false
		WHERE id = $1
		RETURNING user_id::text, platform
	`

	var userID string
	var platform string
	if err := tx.QueryRowContext(ctx, query, integrationID).Scan(&userID, &platform); err != nil {
		return err
	}

	if err := syncUserIntegrationHandleTx(ctx, tx, userID, platform, "", false); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func syncUserIntegrationHandleTx(ctx context.Context, tx *sql.Tx, userID, platform, handle string, isActive bool) error {
	var value sql.NullString
	if isActive && handle != "" {
		value = sql.NullString{String: handle, Valid: true}
	}

	switch strings.ToLower(platform) {
	case "github":
		_, err := tx.ExecContext(ctx, `UPDATE users SET github_handle = $1, updated_at = NOW() WHERE id = $2`, value, userID)
		return err
	case "leetcode":
		_, err := tx.ExecContext(ctx, `UPDATE users SET leetcode_handle = $1, updated_at = NOW() WHERE id = $2`, value, userID)
		return err
	case "codeforces":
		_, err := tx.ExecContext(ctx, `UPDATE users SET codeforces_handle = $1, updated_at = NOW() WHERE id = $2`, value, userID)
		return err
	default:
		return nil
	}
}
