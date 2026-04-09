package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/api"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/config"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/database"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

func main() {
	if err := config.LoadLocalEnv(".env", "backend/.env"); err != nil {
		log.Fatalf("failed to load local env: %v", err)
	}

	database.InitDB()
	db := database.DB

	userRepo := repository.NewUserRepository(db)
	integrationRepo := repository.NewIntegrationRepository(db)
	activityRepo := repository.NewActivityRepository(db)
	metricRepo := repository.NewMetricRepository(db)
	emailRepo := repository.NewEmailRepository(db)

	aggregatorService := services.NewAggregatorService(userRepo, integrationRepo, activityRepo, metricRepo, log.Default())
	digestService := services.NewDigestService()
	emailService := services.NewEmailServiceFromEnv()
	authService := services.NewAuthService(
		userRepo,
		os.Getenv("GITHUB_CLIENT_ID"),
		os.Getenv("GITHUB_CLIENT_SECRET"),
		os.Getenv("GITHUB_REDIRECT_URL"),
		os.Getenv("AUTH_TOKEN_SECRET"),
	)
	schedulerService := services.NewSchedulerService(
		aggregatorService,
		digestService,
		emailService,
		userRepo,
		metricRepo,
		emailRepo,
		log.Default(),
	)

	frontendOAuthCallbackURL := os.Getenv("FRONTEND_OAUTH_CALLBACK_URL")
	router := api.NewRouter(
		userRepo,
		integrationRepo,
		metricRepo,
		authService,
		frontendOAuthCallbackURL,
		aggregatorService,
		schedulerService,
	)
	allowedOrigins := getAllowedOrigins()
	handler := api.WithCORS(router, allowedOrigins)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if os.Getenv("SCHEDULER_ENABLED") == "true" {
		tickMinutes := envInt("SCHEDULER_TICK_MINUTES", 1)
		if tickMinutes <= 0 {
			tickMinutes = 1
		}
		tickInterval := time.Duration(tickMinutes) * time.Minute
		go func() {
			if err := schedulerService.Start(ctx, tickInterval); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("scheduler stopped with error: %v", err)
			}
		}()
		log.Printf("scheduler enabled with %s tick interval", tickInterval)
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Printf("server listening on :%s", port)
	log.Printf("cors allowed origins: %s", strings.Join(allowedOrigins, ", "))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func getAllowedOrigins() []string {
	origins := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	addOrigin := func(origin string) {
		origin = strings.TrimSuffix(strings.TrimSpace(origin), "/")
		if origin == "" {
			return
		}
		if _, ok := seen[origin]; ok {
			return
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}

	addOrigin("http://localhost:5173")
	addOrigin("http://127.0.0.1:5173")
	addOrigin(originFromURL(os.Getenv("FRONTEND_OAUTH_CALLBACK_URL")))

	for _, part := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		addOrigin(part)
	}

	return origins
}

func originFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	return parsed.Scheme + "://" + parsed.Host
}
