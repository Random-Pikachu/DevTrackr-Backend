package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user models.User) (models.User, error) {
	query := `
		INSERT INTO users (email, email_frequency, timezone, digest_time, email_opt_in, profile_public)
		VALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), '20:00'), $5, $6)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		user.Email,          //$1
		user.EmailFrequency, //$2
		user.Timezone,       //$3
		user.DigestTime,     //$4
		user.EmailOptIn,     //$5
		user.ProfilePublic,  //$6
	).Scan(
		&user.ID,        // Returning id
		&user.CreatedAt, //Returning created_at
		&user.UpdatedAt, //Returning updated_at
	)

	if err != nil {
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	query := `
        SELECT id, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, github_handle, leetcode_handle, codeforces_handle, public_slug
        FROM users 
        WHERE email = $1
    `
	var user models.User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.EmailFrequency,
		&user.Timezone,
		&user.DigestTime,
		&user.EmailOptIn,
		&user.ProfilePublic,
		&user.GithubHandle,
		&user.LeetcodeHandle,
		&user.CodeforcesHandle,
		&user.PublicSlug,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, errors.New("user not found") // user not exists
		}
		return models.User{}, err //broken connection
	}

	return user, nil

}

func (r *UserRepository) UpdateDigestTime(ctx context.Context, userId string, digestTime string) error {
	query := `
		UPDATE users
		SET digest_time = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, digestTime, userId)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("User not found")
	}
	return nil
}

func (r *UserRepository) UpdateEmailOptIn(ctx context.Context, userId string, emailOptIn bool) error {
	query := `
		UPDATE users
		SET email_opt_in = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, emailOptIn, userId)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("User not found")
	}
	return nil
}

func (r *UserRepository) UpdatePublicProfile(ctx context.Context, userId string, profilePublic bool) error {
	query := `
		UPDATE users
		SET profile_public = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, profilePublic, userId)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("User not found")
	}
	return nil
}

func (r *UserRepository) GetDigestEligibleUsers(ctx context.Context) ([]models.User, error) {
	query := `
		SELECT id, github_handle, leetcode_handle, codeforces_handle, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, public_slug, created_at, updated_at
		FROM users
		WHERE email_opt_in = true
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID,
			&user.GithubHandle,
			&user.LeetcodeHandle,
			&user.CodeforcesHandle,
			&user.Email,
			&user.EmailFrequency,
			&user.Timezone,
			&user.DigestTime,
			&user.EmailOptIn,
			&user.ProfilePublic,
			&user.PublicSlug,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *UserRepository) GetAllUsers(ctx context.Context) ([]models.User, error) {
	query := `
		SELECT id, github_handle, leetcode_handle, codeforces_handle, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, public_slug, created_at, updated_at
		FROM users
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID,
			&user.GithubHandle,
			&user.LeetcodeHandle,
			&user.CodeforcesHandle,
			&user.Email,
			&user.EmailFrequency,
			&user.Timezone,
			&user.DigestTime,
			&user.EmailOptIn,
			&user.ProfilePublic,
			&user.PublicSlug,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}
