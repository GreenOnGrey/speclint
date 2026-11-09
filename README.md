# speclint

The document contract of [Hammurapi](https://github.com/GreenOnGrey/hammurapi) and the linter of a
specification repository.

Hammurapi reads a few elements in every specification: the line «Depends on», requirements `R<n>`
with acceptance criteria, architecture decisions `A<n>`, the section «Changes to other features» and
the notes «Changed in». This module is that contract. Hammurapi's indexer and `speclint` use the same
code, so a pull request is checked exactly the way Hammurapi will read it. Russian and English
documents are both read.

## Run

```sh
go run github.com/GreenOnGrey/speclint/cmd/speclint@<version> -root . -baseline speclint-baseline.txt
```

- `-baseline` — known errors that do not fail the run; the file only shrinks.
- `-write-baseline` — accept the current errors.
- `-errors-only` — print only new errors and the summary.
- `-code <repo>` — also look for the IDs of test cases in a code repository (repeatable).

Other commands: `speclint migrate-arch -root .` numbers the decisions of `arch` documents `A<n>`;
`speclint version`.

## Library

```go
import "github.com/GreenOnGrey/speclint/contract"

d := contract.Parse(doc, contract.Product) // DependsOn, Items, Changes, Notes, Violations
v := contract.CheckTemplate(contract.Arch, contract.FileTemplate, tpl)
```

`starter.Files(contract.EN)` returns the starter rules templates.

## License

Apache 2.0.
