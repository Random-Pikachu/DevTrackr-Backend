package collectors

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

type GitHubCollector struct {
	apiBase string
	client  *http.Client
	token   string
}

func NewGitHubCollector(token string) *GitHubCollector {
	return &GitHubCollector{
		apiBase: "https://api.github.com",
		client:  &http.Client{Timeout: 10 * time.Second},
		token:   token,
	}
}

func (g *GitHubCollector) FetchDailyActivity(handle string, date time.Time) ([]ActivityData, error) {
	normalizedDate := date.UTC().Truncate(24 * time.Hour)

	searchActivities, searchErr := g.fetchDailyActivityByCommitSearch(handle, normalizedDate)
	if searchErr == nil && len(searchActivities) > 0 {
		return searchActivities, nil
	}

	eventActivities, eventErr := g.fetchDailyActivityFromEvents(handle, normalizedDate)
	if eventErr == nil {
		if len(eventActivities) > 0 {
			return eventActivities, nil
		}
		if searchErr == nil {
			return nil, nil
		}
	}

	if searchErr != nil && eventErr != nil {
		return nil, fmt.Errorf("github commit-search failed: %v; events fallback failed: %w", searchErr, eventErr)
	}
	if searchErr != nil {
		return nil, searchErr
	}
	return nil, eventErr
}

func (g *GitHubCollector) fetchDailyActivityByCommitSearch(handle string, date time.Time) ([]ActivityData, error) {
	type searchItem struct {
		SHA        string `json:"sha"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}

	type searchResponse struct {
		Items []searchItem `json:"items"`
	}

	type repoActivity struct {
		commitCount int
		messages    []string
		seenSHAs    map[string]struct{}
	}

	repoActivities := map[string]*repoActivity{}
	startIST, endIST := istDayRange(date)
	targetDay := startIST.Format("2006-01-02")
	query := fmt.Sprintf("author:%s committer-date:%s..%s", handle, startIST.Format(time.RFC3339), endIST.Format(time.RFC3339))

	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("%s/search/commits?q=%s&per_page=100&page=%d", g.apiBase, url.QueryEscape(query), page)
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if g.token != "" {
			req.Header.Set("Authorization", "Bearer "+g.token)
		}

		resp, err := g.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github commit-search request failed: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read github commit-search response: %w", readErr)
		}
		// log.Printf("github raw response endpoint=search/commits handle=%s date=%s page=%d status=%d body=%s",
		// 	handle,
		// 	targetDay,
		// 	page,
		// 	resp.StatusCode,
		// 	string(body),
		// )

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github commit-search failed with status %d: %s", resp.StatusCode, string(body))
		}

		var result searchResponse
		decodeErr := json.Unmarshal(body, &result)
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to decode github commit-search response: %w", decodeErr)
		}
		log.Printf("github commit-search summary handle=%s date=%s page=%d status=%d items=%d",
			handle,
			targetDay,
			page,
			resp.StatusCode,
			len(result.Items),
		)

		if len(result.Items) == 0 {
			break
		}

		for _, item := range result.Items {
			repo := item.Repository.FullName
			if repo == "" {
				repo = "unknown"
			}

			if _, ok := repoActivities[repo]; !ok {
				repoActivities[repo] = &repoActivity{seenSHAs: make(map[string]struct{})}
			}

			if _, seen := repoActivities[repo].seenSHAs[item.SHA]; seen {
				continue
			}

			repoActivities[repo].seenSHAs[item.SHA] = struct{}{}
			repoActivities[repo].commitCount++
			repoActivities[repo].messages = append(repoActivities[repo].messages, item.Commit.Message)
		}

		if len(result.Items) < 100 {
			break
		}
	}

	activities := make([]ActivityData, 0, len(repoActivities))
	for repo, activity := range repoActivities {
		activities = append(activities, ActivityData{
			Platform:     "github",
			Date:         date,
			ActivityType: "push",
			Metadata: map[string]interface{}{
				"repo":         repo,
				"commit_count": activity.commitCount,
				"messages":     activity.messages,
			},
		})
	}

	return activities, nil
}

func (g *GitHubCollector) fetchDailyActivityFromEvents(handle string, date time.Time) ([]ActivityData, error) {
	type githubEvent struct {
		Type      string `json:"type"`
		CreatedAt string `json:"created_at"`
		Repo      struct {
			Name string `json:"name"`
		} `json:"repo"`
		Payload struct {
			Head   string `json:"head"`
			Before string `json:"before"`
		} `json:"payload"`
	}

	type repoActivity struct {
		commitCount int
		messages    []string
		seenSHAs    map[string]struct{}
	}

	repoActivities := map[string]*repoActivity{}

	targetDayStartIST, _ := istDayRange(date)

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/users/%s/events?per_page=100&page=%d", g.apiBase, handle, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if g.token != "" {
			req.Header.Set("Authorization", "Bearer "+g.token)
		}

		resp, err := g.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github request failed: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read github events response: %w", readErr)
		}
		// log.Printf("github raw response endpoint=users/events handle=%s date=%s page=%d status=%d body=%s",
		// 	handle,
		// 	date.Format("2006-01-02"),
		// 	page,
		// 	resp.StatusCode,
		// 	string(body),
		// )

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var events []githubEvent
		if err := json.Unmarshal(body, &events); err != nil {
			return nil, fmt.Errorf("failed to decode github response: %w", err)
		}
		log.Printf("github events summary handle=%s date=%s page=%d status=%d events=%d",
			handle,
			date.Format("2006-01-02"),
			page,
			resp.StatusCode,
			len(events),
		)

		if len(events) == 0 {
			break
		}

		allEventsOlderThanTargetDay := true
		for _, event := range events {
			eventTime, err := time.Parse(time.RFC3339, event.CreatedAt)
			if err != nil {
				continue
			}
			eventTimeIST := eventTime.In(istLocation)

			if !eventTimeIST.Before(targetDayStartIST) {
				allEventsOlderThanTargetDay = false
			}

			if eventTimeIST.Before(targetDayStartIST) {
				continue
			}

			if !sameDay(eventTime, date) || event.Type != "PushEvent" {
				continue
			}

			repo := event.Repo.Name
			if repo == "" {
				continue
			}

			if _, ok := repoActivities[repo]; !ok {
				repoActivities[repo] = &repoActivity{
					seenSHAs: make(map[string]struct{}),
				}
			}

			commits, err := g.fetchPushCommits(repo, event.Payload.Before, event.Payload.Head)
			if err != nil {
				return nil, err
			}

			for _, commit := range commits {
				if _, seen := repoActivities[repo].seenSHAs[commit.SHA]; seen {
					continue
				}
				repoActivities[repo].seenSHAs[commit.SHA] = struct{}{}
				repoActivities[repo].commitCount++
				repoActivities[repo].messages = append(repoActivities[repo].messages, commit.Message)
			}
		}

		if allEventsOlderThanTargetDay {
			break
		}
	}

	var activities []ActivityData
	for repo, activity := range repoActivities {
		activities = append(activities, ActivityData{
			Platform:     "github",
			Date:         date,
			ActivityType: "push",
			Metadata: map[string]interface{}{
				"repo":         repo,
				"commit_count": activity.commitCount,
				"messages":     activity.messages,
			},
		})
	}

	return activities, nil
}

func (g *GitHubCollector) fetchPushCommits(repo, beforeSHA, headSHA string) ([]struct {
	SHA     string
	Message string
}, error) {
	if headSHA == "" {
		return nil, nil
	}

	if beforeSHA == "" || beforeSHA == "0000000000000000000000000000000000000000" {
		commit, err := g.fetchCommit(repo, headSHA)
		if err != nil {
			return nil, err
		}
		return []struct {
			SHA     string
			Message string
		}{commit}, nil
	}

	url := fmt.Sprintf("%s/repos/%s/compare/%s...%s", g.apiBase, repo, beforeSHA, headSHA)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github compare request failed: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("failed to read github compare response: %w", readErr)
	}
	// log.Printf("github raw response endpoint=repos/compare repo=%s before=%s head=%s status=%d body=%s",
	// 	repo,
	// 	beforeSHA,
	// 	headSHA,
	// 	resp.StatusCode,
	// 	string(body),
	// )

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github compare request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"commits"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode github compare response: %w", err)
	}
	log.Printf("github compare summary repo=%s before=%s head=%s status=%d commits=%d",
		repo,
		beforeSHA,
		headSHA,
		resp.StatusCode,
		len(result.Commits),
	)

	commits := make([]struct {
		SHA     string
		Message string
	}, 0, len(result.Commits))
	for _, commit := range result.Commits {
		commits = append(commits, struct {
			SHA     string
			Message string
		}{
			SHA:     commit.SHA,
			Message: commit.Commit.Message,
		})
	}

	return commits, nil
}

func (g *GitHubCollector) fetchCommit(repo, sha string) (struct {
	SHA     string
	Message string
}, error) {
	url := fmt.Sprintf("%s/repos/%s/commits/%s", g.apiBase, repo, sha)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return struct {
			SHA     string
			Message string
		}{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return struct {
			SHA     string
			Message string
		}{}, fmt.Errorf("github commit request failed: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return struct {
			SHA     string
			Message string
		}{}, fmt.Errorf("failed to read github commit response: %w", readErr)
	}
	// log.Printf("github raw response endpoint=repos/commits repo=%s sha=%s status=%d body=%s",
	// 	repo,
	// 	sha,
	// 	resp.StatusCode,
	// 	string(body),
	// )

	if resp.StatusCode != http.StatusOK {
		return struct {
			SHA     string
			Message string
		}{}, fmt.Errorf("github commit request failed with status %d", resp.StatusCode)
	}

	var result struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return struct {
			SHA     string
			Message string
		}{}, fmt.Errorf("failed to decode github commit response: %w", err)
	}

	return struct {
		SHA     string
		Message string
	}{
		SHA:     result.SHA,
		Message: result.Commit.Message,
	}, nil
}

func (g *GitHubCollector) ValidateHandle(handle string) (bool, error) {
	url := fmt.Sprintf("%s/users/%s", g.apiBase, handle)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
