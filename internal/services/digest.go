package services

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
)

type DigestService struct{}

func NewDigestService() *DigestService {
	return &DigestService{}
}

func (s *DigestService) BuildDailyDigestHTML(
	user models.User,
	metric models.DailyMetric,
	digestDate time.Time,
) (string, string) {
	dateLabel := digestDate.UTC().Format("January 2, 2006")
	subject := fmt.Sprintf("Your DevTrackr Daily Digest - %s", dateLabel)

	totalSolved := metric.LcEasySolved + metric.LcMediumSolved + metric.LcHardSolved + metric.CfProblemsSolved
	var streakMessage string
	if metric.StreakDays > 0 {
		streakMessage = fmt.Sprintf("%d day streak", metric.StreakDays)
	} else {
		streakMessage = "No active streak yet. Tomorrow is a fresh start."
	}

	html := fmt.Sprintf(`
<!doctype html>
<html>
  <body style="font-family: Arial, sans-serif; color: #1f2937; background: #f7fafc; margin: 0; padding: 24px;">
    <div style="max-width: 640px; margin: 0 auto; background: white; border: 1px solid #e5e7eb; border-radius: 12px; padding: 24px;">
      <h2 style="margin-top: 0;">Daily Digest for %s</h2>
      <p style="margin-bottom: 24px;">Hi %s, here is your coding activity snapshot.</p>

      <table style="width: 100%%; border-collapse: collapse;">
        <tr><td style="padding: 8px 0;">GitHub Commits</td><td style="padding: 8px 0; text-align: right;"><strong>%d</strong></td></tr>
        <tr><td style="padding: 8px 0;">LeetCode Easy Solved</td><td style="padding: 8px 0; text-align: right;"><strong>%d</strong></td></tr>
        <tr><td style="padding: 8px 0;">LeetCode Medium Solved</td><td style="padding: 8px 0; text-align: right;"><strong>%d</strong></td></tr>
        <tr><td style="padding: 8px 0;">LeetCode Hard Solved</td><td style="padding: 8px 0; text-align: right;"><strong>%d</strong></td></tr>
        <tr><td style="padding: 8px 0;">Codeforces Problems Solved</td><td style="padding: 8px 0; text-align: right;"><strong>%d</strong></td></tr>
      </table>

      <hr style="border: 0; border-top: 1px solid #e5e7eb; margin: 20px 0;" />
      <p style="margin: 8px 0;"><strong>Total problems solved today:</strong> %d</p>
      <p style="margin: 8px 0;"><strong>Streak:</strong> %s</p>
    </div>
  </body>
</html>
`,
		dateLabel,
		displayNameFromEmail(user.Email),
		metric.GithubCommits,
		metric.LcEasySolved,
		metric.LcMediumSolved,
		metric.LcHardSolved,
		metric.CfProblemsSolved,
		totalSolved,
		streakMessage,
	)

	return subject, html
}

func displayNameFromEmail(email string) string {
	localPart := email
	if idx := strings.Index(email, "@"); idx > 0 {
		localPart = email[:idx]
	}
	if localPart == "" {
		return "Developer"
	}
	runes := []rune(localPart)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
