package cli

// T7 (speed-tokens): `trace record-batch --ndjson` records one flush in
// one spawn (same fields as `trace record`, including
// duration/output-bytes). Falla-antes: the subcommand does not exist
// yet, so every test below fails until traceRecordBatchCmd lands.

import (
	"strings"
	"testing"
)

func withBatchStdin(t *testing.T, stdin string) {
	t.Helper()
	old := traceStdin
	traceStdin = strings.NewReader(stdin)
	t.Cleanup(func() { traceStdin = old })
}

func TestTraceRecordBatchStdinRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	withBatchStdin(t, strings.Join([]string{
		`{"session":"s1","message":"m1","call":"c1","tool":"read","status":"completed","files":["a.go","b.go"],"task":"t7","duration_ms":120,"output_bytes":2048}`,
		`{"session":"s1","message":"m2","call":"c2","tool":"write","status":"completed","task":"t7","duration_ms":30,"output_bytes":512}`,
		`{"session":"s1","message":"m3","call":"c3","tool":"bash","status":"error"}`,
		"",
	}, "\n"))
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record-batch", "--ndjson"}); code != 0 {
		t.Fatalf("record-batch exit = %d, stderr %q", code, errb.String())
	}
	if got := out.String(); got != "trace recorded 3 entr(y/ies)\n" {
		t.Fatalf("record-batch output = %q, want one summary line", got)
	}

	cS, outS, errS := newCLI(nil)
	if code := cS.Run([]string{"trace", "show", "--session", "s1"}); code != 0 {
		t.Fatalf("show exit = %d, stderr %q", code, errS.String())
	}
	for _, want := range []string{"1 read completed", "2 write completed", "3 bash error"} {
		if !strings.Contains(outS.String(), want) {
			t.Fatalf("show must carry %q, got:\n%s", want, outS.String())
		}
	}

	cF, outF, errF := newCLI(nil)
	if code := cF.Run([]string{"trace", "files", "--session", "s1"}); code != 0 {
		t.Fatalf("files exit = %d, stderr %q", code, errF.String())
	}
	for _, want := range []string{"a.go", "b.go"} {
		if !strings.Contains(outF.String(), want) {
			t.Fatalf("files must carry %q, got:\n%s", want, outF.String())
		}
	}

	cC, outC, errC := newCLI(nil)
	if code := cC.Run([]string{"trace", "cost", "--task", "t7"}); code != 0 {
		t.Fatalf("cost exit = %d, stderr %q", code, errC.String())
	}
	for _, want := range []string{"2 call(s)", "150ms", "2560B"} {
		if !strings.Contains(outC.String(), want) {
			t.Fatalf("cost must carry %q, got:\n%s", want, outC.String())
		}
	}
}

func TestTraceRecordBatchArgvFallbackRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	withBatchStdin(t, "")
	line1 := `{"session":"s2","message":"m1","call":"c1","tool":"read","status":"completed"}`
	line2 := `{"session":"s2","message":"m2","call":"c2","tool":"write","status":"error","duration_ms":7,"output_bytes":9}`
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record-batch", "--ndjson", line1, line2}); code != 0 {
		t.Fatalf("record-batch exit = %d, stderr %q", code, errb.String())
	}
	if got := out.String(); got != "trace recorded 2 entr(y/ies)\n" {
		t.Fatalf("record-batch output = %q, want one summary line", got)
	}
	cS, outS, errS := newCLI(nil)
	if code := cS.Run([]string{"trace", "show", "--session", "s2"}); code != 0 {
		t.Fatalf("show exit = %d, stderr %q", code, errS.String())
	}
	for _, want := range []string{"1 read completed", "2 write error"} {
		if !strings.Contains(outS.String(), want) {
			t.Fatalf("show must carry %q, got:\n%s", want, outS.String())
		}
	}
}

func TestTraceRecordBatchMalformedLineRefused(t *testing.T) {
	t.Chdir(t.TempDir())
	withBatchStdin(t, "{\"session\":\"s\",\"message\":\"m\",\"call\":\"c1\",\"tool\":\"read\",\"status\":\"completed\"}\nnot json\n")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record-batch", "--ndjson"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "line 2") {
		t.Fatalf("stderr must name the bad line, got %q", errb.String())
	}
}

func TestTraceRecordBatchInvalidEntryRefused(t *testing.T) {
	t.Chdir(t.TempDir())
	withBatchStdin(t, "{\"session\":\"s\",\"message\":\"m\",\"call\":\"c1\",\"tool\":\"read\",\"status\":\"bogus\"}\n")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record-batch", "--ndjson"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "line 1") || !strings.Contains(errb.String(), "status must be") {
		t.Fatalf("stderr must name line 1 and the refusal, got %q", errb.String())
	}
}

func TestTraceRecordBatchRequiresNdjson(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record-batch"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus trace record-batch --ndjson") {
		t.Fatalf("stderr must print record-batch usage, got %q", errb.String())
	}
}

func TestTraceRecordBatchEmptyInput(t *testing.T) {
	t.Chdir(t.TempDir())
	withBatchStdin(t, "")
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record-batch", "--ndjson"}); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errb.String())
	}
	if got := out.String(); got != "trace recorded 0 entr(y/ies)\n" {
		t.Fatalf("output = %q, want the zero summary", got)
	}
}

// `trace record` stays untouched (backwards compat): same argv, same
// single line, same exit.
func TestTraceRecordUntouchedByBatch(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", "s9", "--message", "m1", "--call", "c1",
		"--tool", "read", "--status", "completed"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("record exit = %d, stderr %q", code, errb.String())
	}
	if got := out.String(); got != "trace recorded\n" {
		t.Fatalf("record output = %q, want the single line", got)
	}
}
