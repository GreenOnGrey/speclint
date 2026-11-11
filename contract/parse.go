// Package contract is the machine structure of the documents of Hammurapi
// (FTR.HMR.DEV-0003 R17): what the indexer of Hammurapi and speclint read in
// every document, whatever the rules template of an installation says. The
// contract reads Russian and English and writes one of them (R22).
package contract

import (
	"regexp"
	"strconv"
	"strings"
)

// Version is the version of the contract: the version of this module. A new
// major version means a document that passed the previous one may fail it.
const Version = "2.1.0"

// Area is a gate of a feature.
type Area string

// Areas of a feature.
const (
	Product Area = "product"
	Design  Area = "design"
	Arch    Area = "arch"
	Tech    Area = "tech"
	QA      Area = "qa"
)

// Kinds of an item of a document.
const (
	KindRequirement = "requirement" // R<n> of product
	KindDecision    = "decision"    // A<n> of arch
)

// Ref is a feature named in the line «Опирается на» with the items named next
// to it (R12–R16, A3).
type Ref struct {
	Key   string
	Items []string
}

// Item is a requirement R<n> of product or a decision A<n> of arch.
type Item struct {
	ID   string
	Kind string
	Text string
	Line int
}

// Change is a row of the section «Изменения в других решениях»: the changed
// feature, the item as written ("" for the whole feature) and what changed.
type Change struct {
	Key  string
	Item string
	Text string
	Line int
}

// Note is a note of another feature in this document: «Изменено в `<source>`».
type Note struct {
	Target     string // the item of this document, "" for the whole feature
	Source     string // the key of the feature that changed it
	SourceItem string // the item of the source describing the change, "" if none
	Text       string
	Line       int
}

// Violation is a broken element of the contract.
type Violation struct {
	Element string
	Message string
	Line    int
}

// Elements of the contract (FTR.HMR.DEV-0003 tech §3.1).
const (
	ElemDependsOn        = "depends_on"
	ElemRequirement      = "requirement"
	ElemDecisions        = "decisions"
	ElemChanges          = "changes_section"
	ElemParent           = "parent"
	ViolDecisionsUnnumb  = "decisions-unnumbered"
	ViolRequirementUnnum = "requirement-unnumbered"
	ViolChangesMalformed = "changes-malformed"
	ViolDependsMalformed = "depends-on-malformed"
	ViolDecisionDup      = "decision-duplicate"
)

// Document is what the contract reads in a document of an area.
type Document struct {
	Title      string
	DependsOn  []Ref
	Items      []Item
	Changes    []Change
	Notes      []Note
	Parent     string
	Violations []Violation
	// FTR.HMR.SPEC-0004: the services of arch (the table of affected services) and of tech (the changes by
	// service), the line of their section (0 if none), whether the section has the table, and the C4 diagram of arch.
	Services     []Service
	ServicesLine int
	HasServices  bool
	C4           *C4Block
}

var (
	// reqRe is the first line of a requirement: **R1.**, **FTR.X.Y-0001-R1.**, **R1. Title.** text.
	reqRe = regexp.MustCompile(`^\*\*(?:` + FeatureKey + `-)?(` + ReqID + `)(?:\.\*\*|\*\*|\.\s+([^*]+)\*\*)\.?\s*(.*)$`)
	// itemRefRe is an item named in text: R3, R3a, R12–R16, A5, A3–A4.
	itemRefRe = regexp.MustCompile(`\b([RA])(\d+)((?:[a-z]\b)?)(?:\s*[–—-]\s*[RA]?(\d+)((?:[a-z]\b)?))?`)
	// noteRes are the notes of other features: a line of product, a list item under the decisions of arch.
	noteRe     = regexp.MustCompile("^\\*(?i:" + strings.Join(quoteAll(allWords("changed_in")), "|") + ") `(" + FeatureKey + ")`(?: ((?:" + ReqID + "|" + DecisionID + ")(?:,\\s*(?:" + ReqID + "|" + DecisionID + "))*))?(?::\\s*(.*?))?\\.?\\*$")
	archNoteRe = regexp.MustCompile("^(?:[-*+]\\s+)?\\*(" + DecisionID + ") — (?i:" + strings.Join(quoteAll(allWords("changed_in")), "|") + ") `(" + FeatureKey + ")`(?: (" + DecisionID + "))?(?::\\s*(.*?))?\\.?\\*$")
)

func quoteAll(ws []string) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = regexp.QuoteMeta(w)
	}
	return out
}

// Parse reads a document of an area. It never fails: what it cannot read is a
// violation or is absent.
func Parse(doc string, area Area) Document {
	front, body, off := splitFrontMatter(doc)
	ls := lines(body, off)
	d := Document{Parent: front["parent"]}
	for _, l := range ls {
		if lvl, _, ok := heading(l); ok && lvl == 1 {
			d.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l.text), "#"))
			break
		}
	}
	d.DependsOn, d.Violations = parseDependsOn(ls)
	switch area {
	case Product:
		d.Items = parseRequirements(ls)
		d.Violations = append(d.Violations, unnumberedRequirements(ls)...)
		d.Notes = parseNotes(ls)
	case Arch:
		items, viol := parseDecisions(ls)
		d.Items = items
		d.Violations = append(d.Violations, viol...)
		d.Notes = append(parseNotes(ls), parseArchNotes(ls)...)
		d.Services, d.ServicesLine, d.HasServices = parseServices(ls, "services", true)
		d.C4 = parseC4Section(ls)
	case Tech:
		d.Services, d.ServicesLine, d.HasServices = parseServices(ls, "tech_services", false)
	}
	if area == Product || area == Arch {
		ch, viol := parseChanges(ls)
		d.Changes = ch
		d.Violations = append(d.Violations, viol...)
	}
	return d
}

// header is the lines of the header of a document: up to the rule "---" or the
// first section heading below the title.
func header(ls []line) []line {
	for i, l := range ls {
		if l.code {
			continue
		}
		t := strings.TrimSpace(l.text)
		if lvl, _, ok := heading(l); ok && lvl > 1 || t == "---" && i > 0 {
			return ls[:i]
		}
	}
	return ls
}

// parseDependsOn reads «**Опирается на:**»: every feature key with the items
// named after it up to the next key.
func parseDependsOn(ls []line) ([]Ref, []Violation) {
	re := regexp.MustCompile(`^\*\*(?i:` + strings.Join(quoteAll(allWords("depends_on")), "|") + `):\*\*(.*)$`)
	for _, l := range header(ls) {
		m := re.FindStringSubmatch(strings.TrimSpace(l.text))
		if m == nil || l.code {
			continue
		}
		rest := m[1]
		locs := featureKeyRe.FindAllStringIndex(rest, -1)
		if len(locs) == 0 {
			return nil, []Violation{{Element: ViolDependsMalformed, Message: "the line names no feature key", Line: l.n}}
		}
		var refs []Ref
		seen := map[string]int{}
		for i, loc := range locs {
			key := rest[loc[0]:loc[1]]
			end := len(rest)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			items := ParseItems(rest[loc[1]:end])
			if j, ok := seen[key]; ok {
				refs[j].Items = mergeItems(refs[j].Items, items)
				continue
			}
			seen[key] = len(refs)
			refs = append(refs, Ref{Key: key, Items: items})
		}
		return refs, nil
	}
	return nil, nil
}

// ParseItems expands the items named in text: "R1, R3–R5" is R1 R3 R4 R5,
// "R26a–R26c" is R26a R26b R26c, "A3, A5" is A3 A5. A range of numbers does not
// list the lettered items inside it.
func ParseItems(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, m := range itemRefRe.FindAllStringSubmatch(s, -1) {
		p := m[1]
		a, _ := strconv.Atoi(m[2])
		add(p + m[2] + m[3])
		if m[4] == "" {
			continue
		}
		b, _ := strconv.Atoi(m[4])
		if b < a || b-a > 200 {
			continue
		}
		if a == b {
			from := byte('a')
			if m[3] != "" {
				from = m[3][0] + 1
			}
			for c := from; m[5] != "" && c <= m[5][0]; c++ {
				add(p + m[2] + string(c))
			}
			continue
		}
		for n := a + 1; n <= b; n++ {
			add(p + strconv.Itoa(n))
		}
		if m[5] != "" {
			add(p + m[4] + m[5])
		}
	}
	return out
}

func mergeItems(a, b []string) []string {
	for _, x := range b {
		found := false
		for _, y := range a {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

// parseRequirements reads the requirements **R<n>.** of product.
func parseRequirements(ls []line) []Item {
	var out []Item
	seen := map[string]bool{}
	for _, l := range ls {
		if l.code {
			continue
		}
		t := strings.TrimSpace(l.text)
		if m := listItemRe.FindStringSubmatch(t); m != nil && !strings.HasPrefix(t, "**") {
			t = strings.TrimSpace(m[1])
		}
		m := reqRe.FindStringSubmatch(t)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, Item{ID: m[1], Kind: KindRequirement, Text: strings.TrimSpace(m[2] + " " + m[3]), Line: l.n})
	}
	return out
}

// unnumberedRequirements reports top-level list items of a requirements
// section that have no R-ID; acceptance criteria and details under a
// requirement paragraph are not requirements.
func unnumberedRequirements(ls []line) []Violation {
	var out []Violation
	in, level, under := false, 0, false
	for _, l := range ls {
		if l.code {
			continue
		}
		if lvl, title, ok := heading(l); ok {
			under = false
			if strings.Contains(title, "требован") || strings.Contains(title, "requirement") {
				in, level = true, lvl
			} else if in && lvl <= level {
				in = false
			}
			continue
		}
		if !in || strings.HasPrefix(l.text, " ") || strings.HasPrefix(l.text, "\t") {
			continue
		}
		t := strings.TrimSpace(l.text)
		m := listItemRe.FindStringSubmatch(t)
		if m == nil {
			if strings.HasPrefix(t, "**") && reqRe.MatchString(t) {
				under = true
			}
			continue
		}
		item := strings.TrimSpace(m[1])
		if reqRe.MatchString(item) {
			under = false
			continue
		}
		if item == "" || under || isCriterion(item) {
			continue
		}
		out = append(out, Violation{Element: ViolRequirementUnnum, Message: item, Line: l.n})
	}
	return out
}

func isCriterion(s string) bool {
	low := strings.ToLower(strings.Trim(s, "*_` "))
	for _, p := range []string{"дано", "given", "когда", "when", "тогда", "then", "критерии", "acceptance"} {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}

// parseDecisions reads the table of the section «Принятые решения» of arch:
// a row whose first cell is A<n> is a decision; a row with a bare number is a
// violation (FTR.HMR.DEV-0003 R16).
func parseDecisions(ls []line) ([]Item, []Violation) {
	sec, _ := section(ls, "decisions")
	var out []Item
	var viol []Violation
	seen := map[string]bool{}
	for _, t := range tables(sec) {
		for i, r := range t.rows {
			id := bare(cell(r, 0))
			text := strings.TrimSpace(strings.Join(nonEmpty(r[min(1, len(r)):]), " — "))
			switch {
			case decIDRe.MatchString(id):
				if seen[id] {
					viol = append(viol, Violation{Element: ViolDecisionDup, Message: id, Line: t.lines[i]})
					continue
				}
				seen[id] = true
				out = append(out, Item{ID: id, Kind: KindDecision, Text: text, Line: t.lines[i]})
			case id != "":
				viol = append(viol, Violation{Element: ViolDecisionsUnnumb, Message: strings.TrimSpace(cell(r, 1)), Line: t.lines[i]})
			}
		}
	}
	return out, viol
}

func nonEmpty(xs []string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, strings.TrimSpace(x))
		}
	}
	return out
}

// parseChanges reads the table of the section «Изменения в других решениях».
func parseChanges(ls []line) ([]Change, []Violation) {
	sec, at := section(ls, "changes")
	if at < 0 {
		return nil, nil
	}
	var out []Change
	var viol []Violation
	for _, t := range tables(sec) {
		itemCol := col(t.header, false, nil, "col_req", "col_dec")
		keyCol := col(t.header, false, map[int]bool{itemCol: true}, "col_feature")
		whatCol := col(t.header, true, map[int]bool{itemCol: true, keyCol: true}, "col_what")
		if itemCol < 0 || keyCol < 0 {
			viol = append(viol, Violation{Element: ViolChangesMalformed, Message: "the table needs the columns of the feature and of the item", Line: ls[at].n})
			continue
		}
		for i, r := range t.rows {
			key := featureKeyRe.FindString(cell(r, keyCol))
			if key == "" {
				continue
			}
			item := bare(cell(r, itemCol))
			if isWhole(item) {
				item = ""
			}
			out = append(out, Change{Key: key, Item: item, Text: cell(r, whatCol), Line: t.lines[i]})
		}
	}
	return out, viol
}

// parseNotes reads the line notes «*Изменено в `<key>` R<n>: …*».
func parseNotes(ls []line) []Note {
	var out []Note
	target := ""
	for _, l := range ls {
		if l.code {
			continue
		}
		t := strings.TrimSpace(l.text)
		if m := reqRe.FindStringSubmatch(t); m != nil {
			target = m[1]
			continue
		}
		if _, _, ok := heading(l); ok && target == "" {
			continue
		}
		m := noteRe.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		out = append(out, Note{Target: target, Source: m[1], SourceItem: m[2], Text: m[3], Line: l.n})
	}
	return out
}

// parseArchNotes reads the notes under the decisions of arch: «*A10 — изменено в `<key>` A2: …*».
func parseArchNotes(ls []line) []Note {
	var out []Note
	for _, l := range ls {
		if l.code {
			continue
		}
		if m := archNoteRe.FindStringSubmatch(strings.TrimSpace(l.text)); m != nil {
			out = append(out, Note{Target: m[1], Source: m[2], SourceItem: m[3], Text: m[4], Line: l.n})
		}
	}
	return out
}

// ItemKind is the kind of an ID: requirement, decision or "" for neither.
func ItemKind(id string) string {
	switch {
	case reqIDRe.MatchString(id):
		return KindRequirement
	case decIDRe.MatchString(id):
		return KindDecision
	}
	return ""
}

// KindOf is the kind of the items an area holds.
func KindOf(area Area) string {
	switch area {
	case Product:
		return KindRequirement
	case Arch:
		return KindDecision
	}
	return ""
}

// SourceItem is the first item of a source document that names target: the
// item the note of the change points to (FTR.HMR.DEV-0002 R4).
func SourceItem(d Document, target string) string {
	for _, it := range d.Items {
		if strings.Contains(it.Text, target) {
			return it.ID
		}
	}
	return ""
}
