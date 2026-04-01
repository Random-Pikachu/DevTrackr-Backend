package collectors

import "fmt"

func GetCollector(platform string, token string) (Collector, error) {
	switch platform {
	case "github":
		return NewGitHubCollector(token), nil
	case "leetcode":
		return NewLeetcodeCollector(), nil
	case "codeforces":
		return NewCodeforcesCollector(), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}