package contract

import (
	"regexp"
	"strings"
)

var (
	bareNumRe   = regexp.MustCompile(`^(\s*\|\s*)(\d+)([a-z]?)(\s*\|)`)
	headNumRe   = regexp.MustCompile(`^(\s*\|\s*)(#|№|N)(\s*\|)`)
	docVerRe    = regexp.MustCompile(`\s+v\d+(?:\.\d+)*`)
	dependsLnRe = regexp.MustCompile(`^\*\*(?i:` + strings.Join(quoteAll(allWords("depends_on")), "|") + `):\*\*`)
)

// MigrateArch turns an arch document into the format of the contract
// (FTR.HMR.DEV-0003 R25): the first column «#» of the table of decisions
// becomes «A» and its numbers A<n> with the same numbers; the versions of
// documents leave the line «Опирается на». Every other byte stays as it was.
func MigrateArch(doc string) string {
	_, body, off := splitFrontMatter(doc)
	all := strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n")
	ls := lines(body, off)
	sec, at := section(ls, "decisions")
	if at >= 0 {
		for _, t := range tables(sec) {
			hdr := sec[t.first].n - 1
			if !headNumRe.MatchString(all[hdr]) {
				continue
			}
			all[hdr] = headNumRe.ReplaceAllString(all[hdr], "${1}A${3}")
			for _, n := range t.lines {
				all[n-1] = bareNumRe.ReplaceAllString(all[n-1], "${1}A${2}${3}${4}")
			}
		}
	}
	for _, l := range header(ls) {
		if !l.code && dependsLnRe.MatchString(strings.TrimSpace(l.text)) {
			all[l.n-1] = docVerRe.ReplaceAllString(all[l.n-1], "")
		}
	}
	return strings.Join(all, "\n")
}
