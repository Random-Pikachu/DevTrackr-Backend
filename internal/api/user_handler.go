package api

import (
	"net/http"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
)

type UserHandler struct {
	userRepo        *repository.UserRepository
	integrationRepo *repository.IntegrationRepository
	metricRepo      *repository.MetricRepository
}

func NewUserHandler(
	userRepo *repository.UserRepository,
	integrationRepo *repository.IntegrationRepository,
	metricRepo *repository.MetricRepository,
) *UserHandler {
	return &UserHandler{
		userRepo:        userRepo,
		integrationRepo: integrationRepo,
		metricRepo:      metricRepo,
	}
}

func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email          string `json:"email"`
		EmailFrequency string `json:"email_frequency"`
		Timezone       string `json:"timezone"`
		DigestTime     string `json:"digest_time"`
		EmailOptIn     bool   `json:"email_opt_in"`
		ProfilePublic  bool   `json:"profile_public"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if req.DigestTime == "" {
		req.DigestTime = "20:00"
	} else if _, err := time.Parse("15:04", req.DigestTime); err != nil {
		writeError(w, http.StatusBadRequest, "digest_time must be HH:MM (24-hour format)")
		return
	}

	user, err := h.userRepo.CreateUser(r.Context(), models.User{
		Email:          req.Email,
		EmailFrequency: req.EmailFrequency,
		Timezone:       req.Timezone,
		DigestTime:     req.DigestTime,
		EmailOptIn:     req.EmailOptIn,
		ProfilePublic:  req.ProfilePublic,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userRepo.GetAllUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *UserHandler) GetUserByEmail(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email query parameter is required")
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *UserHandler) UpdateEmailOptIn(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var req struct {
		EmailOptIn bool `json:"email_opt_in"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.userRepo.UpdateEmailOptIn(r.Context(), userID, req.EmailOptIn); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *UserHandler) UpdatePublicProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var req struct {
		ProfilePublic bool `json:"profile_public"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.userRepo.UpdatePublicProfile(r.Context(), userID, req.ProfilePublic); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *UserHandler) UpdateDigestTime(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var req struct {
		DigestTime string `json:"digest_time"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := time.Parse("15:04", req.DigestTime); err != nil {
		writeError(w, http.StatusBadRequest, "digest_time must be HH:MM (24-hour format)")
		return
	}

	if err := h.userRepo.UpdateDigestTime(r.Context(), userID, req.DigestTime); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *UserHandler) GetActiveIntegrations(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	integrations, err := h.integrationRepo.GetActiveIntegrations(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, integrations)
}

func (h *UserHandler) GetDailyMetric(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	dateParam := r.URL.Query().Get("date")
	if userID == "" || dateParam == "" {
		writeError(w, http.StatusBadRequest, "user id and date are required")
		return
	}

	date, err := time.Parse("2006-01-02", dateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	metric, err := h.metricRepo.GetDailyMetric(r.Context(), userID, date.UTC())
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metric)
}

func (h *UserHandler) GetMetricRange(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	startParam := r.URL.Query().Get("start")
	endParam := r.URL.Query().Get("end")
	if userID == "" || startParam == "" || endParam == "" {
		writeError(w, http.StatusBadRequest, "user id, start and end are required")
		return
	}

	startDate, err := time.Parse("2006-01-02", startParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "start must be YYYY-MM-DD")
		return
	}
	endDate, err := time.Parse("2006-01-02", endParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "end must be YYYY-MM-DD")
		return
	}

	metrics, err := h.metricRepo.ListDailyMetricsByRange(r.Context(), userID, startDate.UTC(), endDate.UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}
