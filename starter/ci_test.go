package starter

import (
	"strings"
	"testing"
)

// FTR.HMR.SPEC-0002 R6, R7, R11: the files of the check, their version line and the marker.
func TestCIFiles(t *testing.T) {
	svc := CIParams{Version: "1.2.0", Mode: "service", Hammurapi: "https://hammurapi.example", Service: "booking", Branch: "trunk"}
	for _, tc := range []struct {
		provider, path string
		p              CIParams
		want, not      []string
	}{
		{"github", GitHubCIPath, svc, []string{"uses: GreenOnGrey/speclint@v1.2.0", "mode: service", `service: "booking"`, `branches: ["trunk"]`, "${{ secrets.HAMMURAPI_SERVICE_TOKEN }}"}, nil},
		{"github", GitHubCIPath, CIParams{Version: "1.2.0", Mode: "spec"}, []string{"mode: spec", `branches: ["main"]`}, []string{"token:", "service:"}},
		{"gitlab", GitLabCIPath, svc, []string{`SPECLINT_VERSION: "1.2.0"`, `HAMMURAPI_SERVICE: "booking"`, "-format gitlab", "codequality: gl-code-quality-report.json"}, nil},
		{"gitlab", GitLabCIPath, CIParams{Version: "1.2.0", Mode: "spec"}, []string{`SPECLINT_MODE: "spec"`}, []string{"HAMMURAPI_URL"}},
	} {
		path, b, err := CIFile(tc.provider, tc.p)
		if err != nil || path != tc.path {
			t.Fatalf("%s %s: %s %v", tc.provider, tc.p.Mode, path, err)
		}
		s := string(b)
		if !strings.HasPrefix(s, CIMarker+"\n") {
			t.Errorf("%s: no marker", tc.provider)
		}
		for _, w := range tc.want {
			if !strings.Contains(s, w) {
				t.Errorf("%s %s: no %q in\n%s", tc.provider, tc.p.Mode, w, s)
			}
		}
		for _, w := range tc.not {
			if strings.Contains(s, w) {
				t.Errorf("%s %s: %q in\n%s", tc.provider, tc.p.Mode, w, s)
			}
		}
		if v, ok := CIVersion(b); !ok || v != "1.2.0" {
			t.Errorf("%s: version %q %v", tc.provider, v, ok)
		}
		bumped, err := SetCIVersion(b, "1.3.0")
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := CIVersion(bumped); v != "1.3.0" || strings.Count(string(bumped), "\n") != strings.Count(s, "\n") {
			t.Errorf("%s: bump to %q", tc.provider, v)
		}
		if diff := lineDiff(s, string(bumped)); diff != 1 {
			t.Errorf("%s: the bump changes %d lines", tc.provider, diff)
		}
	}
}

func lineDiff(a, b string) int {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	n := 0
	for i := range x {
		if x[i] != y[i] {
			n++
		}
	}
	return n
}

func TestCIFileRefusesBadInput(t *testing.T) {
	for _, p := range []CIParams{
		{Version: "1.2", Mode: "spec"},
		{Version: "1.2.0", Mode: "other"},
		{Version: "1.2.0", Mode: "service"},
		{Version: "1.2.0", Mode: "service", Hammurapi: "https://h", Service: "a\"\nb"},
	} {
		if _, _, err := CIFile("github", p); err == nil {
			t.Errorf("%+v must be refused", p)
		}
	}
}

func TestCIVersionOfManualFiles(t *testing.T) {
	if _, ok := CIVersion([]byte("name: speclint\n- uses: GreenOnGrey/speclint@v1.2.0\n")); ok {
		t.Error("no marker: manual")
	}
	if _, ok := CIVersion([]byte(CIMarker + "\n- uses: GreenOnGrey/speclint@abc123\n")); ok {
		t.Error("pinned by a digest: manual")
	}
	if c, ok := CompareVersions("1.10.0", "1.9.3"); !ok || c != 1 {
		t.Error("compare by numbers")
	}
}
