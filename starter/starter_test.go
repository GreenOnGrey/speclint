package starter

import (
	"strings"
	"testing"

	"github.com/GreenOnGrey/speclint/v2/contract"
)

func TestStarterKeepsTheContract(t *testing.T) {
	for _, lang := range contract.Langs {
		fs := Files(lang)
		if len(fs) != 10 {
			t.Fatalf("%s: %d files, want 10", lang, len(fs))
		}
		for p, b := range fs {
			parts := strings.Split(p, "/") // rules/<area>/<file>.md
			if len(parts) != 3 || parts[0] != "rules" {
				t.Fatalf("%s: unexpected path %s", lang, p)
			}
			file := strings.TrimSuffix(parts[2], ".md")
			if v := contract.CheckTemplate(contract.Area(parts[1]), file, string(b)); len(v) != 0 {
				t.Errorf("%s %s: %+v", lang, p, v)
			}
		}
	}
}

// TPL-01 (FTR.HMR.SPEC-0004): the template of arch has the table of services and a diagram that parses.
func TestStarterArchDiagram(t *testing.T) {
	for _, lang := range contract.Langs {
		d := contract.Parse(string(Files(lang)["rules/arch/template.md"]), contract.Arch)
		if f := contract.CheckArch(d); len(f) != 0 || d.C4 == nil || d.C4.Diagram == nil || !d.C4.Diagram.IsContainer("booking") {
			t.Errorf("%s: %+v %+v", lang, f, d.C4)
		}
	}
}
