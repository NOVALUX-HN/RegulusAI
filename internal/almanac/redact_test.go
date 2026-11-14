package almanac

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactSecretFormats(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		check func(got string) bool
	}{
		{"bearer", "auth Bearer abcDEF123._~+/-=", func(got string) bool {
			return strings.Contains(got, "bearer "+Redacted) && !strings.Contains(got, "abcDEF123")
		}},
		{"bare kv equals", "api_key=secret123", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "secret123")
		}},
		{"bare kv colon", "token: hunter2", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "hunter2")
		}},
		{"json kv", `"api_key": "secret123"`, func(got string) bool {
			return strings.Contains(got, `"api_key": "`+Redacted+`"`) && !strings.Contains(got, "secret123")
		}},
		{"private key block", "head\n-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKC\n-----END RSA PRIVATE KEY-----\ntail", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "MIIEpAIBAAKC") && strings.Contains(got, "head") && strings.Contains(got, "tail")
		}},
		{"url creds", "fetch https://user:hunter2@example.com/repo now", func(got string) bool {
			return strings.Contains(got, "https://"+Redacted+"@example.com") && !strings.Contains(got, "hunter2")
		}},
		{"ghp prefix", "key ghp_abcDEF1234567890 here", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "ghp_abcDEF")
		}},
		{"gho prefix", "key gho_xyz987 here", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "gho_xyz987")
		}},
		{"sk-live prefix", "key sk-live_abcd1234-XYZ here", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "sk-live_abcd")
		}},
		{"sk-proj prefix", "key sk-proj_abcd1234_XYZ here", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "sk-proj_abcd")
		}},
		{"akia prefix", "id AKIAIOSFODNN7EXAMPLE here", func(got string) bool {
			return strings.Contains(got, Redacted) && !strings.Contains(got, "AKIAIOSFODNN7EXAMPLE")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := RedactObservation(Observation{
				Type: TypeDecision, Title: tc.in, Content: tc.in,
				TopicKey: tc.in, Domain: "go",
				Lesson:     &Lesson{Cue: tc.in, Lesson: tc.in, Evidence: []string{tc.in}, Confidence: "low"},
				Provenance: &Provenance{Paths: []string{tc.in}},
			})
			for _, field := range []string{o.Title, o.Content, o.TopicKey, o.Lesson.Cue, o.Lesson.Lesson, o.Lesson.Evidence[0], o.Provenance.Paths[0]} {
				if !tc.check(field) {
					t.Fatalf("%s: field not redacted as expected: %q", tc.name, field)
				}
			}
		})
	}
}

func TestRedactFalsePositivesIntact(t *testing.T) {
	benign := []string{
		`C:\Users\alice\project\main.go`,
		`/home/alice/project/main.go`,
		"token expired, please retry",
		"adjust the prompt wording for clarity",
		"the token bucket refills every minute",
	}
	for _, in := range benign {
		got := RedactObservation(Observation{Type: TypeDecision, Title: in, Content: in})
		if got.Title != in || got.Content != in {
			t.Fatalf("benign text must survive verbatim: %q -> title %q content %q", in, got.Title, got.Content)
		}
	}
}

func TestRedactSaveScrubsBeforePersist(t *testing.T) {
	root := t.TempDir()
	saved, err := Save(root, &Observation{
		Type: TypeDecision, Title: "deploys with api_key=hunter2", Content: "uses Bearer abc123",
	}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved.Title, "hunter2") || strings.Contains(saved.Content, "abc123") {
		t.Fatalf("returned copy must be redacted: %+v", saved)
	}
	if !strings.Contains(saved.Title, Redacted) || !strings.Contains(saved.Content, Redacted) {
		t.Fatalf("redaction marker missing: %+v", saved)
	}
	all, err := All(root)
	if err != nil || len(all) != 1 {
		t.Fatalf("all = %+v (%v)", all, err)
	}
	if strings.Contains(all[0].Title, "hunter2") || strings.Contains(all[0].Content, "abc123") {
		t.Fatalf("persisted bytes must be redacted: %+v", all[0])
	}
}

func TestRedactImportScrubsBeforePersist(t *testing.T) {
	root := t.TempDir()
	line, err := json.Marshal(Observation{
		ID: "obs_importsecret1", Type: TypeDiscovery, Title: "imported", Content: "token: hunter2 and ghp_secretABC123",
		SavedAt: t0,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "bundle.jsonl")
	if err := os.WriteFile(bundle, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := Import(root, bundle)
	if err != nil || got != 1 || skipped != 0 {
		t.Fatalf("import = %d/%d (%v)", got, skipped, err)
	}
	all, err := All(root)
	if err != nil || len(all) != 1 {
		t.Fatalf("all = %+v (%v)", all, err)
	}
	if strings.Contains(all[0].Content, "hunter2") || strings.Contains(all[0].Content, "ghp_secretABC123") {
		t.Fatalf("imported bytes must be redacted: %+v", all[0])
	}
	if !strings.Contains(all[0].Content, Redacted) {
		t.Fatalf("redaction marker missing after import: %+v", all[0])
	}
}

func TestRedactIdempotent(t *testing.T) {
	o := Observation{
		Type: TypeDecision, Title: "api_key=hunter2 plus Bearer abc123", Content: "https://user:pass@example.com/x and ghp_abc123",
		TopicKey: "t AKIAIOSFODNN7EXAMPLE", Domain: "d",
		Lesson:     &Lesson{Cue: "token: xyz", Lesson: `"secret": "s3cr3t"`, Evidence: []string{"-----BEGIN PRIVATE KEY-----\nABC\n-----END PRIVATE KEY-----"}, Confidence: "low"},
		Provenance: &Provenance{Paths: []string{"token: xyz"}},
	}
	once := RedactObservation(o)
	twice := RedactObservation(once)
	if twice.Title != once.Title || twice.Content != once.Content || twice.TopicKey != once.TopicKey {
		t.Fatalf("redaction must be idempotent:\nonce=%+v\ntwice=%+v", once, twice)
	}
	rawOnce, _ := json.Marshal(once)
	rawTwice, _ := json.Marshal(twice)
	if string(rawOnce) != string(rawTwice) {
		t.Fatalf("redaction must be byte-idempotent:\n%s\n%s", rawOnce, rawTwice)
	}
}
