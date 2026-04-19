package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

type AuthHandler struct {
	authService              *services.AuthService
	frontendOAuthCallbackURL string
}

func NewAuthHandler(authService *services.AuthService, frontendOAuthCallbackURL string) *AuthHandler {
	return &AuthHandler{
		authService:              authService,
		frontendOAuthCallbackURL: frontendOAuthCallbackURL,
	}
}

func (h *AuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	if !h.authService.IsConfigured() {
		writeError(w, http.StatusInternalServerError, "github oauth is not configured")
		return
	}

	state, err := generateStateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create oauth state")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "github_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	http.Redirect(w, r, h.authService.GetGitHubAuthURL(state), http.StatusTemporaryRedirect)
}

func (h *AuthHandler) RegisterWithPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)

	if req.Email == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email, username and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, token, err := h.authService.RegisterWithPassword(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		switch err {
		case repository.ErrEmailTaken:
			writeError(w, http.StatusConflict, "email already taken")
			return
		case repository.ErrUsernameTaken:
			writeError(w, http.StatusConflict, "username already taken")
			return
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "devtrackr_auth",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{"token": token, "user": user, "is_new_user": true})
}

func (h *AuthHandler) LoginWithPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, token, err := h.authService.LoginWithPassword(r.Context(), req.Username, req.Password)
	if err != nil {
		if err == services.ErrInvalidCredentials {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "devtrackr_auth",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "is_new_user": false})
}

func (h *AuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	if !h.authService.IsConfigured() {
		writeError(w, http.StatusInternalServerError, "github oauth is not configured")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	stateCookie, err := r.Cookie("github_oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != state {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	user, token, isNewUser, err := h.authService.HandleGitHubCallback(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "github_oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "devtrackr_auth",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	if h.frontendOAuthCallbackURL != "" {
		callbackURL, err := url.Parse(h.frontendOAuthCallbackURL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "invalid frontend oauth callback url")
			return
		}
		params := callbackURL.Query()
		params.Set("token", token)
		params.Set("user_id", user.ID.String())
		if user.GithubHandle.Valid {
			params.Set("login", user.GithubHandle.String)
		}
		params.Set("email", user.Email)
		if isNewUser {
			params.Set("is_new_user", "true")
		} else {
			params.Set("is_new_user", "false")
		}
		callbackURL.RawQuery = params.Encode()

		http.Redirect(w, r, callbackURL.String(), http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "is_new_user": isNewUser})
}

func generateStateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
