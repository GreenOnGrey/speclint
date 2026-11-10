package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The formats of the output (FTR.HMR.SPEC-0002 tech §2): text for a person, github for annotations on the
// lines of a pull request and a job summary, gitlab for the Code Quality widget of a merge request.
const (
	formatText   = "text"
	formatGitHub = "github"
	formatGitLab = "gitlab"
)

// codeQualityFile is the report of -format gitlab, in the directory of the run.
const codeQualityFile = "gl-code-quality-report.json"

type output struct {
	format   string
	root     string // the paths of issues are relative to it; annotations need them relative to the repository
	baseline string
	quiet    bool
	w        io.Writer
	summary  string // $GITHUB_STEP_SUMMARY, for github
}

func mustOutput(format, root string, quiet bool) output {
	o, err := newOutput(format, root, quiet, os.Stdout)
	if err != nil {
		fail(err)
	}
	return o
}

func newOutput(format, root string, quiet bool, w io.Writer) (output, error) {
	switch format {
	case formatText, formatGitHub, formatGitLab:
	default:
		return output{}, fmt.Errorf("unknown -format %q: text, github or gitlab", format)
	}
	return output{format: format, root: root, quiet: quiet, w: w, summary: os.Getenv("GITHUB_STEP_SUMMARY")}, nil
}

// alwaysShown are the warnings that -errors-only keeps: an older speclint must be seen to be updated (R13).
var alwaysShown = map[string]bool{checkVersionMinor: true}

// result is an issue compared with the baseline.
type result struct {
	issue
	sev string // error (new), known, warning
}

// write prints the issues and a summary and returns the exit code: 1 for new errors or stale lines of the
// baseline, which only shrinks.
func (o output) write(issues []issue, known map[string]bool) (int, error) {
	counts := map[string]int{}
	current := map[string]bool{}
	var results []result
	newErrors, knownErrors := 0, 0
	for _, i := range issues {
		counts[i.check]++
		sev := i.severity()
		if sev == "error" {
			current[i.key()] = true
			if known[i.key()] {
				sev = "known"
				knownErrors++
			} else {
				newErrors++
			}
		}
		results = append(results, result{i, sev})
	}
	var fixed []string
	for k := range known {
		if !current[k] {
			fixed = append(fixed, k)
		}
	}
	sort.Strings(fixed)

	for _, r := range results {
		if o.quiet && r.sev != "error" && !alwaysShown[r.check] {
			continue
		}
		fmt.Fprintf(o.w, "%s:%d: %s [%s] %s\n", r.path, r.line, r.sev, r.check, r.msg)
		if o.format == formatGitHub && r.sev != "known" {
			o.annotate(r)
		}
	}
	var checks []string
	for c := range counts {
		checks = append(checks, c)
	}
	sort.Strings(checks)
	fmt.Fprintln(o.w)
	for _, c := range checks {
		fmt.Fprintf(o.w, "%6d  %s\n", counts[c], c)
	}
	for _, k := range fixed {
		fmt.Fprintf(o.w, "fixed, remove from the baseline: %s\n", k)
		if o.format == formatGitHub {
			fmt.Fprintf(o.w, "::error file=%s,title=baseline::%s\n", escapeProp(o.baseline), escapeData("fixed, remove the line: "+k))
		}
	}
	fmt.Fprintf(o.w, "new errors: %d, known errors: %d, baseline lines to remove: %d\n", newErrors, knownErrors, len(fixed))

	switch o.format {
	case formatGitHub:
		if err := o.writeSummary(results, fixed, newErrors, knownErrors); err != nil {
			return 2, err
		}
	case formatGitLab:
		if err := o.writeCodeQuality(results, fixed); err != nil {
			return 2, err
		}
	}
	if newErrors > 0 || len(fixed) > 0 {
		return 1, nil
	}
	return 0, nil
}

// file is the path of an issue relative to the directory of the run (the root of the repository in CI), or ""
// when it names no file: a qa document of a feature in Hammurapi, in the mode service.
func (o output) file(i issue) string {
	p := filepath.Join(o.root, i.path)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return ""
	}
	if wd, err := os.Getwd(); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			if r, err := filepath.Rel(wd, abs); err == nil && !strings.HasPrefix(r, "..") {
				p = r
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// annotate prints a workflow command: an annotation on the line of the file in the pull request.
func (o output) annotate(r result) {
	level := "error"
	if r.sev == "warning" {
		level = "warning"
	}
	props := "title=" + escapeProp(r.check)
	if f := o.file(r.issue); f != "" {
		props = "file=" + escapeProp(f) + ",line=" + fmt.Sprint(max(r.line, 1)) + "," + props
	}
	fmt.Fprintf(o.w, "::%s %s::%s\n", level, props, escapeData(r.msg))
}

// The escapes of workflow commands.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

func escapeProp(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}

// writeSummary appends Markdown to the job summary: the counts and the new errors.
func (o output) writeSummary(results []result, fixed []string, newErrors, knownErrors int) error {
	if o.summary == "" {
		return nil
	}
	var b strings.Builder
	b.WriteString("### speclint\n\n")
	fmt.Fprintf(&b, "New errors: **%d**, known errors: %d, baseline lines to remove: **%d**\n\n", newErrors, knownErrors, len(fixed))
	if newErrors > 0 {
		b.WriteString("| File | Check | Message |\n|---|---|---|\n")
		for _, r := range results {
			if r.sev == "error" {
				fmt.Fprintf(&b, "| `%s:%d` | `%s` | %s |\n", r.path, r.line, r.check, mdCell(r.msg))
			}
		}
		b.WriteString("\n")
	}
	if len(fixed) > 0 {
		b.WriteString("Remove from the baseline:\n\n")
		for _, k := range fixed {
			fmt.Fprintf(&b, "- `%s`\n", strings.ReplaceAll(k, "`", "'"))
		}
	}
	f, err := os.OpenFile(o.summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func mdCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ").Replace(s)
}

// codeQuality is an entry of the Code Quality report of GitLab.
type codeQuality struct {
	Description string `json:"description"`
	CheckName   string `json:"check_name"`
	Fingerprint string `json:"fingerprint"`
	Severity    string `json:"severity"`
	Location    struct {
		Path  string `json:"path"`
		Lines struct {
			Begin int `json:"begin"`
		} `json:"lines"`
	} `json:"location"`
}

// writeCodeQuality writes the report of the new errors, the warnings and the stale lines of the baseline. The
// fingerprint is the hash of the line of the baseline: one issue keeps it between runs (tech §5, decision 2).
func (o output) writeCodeQuality(results []result, fixed []string) error {
	entries := []codeQuality{}
	add := func(desc, check, key, path, sev string, line int) {
		var e codeQuality
		sum := sha1.Sum([]byte(key))
		e.Description, e.CheckName, e.Fingerprint, e.Severity = desc, check, hex.EncodeToString(sum[:]), sev
		e.Location.Path, e.Location.Lines.Begin = path, max(line, 1)
		entries = append(entries, e)
	}
	for _, r := range results {
		if r.sev == "known" || (o.quiet && r.sev == "warning" && !alwaysShown[r.check]) {
			continue
		}
		sev := "major"
		if r.sev == "warning" {
			sev = "minor"
		}
		path := o.file(r.issue)
		if path == "" {
			path = r.path // a qa document of a feature in Hammurapi
		}
		if path == "" {
			path = filepath.ToSlash(o.baseline) // the run as a whole: the version of speclint
		}
		add(r.msg, r.check, r.key(), path, sev, r.line)
	}
	for _, k := range fixed {
		add("fixed, remove the line from the baseline: "+k, "baseline", "baseline "+k, filepath.ToSlash(o.baseline), "major", 1)
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(codeQualityFile, append(b, '\n'), 0o644)
}
