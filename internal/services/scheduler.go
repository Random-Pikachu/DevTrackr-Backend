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

const defaultDigestTimezone = "Asia/Kolkata"

type SchedulerUserRepository interface {
	GetDigestEligibleUsers(ctx context.Context) ([]models.User, error)
	GetUserByID(ctx context.Context, userID string) (models.User, error)
}

type SchedulerMetricRepository interface {
	GetDailyMetric(ctx context.Context, userID string, metricDate time.Time) (models.DailyMetric, error)
}

type SchedulerEmailRepository interface {
	HasDigestBeenSent(ctx context.Context, userID string, digestDate time.Time) (bool, error)
	CreateEmailLog(ctx context.Context, log models.EmailLog) (models.EmailLog, error)
}

type SchedulerService struct {
	aggregator                  AggregatorRunner
	digest                      *DigestService
	email                       EmailSender
	userRepo                    SchedulerUserRepository
	metricRepo                  SchedulerMetricRepository
	emailRepo                   SchedulerEmailRepository
	logger                      *log.Logger
	lastEndOfDayAggregationDate time.Time
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

func (s *SchedulerService) SendDigestNowForUser(ctx context.Context, userID string, digestDate time.Time) error {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	s.logger.Printf("manual digest requested user_id=%s email=%s date=%s", user.ID, user.Email, digestDate.UTC().Truncate(24*time.Hour).Format("2006-01-02"))

	return s.sendFreshDigestToUser(ctx, user, digestDate.UTC().Truncate(24*time.Hour))
}

func (s *SchedulerService) sendFreshDigestToUser(ctx context.Context, user models.User, digestDate time.Time) error {
	sent, err := s.emailRepo.HasDigestBeenSent(ctx, user.ID.String(), digestDate)
	if err != nil {
		return err
	}
	if sent {
		s.logger.Printf("digest skipped already-sent user_id=%s email=%s date=%s", user.ID, user.Email, digestDate.Format("2006-01-02"))
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
		s.logger.Printf("digest skipped already-sent user_id=%s email=%s date=%s", user.ID, user.Email, digestDate.Format("2006-01-02"))
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
		s.logger.Printf("digest send failed user_id=%s email=%s date=%s error=%v", user.ID, user.Email, digestDate.Format("2006-01-02"), sendErr)
		return errors.Join(sendErr, logErr)
	}
	s.logger.Printf("digest sent user_id=%s email=%s date=%s provider_message_id=%s", user.ID, user.Email, digestDate.Format("2006-01-02"), messageID)
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
			nowUTC := now.UTC()
			if err := s.RunDueDigests(ctx, nowUTC); err != nil {
				s.logger.Printf("due-digests tick failed at %s: %v", nowUTC.Format(time.RFC3339), err)
			}

			if err := s.runEndOfDayAggregationIfDue(ctx, nowUTC); err != nil {
				s.logger.Printf("end-of-day aggregation tick failed at %s: %v", nowUTC.Format(time.RFC3339), err)
			}
		}
	}
}

func (s *SchedulerService) runEndOfDayAggregationIfDue(ctx context.Context, nowUTC time.Time) error {
	localNow := nowUTC.In(istLocation)
	if localNow.Hour() != 23 || localNow.Minute() != 59 {
		return nil
	}

	targetDate := normalizeDateToISTDayUTC(localNow)
	if !s.lastEndOfDayAggregationDate.IsZero() && s.lastEndOfDayAggregationDate.Equal(targetDate) {
		return nil
	}

	if err := s.aggregator.RunDailyAggregation(ctx, targetDate); err != nil {
		return fmt.Errorf("failed for date %s: %w", targetDate.Format("2006-01-02"), err)
	}

	s.lastEndOfDayAggregationDate = targetDate
	s.logger.Printf("end-of-day aggregation completed date=%s", targetDate.Format("2006-01-02"))
	return nil
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
		timezone = defaultDigestTimezone
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("invalid timezone %q", timezone)
	}

	digestTime := user.DigestTime
	if digestTime == "" {
		digestTime = "23:30"
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
