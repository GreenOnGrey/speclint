package contract

import (
	"regexp"
	"strings"
)

// Lang is a language of the documents of an installation (FTR.HMR.DEV-0003 R22).
type Lang string

// Languages of the contract: documents are read in any of them, written in one.
const (
	RU Lang = "ru"
	EN Lang = "en"
)

// Langs are the languages the contract reads.
var Langs = []Lang{RU, EN}

// ParseLang maps a setting to a language of the contract: ru or en, anything
// else is English (FTR.HMR.DEV-0003 R23).
func ParseLang(s string) Lang {
	if strings.EqualFold(strings.TrimSpace(s), string(RU)) {
		return RU
	}
	return EN
}

// Words of the contract by language. The first word of a list is the one the
// contract writes; the others are only read.
var words = map[string]map[Lang][]string{
	"depends_on":  {RU: {"Опирается на"}, EN: {"Depends on", "Builds on"}},
	"changes":     {RU: {"Изменения в других решениях"}, EN: {"Changes to other features"}},
	"decisions":   {RU: {"Принятые решения"}, EN: {"Decisions"}},
	"changed_in":  {RU: {"Изменено в"}, EN: {"Changed in"}},
	"whole":       {RU: {"всё решение", "все решение"}, EN: {"whole feature"}},
	"given":       {RU: {"Дано"}, EN: {"Given"}},
	"col_feature": {RU: {"Решение"}, EN: {"Feature"}},
	"col_req":     {RU: {"Требование"}, EN: {"Requirement"}},
	"col_dec":     {RU: {"Архитектурное решение"}, EN: {"Decision"}},
	"col_what":    {RU: {"Что изменилось"}, EN: {"What changed"}},
	"col_reason":  {RU: {"Обоснование"}, EN: {"Reason"}},
	// FTR.HMR.SPEC-0004
	"services":      {RU: {"Затрагиваемые сервисы и контракты", "Затрагиваемые сервисы"}, EN: {"Affected services and contracts", "Affected services"}},
	"c4":            {RU: {"Схема C4"}, EN: {"C4 diagram"}},
	"tech_services": {RU: {"Изменения по сервисам"}, EN: {"Changes by service"}},
	"col_service":   {RU: {"Сервис"}, EN: {"Service"}},
}

// word is the word the contract writes in lang; an unknown language writes English.
func word(key string, lang Lang) string {
	if ws := words[key][lang]; len(ws) > 0 {
		return ws[0]
	}
	return words[key][EN][0]
}

// allWords are the words of key in every language.
func allWords(key string) []string {
	var out []string
	for _, l := range Langs {
		out = append(out, words[key][l]...)
	}
	return out
}

// FeatureKey is the pattern of a feature key: FTR.<DOMAIN>.<SYSTEM>-NNNN.
const FeatureKey = `FTR\.[A-Z][A-Z0-9]{1,9}\.[A-Z][A-Z0-9]{1,9}-\d{4}`

// ReqID is a requirement: R<n>, or R<n><letter> inserted later.
const ReqID = `R\d+(?:[a-z]\b)?`

// DecisionID is an architecture decision: A<n>, or A<n><letter> inserted later.
const DecisionID = `A\d+(?:[a-z]\b)?`

var (
	featureKeyRe = regexp.MustCompile(FeatureKey)
	headingRe    = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	listItemRe   = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+(.*)$`)
	headerLineRe = regexp.MustCompile(`^\*\*[^*]+:\*\*`)
	sectionNumRe = regexp.MustCompile(`^(?:\d+(?:\.\d+)*\.?\s+)`)
	reqIDRe      = regexp.MustCompile(`^` + ReqID + `$`)
	decIDRe      = regexp.MustCompile(`^` + DecisionID + `$`)
)

// line is a line of a document with its number (1-based) and whether it is
// inside a fenced code block.
type line struct {
	n    int
	text string
	code bool
}

// splitFrontMatter separates a leading "---" block: its keys and the body with
// the number of lines before it.
func splitFrontMatter(doc string) (front map[string]string, body string, offset int) {
	doc = strings.ReplaceAll(doc, "\r\n", "\n")
	if !strings.HasPrefix(doc, "---\n") {
		return nil, doc, 0
	}
	end := strings.Index(doc[4:], "\n---")
	if end < 0 {
		return nil, doc, 0
	}
	block := doc[4 : 4+end]
	front = map[string]string{}
	for _, l := range strings.Split(block, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if i := strings.Index(v, " #"); i >= 0 { // a YAML comment
			v = strings.TrimSpace(v[:i])
		}
		front[strings.TrimSpace(k)] = strings.Trim(v, `"'`)
	}
	rest := doc[4+end+4:]
	offset = strings.Count(doc[:4+end+4], "\n") + 1
	return front, rest, offset - 1
}

// lines splits a body into lines, marking fenced code.
func lines(body string, offset int) []line {
	var out []line
	fence := ""
	for i, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		code := fence != ""
		if fence == "" && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			fence, code = t[:3], true
		} else if fence != "" && strings.HasPrefix(t, fence) {
			fence = ""
		}
		out = append(out, line{n: offset + i + 1, text: l, code: code})
	}
	return out
}

// heading returns the level and the normalized title of a heading line: lower
// case, without a section number and emphasis.
func heading(l line) (int, string, bool) {
	if l.code {
		return 0, "", false
	}
	m := headingRe.FindStringSubmatch(l.text)
	if m == nil {
		return 0, "", false
	}
	title := strings.Trim(m[2], "*_` ")
	title = sectionNumRe.ReplaceAllString(title, "")
	return len(m[1]), strings.ToLower(strings.TrimSpace(title)), true
}

// matchTitle tells whether a normalized title is one of the words of key.
func matchTitle(title, key string) bool {
	for _, w := range allWords(key) {
		if strings.Contains(title, strings.ToLower(w)) { // as the indexer: «Ещё раз: Изменения в …» is the section too
			return true
		}
	}
	return false
}

// section returns the lines of the first section whose title is a word of key,
// without the heading, up to the next heading of the same or higher level, and
// the line of its heading (-1 if none).
func section(ls []line, key string) ([]line, int) {
	for i, l := range ls {
		lvl, title, ok := heading(l)
		if !ok || !matchTitle(title, key) {
			continue
		}
		end := len(ls)
		for j := i + 1; j < len(ls); j++ {
			if l2, _, ok := heading(ls[j]); ok && l2 <= lvl {
				end = j
				break
			}
		}
		return ls[i+1 : end], i
	}
	return nil, -1
}

// table is a markdown table: its header, rows and the line numbers of the rows.
type table struct {
	header []string
	rows   [][]string
	lines  []int // line number of each row
	first  int   // index in the slice of lines of the header
	last   int   // index in the slice of lines of the last row
}

// tables finds the tables of a slice of lines outside code.
func tables(ls []line) []table {
	var out []table
	var cur *table
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for i, l := range ls {
		t := strings.TrimSpace(l.text)
		if l.code || !strings.HasPrefix(t, "|") {
			flush()
			continue
		}
		cells := splitRow(t)
		if cur == nil {
			cur = &table{header: cells, first: i, last: i}
			continue
		}
		cur.last = i
		if isSeparator(cells) {
			continue
		}
		cur.rows = append(cur.rows, cells)
		cur.lines = append(cur.lines, l.n)
	}
	flush()
	return out
}

func splitRow(t string) []string {
	t = strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|")
	// an escaped pipe is a character of the cell
	t = strings.ReplaceAll(t, `\|`, "\x00")
	parts := strings.Split(t, "|")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.TrimSpace(parts[i]), "\x00", "|")
	}
	return parts
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, ":- ") != "" {
			return false
		}
	}
	return true
}

// colStems are the stems by which the indexer finds the columns of a table:
// «Требования» and «Требование» are the same column.
var colStems = map[string][]string{
	"col_feature": {"решени", "feature"},
	"col_req":     {"требован", "requirement"},
	"col_dec":     {"архитектурн", "decision"},
	"col_what":    {"что измен", "what changed", "изменил", "change"},
	"col_service": {"сервис", "service"},
}

// col is the index of the first header cell containing a stem of one of keys,
// in the order of keys; -1 if none. skip excludes columns.
func col(header []string, _ bool, skip map[int]bool, keys ...string) int {
	for _, k := range keys {
		for i, h := range header {
			if skip[i] {
				continue
			}
			low := strings.ToLower(strings.Trim(h, "*_` "))
			for _, w := range colStems[k] {
				if strings.Contains(low, w) {
					return i
				}
			}
		}
	}
	return -1
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// bare strips the emphasis and code marks around a cell or an ID.
func bare(s string) string { return strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "`*_ ")) }

// isWhole tells whether a cell names the whole feature.
func isWhole(s string) bool {
	s = strings.ToLower(bare(s))
	if s == "" || s == "—" || s == "–" || s == "-" {
		return true
	}
	for _, w := range allWords("whole") {
		if s == strings.ToLower(w) {
			return true
		}
	}
	return false
}
