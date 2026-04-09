package main

import "testing"

func TestGetAllowedOriginsIncludesFrontendCallbackOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://devtrackr.example.com")
	t.Setenv("FRONTEND_OAUTH_CALLBACK_URL", "http://localhost:5173/authenticate/github/callback")

	origins := getAllowedOrigins()

	expected := map[string]bool{
		"http://localhost:5173":          true,
		"http://127.0.0.1:5173":         true,
		"https://devtrackr.example.com": true,
	}

	for _, origin := range origins {
		delete(expected, origin)
	}

	if len(expected) != 0 {
		t.Fatalf("missing expected origins: %#v", expected)
	}
}

func TestOriginFromURL(t *testing.T) {
	if got := originFromURL("http://localhost:5173/authenticate/github/callback"); got != "http://localhost:5173" {
		t.Fatalf("expected callback origin, got %q", got)
	}

	if got := originFromURL("not-a-url"); got != "" {
		t.Fatalf("expected invalid URL to be ignored, got %q", got)
	}
}
