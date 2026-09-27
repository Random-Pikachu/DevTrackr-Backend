package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/secrets"
)

// IntegrationRepository persists integrations. Access tokens are encrypted
// before they reach the database and decrypted on read, so callers only ever
// see plaintext in memory.
type IntegrationRepository struct {
	db     *sql.DB
	cipher *secrets.TokenCipher
}

func NewIntegrationRepository(db *sql.DB, cipher *secrets.TokenCipher) *IntegrationRepository {
	return &IntegrationRepository{db: db, cipher: cipher}
}

func (r *IntegrationRepository) sealToken(token sql.NullString) (sql.NullString, error) {
	if !token.Valid || token.String == "" {
		return sql.NullString{}, nil
	}
	if r.cipher == nil {
		return sql.NullString{}, errors.New("integration token cipher is not configured")
	}
	encrypted, err := r.cipher.Encrypt(token.String)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: encrypted, Valid: true}, nil
}

func (r *IntegrationRepository) openToken(stored sql.NullString) (sql.NullString, error) {
	if !stored.Valid || stored.String == "" {
		return sql.NullString{}, nil
	}
	if r.cipher == nil {
		if secrets.IsEncrypted(stored.String) {
			return sql.NullString{}, errors.New("integration token cipher is not configured")
		}
		return stored, nil
	}
	plaintext, err := r.cipher.Decrypt(stored.String)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: plaintext, Valid: plaintext != ""}, nil
}

func (r *IntegrationRepository) AddIntegration(ctx context.Context, integration models.Integration) (models.Integration, error) {
	storedToken, err := r.sealToken(integration.AccessToken)
	if err != nil {
		return models.Integration{}, err
	}

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
		storedToken,
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

	integration.HasToken = storedToken.Valid
	return integration, nil
}

// UpsertIntegration inserts or updates an integration. When the incoming
// AccessToken is empty the existing stored token is preserved, so re-saving a
// handle never wipes an OAuth token. A non-empty token always replaces the old one.
func (r *IntegrationRepository) UpsertIntegration(ctx context.Context, integration models.Integration) (models.Integration, error) {
	storedToken, err := r.sealToken(integration.AccessToken)
	if err != nil {
		return models.Integration{}, err
	}

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
			access_token = COALESCE(EXCLUDED.access_token, integrations.access_token),
			is_active = EXCLUDED.is_active,
			last_synced_at = NULL
		RETURNING id, created_at, last_synced_at, (access_token IS NOT NULL)
	`

	err = tx.QueryRowContext(
		ctx,
		query,
		integration.UserID,
		integration.Platform,
		integration.Handle,
		storedToken,
		integration.IsActive,
	).Scan(
		&integration.ID,
		&integration.CreatedAt,
		&integration.LastSyncedAt,
		&integration.HasToken,
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
		var storedToken sql.NullString

		err := rows.Scan(
			&integration.ID,
			&integration.UserID,
			&integration.Platform,
			&integration.Handle,
			&storedToken,
			&integration.IsActive,
			&integration.LastSyncedAt,
			&integration.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		integration.AccessToken, err = r.openToken(storedToken)
		if err != nil {
			return nil, fmt.Errorf("integration %s: %w", integration.ID, err)
		}
		integration.HasToken = integration.AccessToken.Valid

		integrations = append(integrations, integration)
	}

	if err := rows.Err(); err != nil {
		return nil, err
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

// ReencryptLegacyTokens encrypts every access token that is still stored as
// plaintext. It returns the number of rows updated.
func (r *IntegrationRepository) ReencryptLegacyTokens(ctx context.Context) (int, error) {
	if r.cipher == nil {
		return 0, errors.New("integration token cipher is not configured")
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, access_token
		FROM integrations
		WHERE access_token IS NOT NULL AND access_token <> '' AND access_token NOT LIKE 'v1:%'
	`)
	if err != nil {
		return 0, err
	}

	type legacyRow struct {
		id    string
		token string
	}
	var legacy []legacyRow
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.id, &row.token); err != nil {
			rows.Close()
			return 0, err
		}
		legacy = append(legacy, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	updated := 0
	for _, row := range legacy {
		encrypted, err := r.cipher.Encrypt(row.token)
		if err != nil {
			return updated, err
		}
		if _, err := r.db.ExecContext(ctx, `UPDATE integrations SET access_token = $1 WHERE id = $2`, encrypted, row.id); err != nil {
			return updated, fmt.Errorf("integration %s: %w", row.id, err)
		}
		updated++
	}

	return updated, nil
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
