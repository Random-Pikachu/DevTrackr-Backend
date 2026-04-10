package collectors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CodeforcesCollector struct {
	apiBase string
	client  *http.Client
}

func NewCodeforcesCollector() *CodeforcesCollector {
	return &CodeforcesCollector{
		apiBase: "https://codeforces.com/api",
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

type cfSubmission struct {
	ID        int    `json:"id"`
	Verdict   string `json:"verdict"`
	CreatedAt int64  `json:"creationTimeSeconds"`
	Problem   struct {
		Name   string   `json:"name"`
		Rating int      `json:"rating"`
		Tags   []string `json:"tags"`
	} `json:"problem"`
}

type cfSubmissionsResponse struct {
	Status string         `json:"status"`
	Result []cfSubmission `json:"result"`
}

var istLocation = time.FixedZone("IST", 5*60*60+30*60)

func istDateParts(t time.Time) (int, time.Month, int) {
	return t.In(istLocation).Date()
}

func istDayRange(t time.Time) (time.Time, time.Time) {
	year, month, day := istDateParts(t)
	start := time.Date(year, month, day, 0, 0, 0, 0, istLocation)
	end := start.Add(24*time.Hour - time.Second)
	return start, end
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := istDateParts(a)
	by, bm, bd := istDateParts(b)
	return ay == by && am == bm && ad == bd
}

func (c *CodeforcesCollector) FetchDailyActivity(handle string, date time.Time) ([]ActivityData, error) {
	url := fmt.Sprintf("%s/user.status?handle=%s&from=1&count=100", c.apiBase, handle)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Codeforces request failed: %w", err)
	}
	defer resp.Body.Close()

	var cfResp cfSubmissionsResponse
	err = json.NewDecoder(resp.Body).Decode(&cfResp)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode Codeforces response: %w", err)
	}

	if cfResp.Status != "OK" {
		return nil, fmt.Errorf("Codeforces API error: %s", cfResp.Status)
	}

	var activities []ActivityData
	for _, submission := range cfResp.Result {
		submissionTime := time.Unix(submission.CreatedAt, 0).UTC()
		if !sameDay(submissionTime, date) {
			continue
		}
		activities = append(activities, ActivityData{
			Platform:     "codeforces",
			Date:         date,
			ActivityType: "submission",
			Metadata: map[string]interface{}{
				"problem_name": submission.Problem.Name,
				"verdict":      submission.Verdict,
				"rating":       submission.Problem.Rating,
				"tags":         submission.Problem.Tags,
			},
		})
	}
	// pretty, _ := json.MarshalIndent(activities, "", "  ")
	// fmt.Println(string(pretty))

	return activities, nil
}

func (c *CodeforcesCollector) ValidateHandle(handle string) (bool, error) {
	url := fmt.Sprintf("%s/user.info?handles=%s", c.apiBase, handle)
	resp, err := c.client.Get(url)
	if err != nil {
		return false, fmt.Errorf("Codeforces request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return false, fmt.Errorf("Failed to decode Codeforces response: %w", err)
	}

	return result.Status == "OK", nil
}
