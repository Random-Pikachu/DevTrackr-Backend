package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type AuthUserRepository interface {
	GetUserByEmail(ctx context.Context, email string) (models.User, error)
	GetUserAuthByUsername(ctx context.Context, username string) (models.User, string, error)
	GetUserAuthByIdentifier(ctx context.Context, identifier string) (models.User, string, error)
	IsPasswordSet(ctx context.Context, userID string) (bool, error)
	CreateUser(ctx context.Context, user models.User) (models.User, error)
	UpdateUsername(ctx context.Context, userId string, username string) error
	UpdateGithubHandle(ctx context.Context, userId string, githubHandle string) error
	SetPasswordHash(ctx context.Context, userId string, passwordHash string) error
}

var ErrInvalidCredentials = errors.New("invalid username or password")
var ErrInvalidVerificationCode = errors.New("invalid verification code")
var ErrVerificationCodeExpired = errors.New("verification code expired")

const (
	authPurposePasswordSetup = "password_setup"
	authPurposePasswordReset = "password_reset"
	authCodeLength           = 6
	authCodeExpiryMinutes    = 15
	authCodeMaxAttempts      = 5
)

type AuthIntegrationRepository interface {
	UpsertIntegration(ctx context.Context, integration models.Integration) (models.Integration, error)
}

type AuthCodeRepository interface {
	CreateAuthCode(ctx context.Context, authCode models.AuthCode) (models.AuthCode, error)
	GetLatestActiveAuthCode(ctx context.Context, email string, purpose string) (models.AuthCode, error)
	MarkAuthCodeUsed(ctx context.Context, authCodeID string) error
	IncrementAuthCodeAttempts(ctx context.Context, authCodeID string) error
	InvalidateActiveAuthCodes(ctx context.Context, email string, purpose string) error
}

type AuthService struct {
	userRepo        AuthUserRepository
	integrationRepo AuthIntegrationRepository
	authCodeRepo    AuthCodeRepository
	emailSender     EmailSender
	oauthConfig     *oauth2.Config
	httpClient      *http.Client
	tokenSecret     string
}

func NewAuthService(
	userRepo AuthUserRepository,
	integrationRepo AuthIntegrationRepository,
	authCodeRepo AuthCodeRepository,
	emailSender EmailSender,
	clientID string,
	clientSecret string,
	redirectURL string,
	tokenSecret string,
) *AuthService {
	return &AuthService{
		userRepo:        userRepo,
		integrationRepo: integrationRepo,
		authCodeRepo:    authCodeRepo,
		emailSender:     emailSender,
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		},
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		tokenSecret: tokenSecret,
	}
}

func (s *AuthService) IsConfigured() bool {
	return s.oauthConfig.ClientID != "" &&
		s.oauthConfig.ClientSecret != "" &&
		s.oauthConfig.RedirectURL != "" &&
		s.tokenSecret != ""
}

func (s *AuthService) isTokenConfigured() bool {
	return s.tokenSecret != ""
}

func (s *AuthService) GetGitHubAuthURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state)
}

func (s *AuthService) HandleGitHubCallback(ctx context.Context, code string) (models.User, string, bool, error) {
	if !s.IsConfigured() {
		return models.User{}, "", false, errors.New("github oauth is not configured")
	}

	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return models.User{}, "", false, fmt.Errorf("failed to exchange oauth code: %w", err)
	}

	profile, err := s.fetchGitHubProfile(ctx, token.AccessToken)
	if err != nil {
		return models.User{}, "", false, err
	}
	if profile.Email == "" {
		return models.User{}, "", false, errors.New("github account email is unavailable")
	}

	isNewUser := false
	user, err := s.userRepo.GetUserByEmail(ctx, profile.Email)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return models.User{}, "", false, err
		}
		isNewUser = true
		user, err = s.userRepo.CreateUser(ctx, models.User{
			Email:          profile.Email,
			EmailFrequency: "daily",
			Timezone:       defaultDigestTimezone,
			DigestTime:     "23:30",
			EmailOptIn:     true,
			ProfilePublic:  false,
		})
		if err != nil {
			return models.User{}, "", false, fmt.Errorf("failed to create github oauth user: %w", err)
		}
	}

	if err := s.userRepo.UpdateGithubHandle(ctx, user.ID.String(), profile.Login); err != nil {
		return models.User{}, "", false, err
	}
	user.GithubHandle.String = profile.Login
	user.GithubHandle.Valid = true

	if _, err := s.integrationRepo.UpsertIntegration(ctx, models.Integration{
		UserID:      user.ID,
		Platform:    "github",
		Handle:      profile.Login,
		AccessToken: sql.NullString{String: token.AccessToken, Valid: token.AccessToken != ""},
		IsActive:    true,
	}); err != nil {
		return models.User{}, "", false, fmt.Errorf("failed to upsert github integration: %w", err)
	}

	authToken, err := s.generateSignedToken(user)
	if err != nil {
		return models.User{}, "", false, err
	}

	return user, authToken, isNewUser, nil
}

func (s *AuthService) RegisterWithPassword(ctx context.Context, email string, password string) (models.User, string, error) {
	if !s.isTokenConfigured() {
		return models.User{}, "", errors.New("token secret not configured")
	}

	if _, err := s.userRepo.GetUserByEmail(ctx, email); err == nil {
		return models.User{}, "", repository.ErrEmailTaken
	} else if !isUserNotFoundErr(err) {
		return models.User{}, "", err
	}

	passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return models.User{}, "", fmt.Errorf("failed to hash password: %w", err)
	}

	createdUser, err := s.userRepo.CreateUser(ctx, models.User{
		Email:          email,
		EmailFrequency: "daily",
		Timezone:       defaultDigestTimezone,
		DigestTime:     "23:30",
		EmailOptIn:     true,
		ProfilePublic:  false,
	})
	if err != nil {
		return models.User{}, "", err
	}

	if err := s.userRepo.SetPasswordHash(ctx, createdUser.ID.String(), string(passwordHashBytes)); err != nil {
		return models.User{}, "", err
	}

	authToken, err := s.generateSignedToken(createdUser)
	if err != nil {
		return models.User{}, "", err
	}

	return createdUser, authToken, nil
}

func (s *AuthService) LoginWithPassword(ctx context.Context, identifier string, password string) (models.User, string, error) {
	if !s.isTokenConfigured() {
		return models.User{}, "", errors.New("token secret not configured")
	}

	user, passwordHash, err := s.userRepo.GetUserAuthByIdentifier(ctx, identifier)
	if err != nil {
		if isUserNotFoundErr(err) || strings.Contains(strings.ToLower(err.Error()), "password not set") {
			return models.User{}, "", ErrInvalidCredentials
		}
		return models.User{}, "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return models.User{}, "", ErrInvalidCredentials
	}

	authToken, err := s.generateSignedToken(user)
	if err != nil {
		return models.User{}, "", err
	}

	return user, authToken, nil
}

func (s *AuthService) IsPasswordSet(ctx context.Context, userID string) (bool, error) {
	return s.userRepo.IsPasswordSet(ctx, userID)
}

func (s *AuthService) RequestPasswordSetupCode(ctx context.Context, email string) error {
	return s.requestPasswordCode(ctx, strings.TrimSpace(strings.ToLower(email)), authPurposePasswordSetup)
}

func (s *AuthService) RequestForgotPasswordCode(ctx context.Context, email string) error {
	return s.requestPasswordCode(ctx, strings.TrimSpace(strings.ToLower(email)), authPurposePasswordReset)
}

func (s *AuthService) requestPasswordCode(ctx context.Context, email string, purpose string) error {
	if email == "" {
		return errors.New("email is required")
	}
	if s.authCodeRepo == nil {
		return errors.New("auth code repository not configured")
	}
	if s.emailSender == nil {
		return errors.New("email sender not configured")
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if isUserNotFoundErr(err) {
			return nil
		}
		return err
	}

	if err := s.authCodeRepo.InvalidateActiveAuthCodes(ctx, email, purpose); err != nil {
		return err
	}

	code, err := generateVerificationCode(authCodeLength)
	if err != nil {
		return err
	}

	codeHash := s.hashAuthCode(email, purpose, code)
	_, err = s.authCodeRepo.CreateAuthCode(ctx, models.AuthCode{
		UserID:    user.ID,
		Email:     email,
		Purpose:   purpose,
		CodeHash:  codeHash,
		Attempts:  0,
		ExpiresAt: time.Now().UTC().Add(authCodeExpiryMinutes * time.Minute),
	})
	if err != nil {
		return err
	}

	subject := "DevTrackr verification code"
	html := buildVerificationCodeEmailHTML(code, purpose)
	_, err = s.emailSender.SendDigest(ctx, email, subject, html)
	return err
}

func (s *AuthService) ConfirmPasswordSetup(ctx context.Context, email string, code string, newPassword string, username string) (models.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	username = strings.TrimSpace(username)
	user, err := s.verifyAuthCode(ctx, email, authPurposePasswordSetup, code)
	if err != nil {
		return models.User{}, "", err
	}

	if username != "" {
		if err := s.userRepo.UpdateUsername(ctx, user.ID.String(), username); err != nil {
			return models.User{}, "", err
		}
		user.Username = sql.NullString{String: username, Valid: true}
	}

	if err := s.setUserPassword(ctx, user.ID.String(), newPassword); err != nil {
		return models.User{}, "", err
	}

	token, err := s.generateSignedToken(user)
	if err != nil {
		return models.User{}, "", err
	}

	return user, token, nil
}

func (s *AuthService) ConfirmForgotPassword(ctx context.Context, email string, code string, newPassword string) (models.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	user, err := s.verifyAuthCode(ctx, email, authPurposePasswordReset, code)
	if err != nil {
		return models.User{}, "", err
	}

	if err := s.setUserPassword(ctx, user.ID.String(), newPassword); err != nil {
		return models.User{}, "", err
	}

	token, err := s.generateSignedToken(user)
	if err != nil {
		return models.User{}, "", err
	}

	return user, token, nil
}

func (s *AuthService) verifyAuthCode(ctx context.Context, email string, purpose string, code string) (models.User, error) {
	if email == "" || strings.TrimSpace(code) == "" {
		return models.User{}, ErrInvalidVerificationCode
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if isUserNotFoundErr(err) {
			return models.User{}, ErrInvalidVerificationCode
		}
		return models.User{}, err
	}

	authCode, err := s.authCodeRepo.GetLatestActiveAuthCode(ctx, email, purpose)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return models.User{}, ErrInvalidVerificationCode
		}
		return models.User{}, err
	}

	if time.Now().UTC().After(authCode.ExpiresAt) {
		_ = s.authCodeRepo.MarkAuthCodeUsed(ctx, authCode.ID.String())
		return models.User{}, ErrVerificationCodeExpired
	}

	expectedHash := s.hashAuthCode(email, purpose, strings.TrimSpace(code))
	if !hmac.Equal([]byte(expectedHash), []byte(authCode.CodeHash)) {
		_ = s.authCodeRepo.IncrementAuthCodeAttempts(ctx, authCode.ID.String())
		if authCode.Attempts+1 >= authCodeMaxAttempts {
			_ = s.authCodeRepo.MarkAuthCodeUsed(ctx, authCode.ID.String())
		}
		return models.User{}, ErrInvalidVerificationCode
	}

	if err := s.authCodeRepo.MarkAuthCodeUsed(ctx, authCode.ID.String()); err != nil {
		return models.User{}, err
	}

	return user, nil
}

func (s *AuthService) setUserPassword(ctx context.Context, userID string, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("password is required")
	}
	if len(newPassword) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	return s.userRepo.SetPasswordHash(ctx, userID, string(passwordHashBytes))
}

func (s *AuthService) hashAuthCode(email string, purpose string, code string) string {
	input := strings.ToLower(strings.TrimSpace(email)) + "|" + purpose + "|" + strings.TrimSpace(code)
	mac := hmac.New(sha256.New, []byte(s.tokenSecret))
	_, _ = mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func generateVerificationCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid verification code length")
	}

	buf := strings.Builder{}
	buf.Grow(length)
	for range length {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		buf.WriteByte(byte('0' + n.Int64()))
	}

	return buf.String(), nil
}

func buildVerificationCodeEmailHTML(code string, purpose string) string {
	flowLabel := "password setup"
	if purpose == authPurposePasswordReset {
		flowLabel = "password reset"
	}

	return fmt.Sprintf("<p>Your DevTrackr %s code is:</p><h2 style=\"letter-spacing: 4px;\">%s</h2><p>This code expires in %d minutes.</p>", flowLabel, code, authCodeExpiryMinutes)
}

type githubProfile struct {
	Login string `json:"login"`
	Email string `json:"email"`
}

func (s *AuthService) fetchGitHubProfile(ctx context.Context, accessToken string) (githubProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return githubProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return githubProfile{}, fmt.Errorf("github profile request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return githubProfile{}, fmt.Errorf("github profile request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var profile githubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return githubProfile{}, err
	}

	if profile.Email == "" {
		email, err := s.fetchPrimaryGitHubEmail(ctx, accessToken)
		if err != nil {
			return githubProfile{}, err
		}
		profile.Email = email
	}

	return profile, nil
}

func (s *AuthService) fetchPrimaryGitHubEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github email request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github email request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email, nil
		}
	}
	for _, email := range emails {
		if email.Verified {
			return email.Email, nil
		}
	}

	return "", errors.New("no verified github email found")
}

func (s *AuthService) generateSignedToken(user models.User) (string, error) {
	if s.tokenSecret == "" {
		return "", errors.New("token secret not configured")
	}

	payload := map[string]interface{}{
		"user_id": user.ID.String(),
		"email":   user.Email,
		"exp":     time.Now().UTC().Add(24 * time.Hour).Unix(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(body)

	mac := hmac.New(sha256.New, []byte(s.tokenSecret))
	_, _ = mac.Write([]byte(encodedPayload))
	signature := mac.Sum(nil)
	encodedSig := base64.RawURLEncoding.EncodeToString(signature)

	return encodedPayload + "." + encodedSig, nil
}

func isUserNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
