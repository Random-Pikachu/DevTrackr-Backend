package collectors

import "time"

type ActivityData struct {
	Platform     string
	Date         time.Time
	ActivityType string
	Metadata     map[string]interface{}
}

type Collector interface {
	FetchDailyActivity(handle string, date time.Time) ([]ActivityData, error)
	ValidateHandle(handle string) (bool, error)
}
