package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/api"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/database"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

func main() {
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
	schedulerService := services.NewSchedulerService(
		aggregatorService,
		digestService,
		emailService,
		userRepo,
		metricRepo,
		emailRepo,
		log.Default(),
	)

	router := api.NewRouter(userRepo, integrationRepo, metricRepo, aggregatorService, schedulerService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
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
