package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

const defaultUserTimezone = "Asia/Kolkata"

type UserHandler struct {
	userRepo        *repository.UserRepository
	integrationRepo *repository.IntegrationRepository
	activityRepo    *repository.ActivityRepository
	metricRepo      *repository.MetricRepository
	aggregator      *services.AggregatorService
	scheduler       *services.SchedulerService
}

func NewUserHandler(
	userRepo *repository.UserRepository,
	integrationRepo *repository.IntegrationRepository,
	activityRepo *repository.ActivityRepository,
	metricRepo *repository.MetricRepository,
	aggregator *services.AggregatorService,
	scheduler *services.SchedulerService,
) *UserHandler {
	return &UserHandler{
		userRepo:        userRepo,
		integrationRepo: integrationRepo,
		activityRepo:    activityRepo,
		metricRepo:      metricRepo,
		aggregator:      aggregator,
		scheduler:       scheduler,
	}
}

func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username       string `json:"username"`
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
		req.Timezone = defaultUserTimezone
	}
	if req.DigestTime == "" {
		req.DigestTime = "22:00"
	} else if _, err := time.Parse("15:04", req.DigestTime); err != nil {
		writeError(w, http.StatusBadRequest, "digest_time must be HH:MM (24-hour format)")
		return
	}

	user, err := h.userRepo.CreateUser(r.Context(), models.User{
		Username:       toNullString(req.Username),
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
	log.Printf("user created id=%s email=%s username=%s timezone=%s", user.ID, user.Email, req.Username, user.Timezone)

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
	log.Printf("user fetched by email email=%s user_id=%s", email, user.ID)
	writeJSON(w, http.StatusOK, user)
}

func (h *UserHandler) GetUserByUsername(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username query parameter is required")
		return
	}

	user, err := h.userRepo.GetUserByUsername(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Printf("user fetched by username username=%s user_id=%s", username, user.ID)
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
	log.Printf("user email-opt-in updated user_id=%s enabled=%t", userID, req.EmailOptIn)
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
	log.Printf("user profile-public updated user_id=%s public=%t", userID, req.ProfilePublic)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *UserHandler) UpdateUsername(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	if err := h.userRepo.UpdateUsername(r.Context(), userID, req.Username); err != nil {
		if errors.Is(err, repository.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Printf("user username updated user_id=%s username=%s", userID, req.Username)
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
	log.Printf("user digest-time updated user_id=%s digest_time=%s", userID, req.DigestTime)
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
	log.Printf("active integrations fetched user_id=%s count=%d", userID, len(integrations))
	writeJSON(w, http.StatusOK, integrations)
}

func (h *UserHandler) GetActivitiesByDate(w http.ResponseWriter, r *http.Request) {
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

	activities, err := h.activityRepo.GetActivitiesByUserAndDate(r.Context(), userID, date.UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("activities fetched user_id=%s date=%s count=%d", userID, date.Format("2006-01-02"), len(activities))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":    userID,
		"date":       date.Format("2006-01-02"),
		"activities": activities,
	})
}

func (h *UserHandler) AggregateUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	targetDate, err := parseDateParamOrDefault(r, "date", time.Now().UTC().Truncate(24*time.Hour))
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	metric, err := h.aggregateUserForDate(r.Context(), user, targetDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("user aggregation complete user_id=%s date=%s github=%d lc_easy=%d lc_medium=%d lc_hard=%d cf=%d",
		userID,
		targetDate.Format("2006-01-02"),
		metric.GithubCommits,
		metric.LcEasySolved,
		metric.LcMediumSolved,
		metric.LcHardSolved,
		metric.CfProblemsSolved,
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "aggregation_complete",
		"user_id": userID,
		"date":    targetDate.Format("2006-01-02"),
		"metric":  metric,
	})
}

func (h *UserHandler) AggregateUserRange(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	startParam := r.URL.Query().Get("start")
	endParam := r.URL.Query().Get("end")
	if startParam == "" || endParam == "" {
		writeError(w, http.StatusBadRequest, "start and end query parameters are required")
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

	startDate = normalizeDateToISTDayUTC(startDate)
	endDate = normalizeDateToISTDayUTC(endDate)
	if endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "end must be greater than or equal to start")
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	type dayResult struct {
		Date   string             `json:"date"`
		Metric models.DailyMetric `json:"metric"`
	}

	results := make([]dayResult, 0)
	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		metric, err := h.aggregateUserForDate(r.Context(), user, day)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		results = append(results, dayResult{
			Date:   day.Format("2006-01-02"),
			Metric: metric,
		})
	}

	log.Printf("user bulk aggregation complete user_id=%s start=%s end=%s days=%d",
		userID,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
		len(results),
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "bulk_aggregation_complete",
		"user_id":        userID,
		"start":          startDate.Format("2006-01-02"),
		"end":            endDate.Format("2006-01-02"),
		"days_processed": len(results),
		"results":        results,
	})
}

func (h *UserHandler) aggregateUserForDate(ctx context.Context, user models.User, targetDate time.Time) (models.DailyMetric, error) {
	metric, err := h.aggregator.AggregateUserForDate(ctx, user, targetDate)
	if err != nil {
		return models.DailyMetric{}, err
	}

	log.Printf("user aggregation complete user_id=%s date=%s github=%d lc_easy=%d lc_medium=%d lc_hard=%d cf=%d",
		user.ID,
		targetDate.Format("2006-01-02"),
		metric.GithubCommits,
		metric.LcEasySolved,
		metric.LcMediumSolved,
		metric.LcHardSolved,
		metric.CfProblemsSolved,
	)

	return metric, nil
}

func (h *UserHandler) SendDigestNow(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	targetDate, err := parseDateParamOrDefault(r, "date", currentISTDate())
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	if err := h.scheduler.SendDigestNowForUser(r.Context(), userID, targetDate); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("digest send requested user_id=%s date=%s", userID, targetDate.Format("2006-01-02"))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "digest_processed",
		"user_id": userID,
		"date":    targetDate.Format("2006-01-02"),
	})
}

func currentISTDate() time.Time {
	location, err := time.LoadLocation(defaultUserTimezone)
	if err != nil {
		return time.Now().UTC().Truncate(24 * time.Hour)
	}

	localNow := time.Now().In(location)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
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
	log.Printf("daily metric fetched user_id=%s date=%s github=%d lc_easy=%d lc_medium=%d lc_hard=%d cf=%d",
		userID,
		date.Format("2006-01-02"),
		metric.GithubCommits,
		metric.LcEasySolved,
		metric.LcMediumSolved,
		metric.LcHardSolved,
		metric.CfProblemsSolved,
	)
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
	log.Printf("metric range fetched user_id=%s start=%s end=%s count=%d", userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"), len(metrics))
	writeJSON(w, http.StatusOK, metrics)
}

func (h *UserHandler) GetHeatmap(w http.ResponseWriter, r *http.Request) {
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
	startDate = startDate.UTC().Truncate(24 * time.Hour)
	endDate = endDate.UTC().Truncate(24 * time.Hour)
	if endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "end must be greater than or equal to start")
		return
	}

	metrics, err := h.metricRepo.ListDailyMetricsByRange(r.Context(), userID, startDate, endDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	byDate := make(map[string]models.DailyMetric, len(metrics))
	for _, metric := range metrics {
		byDate[metric.MetricDate.UTC().Format("2006-01-02")] = metric
	}

	type heatmapDay struct {
		Date               string `json:"date"`
		TotalContributions int    `json:"total_contributions"`
		GithubCommits      int    `json:"github_commits"`
		LcEasySolved       int    `json:"lc_easy_solved"`
		LcMediumSolved     int    `json:"lc_medium_solved"`
		LcHardSolved       int    `json:"lc_hard_solved"`
		CfProblemsSolved   int    `json:"cf_problems_solved"`
	}

	series := make([]heatmapDay, 0)
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		metric, ok := byDate[key]
		if !ok {
			series = append(series, heatmapDay{Date: key})
			continue
		}

		total := metric.GithubCommits +
			metric.LcEasySolved +
			metric.LcMediumSolved +
			metric.LcHardSolved +
			metric.CfProblemsSolved

		series = append(series, heatmapDay{
			Date:               key,
			TotalContributions: total,
			GithubCommits:      metric.GithubCommits,
			LcEasySolved:       metric.LcEasySolved,
			LcMediumSolved:     metric.LcMediumSolved,
			LcHardSolved:       metric.LcHardSolved,
			CfProblemsSolved:   metric.CfProblemsSolved,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": userID,
		"start":   startDate.Format("2006-01-02"),
		"end":     endDate.Format("2006-01-02"),
		"days":    series,
	})
	log.Printf("heatmap fetched user_id=%s start=%s end=%s source_metrics=%d days=%d", userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"), len(metrics), len(series))
}

func toNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
