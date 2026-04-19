package api

import (
	"net/http"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

func NewRouter(
	userRepo *repository.UserRepository,
	integrationRepo *repository.IntegrationRepository,
	activityRepo *repository.ActivityRepository,
	metricRepo *repository.MetricRepository,
	authService *services.AuthService,
	frontendOAuthCallbackURL string,
	aggregator *services.AggregatorService,
	scheduler *services.SchedulerService,
) http.Handler {
	userHandler := NewUserHandler(userRepo, integrationRepo, activityRepo, metricRepo, aggregator, scheduler)
	integrationHandler := NewIntegrationHandler(integrationRepo)
	authHandler := NewAuthHandler(authService, frontendOAuthCallbackURL)
	jobHandler := NewJobHandler(aggregator, scheduler)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /auth/github/login", authHandler.GitHubLogin)
	mux.HandleFunc("GET /auth/github/callback", authHandler.GitHubCallback)
	mux.HandleFunc("POST /auth/register", authHandler.RegisterWithPassword)
	mux.HandleFunc("POST /auth/login", authHandler.LoginWithPassword)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Welcome to DevTrackr API!"})
	})
	mux.HandleFunc("POST /users", userHandler.CreateUser)
	mux.HandleFunc("GET /users", userHandler.ListUsers)
	mux.HandleFunc("GET /users/by-email", userHandler.GetUserByEmail)
	mux.HandleFunc("GET /users/by-username", userHandler.GetUserByUsername)
	mux.HandleFunc("PATCH /users/{id}/email-opt-in", userHandler.UpdateEmailOptIn)
	mux.HandleFunc("PATCH /users/{id}/profile-public", userHandler.UpdatePublicProfile)
	mux.HandleFunc("PATCH /users/{id}/username", userHandler.UpdateUsername)
	mux.HandleFunc("PATCH /users/{id}/digest-time", userHandler.UpdateDigestTime)
	mux.HandleFunc("POST /users/{id}/aggregate", userHandler.AggregateUser)
	mux.HandleFunc("POST /users/{id}/aggregate/range", userHandler.AggregateUserRange)
	mux.HandleFunc("POST /users/{id}/send-digest", userHandler.SendDigestNow)
	mux.HandleFunc("GET /users/{id}/integrations/active", userHandler.GetActiveIntegrations)
	mux.HandleFunc("GET /users/{id}/activities", userHandler.GetActivitiesByDate)
	mux.HandleFunc("GET /users/{id}/metrics", userHandler.GetDailyMetric)
	mux.HandleFunc("GET /users/{id}/metrics/range", userHandler.GetMetricRange)
	mux.HandleFunc("GET /users/{id}/heatmap", userHandler.GetHeatmap)
	mux.HandleFunc("DELETE /users/{id}/activities&dmetrics", userHandler.DeleteActivitiesAndMetrics)

	mux.HandleFunc("POST /integrations", integrationHandler.AddIntegration)
	mux.HandleFunc("DELETE /integrations/{id}", integrationHandler.DeactivateIntegration)

	mux.HandleFunc("POST /jobs/aggregate", jobHandler.RunAggregation)
	mux.HandleFunc("POST /jobs/aggregate/backfill-2026", jobHandler.RunAggregationBackfill2026)
	mux.HandleFunc("POST /jobs/nightly", jobHandler.RunNightly)
	mux.HandleFunc("POST /jobs/digest", jobHandler.SendDigestAllForDate)
	mux.HandleFunc("POST /jobs/digest/{id}", jobHandler.SendDigestUserForDate)

	return mux
}
