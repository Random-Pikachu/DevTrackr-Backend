package collectors

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// privateOrgLabel is the display label used for commits GitHub counts for the
// user but that the token cannot read, typically private repositories in an
// organization that restricts OAuth app access.
const privateOrgLabel = "Private organization repositories"

func (g *GitHubCollector) FetchDailyActivity(handle string, date time.Time) ([]ActivityData, error) {
	normalizedDate := date.UTC().Truncate(24 * time.Hour)

	visible, visibleErr := g.fetchVisibleDailyActivity(handle, normalizedDate)

	if g.token == "" {
		return visible, visibleErr
	}

	summary, summaryErr := g.fetchContributionSummary(handle, normalizedDate)
	if summaryErr != nil {
		if visibleErr != nil {
			return nil, fmt.Errorf("%v; contribution summary failed: %w", visibleErr, summaryErr)
		}
		log.Printf("github contribution summary unavailable handle=%s date=%s err=%v; using visible commits only",
			handle,
			normalizedDate.Format("2006-01-02"),
			summaryErr,
		)
		return visible, nil
	}

	if visibleErr != nil {
		log.Printf("github visible commit fetch failed handle=%s date=%s err=%v; using contribution summary only",
			handle,
			normalizedDate.Format("2006-01-02"),
			visibleErr,
		)
		visible = nil
	}

	return mergeHiddenContributions(visible, summary, normalizedDate, handle), nil
}

// fetchVisibleDailyActivity returns per-repository commits the token can read,
// using commit search first and the public events feed as a fallback.
func (g *GitHubCollector) fetchVisibleDailyActivity(handle string, date time.Time) ([]ActivityData, error) {
	searchActivities, searchErr := g.fetchDailyActivityByCommitSearch(handle, date)
	if searchErr == nil {
		return searchActivities, nil
	}

	eventActivities, eventErr := g.fetchDailyActivityFromEvents(handle, date)
	if eventErr == nil {
		return eventActivities, nil
	}

	return nil, fmt.Errorf("github commit-search failed: %v; events fallback failed: %w", searchErr, eventErr)
}

type contributionSummary struct {
	TotalCommitContributions     int
	RestrictedContributionsCount int
}

// fetchContributionSummary asks GitHub's GraphQL API how many commit
// contributions it credits the user with for the day. GitHub computes this
// from the user's contribution graph, so it includes private repositories in
// organizations that block the OAuth app. Those show up in
// RestrictedContributionsCount, which is only populated when the user has
// enabled "Private contributions" in their GitHub profile contribution settings.
func (g *GitHubCollector) fetchContributionSummary(handle string, date time.Time) (contributionSummary, error) {
	startIST, endIST := istDayRange(date)

	const query = `query($login: String!, $from: DateTime!, $to: DateTime!) {
  user(login: $login) {
    contributionsCollection(from: $from, to: $to) {
      totalCommitContributions
      restrictedContributionsCount
    }
  }
}`

	payload, err := json.Marshal(map[string]interface{}{
		"query": query,
		"variables": map[string]string{
			"login": handle,
			"from":  startIST.Format(time.RFC3339),
			"to":    endIST.Format(time.RFC3339),
		},
	})
	if err != nil {
		return contributionSummary{}, err
	}

	req, err := http.NewRequest(http.MethodPost, g.apiBase+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return contributionSummary{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.token)

	resp, err := g.client.Do(req)
	if err != nil {
		return contributionSummary{}, fmt.Errorf("github graphql request failed: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return contributionSummary{}, fmt.Errorf("failed to read github graphql response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return contributionSummary{}, fmt.Errorf("github graphql failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			User *struct {
				ContributionsCollection struct {
					TotalCommitContributions     int `json:"totalCommitContributions"`
					RestrictedContributionsCount int `json:"restrictedContributionsCount"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return contributionSummary{}, fmt.Errorf("failed to decode github graphql response: %w", err)
	}
	if len(result.Errors) > 0 {
		messages := make([]string, 0, len(result.Errors))
		for _, e := range result.Errors {
			messages = append(messages, e.Message)
		}
		return contributionSummary{}, fmt.Errorf("github graphql errors: %s", strings.Join(messages, "; "))
	}
	if result.Data.User == nil {
		return contributionSummary{}, fmt.Errorf("github graphql returned no user for handle %q", handle)
	}

	summary := contributionSummary{
		TotalCommitContributions:     result.Data.User.ContributionsCollection.TotalCommitContributions,
		RestrictedContributionsCount: result.Data.User.ContributionsCollection.RestrictedContributionsCount,
	}
	log.Printf("github contribution summary handle=%s date=%s commits=%d restricted=%d",
		handle,
		startIST.Format("2006-01-02"),
		summary.TotalCommitContributions,
		summary.RestrictedContributionsCount,
	)

	return summary, nil
}

// mergeHiddenContributions appends one extra activity when GitHub credits the
// user with more commits than the token could see. The extra activity carries
// only a count: no repository name and no messages. It never subtracts when the
// visible total is higher (for example commits in forks, which GitHub does not
// count as contributions).
func mergeHiddenContributions(visible []ActivityData, summary contributionSummary, date time.Time, handle string) []ActivityData {
	visibleTotal := 0
	for _, activity := range visible {
		if count, ok := activity.Metadata["commit_count"].(int); ok {
			visibleTotal += count
		}
	}

	expected := summary.TotalCommitContributions + summary.RestrictedContributionsCount
	hidden := expected - visibleTotal
	if hidden <= 0 {
		return visible
	}

	log.Printf("github hidden contributions handle=%s date=%s visible=%d expected=%d hidden=%d",
		handle,
		date.Format("2006-01-02"),
		visibleTotal,
		expected,
		hidden,
	)

	return append(visible, ActivityData{
		Platform:     "github",
		Date:         date,
		ActivityType: "push",
		Metadata: map[string]interface{}{
			"repo":         "",
			"label":        privateOrgLabel,
			"private_org":  true,
			"commit_count": hidden,
			"messages":     []string{},
		},
	})
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
			if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(string(body)), "pagination is limited") {
				log.Printf("github events pagination limit reached handle=%s date=%s page=%d; stopping fallback scan",
					handle,
					date.Format("2006-01-02"),
					page,
				)
				break
			}
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
