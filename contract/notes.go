package contract

import (
	"sort"
	"strings"
)

// NoteLine is the line of a note in lang. A requirement of product and the
// whole feature get «*Изменено в `<source>` R<n>: <text>.*»; a decision of arch
// gets the list item «- *A10 — изменено в `<source>` A<m>: <text>.*».
func NoteLine(n Note, area Area, lang Lang) string {
	text := strings.TrimSpace(strings.Trim(strings.TrimSpace(n.Text), "*"))
	tail := "`" + n.Source + "`"
	if n.SourceItem != "" {
		tail += " " + n.SourceItem
	}
	if text != "" {
		if r := text[len(text)-1]; r != '.' && r != '!' && r != '?' {
			text += "."
		}
		tail += ": " + text
	} else {
		tail += "."
	}
	if area == Arch && n.Target != "" {
		return "- *" + n.Target + " — " + lowerFirst(word("changed_in", lang)) + " " + tail + "*"
	}
	return "*" + word("changed_in", lang) + " " + tail + "*"
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToLower(string(r[0])))[0]
	return string(r)
}

// isNoteOf tells whether a line is a note of source in any language and form.
func isNoteOf(t, source string) bool {
	if m := noteRe.FindStringSubmatch(t); m != nil {
		return m[1] == source
	}
	if m := archNoteRe.FindStringSubmatch(t); m != nil {
		return m[2] == source
	}
	return false
}

func isAnyNote(t string) bool { return noteRe.MatchString(t) || archNoteRe.MatchString(t) }

// ApplyNotes puts the notes of source into a document of another feature
// (FTR.HMR.DEV-0002 R4, FTR.HMR.DEV-0003 R14). The notes of source already there
// are removed first. In product a note goes right after the first line of its
// requirement, after the notes already there. In arch a note goes into the list
// right under the table of decisions, ordered by the number of the decision,
// then by the source. A note of the whole feature goes after the header lines
// «**…:**». Notes of other features and every other line stay as they are; an
// item missing in the document gets no note; applying the same notes again
// gives the same text.
func ApplyNotes(target string, area Area, source string, notes []Note, lang Lang) string {
	front := ""
	body := strings.ReplaceAll(target, "\r\n", "\n")
	if _, b, off := splitFrontMatter(body); off > 0 {
		front = strings.Join(strings.Split(body, "\n")[:off], "\n") + "\n"
		body = b
	}
	all := lines(body, 0)
	var kept []line
	for _, l := range all {
		if !l.code && isNoteOf(strings.TrimSpace(l.text), source) {
			continue
		}
		kept = append(kept, l)
	}
	kept = dropDoubleBlanks(kept, all)

	insert := map[int][]string{} // after index in kept
	var archNotes []string
	for _, n := range notes {
		n.Source = source
		switch {
		case n.Target == "":
			if i := headerEnd(kept); i >= 0 {
				insert[i] = append(insert[i], NoteLine(n, area, lang))
			}
		case area == Arch:
			if hasDecision(kept, n.Target) {
				archNotes = append(archNotes, NoteLine(n, area, lang))
			}
		default:
			if i := reqLine(kept, n.Target); i >= 0 {
				insert[i] = append(insert[i], NoteLine(n, area, lang))
			}
		}
	}
	out := make([]string, 0, len(kept)+len(notes)+2)
	listAt, listEnd := archNotesAt(kept)
	listDone := false
	for i := 0; i < len(kept); i++ {
		if area == Arch && len(archNotes) > 0 && i == listAt && !listDone {
			listDone = true
			// the list under the table: the existing notes and the new ones, sorted
			existing := []string{}
			for j := listAt; j < listEnd; j++ {
				if t := strings.TrimSpace(kept[j].text); t != "" {
					existing = append(existing, t)
				}
			}
			items := sortArchNotes(append(existing, archNotes...))
			out = append(out, "")
			out = append(out, items...)
			if listEnd < len(kept) && strings.TrimSpace(kept[listEnd].text) != "" {
				out = append(out, "")
			}
			i = listEnd - 1
			continue
		}
		out = append(out, kept[i].text)
		add, ok := insert[i]
		if !ok {
			continue
		}
		for i+1 < len(kept) && !kept[i+1].code && isAnyNote(strings.TrimSpace(kept[i+1].text)) {
			i++
			out = append(out, kept[i].text)
		}
		out = append(out, add...)
	}
	if area == Arch && len(archNotes) > 0 && listAt >= len(kept) {
		out = append(out, "")
		out = append(out, sortArchNotes(archNotes)...)
	}
	return front + strings.Join(out, "\n")
}

// dropDoubleBlanks removes the blank line left where a whole list of notes was
// removed between two blank lines.
func dropDoubleBlanks(kept, all []line) []line {
	if len(kept) == len(all) {
		return kept
	}
	var out []line
	for i, l := range kept {
		if i > 0 && strings.TrimSpace(l.text) == "" && strings.TrimSpace(kept[i-1].text) == "" && !l.code {
			continue
		}
		out = append(out, l)
	}
	return out
}

// headerEnd is the index of the last header line «**…:**» (or the title), -1 if none.
func headerEnd(ls []line) int {
	last := -1
	for i, l := range ls {
		if l.code {
			continue
		}
		t := strings.TrimSpace(l.text)
		if lvl, _, ok := heading(l); ok && lvl > 1 || t == "---" {
			break
		}
		if headerLineRe.MatchString(t) || strings.HasPrefix(t, "# ") && last < 0 {
			last = i
		}
	}
	return last
}

func reqLine(ls []line, id string) int {
	for i, l := range ls {
		if l.code {
			continue
		}
		t := strings.TrimSpace(l.text)
		if m := listItemRe.FindStringSubmatch(t); m != nil && !strings.HasPrefix(t, "**") {
			t = strings.TrimSpace(m[1])
		}
		if m := reqRe.FindStringSubmatch(t); m != nil && m[1] == id {
			return i
		}
	}
	return -1
}

func hasDecision(ls []line, id string) bool {
	sec, _ := section(ls, "decisions")
	for _, t := range tables(sec) {
		for _, r := range t.rows {
			if bare(cell(r, 0)) == id {
				return true
			}
		}
	}
	return false
}

// archNotesAt is the place of the list of notes under the table of decisions:
// the index right after the table and the index after the list (equal when
// there is no list). len(ls) if there is no table.
func archNotesAt(ls []line) (int, int) {
	sec, at := section(ls, "decisions")
	if at < 0 {
		return len(ls), len(ls)
	}
	ts := tables(sec)
	if len(ts) == 0 {
		return len(ls), len(ls)
	}
	start := at + 1 + ts[0].last + 1
	end := start
	for end < len(ls) {
		t := strings.TrimSpace(ls[end].text)
		if t != "" && !(archNoteRe.MatchString(t) && !ls[end].code) {
			break
		}
		end++
	}
	// keep the blank lines before the next block outside the list
	for end > start && strings.TrimSpace(ls[end-1].text) == "" {
		end--
	}
	return start, end
}

func sortArchNotes(items []string) []string {
	type k struct {
		num    int
		letter string
		source string
		text   string
	}
	ks := make([]k, 0, len(items))
	for _, t := range items {
		m := archNoteRe.FindStringSubmatch(t)
		if m == nil {
			ks = append(ks, k{num: 1 << 30, text: t})
			continue
		}
		n, l := splitNum(m[1][1:])
		ks = append(ks, k{num: n, letter: l, source: m[2], text: t})
	}
	sort.SliceStable(ks, func(i, j int) bool {
		if ks[i].num != ks[j].num {
			return ks[i].num < ks[j].num
		}
		if ks[i].letter != ks[j].letter {
			return ks[i].letter < ks[j].letter
		}
		return ks[i].source < ks[j].source
	})
	out := make([]string, len(ks))
	for i, x := range ks {
		out[i] = x.text
	}
	return out
}

func splitNum(s string) (int, string) {
	n, i := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, s[i:]
}

// SortItems orders IDs by number, then by letter: R2, R13, R26, R26a.
func SortItems(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		if len(ids[i]) == 0 || len(ids[j]) == 0 {
			return ids[i] < ids[j]
		}
		if ids[i][0] != ids[j][0] {
			return ids[i][0] > ids[j][0] // R before A
		}
		a, as := splitNum(ids[i][1:])
		b, bs := splitNum(ids[j][1:])
		if a != b {
			return a < b
		}
		return as < bs
	})
}
