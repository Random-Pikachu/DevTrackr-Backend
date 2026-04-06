package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	EmailProviderNoop   = "noop"
	EmailProviderResend = "resend"
)

type EmailSender interface {
	SendDigest(ctx context.Context, to string, subject string, html string) (string, error)
}

type EmailService struct {
	provider  string
	apiKey    string
	from      string
	client    *http.Client
	resendURL string
}

func NewEmailService(provider string, apiKey string, from string) *EmailService {
	if provider == "" {
		provider = EmailProviderResend
	}
	return &EmailService{
		provider:  strings.ToLower(provider),
		apiKey:    apiKey,
		from:      from,
		client:    &http.Client{Timeout: 10 * time.Second},
		resendURL: "https://api.resend.com/emails",
	}
}

func NewEmailServiceFromEnv() *EmailService {
	return NewEmailService(
		os.Getenv("EMAIL_PROVIDER"),
		os.Getenv("EMAIL_API_KEY"),
		os.Getenv("EMAIL_FROM"),
	)
}

func (s *EmailService) SendDigest(ctx context.Context, to string, subject string, html string) (string, error) {
	switch s.provider {
	case EmailProviderResend:
		return s.sendViaResend(ctx, to, subject, html)
	case EmailProviderNoop:
		return fmt.Sprintf("noop-%d", time.Now().Unix()), nil
	default:
		return "", fmt.Errorf("unsupported email provider: %s (supported: resend, noop)", s.provider)
	}
}

func (s *EmailService) sendViaResend(ctx context.Context, to string, subject string, html string) (string, error) {
	payload := map[string]interface{}{
		"from":    s.from,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.resendURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resend request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	if parsed.ID != "" {
		return parsed.ID, nil
	}
	return fmt.Sprintf("resend-%d", time.Now().Unix()), nil
}
