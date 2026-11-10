package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// quietReport runs the text output with -errors-only and returns the exit code.
func quietReport(t *testing.T, issues []issue, known map[string]bool) int {
	t.Helper()
	o, err := newOutput(formatText, ".", true, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	code, err := o.write(issues, known)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

const casesJSON = `{"contractVersion":"1.1.0","service":"booking",
 "features":[{"key":"FTR.FMS.CAR-0005","phase":"codegen","required":true},{"key":"FTR.FMS.CAR-0007","phase":"spec","required":false},
  {"key":"FTR.FMS.CAR-0008","phase":"released","required":true}],
 "cases":[{"id":"MOCK-01","feature":"FTR.FMS.CAR-0005","level":"I"},{"id":"MOCK-02","feature":"FTR.FMS.CAR-0005","level":"U"},
  {"id":"MOCK-03","feature":"FTR.FMS.CAR-0005","level":"M"},{"id":"MOCK-09","feature":"FTR.FMS.CAR-0007","level":"U"},
  {"id":"SHARE-01","feature":"FTR.FMS.CAR-0005","level":"U"},{"id":"SHARE-01","feature":"FTR.FMS.CAR-0008","level":"U"},
  {"id":"BOOK-01","feature":"FTR.FMS.CAR-0008","level":"E"}]}`

func cases(t *testing.T) *serviceCases {
	t.Helper()
	var sc serviceCases
	if err := json.Unmarshal([]byte(casesJSON), &sc); err != nil {
		t.Fatal(err)
	}
	return &sc
}

// FTR.HMR.SPEC-0002 qa: the checks of the mode service.
func TestServiceChecks(t *testing.T) {
	code := parseGrep(strings.Join([]string{
		"internal/a_test.go:10:// MOCK-01: a booking",
		"internal/a_test.go:20:// SHARE-01 of FTR.FMS.CAR-0005", // a shared ID counts in a file that names the feature
		"internal/b_test.go:5:// MOCK-18 is a typo",             // R9: the prefix of the service
		"internal/c_test.go:7:// HTTP-2 is not ours",            // R9: another prefix is not an ID of the service
		"internal/d_test.go:1:// FTR.FMS.CAR-0008 qa",
		"internal/d_test.go:3:// MOCK-01", // R9: a case of another feature than the one the file names
	}, "\n"))
	issues := checkService(cases(t), code)
	var got []string
	for _, i := range issues {
		got = append(got, i.check+" "+i.path+" "+i.subject)
	}
	want := []string{
		"qa-not-in-code FTR.FMS.CAR-0005/qa FTR.FMS.CAR-0005 MOCK-02", // MOCK-03 is manual, MOCK-09 of a feature in spec
		"qa-not-in-code FTR.FMS.CAR-0008/qa FTR.FMS.CAR-0008 SHARE-01",
		"qa-not-in-code FTR.FMS.CAR-0008/qa FTR.FMS.CAR-0008 BOOK-01",
		"unknown-test-id internal/b_test.go MOCK-18",
		"unknown-test-id internal/d_test.go MOCK-01",
	}
	if strings.Join(got, "\n") != strings.Join(sortedLike(want, got), "\n") || len(got) != len(want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, i := range issues {
		if i.severity() != "error" && i.check == checkUnknownTestID {
			t.Fatalf("%s must be an error", i.check)
		}
	}
}

// sortedLike returns want when it holds the same lines as got, in the order of got.
func sortedLike(want, got []string) []string {
	set := map[string]bool{}
	for _, w := range want {
		set[w] = true
	}
	for _, g := range got {
		if !set[g] {
			return want
		}
	}
	return got
}

func TestFeatureKeyIsNotAnID(t *testing.T) {
	code := parseGrep("x.go:1:// FTR.FMS.CAR-0005 MOCK-01")
	if len(code.hits) != 1 || code.hits[0].id != "MOCK-01" || !code.files["x.go"]["FTR.FMS.CAR-0005"] {
		t.Fatalf("got %+v", code)
	}
}

func TestVersions(t *testing.T) {
	if w, err := compareVersions("1.1.0", "1.1.0"); w != nil || err != nil {
		t.Fatal("the same version")
	}
	if w, err := compareVersions("1.1.0", "1.2.0"); err != nil || w == nil || w.check != checkVersionMinor {
		t.Fatal("an older minor version warns")
	}
	if _, err := compareVersions("1.1.0", "2.0.0"); err == nil || !strings.Contains(err.Error(), "update speclint to 2.x") {
		t.Fatalf("another major version fails: %v", err)
	}
	if w, err := compareVersions("1.2.0", "1.1.0"); w != nil || err != nil {
		t.Fatal("a newer minor version is fine")
	}
}

func TestFetchCases(t *testing.T) {
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.Header.Get("Authorization") {
		case "Bearer good":
			if r.URL.Path != "/api/v1/ci/services/booking/test-cases" {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(casesJSON))
		case "Bearer old":
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"code":"token_revoked","message":"revoked"}}`))
		case "Bearer busy":
			w.WriteHeader(503)
		default:
			w.WriteHeader(401)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if sc, err := fetchCases(ctx, srv.Client(), srv.URL+"/", "booking", "good"); err != nil || len(sc.Cases) != 7 {
		t.Fatalf("good: %v", err)
	}
	var re *runError
	if _, err := fetchCases(ctx, srv.Client(), srv.URL, "booking", "old"); !errors.As(err, &re) || re.check != "token-revoked" {
		t.Fatalf("old: %v", err)
	}
	if _, err := fetchCases(ctx, srv.Client(), srv.URL, "pricing", "good"); !errors.As(err, &re) || re.check != "service-not-found" {
		t.Fatalf("pricing: %v", err)
	}
	if _, err := fetchCases(ctx, srv.Client(), srv.URL, "booking", ""); !errors.As(err, &re) || re.check != "token-invalid" {
		t.Fatalf("empty: %v", err)
	}
	calls.Store(0)
	if _, err := fetchCases(ctx, srv.Client(), srv.URL, "booking", "busy"); !errors.As(err, &re) || re.check != "hammurapi-unreachable" || !strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("busy: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("three attempts, got %d", calls.Load())
	}
}

// FTR.HMR.SPEC-0002 R2, R3: annotations on lines and the Code Quality report.
func TestFormats(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "specs", "a.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	issues := []issue{{"specs/a.md", checkReqNoCriteria, "R1", "R1 has no criteria, 100%", 3}, {"FTR.X.Y-0001/qa", checkNotInCode, "FTR.X.Y-0001 A-1", "m", 0}}
	t.Chdir(dir) // annotations name files relative to the directory of the run, the root of the repository
	var buf bytes.Buffer
	o, _ := newOutput(formatGitHub, ".", false, &buf)
	o.baseline = "speclint-baseline.txt"
	code, err := o.write(issues, map[string]bool{"gone x y": true})
	if err != nil || code != 1 {
		t.Fatalf("code %d, %v", code, err)
	}
	out := buf.String()
	for _, want := range []string{
		"::error file=specs/a.md,line=3,title=req-no-criteria::R1 has no criteria, 100%25",
		"::warning title=qa-not-in-code::m", // a document of Hammurapi is no file of the repository
		"::error file=speclint-baseline.txt,title=baseline::fixed, remove the line: gone x y",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	if b, _ := os.ReadFile(summary); !strings.Contains(string(b), "New errors: **1**") {
		t.Errorf("summary %s", b)
	}

	o, _ = newOutput(formatGitLab, ".", false, &bytes.Buffer{})
	if _, err := o.write(issues, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	var cq []codeQuality
	b, _ := os.ReadFile(filepath.Join(dir, codeQualityFile))
	if err := json.Unmarshal(b, &cq); err != nil || len(cq) != 2 {
		t.Fatalf("report %s: %v", b, err)
	}
	if cq[0].Severity != "major" || cq[0].Location.Lines.Begin != 3 || len(cq[0].Fingerprint) != 40 || cq[1].Severity != "minor" {
		t.Fatalf("entries %+v", cq)
	}
}

func TestVersionWarningWithErrorsOnly(t *testing.T) {
	var buf bytes.Buffer
	o, _ := newOutput(formatText, ".", true, &buf)
	if _, err := o.write([]issue{{"", checkVersionMinor, "1.2.0", "update speclint", 0}, {"p", checkNotInCode, "K", "hidden", 1}}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "update speclint") || strings.Contains(buf.String(), "hidden") {
		t.Fatalf("got %s", buf.String())
	}
}

func TestBaselineInsideRoot(t *testing.T) {
	if got := inside("svc", "svc/speclint-baseline.txt"); got != "speclint-baseline.txt" {
		t.Errorf("got %q", got)
	}
	if got := inside("svc", "speclint-baseline.txt"); got != "" {
		t.Errorf("a baseline outside the root, got %q", got)
	}
}
