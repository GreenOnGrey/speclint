# AGENTS.md — speclint

Guide for coding agents and developers working in this repository.

## What it is

The document contract of Hammurapi and the linter of specification repositories
(`FTR.HMR.DEV-0003`, `FTR.HMR.SPEC-0002` in `GreenOnGrey/hammurapi-specs`).

- `contract/` — what Hammurapi reads in every document: «Опирается на» / «Depends on», requirements
  `R<n>`, decisions `A<n>` of arch, the section «Изменения в других решениях» / «Changes to other
  features», the notes «Изменено в» / «Changed in», `parent` of a fix. Read in Russian and English,
  written in one of them. `hammurapi-core` imports this package: the indexer and the linter of one
  version read a document the same way.
- `starter/` — the starter rules templates in `ru` and `en`; a test keeps them within the contract.
- `cmd/speclint/` — the CLI: `speclint spec` (default), `speclint service` (the tests of a service against
  Hammurapi, FTR.HMR.SPEC-0002), `speclint migrate-arch`, `speclint version`; `-format text|github|gitlab`.
- `starter/ci/` — the files of the check that Hammurapi proposes to repositories; `action.yml`,
  `templates/speclint.gitlab-ci.yml`, `Dockerfile` and `.github/workflows/release.yml` deliver a version.

## Releases

A tag `v<contract.Version>` builds the binaries, the public image `ghcr.io/greenongrey/speclint:<version>`
and signs both (cosign keyless). Before the tag: `contract.Version` and `SPECLINT_VERSION` of
`templates/speclint.gitlab-ci.yml` name the version (the release checks both), and so do the examples of
the README.

## Rules

- The version of the module is the version of the contract (`contract.Version`). A change that makes
  a document that passed fail it is a new major version.
- Russian stays only in the words of the contract and in test data that exercises Russian text;
  code, comments and messages are in English.
- No dependencies outside the standard library.
- Do not commit, tag or push unless asked. A tag is forever in the Go module proxy.

## Checks before a commit

```sh
test -z "$(gofmt -l .)" && go vet ./... && go test ./...
```
