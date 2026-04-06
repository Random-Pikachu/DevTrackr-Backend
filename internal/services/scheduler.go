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
	AggregateUserForDate(ctx context.Context, user models.User, date time.Time) (models.DailyMetric, error)
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

func (s *SchedulerService) RunDueDigests(ctx context.Context, now time.Time) error {
	users, err := s.userRepo.GetDigestEligibleUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list digest users: %w", err)
	}

	var combinedErr error
	for _, user := range users {
		due, digestDate, err := isUserDueAt(user, now)
		if err != nil {
			s.logger.Printf("skipping user %s due to invalid schedule config: %v", user.ID, err)
			continue
		}
		if !due {
			continue
		}

		err = s.sendFreshDigestToUser(ctx, user, digestDate)
		if err != nil {
			s.logger.Printf("failed due-digest delivery for %s (%s): %v", user.ID, user.Email, err)
			combinedErr = errors.Join(combinedErr, err)
		}
	}

	return combinedErr
}

func (s *SchedulerService) sendFreshDigestToUser(ctx context.Context, user models.User, digestDate time.Time) error {
	sent, err := s.emailRepo.HasDigestBeenSent(ctx, user.ID.String(), digestDate)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}

	if _, err := s.aggregator.AggregateUserForDate(ctx, user, digestDate); err != nil {
		return err
	}

	return s.sendDigestToUser(ctx, user, digestDate)
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

func (s *SchedulerService) Start(ctx context.Context, tickInterval time.Duration) error {
	if tickInterval <= 0 {
		return fmt.Errorf("invalid tick interval: %s", tickInterval)
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			if err := s.RunDueDigests(ctx, now.UTC()); err != nil {
				s.logger.Printf("due-digests tick failed at %s: %v", now.UTC().Format(time.RFC3339), err)
			}
		}
	}
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

func isUserDueAt(user models.User, nowUTC time.Time) (bool, time.Time, error) {
	timezone := user.Timezone
	if timezone == "" {
		timezone = "UTC"
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("invalid timezone %q", timezone)
	}

	digestTime := user.DigestTime
	if digestTime == "" {
		digestTime = "20:00"
	}
	digestClock, err := time.Parse("15:04", digestTime)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("invalid digest time %q", digestTime)
	}

	localNow := nowUTC.In(location)
	if localNow.Hour() != digestClock.Hour() || localNow.Minute() != digestClock.Minute() {
		return false, time.Time{}, nil
	}

	digestDate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	return true, digestDate, nil
}
