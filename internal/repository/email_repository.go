package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type EmailRepository struct {
	db *sql.DB
}

func NewEmailRepository(db *sql.DB) *EmailRepository {
	return &EmailRepository{db: db}
}

func (r *EmailRepository) CreateEmailLog(ctx context.Context, log models.EmailLog) (models.EmailLog, error) {
	query := `
		INSERT INTO email_logs (user_id, digest_date, status, provider_message_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, sent_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		log.UserID,
		log.DigestDate,
		log.Status,
		log.ProviderMessageID,
	).Scan(&log.ID, &log.SentAt)
	if err != nil {
		return models.EmailLog{}, err
	}

	return log, nil
}

func (r *EmailRepository) HasDigestBeenSent(
	ctx context.Context,
	userID string,
	digestDate time.Time,
) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM email_logs
			WHERE user_id = $1
				AND digest_date = $2
				AND status = 'sent'
		)
	`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, digestDate).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *EmailRepository) ListEmailLogsByUser(ctx context.Context, userID string) ([]models.EmailLog, error) {
	query := `
		SELECT id, user_id, digest_date, status, provider_message_id, sent_at
		FROM email_logs
		WHERE user_id = $1
		ORDER BY sent_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.EmailLog
	for rows.Next() {
		var log models.EmailLog
		err := rows.Scan(
			&log.ID,
			&log.UserID,
			&log.DigestDate,
			&log.Status,
			&log.ProviderMessageID,
			&log.SentAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}
