# speclint

The document contract of [Hammurapi](https://github.com/GreenOnGrey/hammurapi) and the linter of a
specification repository.

Hammurapi reads a few elements in every specification: the line «Depends on», requirements `R<n>`
with acceptance criteria, architecture decisions `A<n>`, the section «Changes to other features» and
the notes «Changed in», the services of arch with their C4 diagram in mermaid, the services of tech
and the contract files next to it (`<service>.openapi.yaml`, `<service>.proto`). This module is that contract. Hammurapi's indexer and `speclint` use the same
code, so a pull request is checked exactly the way Hammurapi will read it. Russian and English
documents are both read.

## What it checks

A repository of specifications is a tree `specs/<domain>/<group>/<FTR.DOM.GRP-0001>/<area>/spec.md` with the
areas `product`, `design`, `arch`, `tech` and `qa`. Every check below reports a name, a line and a message.
An error fails the run unless the baseline lists it; a warning never fails it, and no flag turns a check off.

### `spec` — a repository of specifications

The requirements of product and the test cases of qa:

| Check | What it reports |
|---|---|
| `req-duplicate` | a requirement `R<n>` defined more than once in product |
| `req-no-criteria` | a requirement with no acceptance criterion — a list item that starts with «Дано» / «Given» |
| `qa-duplicate-id` | a test case ID listed more than once in qa |
| `qa-unknown-req` | a test case that names a requirement its feature does not define (a fix counts the requirements of its parent) |
| `qa-unknown-service` | a key of the column «Сервисы» / «Services» that is not among the services of the feature (those of arch, or of tech without them); nothing to check when the feature names no services |
| `changes-line` | the line «Меняет:», which the section «Изменения в других решениях» replaced |

References between features, in the line «Опирается на», in text and in the table of changes:

| Check | What it reports |
|---|---|
| `ref-unknown-feature` | a reference to a feature that does not exist, or a row of the section of changes that names one: an example belongs in the text, not in the table |
| `ref-unknown-req` | a reference to `R<n>` that the target does not define, or a row of the section of changes that takes more than one requirement (one row per requirement) |
| `parent-unknown` | `parent` in the front matter of a fix that names no feature |
| `changes-no-backlink` | a row of the section of changes whose target does not point back: the changed requirement names no key of the changing feature, the changed feature no mention of it (a note «*Изменено в `FTR.…`*», or any mention of the key) |
| `note-without-change` | a note «*Изменено в `FTR.…`*» whose source has no row of the section of changes about this requirement, or about the whole feature for a note in the header |

A key of a domain with no specifications in this repository (`FTR.FMS.CAR-0005` of a template) is an example
in text and is skipped; in the table of changes it is still an error, since Hammurapi applies every row.

The decisions of arch:

| Check | What it reports |
|---|---|
| `decisions-unnumbered` | a row of the table «Принятые решения» whose first cell is not a number `A<n>` |
| `decision-duplicate` | a decision `A<n>` defined more than once |
| `ref-unknown-decision` | a reference to `A<n>` that does not exist |
| `changes-wrong-kind` | a row of product that changes a decision `A<n>`, or a row of arch that changes a requirement `R<n>`: product changes requirements, arch changes decisions |
| `changes-no-backlink`, `note-without-change` | the same as for product, for the section of changes and the notes of arch |

The services and the C4 diagram of arch; the services and the contract files of tech:

| Check | What it reports |
|---|---|
| `arch-services-missing` | no table «Затрагиваемые сервисы и контракты» / «Affected services and contracts» whose first column is «Сервис» / «Service» |
| `c4-missing` | no section «Схема C4» / «C4 diagram» with a mermaid block of the kind `C4Container` |
| `c4-invalid` | a mermaid block that is not read: it does not start with `C4Container`, a statement is not C4, a boundary or a quote is not closed, an identifier is not letters, digits and `_`, an identifier is declared twice, a relation names an unknown element, an unknown function |
| `c4-service-missing` | a service of the table that is not a container of the diagram; the identifier of a service is its key with `-` and `.` written as `_` (`booking-api` → `booking_api`), and an external container does not count |
| `arch-service-unknown` | a key of the table that is not in the catalog of Hammurapi; the message suggests the nearest key. Reported with `-hammurapi` only, for an arch that names its services by keys, and skipped with a note when no token of CI makes the catalog available |
| `tech-service-not-in-arch` | a service of tech that arch does not name |
| `contract-file-service` | a file `tech/<key>.openapi.json`, `.openapi.yaml`, `.openapi.yml` or `tech/<key>.proto` whose key is not a service of the feature |
| `contract-file-invalid` | a contract file that is not OpenAPI 3.x with top-level `paths`, or not proto3 with a service or a message, with balanced braces and closed strings and comments |

An arch of a fix (with `parent`) has neither the table of services nor the diagram, so it is not checked for
them; an arch of the contract 1.x, without the diagram, fails `c4-missing` (see «Library»).

The rules templates of the repository:

| Check | What it reports |
|---|---|
| `template-contract` | `rules/product/template.md` or `rules/arch/template.md` that lost an element of the contract: the line «Опирается на» / «Depends on», a requirement with criteria, the table of the section of changes, the decisions `A<n>`, the table of services, the C4 diagram; and any `rules/*/fix-template.md` without `parent` |

The tests of a feature in code (`-code <repo>`, repeatable):

| Check | What it reports |
|---|---|
| `qa-not-in-code` (warning) | an automated test case (a level other than `M`) whose ID is in no repository. An ID that is a test case of several features counts for a feature only in a file that names its key |

### `service` — a repository of a service

| Check | What it reports |
|---|---|
| `qa-not-in-code` (error) | an automated test case (a level other than `M`) of a required feature of the service whose ID is in no tracked file. A feature is required when its service PR is merged or it is released; in the check of the feature's own pull request it is required too |
| `unknown-test-id` | an ID that looks like a test case of the service — its prefix is the prefix of its cases, `BOOKING` for `BOOKING-01` — and is not one, or is not a case of the feature its file names |
| `version-minor` (warning) | speclint is older than the contract of Hammurapi |

An ID that is a test case of several required features counts for a feature only in a file that names that
feature's key; a feature that needs no tests does not share its IDs.

### A run that cannot check

Exit code `2`, no verdict: bad flags, no token (`token-invalid`), a revoked installation token
(`token-revoked`), a token of another repository (`token-scope`), a repository that is not in the catalog of
Hammurapi (`repo-not-in-catalog`), a service Hammurapi does not have (`service-not-found`), Hammurapi
unreachable after three attempts or an answer that does not parse (`hammurapi-unreachable`), and another
major version of the contract (`version-major`).

## GitHub Actions

```yaml
# a repository of specifications; with hammurapi-url (and id-token: write) the keys of services in arch
# are checked against the catalog of Hammurapi
- uses: GreenOnGrey/speclint@v2.3.0
  with: { mode: spec, hammurapi-url: https://hammurapi.example }

# a repository of a service: its tests against the test cases of its features in Hammurapi
- uses: GreenOnGrey/speclint@v2.3.0
  with:
    mode: service
    hammurapi-url: https://hammurapi.example
    service: booking
# with permissions: { contents: read, id-token: write } — the job presents its OIDC token, no secret
```

Errors become annotations on the lines of the pull request; the counts go to the job summary.
The mode `service` needs no secret: the job presents its OIDC token. Without OIDC (GitHub Enterprise
Server without it, other CI), set the installation token of Hammurapi in the environment of the step:
`env: { HAMMURAPI_TOKEN: ${{ secrets.HAMMURAPI_TOKEN }} }`. Pin the
action by a version tag: it runs the image of that version. Hammurapi proposes this file to a
repository itself and keeps its version on the contract of Hammurapi.

## GitLab CI

```yaml
include:
  - remote: https://raw.githubusercontent.com/GreenOnGrey/speclint/v2.3.0/templates/speclint.gitlab-ci.yml
variables:
  SPECLINT_MODE: service            # or spec
  SPECLINT_VERSION: "2.3.0"
  HAMMURAPI_URL: https://hammurapi.example
  HAMMURAPI_SERVICE: booking        # the job presents its OIDC token (id_tokens), no secret
```

Errors go to the Code Quality widget of the merge request. Without access to the internet, copy the
template into the repository and set `SPECLINT_IMAGE` to a mirror of the image.

## Other CI

The image `ghcr.io/greenongrey/speclint:<version>` (linux/amd64, linux/arm64) and the binaries of
the [releases](https://github.com/GreenOnGrey/speclint/releases) for Linux, macOS and Windows:

```sh
speclint spec    -root . -baseline speclint-baseline.txt [-errors-only] [-format text|github|gitlab] [-code <repo>]... [-hammurapi <url>]
speclint service -hammurapi <url> -service <key> -root . -baseline speclint-baseline.txt [-format …]
                 # the token: HAMMURAPI_TOKEN (installation), the OIDC token of GitHub Actions, or HAMMURAPI_ID_TOKEN (GitLab)
speclint version
```

- `-baseline` — known errors that do not fail the run; the file only shrinks: a fixed error left in it fails the run.
- `-write-baseline` — accept the current errors.
- `-errors-only` — print only new errors and the summary. No flag turns a check off.
- `-hammurapi <url>` in `spec` — check the keys of services of arch against the catalog of Hammurapi; without a token
  of CI the check is skipped with a note.
- `-code <repo>` — also look for the IDs of test cases in a code repository (repeatable).
- Exit codes: `0` no new errors, `1` new errors or stale lines of the baseline, `2` the run could not check
  (flags, network, token, version).

The mode `service` asks Hammurapi for the test cases of the features of the service and fails on an
automated test case of a required feature without its ID in the code (`qa-not-in-code`), and on an ID
that looks like a test case of the service and is not one (`unknown-test-id`). A feature is required
when its service PR is merged or the feature is released; while its PR is open, the feature is required
only in the check of that PR — the action passes `GITHUB_HEAD_REF` and the head commit of the pull
request, and a GitLab job its source branch and `CI_COMMIT_SHA`. Nothing is merged to the default
branch during development (`FTR.HMR.CMN-0002` §4), so a feature in code generation or validation does
not make an unrelated pull request red. Its major version must match the contract of Hammurapi.

Verify a release:

```sh
cosign verify ghcr.io/greenongrey/speclint:2.3.0 \
  --certificate-identity https://github.com/GreenOnGrey/speclint/.github/workflows/release.yml@refs/tags/v2.3.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`speclint migrate-arch -root .` numbers the decisions of `arch` documents `A<n>`.

## Library

```go
import "github.com/GreenOnGrey/speclint/v2/contract"

d := contract.Parse(doc, contract.Product) // DependsOn, Items, Changes, Notes, Violations
v := contract.CheckTemplate(contract.Arch, contract.FileTemplate, tpl)
a := contract.Parse(arch, contract.Arch)    // also Services and C4
f := contract.CheckArch(a)                  // arch-services-missing, c4-missing, c4-invalid, c4-service-missing
```

Version 2 adds to arch the table of services by the keys of the catalog and the section «Схема C4» / «C4 diagram»
(FTR.HMR.SPEC-0004): an arch of version 1 fails `c4-missing`; run `speclint spec -write-baseline` once to accept the
existing documents.

`starter.Files(contract.EN)` returns the starter rules templates; `starter.CIFile` the file of the
check that Hammurapi proposes to a repository, `starter.CIVersion` and `starter.SetCIVersion` read and
bump its version line.

## License

Apache 2.0.
