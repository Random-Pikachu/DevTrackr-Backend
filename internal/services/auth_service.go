package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type AuthUserRepository interface {
	GetUserByEmail(ctx context.Context, email string) (models.User, error)
	CreateUser(ctx context.Context, user models.User) (models.User, error)
	UpdateGithubHandle(ctx context.Context, userId string, githubHandle string) error
}

type AuthIntegrationRepository interface {
	UpsertIntegration(ctx context.Context, integration models.Integration) (models.Integration, error)
}

type AuthService struct {
	userRepo        AuthUserRepository
	integrationRepo AuthIntegrationRepository
	oauthConfig     *oauth2.Config
	httpClient      *http.Client
	tokenSecret     string
}

func NewAuthService(
	userRepo AuthUserRepository,
	integrationRepo AuthIntegrationRepository,
	clientID string,
	clientSecret string,
	redirectURL string,
	tokenSecret string,
) *AuthService {
	return &AuthService{
		userRepo:        userRepo,
		integrationRepo: integrationRepo,
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

func (s *AuthService) GetGitHubAuthURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state)
}

func (s *AuthService) HandleGitHubCallback(ctx context.Context, code string) (models.User, string, error) {
	if !s.IsConfigured() {
		return models.User{}, "", errors.New("github oauth is not configured")
	}

	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return models.User{}, "", fmt.Errorf("failed to exchange oauth code: %w", err)
	}

	profile, err := s.fetchGitHubProfile(ctx, token.AccessToken)
	if err != nil {
		return models.User{}, "", err
	}
	if profile.Email == "" {
		return models.User{}, "", errors.New("github account email is unavailable")
	}

	user, err := s.userRepo.GetUserByEmail(ctx, profile.Email)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return models.User{}, "", err
		}
		user, err = s.userRepo.CreateUser(ctx, models.User{
			Email:          profile.Email,
			EmailFrequency: "daily",
			Timezone:       defaultDigestTimezone,
			DigestTime:     "20:00",
			EmailOptIn:     true,
			ProfilePublic:  false,
		})
		if err != nil {
			return models.User{}, "", fmt.Errorf("failed to create github oauth user: %w", err)
		}
	}

	if err := s.userRepo.UpdateGithubHandle(ctx, user.ID.String(), profile.Login); err != nil {
		return models.User{}, "", err
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
		return models.User{}, "", fmt.Errorf("failed to upsert github integration: %w", err)
	}

	authToken, err := s.generateSignedToken(user)
	if err != nil {
		return models.User{}, "", err
	}

	return user, authToken, nil
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
