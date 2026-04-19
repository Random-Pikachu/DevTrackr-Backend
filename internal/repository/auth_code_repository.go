package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type AuthCodeRepository struct {
	db *sql.DB
}

func NewAuthCodeRepository(db *sql.DB) *AuthCodeRepository {
	return &AuthCodeRepository{db: db}
}

func (r *AuthCodeRepository) CreateAuthCode(ctx context.Context, authCode models.AuthCode) (models.AuthCode, error) {
	query := `
		INSERT INTO auth_codes (user_id, email, purpose, code_hash, attempts, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, used_at, created_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		authCode.UserID,
		authCode.Email,
		authCode.Purpose,
		authCode.CodeHash,
		authCode.Attempts,
		authCode.ExpiresAt,
	).Scan(&authCode.ID, &authCode.UsedAt, &authCode.CreatedAt)
	if err != nil {
		return models.AuthCode{}, err
	}

	return authCode, nil
}

func (r *AuthCodeRepository) GetLatestActiveAuthCode(ctx context.Context, email string, purpose string) (models.AuthCode, error) {
	query := `
		SELECT id, user_id, email, purpose, code_hash, attempts, expires_at, used_at, created_at
		FROM auth_codes
		WHERE email = $1
		  AND purpose = $2
		  AND used_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	var authCode models.AuthCode
	err := r.db.QueryRowContext(ctx, query, email, purpose).Scan(
		&authCode.ID,
		&authCode.UserID,
		&authCode.Email,
		&authCode.Purpose,
		&authCode.CodeHash,
		&authCode.Attempts,
		&authCode.ExpiresAt,
		&authCode.UsedAt,
		&authCode.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.AuthCode{}, errors.New("auth code not found")
		}
		return models.AuthCode{}, err
	}

	return authCode, nil
}

func (r *AuthCodeRepository) MarkAuthCodeUsed(ctx context.Context, authCodeID string) error {
	query := `
		UPDATE auth_codes
		SET used_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, authCodeID)
	return err
}

func (r *AuthCodeRepository) IncrementAuthCodeAttempts(ctx context.Context, authCodeID string) error {
	query := `
		UPDATE auth_codes
		SET attempts = attempts + 1
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, authCodeID)
	return err
}

func (r *AuthCodeRepository) InvalidateActiveAuthCodes(ctx context.Context, email string, purpose string) error {
	query := `
		UPDATE auth_codes
		SET used_at = NOW()
		WHERE email = $1
		  AND purpose = $2
		  AND used_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, email, purpose)
	return err
}
