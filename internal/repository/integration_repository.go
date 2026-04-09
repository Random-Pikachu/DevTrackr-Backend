package repository

import (
	"context"
	"database/sql"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type IntegrationRepository struct {
	db *sql.DB
}

func NewIntegrationRepository(db *sql.DB) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

func (r *IntegrationRepository) AddIntegration(ctx context.Context, integration models.Integration) (models.Integration, error) {

	query := `
		INSERT INTO integrations (user_id, platform, handle, access_token, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(
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
	return integration, err
}

func (r *IntegrationRepository) UpsertIntegration(ctx context.Context, integration models.Integration) (models.Integration, error) {
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

	err := r.db.QueryRowContext(
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

	return integration, err
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
	query := `
		UPDATE integrations
		SET is_active = false
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, integrationID)
	return err
}
