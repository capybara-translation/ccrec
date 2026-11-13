package redact

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/betterleaks/betterleaks/v2/report"
)

// Secrets are assembled at runtime so that the repository never contains
// literal credentials that would trip secret scanning on push.
func fakeGitHubPAT() string {
	return "ghp_" + strings.Repeat("aB3dE5fG7h", 3) + "J9kLmN"
}

func fakeAWSAccessKey() string {
	return "AKIA" + "Q3W7ERT2YU5IOPZX"
}

func fakeAWSSecretKey() string {
	return "wJalrXUtnFEMI/K7MDENG/" + "bPxRfiCYzQ8x2B4kLp"
}

func fakePrivateKey() string {
	return "-----BEGIN RSA " + "PRIVATE KEY-----\n" +
		strings.Repeat("MIIEpAIBAAKCAQEA1b2c3d4e5f6g7h8i9j0kLmNoPqRsTuVwXyZaBcDeFgHiJkLm\n", 5) +
		"-----END RSA " + "PRIVATE KEY-----"
}

func newRedactor(t *testing.T) *Redactor {
	t.Helper()
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func redactString(t *testing.T, input string) string {
	t.Helper()
	got, err := newRedactor(t).Redact(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRedact_ReplacesTokenAndKeepsSurroundingText(t *testing.T) {
	token := fakeGitHubPAT()
	got := redactString(t, "before\nGITHUB_TOKEN="+token+" after\nend\n")

	if strings.Contains(got, token) {
		t.Fatalf("token leaked: %q", got)
	}
	want := "before\nGITHUB_TOKEN=[REDACTED:github-pat] after\nend\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRedact_ReplacesOnlyValueOfKeyValueMatch(t *testing.T) {
	got := redactString(t, "password: hunter2\n")

	if got != "password: [REDACTED:generic-password]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRedact_ReplacesUnlabelledRepeatsOfLongValues(t *testing.T) {
	secret := "Xk9#mPq2$vLw8@nR"
	got := redactString(t, "db_password = \""+secret+"\"\nlater I typed "+secret+" again\n")

	if strings.Contains(got, secret) {
		t.Fatalf("repeated secret leaked: %q", got)
	}
}

func TestRedact_KeepsUnlabelledRepeatsOfShortValues(t *testing.T) {
	// Short values such as "hunter2" are too likely to be ordinary words to
	// replace everywhere; only the detected occurrence is redacted.
	got := redactString(t, "password: hunter2\nthe hunter2 example\n")

	if got != "password: [REDACTED:generic-password]\nthe hunter2 example\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRedact_ReplacesEncodedSecretSpan(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("token " + fakeGitHubPAT()))
	got := redactString(t, "blob: "+encoded+"\n")

	if strings.Contains(got, encoded) {
		t.Fatalf("encoded secret leaked: %q", got)
	}
	if got != "blob: [REDACTED:github-pat]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRedact_ReplacesMultiLinePrivateKey(t *testing.T) {
	got := redactString(t, "```\n"+fakePrivateKey()+"\n```\n")

	if got != "```\n[REDACTED:private-key]\n```\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRedact_IgnoresAllowMarkers(t *testing.T) {
	key := fakeAWSAccessKey()
	got := redactString(t, "aws_access_key_id = "+key+" # gitleaks:allow betterleaks:allow\n")

	if strings.Contains(got, key) {
		t.Fatalf("allow marker suppressed redaction: %q", got)
	}
}

func TestRedact_MergesOverlappingFindings(t *testing.T) {
	// A trailing character makes both github-pat and generic-api-key match
	// overlapping spans; no fragment of the secret may survive the merge.
	token := fakeGitHubPAT() + "x"
	got := redactString(t, "GITHUB_TOKEN="+token+"\n")

	if strings.Contains(got, "ghp_") || strings.Contains(got, "J9kLmN") {
		t.Fatalf("secret fragment leaked: %q", got)
	}
	if strings.Count(got, "[REDACTED:") != 1 {
		t.Fatalf("overlapping findings should produce one marker: %q", got)
	}
}

func TestRedact_LeavesTextWithoutSecretsUnchanged(t *testing.T) {
	input := "# Conversation Log\n\nWhat is Go?\n\n```go\nfmt.Println(\"hi\")\n```\n"
	if got := redactString(t, input); got != input {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestApplyFindings_FallsBackToValueWhenLocationDoesNotMatch(t *testing.T) {
	secret := "s3cr3t-value-that-is-long"
	text := "a " + secret + "\nb " + secret + "\n"
	findings := []report.Finding{{
		RuleID:   "test-rule",
		Match:    report.Match{Value: secret, Full: secret},
		Location: report.Location{StartLine: 9, EndLine: 9, StartColumn: 1, EndColumn: 3},
	}}

	got, err := applyFindings(text, findings)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a [REDACTED:test-rule]\nb [REDACTED:test-rule]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyFindings_FallsBackToValueEvenWhenShort(t *testing.T) {
	text := "x pw1 y\n"
	findings := []report.Finding{{
		RuleID:   "test-rule",
		Match:    report.Match{Value: "pw1"},
		Location: report.Location{StartLine: 1, EndLine: 1, StartColumn: 50, EndColumn: 60},
	}}

	got, err := applyFindings(text, findings)
	if err != nil {
		t.Fatal(err)
	}
	if got != "x [REDACTED:test-rule] y\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRedact_ReplacesComponentSecretOfCompositeRule(t *testing.T) {
	// The secret key is reported only as a component of aws-access-token.
	keyID, secret := fakeAWSAccessKey(), fakeAWSSecretKey()
	got := redactString(t, "aws_access_key_id = "+keyID+"\naws_secret_access_key = "+secret+"\n")

	if strings.Contains(got, keyID) || strings.Contains(got, secret) {
		t.Fatalf("credential pair leaked: %q", got)
	}
}

func TestRedact_DetectsSecretAcrossLargeInputWithoutBlankLines(t *testing.T) {
	// sources.Reader would force a chunk cut near 125 KB when no blank line
	// is found, dropping a match that straddles the cut.
	token := fakeGitHubPAT()
	// The token starts at byte 124986 and spans the 125000-byte cut.
	prefix := strings.Repeat("x\n", 124986/2)
	got := redactString(t, prefix+token+"\n"+strings.Repeat("y\n", 10000))

	if strings.Contains(got, token) {
		t.Fatal("token straddling a chunk boundary leaked")
	}
}

func TestApplyFindings_RedactsComponentOnlyAtItsLocation(t *testing.T) {
	// Components are often non-secret (usernames, hosts); masking every
	// repeat would erase unrelated text.
	host := "db.internal.example.com"
	text := "host=" + host + " key=SECRETVALUE\nsee " + host + " docs\n"
	findings := []report.Finding{{
		RuleID:   "parent-rule",
		Match:    report.Match{Value: "SECRETVALUE", Full: "key=SECRETVALUE"},
		Location: report.Location{StartLine: 1, EndLine: 1, StartColumn: 30, EndColumn: 44},
		ComponentSets: []report.ComponentSet{{Components: []report.ComponentFinding{{
			RuleID:   "host-component",
			Match:    report.Match{Value: host, Full: "host=" + host},
			Location: report.Location{StartLine: 1, EndLine: 1, StartColumn: 1, EndColumn: 28},
		}}}},
	}}

	got, err := applyFindings(text, findings)
	if err != nil {
		t.Fatal(err)
	}
	want := "host=[REDACTED:host-component] key=[REDACTED:parent-rule]\nsee " + host + " docs\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyFindings_FailsWhenDetectedSecretCannotBeLocated(t *testing.T) {
	secret := "decoded-secret-not-in-text"
	for name, f := range map[string]report.Finding{
		"decoded with bad location": {
			RuleID:      "test-rule",
			DecodeDepth: 1,
			Match:       report.Match{Value: secret},
			Location:    report.Location{StartLine: 9, EndLine: 9, StartColumn: 1, EndColumn: 3},
		},
		"component value absent": {
			RuleID:   "parent-rule",
			Match:    report.Match{Value: "visible"},
			Location: report.Location{StartLine: 1, EndLine: 1, StartColumn: 1, EndColumn: 7},
			ComponentSets: []report.ComponentSet{{Components: []report.ComponentFinding{{
				RuleID: "test-component",
				Match:  report.Match{Value: secret},
			}}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := applyFindings("visible text\n", []report.Finding{f})
			if err == nil {
				t.Fatal("expected an error for an unlocatable finding")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error message leaks the secret: %v", err)
			}
		})
	}
}

func TestWrap_WritesRedactedOutput(t *testing.T) {
	token := fakeGitHubPAT()
	render := Wrap(func(w io.Writer) error {
		_, err := io.WriteString(w, "token "+token+"\n")
		return err
	})

	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "token [REDACTED:github-pat]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestWrap_WritesNothingWhenRenderFails(t *testing.T) {
	renderErr := errors.New("boom")
	render := Wrap(func(w io.Writer) error {
		_, _ = io.WriteString(w, "token "+fakeGitHubPAT()+"\n")
		return renderErr
	})

	var buf bytes.Buffer
	if err := render(&buf); !errors.Is(err, renderErr) {
		t.Fatalf("err = %v, want %v", err, renderErr)
	}
	if buf.Len() != 0 {
		t.Fatalf("partial output written: %q", buf.String())
	}
}
