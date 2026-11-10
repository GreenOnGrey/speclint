package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keys(t *testing.T, root string) string {
	t.Helper()
	issues, err := lint(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, i := range issues {
		out = append(out, i.check+" "+i.subject)
	}
	return strings.Join(out, "\n")
}

const validProduct = `# P

**R1.** First.

- Критерии приёмки:
  - Дано а, когда б, тогда в.

**R2. Titled.** Second.

- Критерии приёмки:
  - Дано а, когда б, тогда в.
`

const a = "specs/HMR/CMN/FTR.HMR.CMN-0001/"
const b = "specs/HMR/CMN/FTR.HMR.CMN-0002/"

func TestValidSpecsHaveNoIssues(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", validProduct)
	write(t, root, a+"qa/spec.md", "| ID | Требования | Сценарий | Ур. |\n|---|---|---|---|\n| `X-01` | R1, R2 | с | U |\n| X-02 | R1–R2 | с | M |\n| X-03 | — | с | U |\n")
	write(t, root, b+"product/spec.md", "**Опирается на:** `FTR.HMR.CMN-0001` R1, R2; `FTR.HMR.CMN-0001-R1`, пример `FTR.FMS.CAR-0005-R9`\n")
	if got := keys(t, root); got != "" {
		t.Errorf("unexpected issues:\n%s", got)
	}
}

func TestRequirementChecks(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", `**R1.** No criteria.

### Next

- Дано под заголовком не относится к R1.

**R2.** First.

- Дано а.

**R2.** Again.

- Дано а.

**R3a.** Inserted.

- Дано а.
`)
	want := "req-no-criteria R1\nreq-duplicate R2"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCodeBlocksAreIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", "```\n**R9.** Example.\nFTR.HMR.CMN-0404\n```\n")
	if got := keys(t, root); got != "" {
		t.Errorf("unexpected issues:\n%s", got)
	}
}

func TestQAChecks(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", validProduct)
	write(t, root, a+"qa/spec.md", "| ID | Требования | Сценарий | Ур. |\n|---|---|---|---|\n| X-01 | R1, R7 | с | U |\n| X-01 | R1 | с | U |\n| X-02 | R1–R3 | с | U |\n\n| Требования | Тест-кейсы |\n|---|---|\n| R99 | X-01 |\n")
	want := "qa-unknown-req X-01 R7\nqa-duplicate-id X-01\nqa-unknown-req X-02 R3"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFixResolvesRequirementsOfParent(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", validProduct)
	write(t, root, b+"product/spec.md", "---\nparent: FTR.HMR.CMN-0001  # comment\n---\n\n**R5.** New.\n\n- Дано а.\n")
	write(t, root, b+"qa/spec.md", "| ID | Требования | Сценарий | Ур. |\n|---|---|---|---|\n| F-01 | R1, R5 | с | U |\n| F-02 | R8 | с | U |\n")
	if got, want := keys(t, root), "qa-unknown-req F-02 R8"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReferences(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", validProduct)
	write(t, root, b+"tech/spec.md", "`FTR.HMR.CMN-0001` R1, R3; FTR.HMR.CMN-0001-R4; `FTR.HMR.CMN-0404`; FTR.NAB.CMN-0001\n")
	want := "ref-unknown-req FTR.HMR.CMN-0001 R3\nref-unknown-req FTR.HMR.CMN-0001 R4\nref-unknown-feature FTR.HMR.CMN-0404"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

const section = "## Изменения в других решениях\n\n| Решение | Требование | Что изменилось |\n|---|---|---|\n"

// LINT-01: a row of the section without a note in the changed feature; a row with
// an unknown requirement; a feature changed as a whole that does not mention it.
func TestBacklinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", `**R1.** Old.
*Изменено в `+"`FTR.HMR.CMN-0002`"+` R1: new.*

- Дано а.

**R2.** Changed, but the text does not say so.

- Дано а.
`)
	write(t, root, b+"product/spec.md", "# B\n\n**R1.** New.\n\n- Дано а.\n\n"+section+
		"| `FTR.HMR.CMN-0001` | R1 | new |\n| `FTR.HMR.CMN-0001` | R2 | new |\n| `FTR.HMR.CMN-0001` | R9 | no such |\n"+
		"| `FTR.HMR.CMN-0003` | всё решение | whole |\n| `FTR.<D>.<S>-NNNN` | R<n> | template |\n")
	write(t, root, "specs/HMR/CMN/FTR.HMR.CMN-0003/product/spec.md", "Не упоминает.\n")
	want := "changes-no-backlink FTR.HMR.CMN-0001 R2\nref-unknown-req FTR.HMR.CMN-0001 R9\nchanges-no-backlink FTR.HMR.CMN-0003"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// LINT-02: a note without a row in the section of its source. A note stands where Hammurapi puts it:
// under the requirement of its row, or in the header for a row of the whole feature.
func TestNotesWithoutChange(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", "# A\n\n**Версия:** 1.0\n*Изменено в `FTR.HMR.CMN-0002`: whole.*\n*Изменено в `FTR.HMR.CMN-0003`: only R1 is listed.*\n\n---\n\n"+
		"**R1.** Old.\n*Изменено в `FTR.HMR.CMN-0002` R1: listed.*\n\n- Дано а.\n\n"+
		"**R2.** Old.\n*Изменено в `FTR.HMR.CMN-0002` R1: not listed, the whole-feature row is in the header.*\n\n- Дано а.\n")
	write(t, root, b+"product/spec.md", "# B\n\n**R1.** New.\n\n- Дано а.\n\n"+section+"| `FTR.HMR.CMN-0001` | R1 | listed |\n| `FTR.HMR.CMN-0001` | всё решение | whole |\n")
	write(t, root, "specs/HMR/CMN/FTR.HMR.CMN-0003/product/spec.md", "# C\n\n"+section+"| `FTR.HMR.CMN-0001` | R1 | listed |\n")
	want := "note-without-change FTR.HMR.CMN-0003 header\nnote-without-change FTR.HMR.CMN-0002 R2\nchanges-no-backlink FTR.HMR.CMN-0001 R1"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The rows: one requirement per row, an example key is not a row, each table reads its own header,
// and only the first section counts, as in Hammurapi.
func TestChangeRows(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", validProduct)
	write(t, root, b+"product/spec.md", "# B\n\n"+section+"| `FTR.HMR.CMN-0001` | R1–R2 | a range |\n| `FTR.FMS.CAR-0005` | R1 | an example |\n\n"+
		"| Требование | Решение | Что изменилось |\n|---|---|---|\n| R9 | `FTR.HMR.CMN-0001` | swapped columns |\n\n"+
		"## Ещё раз: Изменения в других решениях\n\n| Решение | Требование | Что |\n|---|---|---|\n| `FTR.HMR.CMN-0001` | R7 | second section |\n")
	want := "ref-unknown-req FTR.HMR.CMN-0001 R1–R2\nref-unknown-feature FTR.FMS.CAR-0005\nref-unknown-req FTR.HMR.CMN-0001 R9"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// LINT-03: the line «Меняет:» is the old form.
func TestChangesLine(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"product/spec.md", validProduct)
	write(t, root, b+"product/spec.md", "# B\n\n**Меняет:** `FTR.HMR.CMN-0001` R1\n")
	if got := keys(t, root); got != "changes-line Меняет" {
		t.Errorf("got:\n%s", got)
	}
}

func TestExpand(t *testing.T) {
	for in, want := range map[string]string{
		"R1":          "R1",
		"R1, R26c":    "R1 R26c",
		"R12–R14":     "R12 R13 R14",
		"R12-14":      "R12 R13 R14",
		"R26a—R26c":   "R26a R26b R26c",
		"R26–R26b":    "R26 R26a R26b",
		"R25–R27a":    "R25 R26 R27 R27a",
		"R12e2e, R1":  "R12 R1",
		"п. 5, §3.1":  "",
		"AR1 R2 FR3x": "R2",
	} {
		if got := strings.Join(expand(in), " "); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBaseline(t *testing.T) {
	issues := []issue{{"p", checkRefFeature, "A", "m", 1}, {"p", checkNotInCode, "B", "m", 1}}
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.txt")
	if err := writeBaseline(path, issues); err != nil {
		t.Fatal(err)
	}
	known, err := readBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 1 || !known[issues[0].key()] {
		t.Fatalf("baseline holds errors only, got %v", known)
	}
	if code := quietReport(t, issues, known); code != 0 {
		t.Errorf("known errors and warnings must pass, got %d", code)
	}
	if code := quietReport(t, nil, known); code != 1 {
		t.Errorf("a fixed error left in the baseline must fail, got %d", code)
	}
	if code := quietReport(t, []issue{{"p", checkRefFeature, "C", "m", 1}}, known); code != 1 {
		t.Errorf("a new error must fail, got %d", code)
	}
}

func TestCodeLookup(t *testing.T) {
	f := &feature{key: "K", docs: map[string]*doc{"qa": {path: "q"}}}
	var got []issue
	refs := &codeRefs{ids: map[string]bool{}, qualified: map[string]bool{}}
	refs.addFile([]string{`it("A-01: works", () => {})`})
	checkCode(f, []testCase{{id: "A-01", level: "U"}, {id: "A-02", level: "I"}, {id: "A-03", level: "M"}}, refs, nil, func(i issue) { got = append(got, i) })
	if len(got) != 1 || !strings.Contains(got[0].msg, "1 of 2") || !strings.Contains(got[0].msg, "A-02") || strings.Contains(got[0].msg, "A-03") {
		t.Errorf("got %+v", got)
	}
}

func TestSharedCodeIDs(t *testing.T) {
	// RQ-02 is a case of two features: the test of one must not cover the case of the other
	refs := &codeRefs{ids: map[string]bool{}, qualified: map[string]bool{}}
	refs.addFile([]string{`// FTR.HMR.DEV-0001 qa: the writer`, `t.Run("RQ-02: asks the experts", ...)`})
	refs.addFile([]string{`t.Run("API-01, API-02", ...)`})
	shared := map[string]bool{"RQ-02": true}
	check := func(key string, cases ...testCase) []issue {
		var got []issue
		f := &feature{key: key, docs: map[string]*doc{"qa": {path: "q"}}}
		checkCode(f, cases, refs, shared, func(i issue) { got = append(got, i) })
		return got
	}
	if got := check("FTR.HMR.DEV-0001", testCase{id: "RQ-02", level: "I"}, testCase{id: "API-01", level: "U"}); len(got) != 0 {
		t.Errorf("a shared ID in a file of its feature, a unique ID in a file without a key: covered, got %+v", got)
	}
	got := check("FTR.HMR.DISC-0001", testCase{id: "RQ-02", level: "E"})
	if len(got) != 1 || !strings.Contains(got[0].msg, "1 of 1") || !strings.Contains(got[0].msg, "name FTR.HMR.DISC-0001 in the file") {
		t.Errorf("a shared ID in a file of another feature: missing, with the hint, got %+v", got)
	}
	refs.addFile([]string{`it("RQ-02: in a file without a key", () => {})`})
	if got := check("FTR.HMR.DISC-0001", testCase{id: "RQ-02", level: "E"}); len(got) != 1 {
		t.Errorf("a shared ID without a feature key covers no feature, got %+v", got)
	}
}

const archC4 = "# A\n\n## Затрагиваемые сервисы и контракты\n\n| Сервис | Роль |\n|---|---|\n| `booking` | x |\n\n" +
	"## Схема C4\n\n```mermaid\nC4Container\n  Container(booking, \"Бронь\")\n```\n"

// FTR.HMR.SPEC-0004: the services and the diagram of arch, the services of tech, the contract files (SVC-06, FILE-02,
// FILE-04), a fix without them, and the catalog of Hammurapi (SVC-04).
func TestServicesAndContracts(t *testing.T) {
	root := t.TempDir()
	write(t, root, a+"arch/spec.md", archC4)
	write(t, root, a+"tech/spec.md", "# T\n\n## Изменения по сервисам\n\n| Сервис | Требования |\n|---|---|\n| `booking` | R1 |\n| `pricing` | R2 |\n")
	write(t, root, a+"tech/booking.openapi.yaml", "openapi: 3.0.3\npaths: {}\n")
	write(t, root, a+"tech/billing.proto", "syntax = \"proto3\";\nmessage A {}\n")
	write(t, root, a+"tech/booking.proto", "message A {}\n")
	write(t, root, b+"arch/spec.md", "---\nparent: FTR.HMR.CMN-0001\n---\n\n# Fix\n")
	want := "contract-file-service " + a + "tech/billing.proto\ncontract-file-invalid " + a + "tech/booking.proto\ntech-service-not-in-arch pricing"
	if got := keys(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	// TPL-02: an arch of the contract 1.x fails, and the baseline keeps it from failing the run
	write(t, root, b+"arch/spec.md", "# Old\n\n## Затрагиваемые сервисы и контракты\n\nСвободный текст.\n")
	if got := keys(t, root); !strings.Contains(got, "c4-missing c4\narch-services-missing services") {
		t.Errorf("TPL-02 old arch:\n%s", got)
	}

	// SVC-04: the keys of arch against the catalog of Hammurapi
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ci/services" || r.Header.Get("Authorization") != "Bearer hmr_ci_x" {
			http.Error(w, "{}", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"services":["bookings","pricing"]}`))
	}))
	defer srv.Close()
	t.Setenv("HAMMURAPI_TOKEN", "hmr_ci_x")
	catalog, err := specCatalog(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	issues, err := lint(root, nil, catalog)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range issues {
		found = found || i.check == "arch-service-unknown" && i.subject == "booking" && strings.Contains(i.msg, "bookings?")
	}
	if !found {
		t.Errorf("SVC-04 %+v", issues)
	}

	// Hammurapi refuses: the catalog is skipped, not a failure
	t.Setenv("HAMMURAPI_TOKEN", "hmr_ci_other")
	if c, err := specCatalog(srv.URL); err != nil || c != nil {
		t.Errorf("refused: %v %v", c, err)
	}

	// without a token of CI the catalog is skipped, not a failure
	t.Setenv("HAMMURAPI_TOKEN", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("HAMMURAPI_ID_TOKEN", "")
	if c, err := specCatalog(srv.URL); err != nil || c != nil {
		t.Errorf("no token: %v %v", c, err)
	}
}
