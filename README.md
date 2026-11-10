# speclint

The document contract of [Hammurapi](https://github.com/GreenOnGrey/hammurapi) and the linter of a
specification repository.

Hammurapi reads a few elements in every specification: the line «Depends on», requirements `R<n>`
with acceptance criteria, architecture decisions `A<n>`, the section «Changes to other features» and
the notes «Changed in». This module is that contract. Hammurapi's indexer and `speclint` use the same
code, so a pull request is checked exactly the way Hammurapi will read it. Russian and English
documents are both read.

## GitHub Actions

```yaml
# a repository of specifications
- uses: GreenOnGrey/speclint@v1.1.0
  with: { mode: spec }

# a repository of a service: its tests against the test cases of its features in Hammurapi
- uses: GreenOnGrey/speclint@v1.1.0
  with:
    mode: service
    hammurapi-url: https://hammurapi.example
    service: booking
    token: ${{ secrets.HAMMURAPI_SERVICE_TOKEN }}
```

Errors become annotations on the lines of the pull request; the counts go to the job summary. Pin the
action by a version tag: it runs the image of that version. Hammurapi proposes this file to a
repository itself and keeps its version on the contract of Hammurapi.

## GitLab CI

```yaml
include:
  - remote: https://raw.githubusercontent.com/GreenOnGrey/speclint/v1.1.0/templates/speclint.gitlab-ci.yml
variables:
  SPECLINT_MODE: service            # or spec
  SPECLINT_VERSION: "1.1.0"
  HAMMURAPI_URL: https://hammurapi.example
  HAMMURAPI_SERVICE: booking        # HAMMURAPI_SERVICE_TOKEN: a masked CI/CD variable
```

Errors go to the Code Quality widget of the merge request. Without access to the internet, copy the
template into the repository and set `SPECLINT_IMAGE` to a mirror of the image.

## Other CI

The image `ghcr.io/greenongrey/speclint:<version>` (linux/amd64, linux/arm64) and the binaries of
the [releases](https://github.com/GreenOnGrey/speclint/releases) for Linux, macOS and Windows:

```sh
speclint spec    -root . -baseline speclint-baseline.txt [-errors-only] [-format text|github|gitlab] [-code <repo>]...
speclint service -hammurapi <url> -service <key> -root . -baseline speclint-baseline.txt [-format …]
                 # the token in HAMMURAPI_SERVICE_TOKEN
speclint version
```

- `-baseline` — known errors that do not fail the run; the file only shrinks: a fixed error left in it fails the run.
- `-write-baseline` — accept the current errors.
- `-errors-only` — print only new errors and the summary. No flag turns a check off.
- `-code <repo>` — also look for the IDs of test cases in a code repository (repeatable).
- Exit codes: `0` no new errors, `1` new errors or stale lines of the baseline, `2` the run could not check
  (flags, network, token, version).

The mode `service` asks Hammurapi for the test cases of the features of the service and fails on an
automated test case of a feature in code generation or later without its ID in the code
(`qa-not-in-code`), and on an ID that looks like a test case of the service and is not one
(`unknown-test-id`). Its major version must match the contract of Hammurapi.

Verify a release:

```sh
cosign verify ghcr.io/greenongrey/speclint:1.1.0 \
  --certificate-identity https://github.com/GreenOnGrey/speclint/.github/workflows/release.yml@refs/tags/v1.1.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`speclint migrate-arch -root .` numbers the decisions of `arch` documents `A<n>`.

## Library

```go
import "github.com/GreenOnGrey/speclint/contract"

d := contract.Parse(doc, contract.Product) // DependsOn, Items, Changes, Notes, Violations
v := contract.CheckTemplate(contract.Arch, contract.FileTemplate, tpl)
```

`starter.Files(contract.EN)` returns the starter rules templates; `starter.CIFile` the file of the
check that Hammurapi proposes to a repository, `starter.CIVersion` and `starter.SetCIVersion` read and
bump its version line.

## License

Apache 2.0.
