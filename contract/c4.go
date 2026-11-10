package contract

import (
	"fmt"
	"regexp"
	"strings"
)

// C4 is a diagram of the level of containers in mermaid (FTR.HMR.SPEC-0004 R4): the subset of the C4 syntax of
// mermaid that the contract reads — elements, boundaries, relations and styles.
type C4 struct {
	Title    string
	Elements []C4Element
	Rels     []C4Rel
}

// C4Element is a person, system, container or component of a diagram.
type C4Element struct {
	ID    string
	Kind  string // the function: Container, ContainerDb, System_Ext, Person…
	Label string
	Line  int // in the block, 1-based
}

// C4Rel is a relation between two elements.
type C4Rel struct {
	From, To, Label string
	Line            int
}

// C4Error is a line of a block that does not parse.
type C4Error struct {
	Line int // in the block, 1-based
	Msg  string
}

func (e *C4Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// c4Elements are the functions that declare an element, by kind; the containers are the services.
var c4Elements = map[string]bool{}

func init() {
	for _, base := range []string{"Person", "System", "SystemDb", "SystemQueue", "Container", "ContainerDb", "ContainerQueue", "Component", "ComponentDb", "ComponentQueue"} {
		c4Elements[base] = true
		c4Elements[base+"_Ext"] = true
	}
}

var (
	c4Boundaries = map[string]bool{"Boundary": true, "Enterprise_Boundary": true, "System_Boundary": true, "Container_Boundary": true}
	c4Rels       = map[string]bool{"Rel": true, "BiRel": true, "Rel_Back": true, "Rel_U": true, "Rel_Up": true, "Rel_D": true, "Rel_Down": true,
		"Rel_L": true, "Rel_Left": true, "Rel_R": true, "Rel_Right": true}
	c4Styles = map[string]bool{"UpdateElementStyle": true, "UpdateRelStyle": true, "UpdateBoundaryStyle": true, "UpdateLayoutConfig": true,
		"AddElementTag": true, "AddRelTag": true, "AddBoundaryTag": true}
	c4CallRe = regexp.MustCompile(`^([A-Za-z_]+)\s*\((.*)\)\s*(\{)?\s*$`)
	c4IDRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// C4ID is the identifier of the container of a service on a diagram: the key with - and . written as _
// (booking-api → booking_api).
func C4ID(serviceKey string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(serviceKey)
}

// IsContainer tells whether the diagram has a container (Container, ContainerDb, ContainerQueue) with the id; an
// external one (_Ext) is not a service of the catalog.
func (c *C4) IsContainer(id string) bool {
	for _, e := range c.Elements {
		if e.ID == id && strings.HasPrefix(e.Kind, "Container") && !strings.HasSuffix(e.Kind, "_Ext") {
			return true
		}
	}
	return false
}

// ParseC4 reads the text of a mermaid block of the kind C4Container. The error is a *C4Error with the line.
func ParseC4(src string) (*C4, error) {
	ls := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	out := &C4{}
	started, depth := false, 0
	ids := map[string]int{}
	for i, raw := range ls {
		n := i + 1
		t := strings.TrimSpace(raw)
		if t == "" || strings.HasPrefix(t, "%%") {
			continue
		}
		if !started {
			if t != "C4Container" {
				return nil, &C4Error{n, fmt.Sprintf("the diagram must start with C4Container, not %q", firstWord(t))}
			}
			started = true
			continue
		}
		switch {
		case t == "}":
			if depth == 0 {
				return nil, &C4Error{n, "a } without a boundary"}
			}
			depth--
			continue
		case strings.HasPrefix(t, "title ") || t == "title":
			out.Title = strings.TrimSpace(strings.TrimPrefix(t, "title"))
			continue
		case strings.HasPrefix(t, "accTitle") || strings.HasPrefix(t, "accDescr"):
			continue
		}
		m := c4CallRe.FindStringSubmatch(t)
		if m == nil {
			return nil, &C4Error{n, fmt.Sprintf("not a statement of C4: %q", t)}
		}
		fn, open := m[1], m[3] == "{"
		args, err := c4Args(m[2])
		if err != nil {
			return nil, &C4Error{n, err.Error()}
		}
		pos := positional(args)
		switch {
		case c4Elements[fn] || c4Boundaries[fn]:
			if open != c4Boundaries[fn] {
				if open {
					return nil, &C4Error{n, fn + " is not a boundary: no { after it"}
				}
				return nil, &C4Error{n, fn + " is a boundary: { must follow it"}
			}
			if len(pos) < 2 && !(c4Boundaries[fn] && len(pos) == 1) {
				return nil, &C4Error{n, fn + " needs an identifier and a label"}
			}
			id := pos[0]
			if !c4IDRe.MatchString(id) {
				return nil, &C4Error{n, fmt.Sprintf("the identifier %q has characters other than letters, digits and _", id)}
			}
			if prev, ok := ids[id]; ok {
				return nil, &C4Error{n, fmt.Sprintf("%s is declared twice (line %d)", id, prev)}
			}
			ids[id] = n
			if open {
				depth++
				continue
			}
			out.Elements = append(out.Elements, C4Element{ID: id, Kind: fn, Label: at(pos, 1), Line: n})
		case c4Rels[fn]:
			if open {
				return nil, &C4Error{n, fn + " is not a boundary: no { after it"}
			}
			if len(pos) < 3 {
				return nil, &C4Error{n, fn + " needs two identifiers and a label"}
			}
			out.Rels = append(out.Rels, C4Rel{From: pos[0], To: pos[1], Label: pos[2], Line: n})
		case c4Styles[fn]:
			if open {
				return nil, &C4Error{n, fn + " is not a boundary: no { after it"}
			}
		default:
			return nil, &C4Error{n, fmt.Sprintf("unknown function %s", fn)}
		}
	}
	if !started {
		return nil, &C4Error{1, "the block is empty"}
	}
	if depth > 0 {
		return nil, &C4Error{len(ls), "a boundary is not closed with }"}
	}
	for _, r := range out.Rels {
		for _, id := range []string{r.From, r.To} {
			if _, ok := ids[id]; !ok {
				return nil, &C4Error{r.Line, fmt.Sprintf("the relation names %s, which is not declared", id)}
			}
		}
	}
	return out, nil
}

type c4Arg struct {
	name, value string // name is set for $name="value"
}

// c4Args splits the arguments of a call: "quoted, with commas", bare words and $name="value".
func c4Args(s string) ([]c4Arg, error) {
	var out []c4Arg
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for {
		s = strings.TrimSpace(s)
		var a c4Arg
		if strings.HasPrefix(s, "$") {
			eq := strings.Index(s, "=")
			if eq < 0 {
				return nil, fmt.Errorf("a named argument without =: %q", s)
			}
			a.name = strings.TrimSpace(s[1:eq])
			s = strings.TrimSpace(s[eq+1:])
		}
		if strings.HasPrefix(s, `"`) {
			end := strings.Index(s[1:], `"`)
			if end < 0 {
				return nil, fmt.Errorf("a quote is not closed")
			}
			a.value, s = s[1:1+end], strings.TrimSpace(s[2+end:])
		} else {
			end := strings.Index(s, ",")
			if end < 0 {
				end = len(s)
			}
			a.value, s = strings.TrimSpace(s[:end]), s[end:]
			if strings.Contains(a.value, `"`) {
				return nil, fmt.Errorf("a quote in the middle of %q", a.value)
			}
		}
		out = append(out, a)
		if s == "" {
			return out, nil
		}
		if !strings.HasPrefix(s, ",") {
			return nil, fmt.Errorf("a comma is expected before %q", s)
		}
		s = s[1:]
	}
}

func positional(args []c4Arg) []string {
	var out []string
	for _, a := range args {
		if a.name == "" {
			out = append(out, a.value)
		}
	}
	return out
}

func at(xs []string, i int) string {
	if i < len(xs) {
		return xs[i]
	}
	return ""
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}
