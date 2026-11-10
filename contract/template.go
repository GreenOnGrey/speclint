package contract

import (
	"fmt"
	"strings"
)

// Element is a part of the contract a template of an area must keep
// (FTR.HMR.DEV-0003 R17, R18).
type Element struct {
	Element string `json:"element"`
	Title   string `json:"title"`
	Example string `json:"example"`
}

// Files of the rules of an area.
const (
	FileTemplate    = "template"
	FileFixTemplate = "fix-template"
)

// Describe lists the elements of the contract of a template file in lang.
func Describe(area Area, file string, lang Lang) []Element {
	ru := lang == RU
	pick := func(r, e string) string {
		if ru {
			return r
		}
		return e
	}
	var out []Element
	if file == FileFixTemplate {
		out = append(out, Element{ElemParent, pick("parent в front matter", "parent in the front matter"), "parent: FTR.FMS.CAR-0000"})
	}
	changes := func(col string) Element {
		return Element{ElemChanges, pick("Раздел «Изменения в других решениях»", "The section «Changes to other features»"),
			"## " + word("changes", lang) + "\n| " + word("col_feature", lang) + " | " + word(col, lang) + " | " + word("col_what", lang) + " |"}
	}
	if file == FileFixTemplate {
		switch area {
		case Product:
			out = append(out, changes("col_req"))
		case Arch:
			out = append(out, changes("col_dec"))
		}
		return out
	}
	switch area {
	case Product:
		out = append(out,
			Element{ElemDependsOn, pick("Строка «Опирается на»", "The line «Depends on»"), "**" + word("depends_on", lang) + ":** `FTR.HMR.CMN-0001` (R3)"},
			Element{ElemRequirement, pick("Требования R<n> с критериями «Дано»", "Requirements R<n> with criteria «Given»"),
				"**R1.** …\n- " + word("given", lang) + " …"},
			Element{ElemChanges, pick("Раздел «Изменения в других решениях»", "The section «Changes to other features»"),
				"## " + word("changes", lang) + "\n| " + word("col_feature", lang) + " | " + word("col_req", lang) + " | " + word("col_what", lang) + " |"},
		)
	case Arch:
		out = append(out,
			Element{ElemDependsOn, pick("Строка «Опирается на»", "The line «Depends on»"), "**" + word("depends_on", lang) + ":** `FTR.HMR.CMN-0001` (A3, A5)"},
			Element{ElemDecisions, pick("Таблица «Принятые решения» с номерами A<n>", "The table «Decisions» numbered A<n>"),
				"## " + word("decisions", lang) + "\n| A | " + word("col_feature", lang) + " | " + word("col_reason", lang) + " |\n| A1 | … | … |"},
			Element{ElemServices, pick("Таблица «Затрагиваемые сервисы и контракты» с ключами каталога в первой колонке", "The table «Affected services and contracts» with the keys of the catalog in the first column"),
				"## " + word("services", lang) + "\n| " + word("col_service", lang) + " | … |\n| `booking` | … |"},
			Element{ElemC4, pick("Раздел «Схема C4» с блоком mermaid C4Container", "The section «C4 diagram» with a mermaid block C4Container"),
				"## " + word("c4", lang) + "\n```mermaid\nC4Container\n  Container(booking, \"…\")\n```"},
			Element{ElemChanges, pick("Раздел «Изменения в других решениях»", "The section «Changes to other features»"),
				"## " + word("changes", lang) + "\n| " + word("col_feature", lang) + " | " + word("col_dec", lang) + " | " + word("col_what", lang) + " |"},
		)
	}
	return out
}

// CheckTemplate reports the elements of the contract a rules template breaks
// (FTR.HMR.DEV-0003 R19): each element must be there, in the format of the
// contract, in any language and order.
func CheckTemplate(area Area, file, content string) []Violation {
	front, body, off := splitFrontMatter(content)
	ls := lines(body, off)
	var out []Violation
	missing := func(el, msg string) { out = append(out, Violation{Element: el, Message: msg}) }
	if file == FileFixTemplate {
		if _, ok := front["parent"]; !ok {
			missing(ElemParent, "no parent in the front matter")
		}
	}
	if area != Product && area != Arch {
		return out
	}
	if sec, at := section(ls, "changes"); at < 0 {
		missing(ElemChanges, "no section «Изменения в других решениях» / «Changes to other features»")
	} else if !changesTable(sec) {
		missing(ElemChanges, "the section has no table with the columns of the feature, the item and what changed")
	}
	if file == FileFixTemplate { // a fix names its parent instead of the header and adds to its items
		return out
	}
	if !hasDependsOn(ls) {
		missing(ElemDependsOn, "no line «Опирается на» / «Depends on» in the header")
	}
	switch area {
	case Product:
		if !hasRequirementExample(ls) {
			missing(ElemRequirement, "no requirement **R<n>.** with criteria «Дано» / «Given»")
		}
	case Arch:
		sec, at := section(ls, "decisions")
		switch {
		case at < 0:
			missing(ElemDecisions, "no section «Принятые решения» / «Decisions»")
		case !decisionsTable(sec):
			missing(ElemDecisions, "the table of decisions has no column A<n>")
		}
		if _, at, ok := parseServices(ls, "services", true); !ok {
			if at == 0 {
				missing(ElemServices, "no section «Затрагиваемые сервисы и контракты» / «Affected services and contracts»")
			} else {
				missing(ElemServices, "the section has no table whose first column is «Сервис» / «Service»")
			}
		}
		if b := parseC4Section(ls); b == nil {
			missing(ElemC4, "no section «Схема C4» / «C4 diagram»")
		} else if b.BlockLine == 0 {
			missing(ElemC4, "the section «Схема C4» / «C4 diagram» has no mermaid block")
		}
	}
	return out
}

func hasDependsOn(ls []line) bool {
	for _, l := range header(ls) {
		t := strings.TrimSpace(l.text)
		for _, w := range allWords("depends_on") {
			if !l.code && strings.HasPrefix(strings.ToLower(t), "**"+strings.ToLower(w)+":**") {
				return true
			}
		}
	}
	return false
}

func changesTable(sec []line) bool {
	for _, t := range tables(sec) {
		item := col(t.header, false, nil, "col_req", "col_dec")
		key := col(t.header, false, map[int]bool{item: true}, "col_feature")
		what := col(t.header, true, map[int]bool{item: true, key: true}, "col_what")
		if item >= 0 && key >= 0 && what >= 0 {
			return true
		}
	}
	return false
}

func decisionsTable(sec []line) bool {
	for _, t := range tables(sec) {
		h := strings.ToUpper(bare(cell(t.header, 0)))
		if h == "A" || h == "A<N>" {
			return true
		}
		for _, r := range t.rows {
			if decIDRe.MatchString(bare(cell(r, 0))) {
				return true
			}
		}
	}
	return false
}

func hasRequirementExample(ls []line) bool {
	req := false
	for _, l := range ls {
		if l.code {
			continue
		}
		t := strings.TrimSpace(l.text)
		if reqRe.MatchString(t) {
			req = true
			continue
		}
		if req && isCriterion(strings.TrimSpace(strings.TrimLeft(t, "-*+ "))) {
			return true
		}
	}
	return false
}

// Prompt is the part of the instructions of the writer that states the contract
// of an area (FTR.HMR.DEV-0003 R21). The writer follows it when the template
// disagrees.
func Prompt(area Area, lang Lang) string {
	if area == Tech {
		return techPrompt(lang)
	}
	if area != Product && area != Arch {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The document contract of %s (version %s). Hammurapi reads these elements in every document; when the rules template disagrees, follow the contract. Write the headings and words below exactly, in this language.\n", area, Version)
	fmt.Fprintf(&b, "- Header line: **%s:** followed by the full keys of the features it relies on, with the items in parentheses if any: `FTR.HMR.CMN-0001` (R3, R5).\n", word("depends_on", lang))
	switch area {
	case Product:
		fmt.Fprintf(&b, "- Every requirement starts a paragraph with **R<n>.** and has acceptance criteria starting with «%s». A requirement inserted later between R<n> and R<n+1> gets a letter: R26a. Numbers are never reused.\n", word("given", lang))
		fmt.Fprintf(&b, "- If the feature changes requirements of features already in the default branch: the section «## %s» with the table | %s | %s | %s |, one row per requirement: the full key, R<n> or «%s», what changed in one phrase.\n",
			word("changes", lang), word("col_feature", lang), word("col_req", lang), word("col_what", lang), word("whole", lang))
	case Arch:
		fmt.Fprintf(&b, "- The section «## %s» is a table | A | %s | %s | whose first cell is the number A<n>. A decision inserted later gets a letter: A3a. Numbers are never reused: a cancelled decision stays with a note.\n",
			word("decisions", lang), word("col_feature", lang), word("col_reason", lang))
		fmt.Fprintf(&b, "- The section «## %s» is a table whose first column «%s» holds the keys of the services of the Hammurapi catalog this feature changes (`booking`), one per row; external systems not in the catalog go only to the diagram.\n",
			word("services", lang), word("col_service", lang))
		fmt.Fprintf(&b, "- The section «## %s» holds one ```mermaid block of the kind C4Container: every service of the table is a Container (or ContainerDb, ContainerQueue) whose identifier is its key with - and . written as _ (hammurapi-core → hammurapi_core); people are Person, external systems System_Ext; relations are Rel(from, to, \"label\", \"protocol\"). Identifiers are letters, digits and _.\n",
			word("c4", lang))
		fmt.Fprintf(&b, "- If the feature changes architecture decisions of features already in the default branch: the section «## %s» with the table | %s | %s | %s |, one row per decision: the full key, A<n> or «%s», what changed in one phrase. Architecture changes only architecture: a change of behaviour goes to the section of product.\n",
			word("changes", lang), word("col_feature", lang), word("col_dec", lang), word("col_what", lang), word("whole", lang))
	}
	fmt.Fprintf(&b, "- Do not write the notes «*%s …*» in other documents: Hammurapi puts them.\n", word("changed_in", lang))
	return b.String()
}

// techPrompt states the contract of tech (FTR.HMR.SPEC-0004 R3, R6).
func techPrompt(lang Lang) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The document contract of tech (version %s). Hammurapi reads these elements; when the rules template disagrees, follow the contract.\n", Version)
	fmt.Fprintf(&b, "- The section «## %s» is a table with the column «%s»: the keys of the services, only those of the table of affected services of arch (when arch was skipped, tech names them itself).\n",
		word("tech_services", lang), word("col_service", lang))
	b.WriteString("- A service may have a contract next to the document: tech/<service key>.openapi.yaml (or .yml, .json) — OpenAPI 3 with top-level openapi: 3.x and paths; tech/<service key>.proto — syntax = \"proto3\"; with a service or a message. The key is a service of arch. Name the file in the document.\n")
	return b.String()
}
