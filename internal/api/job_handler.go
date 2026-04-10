package api

import (
	"log"
	"net/http"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

type JobHandler struct {
	aggregator *services.AggregatorService
	scheduler  *services.SchedulerService
}

func NewJobHandler(aggregator *services.AggregatorService, scheduler *services.SchedulerService) *JobHandler {
	return &JobHandler{
		aggregator: aggregator,
		scheduler:  scheduler,
	}
}

func (h *JobHandler) RunAggregation(w http.ResponseWriter, r *http.Request) {
	targetDate, err := parseDateParamOrDefault(r, "date", time.Now().UTC().AddDate(0, 0, -1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	if err := h.aggregator.RunDailyAggregation(r.Context(), targetDate); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("manual aggregation job complete date=%s", targetDate.Format("2006-01-02"))

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "aggregation_complete",
		"date":   targetDate.Format("2006-01-02"),
	})
}

func (h *JobHandler) RunNightly(w http.ResponseWriter, r *http.Request) {
	targetDate, err := parseDateParamOrDefault(r, "date", time.Now().UTC().AddDate(0, 0, -1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	if err := h.scheduler.RunNightlyJob(r.Context(), targetDate); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("manual nightly job complete date=%s", targetDate.Format("2006-01-02"))

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "nightly_complete",
		"date":   targetDate.Format("2006-01-02"),
	})
}

func parseDateParamOrDefault(r *http.Request, key string, fallback time.Time) (time.Time, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback.UTC().Truncate(24 * time.Hour), nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC().Truncate(24 * time.Hour), nil
}
