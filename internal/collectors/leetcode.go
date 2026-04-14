package collectors

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type LeetcodeCollector struct {
	endpoint string
	client   *http.Client
}

func NewLeetcodeCollector() *LeetcodeCollector {
	return &LeetcodeCollector{
		endpoint: "https://leetcode.com/graphql",
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type lcRecentSubmission struct {
	Title     string `json:"title"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"statusDisplay"`
	Lang      string `json:"lang"`
	TitleSlug string `json:"titleSlug"`
}

type lcSubmissionsResponse struct {
	Data struct {
		RecentSubmissionList []lcRecentSubmission `json:"recentSubmissionList"`
	} `json:"data"`
}

func (l *LeetcodeCollector) FetchDailyActivity(handle string, date time.Time) ([]ActivityData, error) {
	submissions, err := l.fetchSubmissionsandSnapshot(handle)
	if err != nil {
		return nil, err
	}

	submissionCountForDay, err := l.fetchSubmissionCountForDate(handle, date)
	if err != nil {
		return nil, err
	}

	var todaysSubs []lcRecentSubmission
	for _, sub := range submissions {
		ts, parseErr := strconv.ParseInt(sub.Timestamp, 10, 64)
		if parseErr != nil {
			continue
		}
		if sameLeetcodeDay(time.Unix(ts, 0), date) {
			todaysSubs = append(todaysSubs, sub)
		}
	}

	slugDifficulty, err := l.fetchDifficulties(todaysSubs)
	if err != nil {
		return nil, err
	}

	var activities []ActivityData

	for _, sub := range todaysSubs {
		activities = append(activities, ActivityData{
			Platform:     "leetcode",
			Date:         date,
			ActivityType: "submission",
			Metadata: map[string]interface{}{
				"title":      sub.Title,
				"title_slug": sub.TitleSlug,
				"status":     sub.Status,
				"lang":       sub.Lang,
				"difficulty": slugDifficulty[sub.TitleSlug],
			},
		})
	}

	fallbackCount := submissionCountForDay - len(todaysSubs)
	if fallbackCount > 0 {
		activities = append(activities, ActivityData{
			Platform:     "leetcode",
			Date:         date,
			ActivityType: "submission_count",
			Metadata: map[string]interface{}{
				"submission_count": fallbackCount,
				"status":           "Accepted",
				"difficulty":       "unknown",
				"source":           "submission_calendar",
			},
		})
	}

	// pretty, _ := json.MarshalIndent(activities, "", "  ")
	// fmt.Println(string(pretty))

	return activities, nil
}

func (l *LeetcodeCollector) fetchSubmissionCountForDate(handle string, date time.Time) (int, error) {
	query := map[string]interface{}{
		"query": `
			query userCalendar($username: String!) {
				matchedUser(username: $username) {
					userCalendar {
						submissionCalendar
					}
				}
			}
		`,
		"variables": map[string]string{"username": handle},
	}

	body, _ := json.Marshal(query)
	req, _ := http.NewRequest("POST", l.endpoint, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://leetcode.com")

	resp, err := l.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("leetcode calendar request failed: %w", err)
	}
	defer resp.Body.Close()

	var lcResp struct {
		Data struct {
			MatchedUser *struct {
				UserCalendar struct {
					SubmissionCalendar string `json:"submissionCalendar"`
				} `json:"userCalendar"`
			} `json:"matchedUser"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lcResp); err != nil {
		return 0, fmt.Errorf("failed to decode calendar response: %w", err)
	}
	if lcResp.Data.MatchedUser == nil {
		return 0, nil
	}

	raw := lcResp.Data.MatchedUser.UserCalendar.SubmissionCalendar
	if raw == "" {
		return 0, nil
	}

	byEpoch := map[string]int{}
	if err := json.Unmarshal([]byte(raw), &byEpoch); err != nil {
		return 0, fmt.Errorf("failed to decode submissionCalendar: %w", err)
	}

	total := 0
	for epoch, count := range byEpoch {
		ts, parseErr := strconv.ParseInt(epoch, 10, 64)
		if parseErr != nil {
			continue
		}
		if sameLeetcodeDay(time.Unix(ts, 0), date) {
			total += count
		}
	}

	return total, nil
}

func sameLeetcodeDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func (l *LeetcodeCollector) fetchSubmissionsandSnapshot(handle string) ([]lcRecentSubmission, error) {
	query := map[string]interface{}{
		"query": `
			query userActivityAndStats($username: String!) {
				recentSubmissionList(username: $username, limit: 20) {
					title
					titleSlug
					timestamp
					statusDisplay
					lang
				}
				matchedUser(username: $username) {
					submitStatsGlobal {
						acSubmissionNum {
							difficulty
							count
						}
					}
				}
			}
		`,
		"variables": map[string]string{"username": handle},
	}

	body, _ := json.Marshal(query)
	req, _ := http.NewRequest("POST", l.endpoint, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://leetcode.com")
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("leetcode request failed: %w", err)
	}
	defer resp.Body.Close()

	var lcResp lcSubmissionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&lcResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return lcResp.Data.RecentSubmissionList, nil
}

func (l *LeetcodeCollector) fetchDifficulties(subs []lcRecentSubmission) (map[string]string, error) {
	if len(subs) == 0 {
		return map[string]string{}, nil
	}

	queryFields := ""
	variables := map[string]interface{}{}
	varDeclarations := ""

	for i, sub := range subs {
		alias := fmt.Sprintf("p%d", i)
		varName := fmt.Sprintf("slug%d", i)
		varDeclarations += fmt.Sprintf("$%s: String! ", varName)
		queryFields += fmt.Sprintf(`%s: question(titleSlug: $%s) { difficulty }`, alias, varName) + "\n"
		variables[varName] = sub.TitleSlug
	}

	fullQuery := fmt.Sprintf(`query getDifficulties(%s) { %s }`, varDeclarations, queryFields)

	payload := map[string]interface{}{
		"query":     fullQuery,
		"variables": variables,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", l.endpoint, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://leetcode.com")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("leetcode difficulty request failed: %w", err)
	}
	defer resp.Body.Close()

	var raw struct {
		Data map[string]struct {
			Difficulty string `json:"difficulty"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode difficulty response: %w", err)
	}

	result := map[string]string{}
	for i, sub := range subs {
		alias := fmt.Sprintf("p%d", i)
		if q, ok := raw.Data[alias]; ok {
			result[sub.TitleSlug] = q.Difficulty
		}
	}

	return result, nil
}

func (l *LeetcodeCollector) ValidateHandle(handle string) (bool, error) {
	query := map[string]interface{}{
		"query": `
			query userProfile($username: String!) {
				matchedUser(username: $username) {
					username
				}
			}
		`,
		"variables": map[string]string{
			"username": handle,
		},
	}

	body, err := json.Marshal(query)
	if err != nil {
		return false, fmt.Errorf("failed to marshal graphql query: %w", err)
	}

	req, err := http.NewRequest("POST", l.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://leetcode.com")

	resp, err := l.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("leetcode request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			MatchedUser *struct {
				Username string `json:"username"`
			} `json:"matchedUser"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Data.MatchedUser != nil, nil
}
