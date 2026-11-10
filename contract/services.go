package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Service is a service of the catalog named by a document (FTR.HMR.SPEC-0004 R1, R3): the first column of the
// table «Затрагиваемые сервисы и контракты» of arch, the column «Сервис» of «Изменения по сервисам» of tech.
type Service struct {
	Key  string
	Line int
}

// C4Block is the section «Схема C4» of arch (R4). Lines are of the document.
type C4Block struct {
	Line      int // the heading
	BlockLine int // the opening fence of the mermaid block, 0 if the section has none
	Diagram   *C4
	Err       *C4Error // Line is of the document
}

// Checks of FTR.HMR.SPEC-0004 (tech §2.3): the names speclint reports and Hammurapi refuses a submission with.
const (
	CheckArchServicesMissing  = "arch-services-missing"
	CheckArchServiceUnknown   = "arch-service-unknown"
	CheckC4Missing            = "c4-missing"
	CheckC4Invalid            = "c4-invalid"
	CheckC4ServiceMissing     = "c4-service-missing"
	CheckTechServiceNotInArch = "tech-service-not-in-arch"
	CheckContractFileService  = "contract-file-service"
	CheckContractFileInvalid  = "contract-file-invalid"
)

// Elements of the contract of arch added by FTR.HMR.SPEC-0004.
const (
	ElemServices = "services"
	ElemC4       = "c4"
)

// Finding is a broken check of FTR.HMR.SPEC-0004: Subject identifies it in the baseline of speclint without the
// line; Suggest is the nearest key of the catalog for an unknown one.
type Finding struct {
	Check   string
	Subject string
	Message string
	Line    int
	Suggest string
}

// parseServices reads the keys of the first table of a section: the column of the service (the first column for
// arch, R1), "" if the section has no such table. A placeholder <…> and a dash are not keys.
func parseServices(ls []line, key string, first bool) ([]Service, int, bool) {
	sec, at := section(ls, key)
	if at < 0 {
		return nil, 0, false
	}
	for _, t := range tables(sec) {
		c := col(t.header, false, nil, "col_service")
		if first && c != 0 || c < 0 {
			continue
		}
		var out []Service
		seen := map[string]bool{}
		for i, r := range t.rows {
			k := serviceKey(cell(r, c))
			if k == "" || isWhole(k) || strings.HasPrefix(k, "<") || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, Service{Key: k, Line: t.lines[i]})
		}
		return out, ls[at].n, true
	}
	return nil, ls[at].n, false
}

// serviceKey is the key of a cell: the first code span when the cell starts with one (`booking` (API)), otherwise
// the cell without emphasis; backticks are optional (tech §5, decision 1).
func serviceKey(c string) string {
	c = strings.TrimSpace(c)
	if strings.HasPrefix(c, "`") {
		if end := strings.Index(c[1:], "`"); end > 0 {
			return strings.TrimSpace(c[1 : 1+end])
		}
	}
	k := bare(c)
	if i := strings.IndexAny(k, " (\t"); i > 0 { // booking (API)
		k = k[:i]
	}
	return k
}

// parseC4Section reads the first mermaid block of the section «Схема C4».
func parseC4Section(ls []line) *C4Block {
	sec, at := section(ls, "c4")
	if at < 0 {
		return nil
	}
	b := &C4Block{Line: ls[at].n}
	for i, l := range sec {
		t := strings.TrimSpace(l.text)
		fence, info := splitFence(t)
		if !l.code || fence == "" || firstWord(info) != "mermaid" {
			continue
		}
		b.BlockLine = l.n
		var src []string
		for _, l2 := range sec[i+1:] {
			if t2 := strings.TrimSpace(l2.text); strings.HasPrefix(t2, fence) && strings.TrimLeft(t2, fence[:1]) == "" {
				break
			}
			src = append(src, l2.text)
		}
		d, err := ParseC4(strings.Join(src, "\n"))
		if err != nil {
			var e *C4Error
			if errors.As(err, &e) {
				b.Err = &C4Error{Line: b.BlockLine + e.Line, Msg: e.Msg}
			}
		}
		b.Diagram = d
		return b
	}
	return b
}

// splitFence reads an opening fence: ``` or ~~~ (or longer) and the info string; "" if the line is not one.
func splitFence(t string) (fence, info string) {
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return "", ""
	}
	n := len(t) - len(strings.TrimLeft(t, t[:1]))
	if n < 3 {
		return "", ""
	}
	return t[:n], strings.TrimSpace(t[n:])
}

// CheckArch checks the services and the diagram of an arch document (FTR.HMR.SPEC-0004 R1, R4). A fix (with a
// parent) follows its fix template, which has neither.
func CheckArch(d Document) []Finding {
	if d.Parent != "" {
		return nil
	}
	var out []Finding
	if !d.HasServices {
		out = append(out, Finding{Check: CheckArchServicesMissing, Subject: "services",
			Message: "no table «Затрагиваемые сервисы и контракты» / «Affected services and contracts» whose first column is «Сервис» / «Service»", Line: max(d.ServicesLine, 1)})
	}
	switch {
	case d.C4 == nil || d.C4.BlockLine == 0:
		line := 1
		if d.C4 != nil {
			line = d.C4.Line
		}
		out = append(out, Finding{Check: CheckC4Missing, Subject: "c4", Message: "no section «Схема C4» / «C4 diagram» with a mermaid block C4Container", Line: line})
	case d.C4.Err != nil:
		out = append(out, Finding{Check: CheckC4Invalid, Subject: "c4", Message: d.C4.Err.Msg, Line: d.C4.Err.Line})
	default:
		for _, s := range d.Services {
			if !d.C4.Diagram.IsContainer(C4ID(s.Key)) {
				out = append(out, Finding{Check: CheckC4ServiceMissing, Subject: s.Key,
					Message: fmt.Sprintf("service %s is not a container of the C4 diagram: Container(%s, …)", s.Key, C4ID(s.Key)), Line: s.Line})
			}
		}
	}
	return out
}

// CheckCatalog reports the services of a document that are not keys of the catalog (R2), with the nearest key.
func CheckCatalog(d Document, catalog []string) []Finding {
	known := map[string]bool{}
	for _, k := range catalog {
		known[k] = true
	}
	var out []Finding
	for _, s := range d.Services {
		if known[s.Key] {
			continue
		}
		f := Finding{Check: CheckArchServiceUnknown, Subject: s.Key, Message: fmt.Sprintf("service %s is not in the catalog", s.Key), Line: s.Line}
		if n := Nearest(s.Key, catalog); n != "" {
			f.Suggest = n
			f.Message += fmt.Sprintf(": %s?", n)
		}
		out = append(out, f)
	}
	return out
}

// NamesServices tells whether an arch document names the services of its feature (R3): it is written by the
// contract 2.0 — it has the section «Схема C4» — and its table has keys. The first column of an arch of 1.x is free
// text («Запуски», «hammurapi-core (`api`)»): its feature keeps the services of tech (R9).
func (d Document) NamesServices() bool {
	return d.C4 != nil && len(d.Services) > 0
}

// CheckTech reports the services of tech that arch does not name (R3). Without arch, or with an arch that does not
// name the services (NamesServices), tech names the services itself.
func CheckTech(tech Document, arch *Document) []Finding {
	if arch == nil || !arch.NamesServices() {
		return nil
	}
	in := map[string]bool{}
	for _, s := range arch.Services {
		in[s.Key] = true
	}
	var out []Finding
	for _, s := range tech.Services {
		if !in[s.Key] {
			out = append(out, Finding{Check: CheckTechServiceNotInArch, Subject: s.Key, Message: fmt.Sprintf("service %s is not in the architecture", s.Key), Line: s.Line})
		}
	}
	return out
}

// FeatureServices are the services of a feature (R3): those of arch when it names any, otherwise those of tech.
func FeatureServices(arch, tech *Document) []string {
	var src []Service
	switch {
	case arch != nil && arch.NamesServices():
		src = arch.Services
	case tech != nil:
		src = tech.Services
	}
	out := []string{}
	for _, s := range src {
		out = append(out, s.Key)
	}
	return out
}

// Formats of the contract files of tech (R6).
const (
	FormatOpenAPIJSON = "openapi-json"
	FormatOpenAPIYAML = "openapi-yaml"
	FormatProto       = "proto"
)

// ContractFile reads the name of a file of tech: <service>.openapi.json|yaml|yml or <service>.proto.
func ContractFile(name string) (service, format string, ok bool) {
	for suffix, f := range map[string]string{".openapi.json": FormatOpenAPIJSON, ".openapi.yaml": FormatOpenAPIYAML, ".openapi.yml": FormatOpenAPIYAML, ".proto": FormatProto} {
		if s, found := strings.CutSuffix(name, suffix); found && s != "" && !strings.Contains(s, "/") {
			return s, f, true
		}
	}
	return "", "", false
}

// CheckContractFile checks a contract file of tech (tech §2.2): the service of its name is one of services, and
// the file is of its format. path is reported as the subject.
func CheckContractFile(path string, content []byte, services []string) []Finding {
	name := path[strings.LastIndex(path, "/")+1:]
	svc, format, ok := ContractFile(name)
	if !ok {
		return nil
	}
	var out []Finding
	known := false
	for _, s := range services {
		known = known || s == svc
	}
	if !known {
		out = append(out, Finding{Check: CheckContractFileService, Subject: path, Message: fmt.Sprintf("service %s of %s is not among the services of the feature", svc, name), Line: 1})
	}
	if line, err := checkFormat(format, content); err != nil {
		out = append(out, Finding{Check: CheckContractFileInvalid, Subject: path, Message: err.Error(), Line: line})
	}
	return out
}

func checkFormat(format string, b []byte) (int, error) {
	switch format {
	case FormatOpenAPIJSON:
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			var se *json.SyntaxError
			if errors.As(err, &se) {
				return 1 + bytes.Count(b[:min(int(se.Offset), len(b))], []byte("\n")), err
			}
			return 1, err
		}
		if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
			return 1, errors.New(`"openapi" is not 3.x`)
		}
		if _, ok := doc["paths"].(map[string]any); !ok {
			return 1, errors.New(`"paths" is not an object`)
		}
	case FormatOpenAPIYAML:
		if line, err := checkFormat(FormatOpenAPIJSON, b); err == nil || json.Valid(b) {
			return line, err // JSON is YAML: a generator may write a .yaml file in it
		}
		version, paths := false, false
		for i, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "\t") {
				return i + 1, errors.New("a tab in the indentation")
			}
			k, v, ok := strings.Cut(strings.TrimRight(l, "\r "), ":")
			if !ok || strings.HasPrefix(l, " ") {
				continue
			}
			switch strings.Trim(k, `"'`) {
			case "openapi":
				version = strings.HasPrefix(strings.Trim(strings.TrimSpace(v), `"'`), "3.")
			case "paths":
				paths = true
			}
		}
		if !version {
			return 1, errors.New("no top-level openapi: 3.x")
		}
		if !paths {
			return 1, errors.New("no top-level paths:")
		}
	case FormatProto:
		return checkProto(string(b))
	}
	return 0, nil
}

var (
	protoSyntaxRe = regexp.MustCompile(`^\s*syntax\s*=\s*["']proto3["']\s*;`)
	protoDeclRe   = regexp.MustCompile(`(?m)^\s*(service|message)\s+[A-Za-z_]\w*\s*\{`)
)

// checkProto: syntax = "proto3" first, balanced braces outside comments and strings, a service or a message.
func checkProto(src string) (int, error) {
	src = strings.TrimPrefix(src, "\ufeff")
	var code strings.Builder
	line, depth, open := 1, 0, 0
	var opens []int
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			line++
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
			i--
			continue
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return line, errors.New("a comment /* is not closed")
			}
			line += strings.Count(src[i:i+2+end], "\n")
			i += end + 3
			continue
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(src) || src[j] != c {
				return line, errors.New("a string is not closed")
			}
			code.WriteString(src[i : j+1])
			i = j
			continue
		case c == '{':
			depth++
			opens = append(opens, line)
		case c == '}':
			if depth == 0 {
				return line, errors.New("a } without {")
			}
			depth--
			opens = opens[:len(opens)-1]
		}
		code.WriteByte(c)
	}
	if depth > 0 {
		open = opens[len(opens)-1]
		return open, errors.New("a { is not closed")
	}
	s := code.String()
	if !protoSyntaxRe.MatchString(s) {
		return 1, errors.New(`the file does not start with syntax = "proto3";`)
	}
	if !protoDeclRe.MatchString(s) {
		return 1, errors.New("no service or message")
	}
	return 0, nil
}

// Nearest is the key of keys closest to k by the edit distance, "" if none is close: at most a third of the
// length, and at least 2.
func Nearest(k string, keys []string) string {
	best, bestD := "", max(2, len(k)/3)+1
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for _, c := range sorted {
		if d := editDistance(k, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
