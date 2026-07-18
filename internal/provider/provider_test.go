package provider

import (
	"strings"
	"testing"
)

func TestScrubErrorRedactsSecrets(t *testing.T) {
	cases := []struct {
		in          string
		mustNotHave string
		mustContain string
	}{
		{`{"error":"invalid api key sk-proj-ABCDEF0123456789xyz"}`, "sk-proj-ABCDEF0123456789xyz", "[redacted]"},
		{`bad token fw_REQXzpkvDK7JYF3afLby6f here`, "fw_REQXzpkvDK7JYF3afLby6f", "[redacted]"},
		{`Authorization: Bearer abcdef0123456789abcdef`, "abcdef0123456789abcdef", "[redacted]"},
	}
	for _, c := range cases {
		got := scrubError(c.in)
		if strings.Contains(got, c.mustNotHave) {
			t.Errorf("scrubError leaked secret: %q -> %q", c.in, got)
		}
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("scrubError = %q, want it to contain %q", got, c.mustContain)
		}
	}
}

func TestScrubErrorKeepsOrdinaryText(t *testing.T) {
	in := "model not found: gpt-9"
	if got := scrubError(in); got != in {
		t.Errorf("scrubError altered benign text: %q -> %q", in, got)
	}
}

func TestNewUnknownProvider(t *testing.T) {
	if _, err := New("does-not-exist", "https://api.openai.com/v1", "k"); err == nil {
		t.Error("expected error for unknown provider id")
	}
}

func TestValidateBaseURL(t *testing.T) {
	ok := []string{"https://api.openai.com/v1", "https://api.anthropic.com/v1", "https://api.fireworks.ai/inference/v1", "https://api.deepseek.com/v1"}
	for _, u := range ok {
		if err := validateBaseURL(u); err != nil {
			t.Errorf("expected %q allowed, got %v", u, err)
		}
	}
	bad := []string{
		"https://evil.attacker.com/v1", // not allow-listed — the exfil vector
		"http://api.openai.com/v1",     // non-https
		"https://api.0penai.com/v1",    // typo-squat host
		"https://api.openai.com.evil.com/v1",
		"ftp://api.openai.com",
	}
	for _, u := range bad {
		if err := validateBaseURL(u); err == nil {
			t.Errorf("expected %q rejected, but it was allowed", u)
		}
	}
}

func TestNewRejectsUnlistedHost(t *testing.T) {
	// A real registered provider with an attacker base_url must be refused
	// before any request is built.
	if _, err := New("openai", "https://evil.attacker.com/v1", "sk-realkey"); err == nil {
		t.Error("New allowed an unlisted host — key exfiltration vector open")
	}
}
