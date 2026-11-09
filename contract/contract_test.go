package contract

import (
	"reflect"
	"strings"
	"testing"
)

const productRU = `# Решение — продуктовая спецификация

**Версия:** 1.0
**Опирается на:** ` + "`FTR.HMR.DEV-0001`" + ` (R12–R14, писатель), ` + "`FTR.HMR.CMN-0005`" + `

---

## 4. Требования

**R1.** Первое требование.
*Изменено в ` + "`FTR.HMR.DEV-0009`" + ` R2: другое.*
- Критерии приёмки:
  - Дано одно, когда другое, тогда третье.

**R2. Название.** Второе.
- Дано …

### 4.2 Другое

- Требование без номера

## Изменения в других решениях

| Решение | Требование | Что изменилось |
|---|---|---|
| ` + "`FTR.HMR.CMN-0002`" + ` | R13 | Поменялось. |
| ` + "`FTR.HMR.CMN-0001`" + ` | всё решение | Всё. |

` + "```" + `
| Решение | Требование | Что изменилось |
| ` + "`FTR.X.Y-0001`" + ` | R1 | пример в коде |
` + "```" + `
`

func TestParseProduct(t *testing.T) {
	d := Parse(productRU, Product)
	if d.Title != "Решение — продуктовая спецификация" {
		t.Errorf("title %q", d.Title)
	}
	wantDeps := []Ref{{Key: "FTR.HMR.DEV-0001", Items: []string{"R12", "R13", "R14"}}, {Key: "FTR.HMR.CMN-0005", Items: []string{}}}
	if !reflect.DeepEqual(d.DependsOn, wantDeps) {
		t.Errorf("depends on %+v", d.DependsOn)
	}
	if len(d.Items) != 2 || d.Items[0].ID != "R1" || d.Items[1].ID != "R2" || d.Items[1].Text != "Название. Второе." {
		t.Errorf("items %+v", d.Items)
	}
	wantCh := []Change{{Key: "FTR.HMR.CMN-0002", Item: "R13", Text: "Поменялось."}, {Key: "FTR.HMR.CMN-0001", Item: "", Text: "Всё."}}
	for i := range d.Changes {
		d.Changes[i].Line = 0
	}
	if !reflect.DeepEqual(d.Changes, wantCh) {
		t.Errorf("changes %+v", d.Changes)
	}
	if len(d.Notes) != 1 || d.Notes[0].Target != "R1" || d.Notes[0].Source != "FTR.HMR.DEV-0009" || d.Notes[0].SourceItem != "R2" || d.Notes[0].Text != "другое" {
		t.Errorf("notes %+v", d.Notes)
	}
	if len(d.Violations) != 1 || d.Violations[0].Element != ViolRequirementUnnum {
		t.Errorf("violations %+v", d.Violations)
	}
}

const archEN = `# Feature — architecture

**Depends on:** ` + "`FTR.HMR.CMN-0001`" + ` (A3, A5)

## Decisions

| A | Decision | Reason |
|---|---|---|
| A1 | One | Because |
| A2 | Two | Because |
| 3 | Three without a number | Because |

## Changes to other features

| Feature | Decision | What changed |
|---|---|---|
| ` + "`FTR.HMR.DEV-0002`" + ` | A5 | The layer of a link. |
`

func TestParseArchEnglish(t *testing.T) {
	d := Parse(archEN, Arch)
	if len(d.DependsOn) != 1 || !reflect.DeepEqual(d.DependsOn[0].Items, []string{"A3", "A5"}) {
		t.Errorf("depends on %+v", d.DependsOn)
	}
	if len(d.Items) != 2 || d.Items[0].ID != "A1" || d.Items[0].Kind != KindDecision || d.Items[1].Text != "Two — Because" {
		t.Errorf("items %+v", d.Items)
	}
	if len(d.Violations) != 1 || d.Violations[0].Element != ViolDecisionsUnnumb {
		t.Errorf("violations %+v", d.Violations)
	}
	if len(d.Changes) != 1 || d.Changes[0].Item != "A5" || d.Changes[0].Key != "FTR.HMR.DEV-0002" {
		t.Errorf("changes %+v", d.Changes)
	}
}

func TestParseItems(t *testing.T) {
	cases := map[string][]string{
		"R1, R3–R5":        {"R1", "R3", "R4", "R5"},
		"R26a–R26c":        {"R26a", "R26b", "R26c"},
		"R25–R27":          {"R25", "R26", "R27"},
		"A3, A5":           {"A3", "A5"},
		"(писатель)":       {},
		"R12–R16 — генер.": {"R12", "R13", "R14", "R15", "R16"},
	}
	for in, want := range cases {
		if got := ParseItems(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}

func TestApplyNotesProduct(t *testing.T) {
	notes := []Note{{Target: "R2", SourceItem: "R7", Text: "ещё одно"}, {Target: "", Text: "всё решение"}, {Target: "R99", Text: "нет такого"}}
	got := ApplyNotes(productRU, Product, "FTR.HMR.DEV-0003", notes, RU)
	if !strings.Contains(got, "**R2. Название.** Второе.\n*Изменено в `FTR.HMR.DEV-0003` R7: ещё одно.*\n- Дано") {
		t.Errorf("note of R2 misplaced:\n%s", got)
	}
	if !strings.Contains(got, "**Опирается на:**") || !strings.Contains(got, "(R12–R14, писатель), `FTR.HMR.CMN-0005`\n*Изменено в `FTR.HMR.DEV-0003`: всё решение.*") {
		t.Errorf("note of the whole feature misplaced:\n%s", got)
	}
	if strings.Contains(got, "нет такого") {
		t.Error("a note of a missing requirement was put")
	}
	if again := ApplyNotes(got, Product, "FTR.HMR.DEV-0003", notes, RU); again != got {
		t.Errorf("not idempotent:\n%s", again)
	}
	if clean := ApplyNotes(got, Product, "FTR.HMR.DEV-0003", nil, RU); clean != productRU {
		t.Errorf("removing the notes does not restore the document:\n%s", clean)
	}
	// the note of another feature stays first
	if !strings.Contains(got, "**R1.** Первое требование.\n*Изменено в `FTR.HMR.DEV-0009` R2: другое.*\n- Критерии") {
		t.Errorf("the note of another feature was moved:\n%s", got)
	}
}

func TestApplyNotesArch(t *testing.T) {
	doc := strings.Replace(archEN, "| 3 | Three without a number | Because |\n", "", 1)
	a := ApplyNotes(doc, Arch, "FTR.HMR.DEV-0009", []Note{{Target: "A2", SourceItem: "A1", Text: "other"}}, EN)
	got := ApplyNotes(a, Arch, "FTR.HMR.DEV-0003", []Note{{Target: "A1", SourceItem: "A4", Text: "first"}, {Target: "A2", Text: "second"}}, EN)
	want := "| A2 | Two | Because |\n\n- *A1 — changed in `FTR.HMR.DEV-0003` A4: first.*\n- *A2 — changed in `FTR.HMR.DEV-0003`: second.*\n- *A2 — changed in `FTR.HMR.DEV-0009` A1: other.*\n\n## Changes to other features"
	if !strings.Contains(got, want) {
		t.Errorf("arch notes:\n%s", got)
	}
	if again := ApplyNotes(got, Arch, "FTR.HMR.DEV-0003", []Note{{Target: "A1", SourceItem: "A4", Text: "first"}, {Target: "A2", Text: "second"}}, EN); again != got {
		t.Errorf("not idempotent:\n%s", again)
	}
	back := ApplyNotes(ApplyNotes(got, Arch, "FTR.HMR.DEV-0003", nil, EN), Arch, "FTR.HMR.DEV-0009", nil, EN)
	if back != doc {
		t.Errorf("removing the notes does not restore the document:\n%q\n%q", back, doc)
	}
	d := Parse(got, Arch)
	if len(d.Notes) != 3 || d.Notes[0].Target != "A1" || d.Notes[0].SourceItem != "A4" {
		t.Errorf("parsed arch notes %+v", d.Notes)
	}
}

func TestCheckTemplate(t *testing.T) {
	product := "# Продукт: <название>\n\n**Опирается на:** <ключи>\n\n## Требования\n\n**R1.** <Требование>\n\n- Критерии приёмки:\n  - Дано <условие>.\n\n## Изменения в других решениях\n\n| Решение | Требование | Что изменилось |\n|---|---|---|\n"
	if v := CheckTemplate(Product, FileTemplate, product); len(v) != 0 {
		t.Errorf("valid product template: %+v", v)
	}
	broken := strings.Replace(product, "## Изменения в других решениях", "## Прочее", 1)
	if v := CheckTemplate(Product, FileTemplate, broken); len(v) != 1 || v[0].Element != ElemChanges {
		t.Errorf("product without the section: %+v", v)
	}
	renamed := strings.Replace(product, "## Требования", "## Что нужно", 1)
	if v := CheckTemplate(Product, FileTemplate, renamed); len(v) != 0 {
		t.Errorf("a renamed free section is not a violation: %+v", v)
	}
	arch := "# Arch: <name>\n\n**Depends on:** <keys>\n\n## Decisions\n\n| # | Decision | Reason |\n|---|---|---|\n\n## Changes to other features\n\n| Feature | Decision | What changed |\n|---|---|---|\n"
	if v := CheckTemplate(Arch, FileTemplate, arch); len(v) != 1 || v[0].Element != ElemDecisions {
		t.Errorf("arch with #: %+v", v)
	}
	if v := CheckTemplate(Arch, FileTemplate, strings.Replace(arch, "| # |", "| A |", 1)); len(v) != 0 {
		t.Errorf("valid arch: %+v", v)
	}
	if v := CheckTemplate(Arch, FileFixTemplate, strings.Replace(arch, "| # |", "| A |", 1)); len(v) != 1 || v[0].Element != ElemParent {
		t.Errorf("fix without parent: %+v", v)
	}
	if v := CheckTemplate(QA, FileTemplate, "# QA"); len(v) != 0 {
		t.Errorf("qa has no elements: %+v", v)
	}
}

func TestPromptAndDescribe(t *testing.T) {
	if p := Prompt(Arch, RU); !strings.Contains(p, "«## Принятые решения»") || !strings.Contains(p, "**Опирается на:**") {
		t.Errorf("prompt: %s", p)
	}
	if p := Prompt(Product, EN); !strings.Contains(p, "«## Changes to other features»") {
		t.Errorf("prompt: %s", p)
	}
	if len(Describe(Arch, FileFixTemplate, RU)) != 2 || len(Describe(Arch, FileTemplate, RU)) != 3 || len(Describe(QA, FileTemplate, EN)) != 0 {
		t.Error("describe")
	}
}

func TestSortItems(t *testing.T) {
	ids := []string{"R26a", "A2", "R13", "R2", "A10", "R26"}
	SortItems(ids)
	if want := []string{"R2", "R13", "R26", "R26a", "A2", "A10"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("%v", ids)
	}
}

func TestMigrateArch(t *testing.T) {
	in := "# A\n\n**Опирается на:** продуктовую спецификацию v1.1; архитектуру `FTR.HMR.CMN-0001` v1.5\n\n## 9. Принятые решения\n\n| # | Решение | Обоснование |\n|---|---|---|\n| 1 | Один | 1 раз |\n| 2a | Два | — |\n\n| # | Другая таблица |\n|---|---|\n| 1 | не решения? тоже решения раздела |\n\n## Потом\n\n| # | x |\n|---|---|\n| 1 | не трогать |\n"
	want := "# A\n\n**Опирается на:** продуктовую спецификацию; архитектуру `FTR.HMR.CMN-0001`\n\n## 9. Принятые решения\n\n| A | Решение | Обоснование |\n|---|---|---|\n| A1 | Один | 1 раз |\n| A2a | Два | — |\n\n| A | Другая таблица |\n|---|---|\n| A1 | не решения? тоже решения раздела |\n\n## Потом\n\n| # | x |\n|---|---|\n| 1 | не трогать |\n"
	if got := MigrateArch(in); got != want {
		t.Errorf("got:\n%s", got)
	}
	if MigrateArch(want) != want {
		t.Error("not idempotent")
	}
}

func TestUnknownLanguageWritesEnglish(t *testing.T) {
	if got := NoteLine(Note{Source: "FTR.HMR.DEV-0003", Text: "x"}, Product, ""); got != "*Changed in `FTR.HMR.DEV-0003`: x.*" {
		t.Errorf("%q", got)
	}
}
