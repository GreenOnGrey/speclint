package contract

import (
	"errors"
	"strings"
	"testing"
)

const archRU = "# Архитектура: отмена брони\n\n**Опирается на:** `FTR.FMS.CAR-0001` (A3)\n\n" +
	"## Затрагиваемые сервисы и контракты\n\n| Сервис | Роль в фиче | Меняющиеся контракты |\n|---|---|---|\n" +
	"| `booking` (API) | Бронь | `POST /bookings/{id}/cancel` |\n| pricing-api | Цены | — |\n\n" +
	"## Схема C4\n\n```mermaid\nC4Container\n  title Отмена брони\n  Person(client, \"Клиент\")\n" +
	"  System_Boundary(rent, \"Аренда\") {\n    Container(booking, \"Бронь\", \"Go\", \"Брони и отмены\")\n" +
	"    ContainerDb(pricing_api, \"Цены\", \"Postgres\")\n  }\n  System_Ext(pay, \"Платёжный шлюз, банк\")\n" +
	"  %% a comment\n  Rel(client, booking, \"отменяет бронь\", \"HTTPS\")\n  Rel_Back(booking, pay, \"возврат\", $tags=\"async\")\n" +
	"  UpdateLayoutConfig($c4ShapeInRow=\"3\")\n```\n\n## Принятые решения\n\n| A | Решение | Обоснование |\n|---|---|---|\n| A1 | x | y |\n"

// SVC-01: the table of arch gives the services with their lines.
func TestArchServices(t *testing.T) {
	d := Parse(archRU, Arch)
	if !d.HasServices || len(d.Services) != 2 || d.Services[0].Key != "booking" || d.Services[1].Key != "pricing-api" || d.Services[0].Line != 9 {
		t.Fatalf("SVC-01 services %+v", d.Services)
	}
	if f := CheckArch(d); len(f) != 0 {
		t.Errorf("valid arch: %+v", f)
	}
}

// C4-01: elements, a boundary, relations, styles, named arguments and commas in quotes.
func TestParseC4(t *testing.T) {
	d := Parse(archRU, Arch)
	c := d.C4.Diagram
	if c == nil || d.C4.Err != nil {
		t.Fatalf("C4-01 %+v", d.C4)
	}
	if c.Title != "Отмена брони" || len(c.Elements) != 4 || len(c.Rels) != 2 || !c.IsContainer("booking") || !c.IsContainer("pricing_api") || c.IsContainer("pay") {
		t.Errorf("C4-01 diagram %+v", c)
	}
	if c.Elements[3].Label != "Платёжный шлюз, банк" || c.Rels[1].Label != "возврат" {
		t.Errorf("C4-01 labels %+v %+v", c.Elements[3], c.Rels[1])
	}
}

// C4-02: a service of the table without a container.
func TestC4ServiceMissing(t *testing.T) {
	doc := strings.Replace(archRU, "ContainerDb(pricing_api", "System_Ext(pricing_api", 1)
	f := CheckArch(Parse(doc, Arch))
	if len(f) != 1 || f[0].Check != CheckC4ServiceMissing || f[0].Subject != "pricing-api" || f[0].Line != 10 {
		t.Errorf("C4-02 %+v", f)
	}
}

// C4-03: an unclosed quote, an unknown function, a relation to an unknown element — with the line.
func TestC4Invalid(t *testing.T) {
	for name, tc := range map[string]struct {
		from, to string
		line     int
	}{
		"quote":    {`Person(client, "Клиент")`, `Person(client, "Клиент)`, 17},
		"function": {`Person(client, "Клиент")`, `Persona(client, "Клиент")`, 17},
		"relation": {`Rel(client, booking`, `Rel(client, bookng`, 24},
		"boundary": {"    ContainerDb(pricing_api, \"Цены\", \"Postgres\")\n  }", "    ContainerDb(pricing_api, \"Цены\", \"Postgres\")", 25},
		"hyphen":   {"ContainerDb(pricing_api", "ContainerDb(pricing-api", 20},
	} {
		f := CheckArch(Parse(strings.Replace(archRU, tc.from, tc.to, 1), Arch))
		if len(f) != 1 || f[0].Check != CheckC4Invalid || f[0].Line != tc.line {
			t.Errorf("C4-03 %s: %+v", name, f)
		}
	}
	if _, err := ParseC4("C4Context\n  Person(a, \"b\")"); err == nil {
		t.Error("C4-03 a context diagram is not of the contract")
	} else if e := new(C4Error); !errors.As(err, &e) || e.Line != 1 {
		t.Errorf("C4-03 %v", err)
	}
}

// C4-04: no section of the diagram; a section without a block.
func TestC4Missing(t *testing.T) {
	i := strings.Index(archRU, "## Схема C4")
	j := strings.Index(archRU, "## Принятые решения")
	f := CheckArch(Parse(archRU[:i]+archRU[j:], Arch))
	if len(f) != 1 || f[0].Check != CheckC4Missing {
		t.Errorf("C4-04 %+v", f)
	}
	f = CheckArch(Parse(archRU[:i]+"## Схема C4\n\nПозже.\n\n"+archRU[j:], Arch))
	if len(f) != 1 || f[0].Check != CheckC4Missing || f[0].Line != 12 {
		t.Errorf("C4-04 empty section %+v", f)
	}
	f = CheckArch(Parse("# A\n\n## Принятые решения\n", Arch))
	if len(f) != 2 || f[0].Check != CheckArchServicesMissing || f[1].Check != CheckC4Missing {
		t.Errorf("an arch of 1.x %+v", f)
	}
}

// C4-05: the English document reads the same.
func TestC4English(t *testing.T) {
	en := strings.NewReplacer("Затрагиваемые сервисы и контракты", "Affected services and contracts", "| Сервис |", "| Service |",
		"Схема C4", "C4 diagram", "Принятые решения", "Decisions", "Опирается на", "Depends on").Replace(archRU)
	d := Parse(en, Arch)
	if len(d.Services) != 2 || d.C4 == nil || d.C4.Diagram == nil || len(CheckArch(d)) != 0 {
		t.Errorf("C4-05 %+v %+v", d.Services, d.C4)
	}
}

// SVC-04: a key not in the catalog, with the nearest one.
func TestCheckCatalog(t *testing.T) {
	d := Parse(strings.Replace(archRU, "`booking`", "`bookng`", 1), Arch)
	f := CheckCatalog(d, []string{"booking", "pricing-api", "billing"})
	if len(f) != 1 || f[0].Check != CheckArchServiceUnknown || f[0].Subject != "bookng" || f[0].Suggest != "booking" {
		t.Errorf("SVC-04 %+v", f)
	}
	if f := CheckCatalog(Parse(strings.Replace(archRU, "`booking`", "`warehouse`", 1), Arch), []string{"booking", "pricing-api"}); len(f) != 1 || f[0].Suggest != "" {
		t.Errorf("no near key %+v", f)
	}
}

// SVC-06, SVC-07: tech names only the services of arch; without arch it names them itself.
func TestCheckTech(t *testing.T) {
	tech := Parse("# Tech\n\n## Изменения по сервисам\n\n| Сервис | Репозиторий | Что меняется | Требования |\n|---|---|---|---|\n| `booking` | org/b | x | R1 |\n| `pricing` | org/p | y | R2 |\n", Tech)
	arch := Parse(archRU, Arch)
	f := CheckTech(tech, &arch)
	if len(f) != 1 || f[0].Check != CheckTechServiceNotInArch || f[0].Subject != "pricing" || f[0].Line != 8 {
		t.Errorf("SVC-06 %+v", f)
	}
	if CheckTech(tech, nil) != nil || strings.Join(FeatureServices(nil, &tech), ",") != "booking,pricing" {
		t.Error("SVC-07 without arch")
	}
	if strings.Join(FeatureServices(&arch, &tech), ",") != "booking,pricing-api" {
		t.Error("the services of arch first")
	}
}

// FILE-02, FILE-04 and the formats of the contract files.
func TestContractFiles(t *testing.T) {
	services := []string{"booking"}
	ok := map[string]string{
		"tech/booking.openapi.yaml": "openapi: \"3.0.3\"\ninfo:\n  title: b\npaths:\n  /bookings:\n    get: {}\n",
		"tech/booking.openapi.yml":  "openapi: 3.1.0\npaths: {}\n",
		"tech/booking.openapi.json": `{"openapi": "3.1.0", "paths": {}}`,
		"tech/booking.proto":        "// c\nsyntax = \"proto3\";\npackage b;\n/* { */\nservice Booking {\n  rpc Cancel(Req) returns (Resp);\n}\nmessage Req { string id = 1; // }\n}\n",
	}
	for p, c := range ok {
		if f := CheckContractFile(p, []byte(c), services); len(f) != 0 {
			t.Errorf("%s: %+v", p, f)
		}
	}
	if f := CheckContractFile("tech/billing.proto", []byte(ok["tech/booking.proto"]), services); len(f) != 1 || f[0].Check != CheckContractFileService {
		t.Errorf("FILE-02 %+v", f)
	}
	bad := map[string]struct {
		content string
		line    int
	}{
		"tech/booking.proto":        {"package b;\nmessage A {}\n", 1}, // FILE-04
		"tech/booking.openapi.json": {"{\n\"openapi\": \"3.0.0\",\n\"paths\": {\n}", 4},
		"tech/booking.openapi.yaml": {"swagger: \"2.0\"\npaths: {}\n", 1},
		"tech/booking.openapi.yml":  {"openapi: 3.0.0\n\tpaths: {}\n", 2},
	}
	for p, tc := range bad {
		if f := CheckContractFile(p, []byte(tc.content), services); len(f) != 1 || f[0].Check != CheckContractFileInvalid || f[0].Line != tc.line {
			t.Errorf("%s: %+v", p, f)
		}
	}
	if f := CheckContractFile("tech/booking.proto", []byte("syntax = \"proto3\";\nmessage A {\n  string a = 1;\n"), services); len(f) != 1 || f[0].Line != 2 {
		t.Errorf("an unclosed { names its line: %+v", f)
	}
	if _, _, ok := ContractFile("spec.md"); ok {
		t.Error("spec.md is not a contract")
	}
}

// The forms found in review: a long fence with an info string, an external container, a key with text after it,
// proto with single quotes and a BOM, YAML with quoted keys or in JSON.
func TestServiceAndContractForms(t *testing.T) {
	doc := strings.Replace(strings.Replace(archRU, "```mermaid\n", "````mermaid title=x\n", 1), "UpdateLayoutConfig($c4ShapeInRow=\"3\")\n```", "UpdateLayoutConfig($c4ShapeInRow=\"3\")\n````", 1)
	if f := CheckArch(Parse(doc, Arch)); len(f) != 0 {
		t.Errorf("a long fence: %+v", f)
	}
	ext := strings.Replace(archRU, "Container(booking,", "Container_Ext(booking,", 1)
	if f := CheckArch(Parse(ext, Arch)); len(f) != 1 || f[0].Check != CheckC4ServiceMissing || f[0].Subject != "booking" {
		t.Errorf("an external container is not the service: %+v", f)
	}
	if d := Parse(strings.Replace(archRU, "| pricing-api |", "| pricing-api (цены) |", 1), Arch); d.Services[1].Key != "pricing-api" {
		t.Errorf("a key with text: %+v", d.Services)
	}
	for p, c := range map[string]string{
		"tech/booking.proto":        "\ufeffsyntax = 'proto3';\nmessage A {}\n",
		"tech/booking.openapi.yaml": "\"openapi\": \"3.0.3\"\n'paths': {}\n",
		"tech/booking.openapi.yml":  `{"openapi": "3.1.0", "paths": {}}`,
	} {
		if f := CheckContractFile(p, []byte(c), []string{"booking"}); len(f) != 0 {
			t.Errorf("%s: %+v", p, f)
		}
	}
}
