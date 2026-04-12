package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/collectors"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/google/uuid"
)

type AggregatorUserRepository interface {
	GetAllUsers(ctx context.Context) ([]models.User, error)
}

type AggregatorIntegrationRepository interface {
	GetActiveIntegrations(ctx context.Context, userID string) ([]models.Integration, error)
}

type AggregatorActivityRepository interface {
	ReplaceActivitiesForUserAndDate(ctx context.Context, userID string, activityDate time.Time, activities []models.Activity) error
}

type AggregatorMetricRepository interface {
	UpsertDailyMetric(ctx context.Context, metric models.DailyMetric) (models.DailyMetric, error)
	GetMostRecentMetricBeforeDate(ctx context.Context, userID string, beforeDate time.Time) (models.DailyMetric, error)
}

type AggregatorService struct {
	userRepo         AggregatorUserRepository
	integrationRepo  AggregatorIntegrationRepository
	activityRepo     AggregatorActivityRepository
	metricRepo       AggregatorMetricRepository
	collectorFactory func(platform string, token string) (collectors.Collector, error)
	logger           *log.Logger
}

var istLocation = time.FixedZone("IST", 5*60*60+30*60)

func normalizeDateToISTDayUTC(input time.Time) time.Time {
	year, month, day := input.In(istLocation).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func NewAggregatorService(
	userRepo AggregatorUserRepository,
	integrationRepo AggregatorIntegrationRepository,
	activityRepo AggregatorActivityRepository,
	metricRepo AggregatorMetricRepository,
	logger *log.Logger,
) *AggregatorService {
	if logger == nil {
		logger = log.Default()
	}
	return &AggregatorService{
		userRepo:         userRepo,
		integrationRepo:  integrationRepo,
		activityRepo:     activityRepo,
		metricRepo:       metricRepo,
		collectorFactory: collectors.GetCollector,
		logger:           logger,
	}
}

func (s *AggregatorService) RunDailyAggregation(ctx context.Context, date time.Time) error {
	normalizedDate := normalizeDateToISTDayUTC(date)

	users, err := s.userRepo.GetAllUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch users for aggregation: %w", err)
	}

	var combinedErr error
	for _, user := range users {
		_, err := s.AggregateUserForDate(ctx, user, normalizedDate)
		if err != nil {
			s.logger.Printf("aggregation failed for user %s (%s): %v", user.ID, user.Email, err)
			combinedErr = errors.Join(combinedErr, err)
		}
	}

	return combinedErr
}

func (s *AggregatorService) AggregateUserForDate(
	ctx context.Context,
	user models.User,
	date time.Time,
) (models.DailyMetric, error) {
	normalizedDate := normalizeDateToISTDayUTC(date)

	integrations, err := s.integrationRepo.GetActiveIntegrations(ctx, user.ID.String())
	if err != nil {
		return models.DailyMetric{}, fmt.Errorf("failed to get active integrations: %w", err)
	}

	rawActivities := make([]models.Activity, 0)
	aggregatedActivities := make([]collectors.ActivityData, 0)

	for _, integration := range integrations {
		token := ""
		if integration.AccessToken.Valid {
			token = integration.AccessToken.String
		}

		collector, err := s.collectorFactory(integration.Platform, token)
		if err != nil {
			return models.DailyMetric{}, fmt.Errorf("collector init failed for %s: %w", integration.Platform, err)
		}

		activities, err := collector.FetchDailyActivity(integration.Handle, normalizedDate)
		if err != nil {
			return models.DailyMetric{}, fmt.Errorf("collector fetch failed for %s: %w", integration.Platform, err)
		}
		// activitiesJSON, marshalErr := json.Marshal(activities)
		// if marshalErr != nil {
		// 	s.logger.Printf("collector raw output encode failed user_id=%s platform=%s handle=%s date=%s err=%v",
		// 		user.ID,
		// 		integration.Platform,
		// 		integration.Handle,
		// 		normalizedDate.Format("2006-01-02"),
		// 		marshalErr,
		// 	)
		// } else {
		// 	s.logger.Printf("collector raw output user_id=%s platform=%s handle=%s date=%s payload=%s",
		// 		user.ID,
		// 		integration.Platform,
		// 		integration.Handle,
		// 		normalizedDate.Format("2006-01-02"),
		// 		string(activitiesJSON),
		// 	)
		// }
		s.logger.Printf("collector fetch complete user_id=%s platform=%s handle=%s date=%s activities=%d",
			user.ID,
			integration.Platform,
			integration.Handle,
			normalizedDate.Format("2006-01-02"),
			len(activities),
		)

		for _, activity := range activities {
			metadataBytes, err := json.Marshal(activity.Metadata)
			if err != nil {
				return models.DailyMetric{}, fmt.Errorf("failed to encode activity metadata: %w", err)
			}

			rawActivities = append(rawActivities, models.Activity{
				UserID:        user.ID,
				IntegrationID: integration.ID,
				Platform:      activity.Platform,
				ActivityDate:  normalizedDate,
				ActivityType:  activity.ActivityType,
				Metadata:      metadataBytes,
			})
		}

		aggregatedActivities = append(aggregatedActivities, activities...)
	}

	if err := s.activityRepo.ReplaceActivitiesForUserAndDate(ctx, user.ID.String(), normalizedDate, rawActivities); err != nil {
		return models.DailyMetric{}, fmt.Errorf("failed to replace activities: %w", err)
	}

	metric := buildMetricFromActivities(user.ID, normalizedDate, aggregatedActivities)
	metric.StreakDays, err = s.computeStreak(ctx, user.ID.String(), normalizedDate, hasActivity(metric))
	if err != nil {
		return models.DailyMetric{}, fmt.Errorf("failed to compute streak: %w", err)
	}

	savedMetric, err := s.metricRepo.UpsertDailyMetric(ctx, metric)
	if err != nil {
		return models.DailyMetric{}, fmt.Errorf("failed to save daily metric: %w", err)
	}
	s.logger.Printf("daily metric upserted user_id=%s date=%s github=%d lc_easy=%d lc_medium=%d lc_hard=%d cf=%d streak=%d raw_activities=%d",
		user.ID,
		normalizedDate.Format("2006-01-02"),
		savedMetric.GithubCommits,
		savedMetric.LcEasySolved,
		savedMetric.LcMediumSolved,
		savedMetric.LcHardSolved,
		savedMetric.CfProblemsSolved,
		savedMetric.StreakDays,
		len(rawActivities),
	)

	return savedMetric, nil
}

func (s *AggregatorService) computeStreak(
	ctx context.Context,
	userID string,
	date time.Time,
	hasAnyActivity bool,
) (int, error) {
	if !hasAnyActivity {
		return 0, nil
	}

	previousMetric, err := s.metricRepo.GetMostRecentMetricBeforeDate(ctx, userID, date)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return 1, nil
		}
		return 0, err
	}

	yesterday := normalizeDateToISTDayUTC(date.AddDate(0, 0, -1))
	previousDate := normalizeDateToISTDayUTC(previousMetric.MetricDate)
	if previousDate.Equal(yesterday) && previousMetric.StreakDays > 0 {
		return previousMetric.StreakDays + 1, nil
	}

	return 1, nil
}

func buildMetricFromActivities(userID uuid.UUID, date time.Time, activities []collectors.ActivityData) models.DailyMetric {
	metric := models.DailyMetric{
		UserID:     userID,
		MetricDate: normalizeDateToISTDayUTC(date),
	}

	solvedCFProblems := make(map[string]struct{})

	for _, activity := range activities {
		switch activity.Platform {
		case "github":
			metric.GithubCommits += extractInt(activity.Metadata["commit_count"])

		case "leetcode":
			submissionCount := extractInt(activity.Metadata["submission_count"])
			if submissionCount > 0 {
				metric.LcEasySolved += submissionCount
				continue
			}

			if !strings.EqualFold(extractString(activity.Metadata["status"]), "accepted") {
				continue
			}
			switch strings.ToLower(extractString(activity.Metadata["difficulty"])) {
			case "easy":
				metric.LcEasySolved++
			case "medium":
				metric.LcMediumSolved++
			case "hard":
				metric.LcHardSolved++
			}

		case "codeforces":
			if !strings.EqualFold(extractString(activity.Metadata["verdict"]), "ok") {
				continue
			}
			problemName := extractString(activity.Metadata["problem_name"])
			if problemName != "" {
				solvedCFProblems[problemName] = struct{}{}
			}
		}
	}

	metric.CfProblemsSolved = len(solvedCFProblems)
	return metric
}

func hasActivity(metric models.DailyMetric) bool {
	return metric.GithubCommits > 0 ||
		metric.LcEasySolved > 0 ||
		metric.LcMediumSolved > 0 ||
		metric.LcHardSolved > 0 ||
		metric.CfProblemsSolved > 0
}

func extractInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func extractString(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}
