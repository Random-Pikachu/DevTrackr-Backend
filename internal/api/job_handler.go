package api

import (
	"log"
	"net/http"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/services"
)

var istLocation = time.FixedZone("IST", 5*60*60+30*60)

func normalizeDateToISTDayUTC(input time.Time) time.Time {
	year, month, day := input.In(istLocation).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

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

func (h *JobHandler) SendDigestAllForDate(w http.ResponseWriter, r *http.Request) {
	targetDate, err := parseDateParamOrDefault(r, "date", time.Now().In(istLocation))
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	if err := h.scheduler.RunNightlyJob(r.Context(), targetDate); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("manual digest-all complete date=%s", targetDate.Format("2006-01-02"))
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "digest_all_processed",
		"date":   targetDate.Format("2006-01-02"),
	})
}

func (h *JobHandler) SendDigestUserForDate(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	targetDate, err := parseDateParamOrDefault(r, "date", time.Now().In(istLocation))
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	if err := h.scheduler.SendDigestNowForUser(r.Context(), userID, targetDate); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("manual digest-user complete user_id=%s date=%s", userID, targetDate.Format("2006-01-02"))
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "digest_user_processed",
		"date":    targetDate.Format("2006-01-02"),
		"user_id": userID,
	})
}

func (h *JobHandler) RunAggregationBackfill2026(w http.ResponseWriter, r *http.Request) {
	startDate := normalizeDateToISTDayUTC(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	endDate := normalizeDateToISTDayUTC(time.Now().In(istLocation))

	type failedDay struct {
		Date  string `json:"date"`
		Error string `json:"error"`
	}

	failed := make([]failedDay, 0)
	processed := 0

	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		if err := h.aggregator.RunDailyAggregation(r.Context(), day); err != nil {
			failed = append(failed, failedDay{
				Date:  day.Format("2006-01-02"),
				Error: err.Error(),
			})
			log.Printf("aggregation backfill skipped date=%s error=%v", day.Format("2006-01-02"), err)
			continue
		}
		processed++
	}

	status := "backfill_complete"
	if len(failed) > 0 {
		status = "backfill_partial"
	}

	log.Printf("aggregation backfill complete start=%s end=%s days_processed=%d days_failed=%d",
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
		processed,
		len(failed),
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         status,
		"start":          startDate.Format("2006-01-02"),
		"end":            endDate.Format("2006-01-02"),
		"days_processed": processed,
		"days_failed":    len(failed),
		"failed_dates":   failed,
	})
}

func parseDateParamOrDefault(r *http.Request, key string, fallback time.Time) (time.Time, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return normalizeDateToISTDayUTC(fallback), nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	return normalizeDateToISTDayUTC(parsed), nil
}
