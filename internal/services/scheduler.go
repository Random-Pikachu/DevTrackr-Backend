package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type SchedulerUserRepository interface {
	GetDigestEligibleUsers(ctx context.Context) ([]models.User, error)
}

type SchedulerMetricRepository interface {
	GetDailyMetric(ctx context.Context, userID string, metricDate time.Time) (models.DailyMetric, error)
}

type SchedulerEmailRepository interface {
	HasDigestBeenSent(ctx context.Context, userID string, digestDate time.Time) (bool, error)
	CreateEmailLog(ctx context.Context, log models.EmailLog) (models.EmailLog, error)
}

type SchedulerService struct {
	aggregator AggregatorRunner
	digest     *DigestService
	email      EmailSender
	userRepo   SchedulerUserRepository
	metricRepo SchedulerMetricRepository
	emailRepo  SchedulerEmailRepository
	logger     *log.Logger
}

type AggregatorRunner interface {
	RunDailyAggregation(ctx context.Context, date time.Time) error
}

func NewSchedulerService(
	aggregator AggregatorRunner,
	digest *DigestService,
	email EmailSender,
	userRepo SchedulerUserRepository,
	metricRepo SchedulerMetricRepository,
	emailRepo SchedulerEmailRepository,
	logger *log.Logger,
) *SchedulerService {
	if logger == nil {
		logger = log.Default()
	}
	return &SchedulerService{
		aggregator: aggregator,
		digest:     digest,
		email:      email,
		userRepo:   userRepo,
		metricRepo: metricRepo,
		emailRepo:  emailRepo,
		logger:     logger,
	}
}

func (s *SchedulerService) RunNightlyJob(ctx context.Context, digestDate time.Time) error {
	normalizedDate := digestDate.UTC().Truncate(24 * time.Hour)

	aggregateErr := s.aggregator.RunDailyAggregation(ctx, normalizedDate)

	users, err := s.userRepo.GetDigestEligibleUsers(ctx)
	if err != nil {
		return errors.Join(aggregateErr, fmt.Errorf("failed to list digest users: %w", err))
	}

	var combinedErr error
	for _, user := range users {
		err := s.sendDigestToUser(ctx, user, normalizedDate)
		if err != nil {
			s.logger.Printf("failed digest delivery for %s (%s): %v", user.ID, user.Email, err)
			combinedErr = errors.Join(combinedErr, err)
		}
	}

	return errors.Join(aggregateErr, combinedErr)
}

func (s *SchedulerService) sendDigestToUser(ctx context.Context, user models.User, digestDate time.Time) error {
	sent, err := s.emailRepo.HasDigestBeenSent(ctx, user.ID.String(), digestDate)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}

	metric, err := s.metricRepo.GetDailyMetric(ctx, user.ID.String(), digestDate)
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return err
	}

	subject, html := s.digest.BuildDailyDigestHTML(user, metric, digestDate)
	messageID, sendErr := s.email.SendDigest(ctx, user.Email, subject, html)

	logStatus := "sent"
	var providerMessageID sql.NullString
	if sendErr != nil {
		logStatus = "failed"
	} else {
		providerMessageID = sql.NullString{String: messageID, Valid: messageID != ""}
	}

	_, logErr := s.emailRepo.CreateEmailLog(ctx, models.EmailLog{
		UserID:            user.ID,
		DigestDate:        digestDate,
		Status:            logStatus,
		ProviderMessageID: providerMessageID,
	})

	if sendErr != nil {
		return errors.Join(sendErr, logErr)
	}
	return logErr
}

func (s *SchedulerService) Start(ctx context.Context, runHourUTC int, runMinuteUTC int) error {
	if runHourUTC < 0 || runHourUTC > 23 || runMinuteUTC < 0 || runMinuteUTC > 59 {
		return fmt.Errorf("invalid run time: hour=%d minute=%d", runHourUTC, runMinuteUTC)
	}

	for {
		next := nextRunAtUTC(time.Now().UTC(), runHourUTC, runMinuteUTC)
		wait := time.Until(next)
		timer := time.NewTimer(wait)

		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			digestDate := next.AddDate(0, 0, -1).UTC().Truncate(24 * time.Hour)
			if err := s.RunNightlyJob(ctx, digestDate); err != nil {
				s.logger.Printf("nightly job failed for %s: %v", digestDate.Format("2006-01-02"), err)
			}
		}
	}
}

func nextRunAtUTC(now time.Time, hour int, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
