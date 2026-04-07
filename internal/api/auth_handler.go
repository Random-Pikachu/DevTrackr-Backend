package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"time"

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

	user, token, err := h.authService.HandleGitHubCallback(r.Context(), code)
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
		callbackURL.RawQuery = params.Encode()

		http.Redirect(w, r, callbackURL.String(), http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user})
}

func generateStateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
