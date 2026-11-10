// Command speclint checks the traceability of the specifications: requirements
// of product specs, test cases of qa specs and references between features. Its
// patterns follow the indexer of Hammurapi (hammurapi-core, specindex and
// platform/markdown), which is internal and cannot be imported.
//
// Errors fail the run unless they are listed in the baseline. Warnings never do.
//
//	speclint spec -root . -baseline speclint-baseline.txt                 # as in CI
//	speclint spec -root . -baseline speclint-baseline.txt -write-baseline # accept the current errors
//	speclint spec -root . -code ../nabu-core                              # also look for test case IDs in a repo
//	speclint service -hammurapi https://h.example -service booking        # the tests of a service (service.go)
//
// With -code, an ID that is a test case of several features counts for a feature
// only in a file that names its key (rules/qa/template.md).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/GreenOnGrey/speclint/v2/contract"
)

// Checks. Errors are broken links; warnings are debts that need a decision.
const (
	checkReqDuplicate  = "req-duplicate"
	checkReqNoCriteria = "req-no-criteria"
	checkQAUnknownReq  = "qa-unknown-req"
	checkQADuplicateID = "qa-duplicate-id"
	checkRefFeature    = "ref-unknown-feature"
	checkRefReq        = "ref-unknown-req"
	checkParent        = "parent-unknown"
	checkBacklink      = "changes-no-backlink"
	checkNoteNoChange  = "note-without-change"
	checkChangesLine   = "changes-line"
	checkNotInCode     = "qa-not-in-code"
	// FTR.HMR.DEV-0003: decisions A<n> of arch, the changes of arch and the contract of the rules
	checkDecUnnumbered = "decisions-unnumbered"
	checkDecDuplicate  = "decision-duplicate"
	checkRefDecision   = "ref-unknown-decision"
	checkWrongKind     = "changes-wrong-kind"
	checkTemplate      = "template-contract"
	// FTR.HMR.SPEC-0004: the services and the C4 diagram of arch, the services and the contract files of tech;
	// the names are those of the contract (contract.Check*)
)

var warnings = map[string]bool{checkNotInCode: true}

var (
	folderRe = regexp.MustCompile(`^FTR\.([A-Z][A-Z0-9]{1,9})\.[A-Z][A-Z0-9]{1,9}-\d{4}$`)
	// A requirement: **R1.**, **R1a.**, **FTR.X.Y-0001-R1.**, **R1. Title.**
	reqDefRe     = regexp.MustCompile(`^(?:[-*+]\s+)?\*\*(?:FTR\.[A-Z0-9.]+-\d+-)?(R\d+[a-z]?)(?:\.\*\*|\*\*|\.\s+[^*]+\*\*)`)
	headingRe    = regexp.MustCompile(`^#{1,6}\s`)
	criterionRe  = regexp.MustCompile(`(?i)^\s*(?:[-*+]|\d+[.)])\s+[*_` + "`" + `]*(?:Дано|Given)[\s,:]`)
	testIDRe     = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*-\d+$`)
	parentRe     = regexp.MustCompile(`(?m)^parent:\s*(\S+)`)
	changesRe    = regexp.MustCompile(`^\*\*Меняет:\*\*`) // the old form, replaced by the section (FTR.HMR.DEV-0002 R1)
	featureKeyRe = regexp.MustCompile(`FTR\.[A-Z][A-Z0-9]{1,9}\.[A-Z][A-Z0-9]{1,9}-\d{4}`)
	// noteRe is a note put into a changed feature: *Изменено в `FTR.X.Y-0001` R4: …*
	noteRe = regexp.MustCompile("^\\*Изменено в `(FTR\\.[A-Z][A-Z0-9]{1,9}\\.[A-Z][A-Z0-9]{1,9}-\\d{4})`")

	// A feature key, then what follows it: "FTR.X.Y-0001-R3" or "`FTR.X.Y-0001` R12–R14, R20".
	reqItem   = `R\d+(?:[a-z]\b)?(?:\s*[–—-]\s*R?\d+(?:[a-z]\b)?)?`
	keyRefRe  = regexp.MustCompile("(FTR\\.[A-Z][A-Z0-9]{1,9}\\.[A-Z][A-Z0-9]{1,9}-\\d{4})(?:-(R\\d+(?:[a-z]\\b)?))?`?(\\s+" + reqItem + "(?:,\\s*" + reqItem + ")*)?")
	reqItemRe = regexp.MustCompile(`\b(R)(\d+)((?:[a-z]\b)?)(?:\s*[–—-]\s*R?(\d+)((?:[a-z]\b)?))?`)
	// A code ID as it appears in tests.
	codeIDRe = `\b[A-Z][A-Z0-9]*(-[A-Z0-9]+)*-[0-9]+\b`
)

type line struct {
	n    int
	text string
}

type doc struct {
	path  string // relative to the root
	raw   string
	lines []line // outside code blocks
}

type requirement struct {
	id   string
	line int
	body []string
}

type feature struct {
	key     string
	dir     string // relative to the root: specs/<domain>/<group>/<key>
	domain  string
	parent  string
	docs    map[string]*doc
	reqs    map[string]*requirement
	changes []change
	// arch: its decisions A<n>, the rows of its section of changes and its notes
	decisions   map[string]bool
	archDoc     *contract.Document // parsed once by parseChanges
	archChanges []change
	archNotes   []contract.Note
}

// change is a row of the section «Изменения в других решениях» of a product
// spec (FTR.HMR.DEV-0002 tech §4.1): req is empty for the whole feature.
type change struct {
	target, req string
	line        int
}

type issue struct {
	path, check, subject, msg string
	line                      int
}

// key identifies an issue in the baseline: it has no line number, so unrelated
// edits of a document do not change it.
func (i issue) key() string { return i.check + " " + i.path + " " + i.subject }

func (i issue) severity() string {
	if warnings[i.check] {
		return "warning"
	}
	return "error"
}

func readDoc(root, rel string) (*doc, error) {
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	d := &doc{path: rel, raw: string(b)}
	fence := ""
	for i, l := range strings.Split(strings.ReplaceAll(d.raw, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		if fence == "" && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			fence = t[:3]
			continue
		}
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		d.lines = append(d.lines, line{i + 1, l})
	}
	return d, nil
}

func load(root string) (map[string]*feature, error) {
	paths, err := filepath.Glob(filepath.Join(root, "specs", "*", "*", "*", "*", "spec.md"))
	if err != nil {
		return nil, err
	}
	features := map[string]*feature{}
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		m := folderRe.FindStringSubmatch(parts[3])
		if m == nil {
			continue
		}
		d, err := readDoc(root, filepath.ToSlash(rel))
		if err != nil {
			return nil, err
		}
		f := features[parts[3]]
		if f == nil {
			f = &feature{key: parts[3], dir: strings.Join(parts[:4], "/"), domain: m[1], docs: map[string]*doc{}, reqs: map[string]*requirement{}}
			features[parts[3]] = f
		}
		f.docs[parts[4]] = d
		if pm := parentRe.FindStringSubmatch(frontMatter(d.raw)); pm != nil {
			f.parent = pm[1]
		}
	}
	return features, nil
}

func frontMatter(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(md, "---\n") {
		return ""
	}
	if end := strings.Index(md[4:], "\n---"); end >= 0 {
		return md[4 : 4+end]
	}
	return ""
}

// parseRequirements fills the requirements of a feature from its product spec.
// A requirement lasts until the next one or the next heading.
func parseRequirements(f *feature, report func(issue)) {
	d := f.docs["product"]
	if d == nil {
		return
	}
	var cur *requirement
	var all []*requirement
	for _, l := range d.lines {
		t := strings.TrimSpace(l.text)
		if headingRe.MatchString(t) {
			cur = nil
			continue
		}
		if m := reqDefRe.FindStringSubmatch(t); m != nil {
			cur = &requirement{id: m[1], line: l.n}
			all = append(all, cur)
			if f.reqs[m[1]] != nil {
				report(issue{d.path, checkReqDuplicate, m[1], m[1] + " is defined more than once", l.n})
				continue
			}
			f.reqs[m[1]] = cur
			continue
		}
		if cur != nil {
			cur.body = append(cur.body, l.text)
		}
	}
	for _, r := range all {
		if !hasCriteria(r) {
			report(issue{d.path, checkReqNoCriteria, r.id, r.id + " has no acceptance criteria", r.line})
		}
	}
}

func hasCriteria(r *requirement) bool {
	for _, b := range r.body {
		if criterionRe.MatchString(b) {
			return true
		}
	}
	return false
}

// expand lists the requirements of a fragment such as "R12–R14, R20, R26c".
func expand(s string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, m := range reqItemRe.FindAllStringSubmatch(s, -1) {
		add("R" + m[2] + m[3])
		if m[4] == "" {
			continue
		}
		from, _ := strconv.Atoi(m[2])
		to, _ := strconv.Atoi(m[4])
		if to < from {
			continue
		}
		if from == to {
			first := byte('a')
			if m[3] != "" {
				first = m[3][0] + 1
			}
			for l := first; m[5] != "" && l <= m[5][0]; l++ {
				add("R" + m[2] + string(l))
			}
			continue
		}
		for n := from + 1; n <= to; n++ {
			add("R" + strconv.Itoa(n))
		}
		if m[5] != "" {
			add("R" + m[4] + m[5])
		}
	}
	return out
}

// has reports whether a requirement exists in the feature or, for a fix, in the
// parent it changes.
func has(fs map[string]*feature, f *feature, id string) bool {
	for seen := map[string]bool{}; f != nil && !seen[f.key]; f = fs[f.parent] {
		seen[f.key] = true
		if f.reqs[id] != nil {
			return true
		}
	}
	return false
}

func splitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "|"), "|")
	cells := strings.Split(strings.ReplaceAll(s, `\|`, "\x00"), "|")
	for i, c := range cells {
		cells[i] = strings.TrimSpace(strings.ReplaceAll(c, "\x00", "|"))
	}
	return cells
}

type testCase struct {
	id, level string
	line      int
}

// parseQA reads the tables of test cases and checks the requirements they name.
func parseQA(fs map[string]*feature, f *feature, report func(issue)) []testCase {
	d := f.docs["qa"]
	if d == nil {
		return nil
	}
	var out []testCase
	seen := map[string]bool{}
	idCol, reqCol, levelCol := -1, -1, -1
	for _, l := range d.lines {
		t := strings.TrimSpace(l.text)
		if !strings.HasPrefix(t, "|") {
			idCol = -1
			continue
		}
		cells := splitRow(t)
		if idCol < 0 {
			reqCol, levelCol = -1, -1
			for i, c := range cells {
				switch {
				case c == "ID":
					idCol = i
				case strings.HasPrefix(c, "Требован"):
					reqCol = i
				case strings.HasPrefix(c, "Ур"):
					levelCol = i
				}
			}
			continue
		}
		if idCol >= len(cells) {
			continue
		}
		id := strings.Trim(cells[idCol], "`*")
		if !testIDRe.MatchString(id) {
			continue
		}
		if seen[id] {
			report(issue{d.path, checkQADuplicateID, id, id + " is listed more than once", l.n})
		}
		seen[id] = true
		tc := testCase{id: id, line: l.n}
		if levelCol >= 0 && levelCol < len(cells) {
			tc.level = cells[levelCol]
		}
		out = append(out, tc)
		if reqCol < 0 || reqCol >= len(cells) {
			continue
		}
		// Requirements after a key belong to that feature: ref checks handle them.
		own := cells[reqCol]
		if i := strings.Index(own, "FTR."); i >= 0 {
			own = own[:i]
		}
		if len(f.reqs) == 0 {
			continue
		}
		for _, r := range expand(own) {
			if !has(fs, f, r) {
				report(issue{d.path, checkQAUnknownReq, id + " " + r, id + " refers to " + r + ", which " + f.key + " does not define", l.n})
			}
		}
	}
	return out
}

type ref struct {
	target string
	reqs   []string
	line   int
}

func parseRefs(text string, n int) []ref {
	var out []ref
	for _, m := range keyRefRe.FindAllStringSubmatch(text, -1) {
		r := ref{target: m[1], line: n}
		if m[2] != "" {
			r.reqs = append(r.reqs, m[2])
		}
		r.reqs = append(r.reqs, expand(m[3])...)
		out = append(out, r)
	}
	return out
}

// checkRefs checks references between features. Keys of domains that have no
// specifications here (FTR.FMS.CAR-0005) are examples and are skipped.
func checkRefs(fs map[string]*feature, f *feature, domains map[string]bool, report func(issue)) {
	for _, d := range sortedDocs(f) {
		for _, l := range d.lines {
			for _, r := range parseRefs(l.text, l.n) {
				domain := strings.SplitN(r.target, ".", 3)[1]
				if !domains[domain] {
					continue
				}
				t := fs[r.target]
				if t == nil {
					report(issue{d.path, checkRefFeature, r.target, "refers to " + r.target + ", which does not exist", l.n})
					continue
				}
				if len(t.reqs) == 0 {
					continue
				}
				for _, id := range r.reqs {
					if !has(fs, t, id) {
						report(issue{d.path, checkRefReq, r.target + " " + id, "refers to " + r.target + " " + id + ", which does not exist", l.n})
					}
				}
			}
		}
	}
	if f.parent != "" && domains[strings.SplitN(f.parent+"..", ".", 3)[1]] && fs[f.parent] == nil {
		for _, d := range sortedDocs(f) {
			report(issue{d.path, checkParent, f.parent, "parent " + f.parent + " does not exist", 1})
		}
	}
}

// parseChanges reads the sections «Изменения в других решениях» of product and
// arch, and the decisions of arch, by the contract shared with Hammurapi.
func parseChanges(f *feature, report func(issue)) {
	rows := func(cs []contract.Change) []change {
		var out []change
		for _, c := range cs {
			out = append(out, change{target: c.Key, req: c.Item, line: c.Line})
		}
		return out
	}
	if d := f.docs["product"]; d != nil {
		f.changes = rows(contract.Parse(d.raw, contract.Product).Changes)
	}
	d := f.docs["arch"]
	if d == nil {
		return
	}
	a := contract.Parse(d.raw, contract.Arch)
	f.archDoc = &a
	f.decisions = map[string]bool{}
	for _, it := range a.Items {
		f.decisions[it.ID] = true
	}
	f.archChanges = rows(a.Changes)
	f.archNotes = a.Notes
	for _, v := range a.Violations {
		switch v.Element {
		case contract.ViolDecisionsUnnumb:
			report(issue{d.path, checkDecUnnumbered, strconv.Itoa(v.Line), "a decision without a number A<n>: " + v.Message, v.Line})
		case contract.ViolDecisionDup:
			report(issue{d.path, checkDecDuplicate, v.Message, v.Message + " is defined more than once", v.Line})
		}
	}
}

// checkBacklinks: a feature that changes another one lists it in its section,
// and the changed text must point back — a note «Изменено в …» or any mention
// of the changing feature in the requirement (whole feature: in its documents).
// Otherwise a reader of the changed feature takes the old text for the current
// behaviour. The old line «Меняет:» is an error: the section replaced it.
func checkBacklinks(fs map[string]*feature, f *feature, domains map[string]bool, report func(issue)) {
	for _, d := range sortedDocs(f) {
		for _, l := range d.lines {
			if changesRe.MatchString(strings.TrimSpace(l.text)) {
				report(issue{d.path, checkChangesLine, "Меняет", "the line «Меняет:» is replaced by the section «Изменения в других решениях» of product (a table: решение, требование, что изменилось)", l.n})
			}
		}
	}
	for _, c := range f.changes {
		t := fs[c.target]
		d := f.docs["product"]
		if contract.ItemKind(c.req) == contract.KindDecision {
			report(issue{d.path, checkWrongKind, c.target + " " + c.req, "product changes requirements R<n>; " + c.req + " is a decision of arch: list it in the section of arch", c.line})
			continue
		}
		if t == nil {
			// a key of a known domain is reported by checkRefs; a row of an unknown one (an example
			// of the template) is still applied by Hammurapi and refused as unknown_target
			if !domains[strings.SplitN(c.target, ".", 3)[1]] {
				report(issue{d.path, checkRefFeature, c.target, "a row of «Изменения в других решениях» names " + c.target + ", which is not a feature here: an example belongs in the text, not in the table", c.line})
			}
			continue
		}
		if c.req == "" {
			if !mentions(t, f.key) {
				report(issue{d.path, checkBacklink, c.target, "changes " + c.target + ", which does not mention " + f.key + ": add «*Изменено в `" + f.key + "`: …*» under its header", c.line})
			}
			continue
		}
		if len(expand(c.req)) != 1 || expand(c.req)[0] != c.req {
			report(issue{d.path, checkRefReq, c.target + " " + c.req, "a row of «Изменения в других решениях» takes one requirement: «" + c.req + "» — one row per requirement", c.line})
			continue
		}
		if len(t.reqs) == 0 {
			continue // as checkRefs: a document whose requirements are not parsed
		}
		q := findReq(fs, t, c.req)
		if q == nil {
			report(issue{d.path, checkRefReq, c.target + " " + c.req, c.target + " has no " + c.req, c.line})
			continue
		}
		if !strings.Contains(strings.Join(q.req.body, "\n")+q.doc.lineText(q.req.line), f.key) {
			report(issue{d.path, checkBacklink, c.target + " " + c.req, "changes " + c.target + " " + c.req + ", which does not mention " + f.key + ": add «*Изменено в `" + f.key + "` R<n>: …*» to " + c.req, c.line})
		}
	}
}

// checkNotes: a note «Изменено в `<key>`» in a product spec needs a row of the
// section of <key> about this feature — about its requirement, or about the
// whole feature; a note in the header needs any row (FTR.HMR.DEV-0002 R12).
func checkNotes(fs map[string]*feature, f *feature, report func(issue)) {
	d := f.docs["product"]
	if d == nil {
		return
	}
	cur := ""
	for _, l := range d.lines {
		t := strings.TrimSpace(l.text)
		if headingRe.MatchString(t) {
			cur = ""
			continue
		}
		if m := reqDefRe.FindStringSubmatch(t); m != nil {
			cur = m[1]
			continue
		}
		m := noteRe.FindStringSubmatch(t)
		if m == nil || fs[m[1]] == nil {
			continue
		}
		// where ApplyNotes puts it: under the requirement of the row, or in the header for the whole feature
		ok := false
		for _, c := range fs[m[1]].changes {
			ok = ok || c.target == f.key && c.req == cur
		}
		if !ok {
			at := cur
			if at == "" {
				at = "header"
			}
			report(issue{d.path, checkNoteNoChange, m[1] + " " + at, "a note of " + m[1] + " at " + f.key + " " + at + ", but its section «Изменения в других решениях» has no row about it (a note of the whole feature stands in the header)", l.n})
		}
	}
}

// reqIn is a requirement with the document that defines it.
type reqIn struct {
	req *requirement
	doc *doc
}

// findReq finds a requirement in the feature or, for a fix, in its parents (as has).
func findReq(fs map[string]*feature, f *feature, id string) *reqIn {
	for seen := map[string]bool{}; f != nil && !seen[f.key]; f = fs[f.parent] {
		seen[f.key] = true
		if r := f.reqs[id]; r != nil {
			return &reqIn{r, f.docs["product"]}
		}
	}
	return nil
}

func (d *doc) lineText(n int) string {
	for _, l := range d.lines {
		if l.n == n {
			return l.text
		}
	}
	return ""
}

func mentions(f *feature, key string) bool {
	for _, d := range f.docs {
		if strings.Contains(d.raw, key) {
			return true
		}
	}
	return false
}

func sortedDocs(f *feature) []*doc {
	var out []*doc
	for _, d := range f.docs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// codeRefs are the test case IDs found in code: every ID, and the IDs of a file
// that names a feature key, as a test file with "FTR.HMR.DEV-0001 qa" in it.
type codeRefs struct {
	ids       map[string]bool
	qualified map[string]bool // "<feature key> <ID>"
}

var codeIDPattern = regexp.MustCompile(codeIDRe)

// codeIDs collects what looks like a test case ID in the tracked files of repos, and with it the feature keys
// named in the same file: the search of the mode service (scanRepo).
func codeIDs(repos []string) (*codeRefs, error) {
	refs := &codeRefs{ids: map[string]bool{}, qualified: map[string]bool{}}
	for _, r := range repos {
		hits, err := scanRepo(r, "")
		if err != nil {
			return nil, err
		}
		refs.add(hits)
	}
	return refs, nil
}

func (c *codeRefs) add(r *repoHits) {
	for _, h := range r.hits {
		c.ids[h.id] = true
		for k := range r.files[h.file] {
			c.qualified[k+" "+h.id] = true
		}
	}
}

// addFile records the IDs of the lines of one file and the feature keys named in them.
func (c *codeRefs) addFile(lines []string) {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "f:%d:%s\n", i+1, l)
	}
	c.add(parseGrep(b.String()))
}

// checkCode reports, per feature, automated test cases whose ID is in no repo.
// An ID that is a case of several features (shared) counts for a feature only
// when its file names that feature's key: otherwise a test of one feature would
// cover the case of another. The result is an estimate: a match does not prove
// that the test exists, and a test named without its ID counts as missing.
func checkCode(f *feature, cases []testCase, refs *codeRefs, shared map[string]bool, report func(issue)) {
	var missing, unqualified []string
	auto := 0
	for _, c := range cases {
		if c.level == "M" {
			continue
		}
		auto++
		switch {
		case shared[c.id] && refs.qualified[f.key+" "+c.id]:
		case shared[c.id]:
			missing = append(missing, c.id)
			if refs.ids[c.id] {
				unqualified = append(unqualified, c.id)
			}
		case !refs.ids[c.id]:
			missing = append(missing, c.id)
		}
	}
	if len(missing) > 0 {
		msg := fmt.Sprintf("%d of %d automated test cases are not in the code: %s", len(missing), auto, strings.Join(missing, " "))
		if len(unqualified) > 0 {
			msg += fmt.Sprintf("; %s are cases of other features too: name %s in the file of the test", strings.Join(unqualified, " "), f.key)
		}
		report(issue{f.docs["qa"].path, checkNotInCode, f.key, msg, 1})
	}
}

// lint checks the repository at root; repos are searched for test case IDs, and catalog, when it is not nil, holds
// the keys of the services of Hammurapi.
func lint(root string, repos, catalog []string) ([]issue, error) {
	fs, err := load(root)
	if err != nil {
		return nil, err
	}
	var refs *codeRefs
	if len(repos) > 0 {
		if refs, err = codeIDs(repos); err != nil {
			return nil, err
		}
	}
	var issues []issue
	report := func(i issue) { issues = append(issues, i) }
	domains := map[string]bool{}
	var keys []string
	for k, f := range fs {
		keys = append(keys, k)
		domains[f.domain] = true
	}
	sort.Strings(keys)
	for _, k := range keys {
		parseRequirements(fs[k], report)
		parseChanges(fs[k], report)
	}
	cases := map[string][]testCase{}
	owners := map[string]map[string]bool{} // test case ID → the features that have it
	for _, k := range keys {
		f := fs[k]
		cases[k] = parseQA(fs, f, report)
		for _, c := range cases[k] {
			if owners[c.id] == nil {
				owners[c.id] = map[string]bool{}
			}
			owners[c.id][k] = true
		}
		checkRefs(fs, f, domains, report)
		checkBacklinks(fs, f, domains, report)
		checkNotes(fs, f, report)
		checkArch(fs, f, domains, report)
		if err := checkServices(root, f, catalog, report); err != nil {
			return nil, err
		}
	}
	checkTemplates(root, report)
	shared := map[string]bool{}
	for id, o := range owners {
		shared[id] = len(o) > 1
	}
	for _, k := range keys {
		if refs != nil && len(cases[k]) > 0 {
			checkCode(fs[k], cases[k], refs, shared, report)
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].path != issues[j].path {
			return issues[i].path < issues[j].path
		}
		return issues[i].line < issues[j].line
	})
	return issues, nil
}

func readBaseline(path string) (map[string]bool, error) {
	out := map[string]bool{}
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if t := s.Text(); t != "" && !strings.HasPrefix(t, "#") {
			out[t] = true
		}
	}
	return out, s.Err()
}

func writeBaseline(path string, issues []issue) error {
	set := map[string]bool{}
	for _, i := range issues {
		if i.severity() == "error" {
			set[i.key()] = true
		}
	}
	var lines []string
	for k := range set {
		lines = append(lines, k)
	}
	sort.Strings(lines)
	content := "# Errors known to speclint when it was added; remove a line when it is fixed.\n" + strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

type repoList []string

func (r *repoList) String() string     { return strings.Join(*r, ",") }
func (r *repoList) Set(v string) error { *r = append(*r, v); return nil }

// version is set at build time (-X main.version=<tag>); otherwise it is the
// version of the module, as go run …@v1.0.1 and go install build it.
var version = ""

func moduleVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func main() {
	args := os.Args[1:]
	cmd := "spec"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "spec":
		runSpec(args)
	case "service":
		runService(args)
	case "migrate-arch":
		runMigrateArch(args)
	case "version":
		fmt.Printf("speclint %s, contract %s\n", moduleVersion(), contract.Version)
	default:
		fmt.Fprintf(os.Stderr, "speclint: unknown command %q; commands: spec, service, migrate-arch, version\n", cmd)
		os.Exit(2)
	}
}

// runMigrateArch turns every arch/spec.md of the repository into the format
// of the contract (FTR.HMR.DEV-0003 R25) and prints the changed files.
func runMigrateArch(args []string) {
	fl := flag.NewFlagSet("migrate-arch", flag.ExitOnError)
	root := fl.String("root", ".", "root of the specifications repository")
	_ = fl.Parse(args)
	paths, err := filepath.Glob(filepath.Join(*root, "specs", "*", "*", "*", "arch", "spec.md"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "speclint:", err)
		os.Exit(2)
	}
	sort.Strings(paths)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "speclint:", err)
			os.Exit(2)
		}
		out := contract.MigrateArch(string(b))
		if out == string(b) {
			continue
		}
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "speclint:", err)
			os.Exit(2)
		}
		fmt.Println(p)
	}
}

func runSpec(args []string) {
	fl := flag.NewFlagSet("spec", flag.ExitOnError)
	root := fl.String("root", ".", "root of the specifications repository")
	baseline := fl.String("baseline", "", "file of known errors, which do not fail the run")
	update := fl.Bool("write-baseline", false, "write the current errors to the baseline and exit")
	quiet := fl.Bool("errors-only", false, "print only new errors and the summary")
	format := fl.String("format", "text", "output: text, github (annotations and a job summary) or gitlab (and a Code Quality report)")
	base := fl.String("hammurapi", "", "address of Hammurapi: the keys of services in arch are checked against its catalog")
	// The inputs of the GitHub action are the same for both modes: spec takes and ignores -service.
	_ = fl.String("service", "", "ignored by spec")
	var repos repoList
	fl.Var(&repos, "code", "git repository to look for test case IDs in; repeatable")
	_ = fl.Parse(args)
	out := mustOutput(*format, *root, *quiet)

	var catalog []string
	if *base != "" {
		var err error
		if catalog, err = specCatalog(*base); err != nil {
			fail(err)
		}
	}
	issues, err := lint(*root, repos, catalog)
	if err != nil {
		fail(err)
	}
	finish(issues, *baseline, *update, out)
}

// finish writes the baseline, or compares the issues with it and exits with the code of the run.
func finish(issues []issue, baseline string, update bool, out output) {
	if update {
		if err := writeBaseline(baseline, issues); err != nil {
			fail(err)
		}
		return
	}
	known, err := readBaseline(baseline)
	if err != nil {
		fail(err)
	}
	out.baseline = baseline
	code, err := out.write(issues, known)
	if err != nil {
		fail(err)
	}
	os.Exit(code)
}

// fail stops a run that could not check (exit code 2, FTR.HMR.SPEC-0002 tech §2).
func fail(err error) {
	fmt.Fprintln(os.Stderr, "speclint:", err)
	os.Exit(2)
}

// decRefRe is a decision named in text: FTR.X.Y-0001-A3, or `FTR.X.Y-0001` A3, A5.
var decRefRe = regexp.MustCompile("(FTR\\.[A-Z][A-Z0-9]{1,9}\\.[A-Z][A-Z0-9]{1,9}-\\d{4})(?:-(A\\d+(?:[a-z]\\b)?))?`?((?:\\s*\\(?\\s*A\\d+(?:[a-z]\\b)?(?:\\s*[–—-]\\s*A?\\d+(?:[a-z]\\b)?)?\\)?,?)*)")

// checkArch checks the decisions A<n> of arch (FTR.HMR.DEV-0003 R10–R14):
// references to decisions, the rows of the section of changes of arch with
// their notes in the changed feature, and the notes of other features under
// the decisions of this one.
func checkArch(fs map[string]*feature, f *feature, domains map[string]bool, report func(issue)) {
	for _, d := range sortedDocs(f) {
		for _, l := range d.lines {
			for _, m := range decRefRe.FindAllStringSubmatch(l.text, -1) {
				ids := contract.ParseItems(m[2] + " " + m[3])
				t := fs[m[1]]
				if len(ids) == 0 || t == nil || len(t.decisions) == 0 || !domains[strings.SplitN(m[1], ".", 3)[1]] {
					continue // no decision, or a feature whose arch has no numbered decisions yet
				}
				for _, id := range ids {
					if contract.ItemKind(id) == contract.KindDecision && !t.decisions[id] {
						report(issue{d.path, checkRefDecision, m[1] + " " + id, "refers to " + m[1] + " " + id + ", which does not exist", l.n})
					}
				}
			}
		}
	}
	d := f.docs["arch"]
	for _, c := range f.archChanges {
		t := fs[c.target]
		if contract.ItemKind(c.req) == contract.KindRequirement {
			report(issue{d.path, checkWrongKind, c.target + " " + c.req, "arch changes only decisions A<n>; " + c.req + " is a requirement: list it in the section of product", c.line})
			continue
		}
		if t == nil {
			if !domains[strings.SplitN(c.target, ".", 3)[1]] {
				report(issue{d.path, checkRefFeature, c.target, "a row of «Изменения в других решениях» names " + c.target + ", which is not a feature here: an example belongs in the text, not in the table", c.line})
			}
			continue
		}
		if c.req == "" {
			if !mentions(t, f.key) {
				report(issue{d.path, checkBacklink, c.target, "changes " + c.target + ", which does not mention " + f.key + ": add «*Изменено в `" + f.key + "`: …*» under the header of its arch", c.line})
			}
			continue
		}
		if !t.decisions[c.req] {
			report(issue{d.path, checkRefDecision, c.target + " " + c.req, c.target + " has no decision " + c.req, c.line})
			continue
		}
		noted := false
		for _, n := range t.archNotes {
			noted = noted || n.Source == f.key && n.Target == c.req
		}
		if !noted {
			report(issue{d.path, checkBacklink, c.target + " " + c.req, "changes " + c.target + " " + c.req + ", which has no note of " + f.key + ": add «- *" + c.req + " — изменено в `" + f.key + "` A<n>: …*» under its table of decisions", c.line})
		}
	}
	if d == nil {
		return
	}
	for _, n := range f.archNotes {
		src := fs[n.Source]
		if src == nil || n.Target == "" {
			continue
		}
		ok := false
		for _, c := range src.archChanges {
			ok = ok || c.target == f.key && c.req == n.Target
		}
		if !ok {
			report(issue{d.path, checkNoteNoChange, n.Source + " " + n.Target, "a note of " + n.Source + " at " + f.key + " " + n.Target + ", but the section of changes of its arch has no row about it", n.Line})
		}
	}
}

// checkTemplates checks the rules templates against the contract (FTR.HMR.DEV-0003 R19).
func checkTemplates(root string, report func(issue)) {
	paths, _ := filepath.Glob(filepath.Join(root, "rules", "*", "*.md"))
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		file := strings.TrimSuffix(filepath.Base(p), ".md")
		if file != contract.FileTemplate && file != contract.FileFixTemplate {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		area := contract.Area(filepath.Base(filepath.Dir(p)))
		for _, v := range contract.CheckTemplate(area, file, string(b)) {
			report(issue{rel, checkTemplate, v.Element, v.Message, max(v.Line, 1)})
		}
	}
}

// checkServices checks the services and the C4 diagram of arch, the services of tech and the contract files next to
// tech (FTR.HMR.SPEC-0004 R1–R4, R6).
func checkServices(root string, f *feature, catalog []string, report func(issue)) error {
	add := func(path string, fs []contract.Finding) {
		for _, x := range fs {
			report(issue{path, x.Check, x.Subject, x.Message, max(x.Line, 1)})
		}
	}
	var tech *contract.Document
	arch := f.archDoc
	if d := f.docs["arch"]; d != nil && arch != nil {
		add(d.path, contract.CheckArch(*arch))
		// an arch of 1.x does not name the services by keys (R9): its free text is not checked against the catalog
		if catalog != nil && arch.NamesServices() {
			add(d.path, contract.CheckCatalog(*arch, catalog))
		}
	}
	if d := f.docs["tech"]; d != nil {
		t := contract.Parse(d.raw, contract.Tech)
		tech = &t
		add(d.path, contract.CheckTech(t, arch))
	}
	entries, err := os.ReadDir(filepath.Join(root, f.dir, "tech"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	services := contract.FeatureServices(arch, tech)
	for _, e := range entries {
		if _, _, ok := contract.ContractFile(e.Name()); !ok || e.IsDir() {
			continue
		}
		rel := f.dir + "/tech/" + e.Name()
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		add(rel, contract.CheckContractFile(rel, b, services))
	}
	return nil
}

// specCatalog reads the catalog of services of Hammurapi for spec -hammurapi (FTR.HMR.SPEC-0004 R2). When the catalog
// is not available — no token of CI, Hammurapi refuses or is unreachable — the check of the catalog is skipped with a
// note and the rest of the run goes on: Hammurapi checks the keys at the submission anyway.
func specCatalog(base string) ([]string, error) {
	ctx, client := context.Background(), &http.Client{}
	if _, err := apiURL(base, ""); err != nil {
		return nil, err
	}
	skip := func(err error) ([]string, error) {
		msg := err.Error()
		var re *runError
		if errors.As(err, &re) {
			msg = re.msg
		}
		fmt.Fprintln(os.Stderr, "speclint: the keys of services are not checked against the catalog:", msg)
		return nil, nil
	}
	token, err := ciToken(ctx, client, strings.TrimRight(base, "/"), os.Getenv)
	if err != nil {
		return skip(err)
	}
	catalog, err := fetchCatalog(ctx, client, base, token)
	if err != nil {
		return skip(err)
	}
	return catalog, nil
}
