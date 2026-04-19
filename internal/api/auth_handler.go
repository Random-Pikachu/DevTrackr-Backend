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
		Password string `json:"password"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, token, err := h.authService.RegisterWithPassword(r.Context(), req.Email, req.Password)
	if err != nil {
		switch err {
		case repository.ErrEmailTaken:
			writeError(w, http.StatusConflict, "email already taken")
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

	writeJSON(w, http.StatusCreated, map[string]interface{}{"token": token, "user": user, "is_new_user": true, "password_set": true})
}

func (h *AuthHandler) LoginWithPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Identifier string `json:"identifier"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Identifier = strings.TrimSpace(req.Identifier)
	req.Username = strings.TrimSpace(req.Username)
	identifier := req.Identifier
	if identifier == "" {
		identifier = req.Username
	}

	if identifier == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "identifier (email or username) and password are required")
		return
	}

	user, token, err := h.authService.LoginWithPassword(r.Context(), identifier, req.Password)
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

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "is_new_user": false, "password_set": true})
}

func (h *AuthHandler) RequestPasswordSetupCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	if err := h.authService.RequestPasswordSetupCode(r.Context(), req.Email); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "code_sent",
		"message": "If the account exists, a verification code has been sent.",
	})
}

func (h *AuthHandler) ConfirmPasswordSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
		Username    string `json:"username"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Code = strings.TrimSpace(req.Code)
	req.Username = strings.TrimSpace(req.Username)
	if req.Email == "" || req.Code == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "email, code and new_password are required")
		return
	}

	user, token, err := h.authService.ConfirmPasswordSetup(r.Context(), req.Email, req.Code, req.NewPassword, req.Username)
	if err != nil {
		switch err {
		case services.ErrInvalidVerificationCode, services.ErrVerificationCodeExpired:
			writeError(w, http.StatusBadRequest, err.Error())
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

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "is_new_user": false, "password_set": true})
}

func (h *AuthHandler) RequestForgotPasswordCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	if err := h.authService.RequestForgotPasswordCode(r.Context(), req.Email); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "code_sent",
		"message": "If the account exists, a verification code has been sent.",
	})
}

func (h *AuthHandler) ConfirmForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}

	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Code = strings.TrimSpace(req.Code)
	if req.Email == "" || req.Code == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "email, code and new_password are required")
		return
	}

	user, token, err := h.authService.ConfirmForgotPassword(r.Context(), req.Email, req.Code, req.NewPassword)
	if err != nil {
		switch err {
		case services.ErrInvalidVerificationCode, services.ErrVerificationCodeExpired:
			writeError(w, http.StatusBadRequest, err.Error())
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

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "is_new_user": false, "password_set": true})
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

	passwordSet, err := h.authService.IsPasswordSet(r.Context(), user.ID.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		if passwordSet {
			params.Set("password_set", "true")
		} else {
			params.Set("password_set", "false")
		}
		callbackURL.RawQuery = params.Encode()

		http.Redirect(w, r, callbackURL.String(), http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "is_new_user": isNewUser, "password_set": passwordSet})
}

func generateStateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
