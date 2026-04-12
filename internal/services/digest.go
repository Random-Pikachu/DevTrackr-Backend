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
	<body style="margin:0; padding:24px; background:#000000; color:#ffffff; font-family: Inter, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif;">
		<div style="max-width:640px; margin:0 auto; border:1px solid rgba(255,255,255,0.12); border-radius:16px; background:#050505; overflow:hidden;">
			<div style="padding:20px 24px; border-bottom:1px solid rgba(255,255,255,0.10);">
				<p style="margin:0 0 8px 0; font-size:11px; letter-spacing:0.14em; text-transform:uppercase; color:rgba(255,255,255,0.55);">
					DevTrackr Daily Digest
				</p>
				<h2 style="margin:0; font-size:24px; line-height:1.25; color:#ffffff;">Daily Digest for %s</h2>
				<p style="margin:10px 0 0 0; font-size:14px; line-height:1.6; color:rgba(255,255,255,0.70);">
					Hi %s, here is your coding activity snapshot.
				</p>
			</div>

			<div style="padding:18px 24px;">
				<table role="presentation" style="width:100%%; border-collapse:collapse;">
					<tr>
						<td style="padding:10px 0; font-size:14px; color:rgba(255,255,255,0.78); border-bottom:1px solid rgba(255,255,255,0.08);">GitHub Commits</td>
						<td style="padding:10px 0; text-align:right; font-size:14px; color:#ffffff; border-bottom:1px solid rgba(255,255,255,0.08);"><strong>%d</strong></td>
					</tr>
					<tr>
						<td style="padding:10px 0; font-size:14px; color:rgba(255,255,255,0.78); border-bottom:1px solid rgba(255,255,255,0.08);">LeetCode Easy Solved</td>
						<td style="padding:10px 0; text-align:right; font-size:14px; color:#ffffff; border-bottom:1px solid rgba(255,255,255,0.08);"><strong>%d</strong></td>
					</tr>
					<tr>
						<td style="padding:10px 0; font-size:14px; color:rgba(255,255,255,0.78); border-bottom:1px solid rgba(255,255,255,0.08);">LeetCode Medium Solved</td>
						<td style="padding:10px 0; text-align:right; font-size:14px; color:#ffffff; border-bottom:1px solid rgba(255,255,255,0.08);"><strong>%d</strong></td>
					</tr>
					<tr>
						<td style="padding:10px 0; font-size:14px; color:rgba(255,255,255,0.78); border-bottom:1px solid rgba(255,255,255,0.08);">LeetCode Hard Solved</td>
						<td style="padding:10px 0; text-align:right; font-size:14px; color:#ffffff; border-bottom:1px solid rgba(255,255,255,0.08);"><strong>%d</strong></td>
					</tr>
					<tr>
						<td style="padding:10px 0; font-size:14px; color:rgba(255,255,255,0.78);">Codeforces Problems Solved</td>
						<td style="padding:10px 0; text-align:right; font-size:14px; color:#ffffff;"><strong>%d</strong></td>
					</tr>
				</table>

				<div style="margin-top:18px; border:1px solid rgba(255,255,255,0.10); border-radius:12px; background:#0b0b0b; padding:14px 16px;">
					<p style="margin:0 0 8px 0; font-size:14px; color:#ffffff;">
						<strong>Total problems solved today:</strong> %d
					</p>
					<p style="margin:0; font-size:14px; color:rgba(255,255,255,0.82);">
						<strong>Streak:</strong> %s
					</p>
				</div>
			</div>
    </div>
  </body>
</html>
`,
		dateLabel,
		displayNameForDigest(user),
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

func displayNameForDigest(user models.User) string {
	if user.Username.Valid {
		username := strings.TrimSpace(user.Username.String)
		if username != "" {
			runes := []rune(username)
			runes[0] = unicode.ToUpper(runes[0])
			return string(runes)
		}
	}

	email := user.Email
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
