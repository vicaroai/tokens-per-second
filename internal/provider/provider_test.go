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
	if _, err := New("does-not-exist", "https://x", "k"); err == nil {
		t.Error("expected error for unknown provider id")
	}
}
