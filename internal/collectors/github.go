package collectors

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	targetDayStart := date.UTC().Truncate(24 * time.Hour)

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
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("github request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var events []githubEvent
		if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
			return nil, fmt.Errorf("failed to decode github response: %w", err)
		}

		if len(events) == 0 {
			break
		}

		allEventsOlderThanTargetDay := true
		for _, event := range events {
			eventTime, err := time.Parse(time.RFC3339, event.CreatedAt)
			if err != nil {
				continue
			}

			if !eventTime.UTC().Before(targetDayStart) {
				allEventsOlderThanTargetDay = false
			}

			if eventTime.UTC().Before(targetDayStart) {
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

	pretty, _ := json.MarshalIndent(activities, "", "  ")
	fmt.Println(string(pretty))

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
	defer resp.Body.Close()

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

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode github compare response: %w", err)
	}

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
	defer resp.Body.Close()

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

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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
