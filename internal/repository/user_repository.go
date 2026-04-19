package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/lib/pq"
)

var ErrUsernameTaken = errors.New("username already taken")
var ErrEmailTaken = errors.New("email already taken")

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user models.User) (models.User, error) {
	query := `
		INSERT INTO users (username, email, email_frequency, timezone, digest_time, email_opt_in, profile_public)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, ''), '22:00'), $6, $7)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		user.Username,       //$1
		user.Email,          //$2
		user.EmailFrequency, //$3
		user.Timezone,       //$4
		user.DigestTime,     //$5
		user.EmailOptIn,     //$6
		user.ProfilePublic,  //$7
	).Scan(
		&user.ID,        // Returning id
		&user.CreatedAt, //Returning created_at
		&user.UpdatedAt, //Returning updated_at
	)

	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if pqErr.Constraint == "users_email_key" {
				return models.User{}, ErrEmailTaken
			}
			if pqErr.Constraint == "users_username_key" || pqErr.Constraint == "idx_users_username_unique" {
				return models.User{}, ErrUsernameTaken
			}
			return models.User{}, ErrUsernameTaken
		}
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	query := `
        SELECT id, username, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, github_handle, leetcode_handle, codeforces_handle, public_slug
        FROM users 
        WHERE email = $1
    `
	var user models.User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
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

func (r *UserRepository) GetUserByID(ctx context.Context, userID string) (models.User, error) {
	query := `
        SELECT id, username, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, github_handle, leetcode_handle, codeforces_handle, public_slug, created_at, updated_at
        FROM users
        WHERE id = $1
    `
	var user models.User
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
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
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, errors.New("user not found")
		}
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (models.User, error) {
	query := `
        SELECT id, username, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, github_handle, leetcode_handle, codeforces_handle, public_slug
        FROM users 
        WHERE username = $1
    `
	var user models.User
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
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
			return models.User{}, errors.New("user not found")
		}
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserAuthByUsername(ctx context.Context, username string) (models.User, string, error) {
	query := `
        SELECT id, username, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, github_handle, leetcode_handle, codeforces_handle, public_slug, password_hash
        FROM users
        WHERE username = $1
    `

	var user models.User
	var passwordHash sql.NullString
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
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
		&passwordHash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, "", errors.New("user not found")
		}
		return models.User{}, "", err
	}

	if !passwordHash.Valid || passwordHash.String == "" {
		return models.User{}, "", errors.New("password not set")
	}

	return user, passwordHash.String, nil
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

func (r *UserRepository) UpdateUsername(ctx context.Context, userId string, username string) error {
	query := `
		UPDATE users
		SET username = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, username, userId)
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

func (r *UserRepository) UpdateGithubHandle(ctx context.Context, userId string, githubHandle string) error {
	query := `
		UPDATE users
		SET github_handle = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, githubHandle, userId)
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

func (r *UserRepository) SetPasswordHash(ctx context.Context, userId string, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, passwordHash, userId)
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
		SELECT id, username, github_handle, leetcode_handle, codeforces_handle, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, public_slug, created_at, updated_at
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
			&user.Username,
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
		SELECT id, username, github_handle, leetcode_handle, codeforces_handle, email, email_frequency, timezone, digest_time, email_opt_in, profile_public, public_slug, created_at, updated_at
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
			&user.Username,
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

func (r *UserRepository) DeleteActivitiesAndMetricsByUserID(ctx context.Context, userID string) (int64, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	activitiesResult, err := tx.ExecContext(ctx, `
		DELETE FROM activities
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return 0, 0, err
	}

	activitiesDeleted, err := activitiesResult.RowsAffected()
	if err != nil {
		return 0, 0, err
	}

	metricsResult, err := tx.ExecContext(ctx, `
		DELETE FROM daily_metrics
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return 0, 0, err
	}

	metricsDeleted, err := metricsResult.RowsAffected()
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	return activitiesDeleted, metricsDeleted, nil
}
