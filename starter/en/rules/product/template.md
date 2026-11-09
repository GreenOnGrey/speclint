# Product specification: <feature title>

**Depends on:** <full keys of the features this one relies on, with the requirements in parentheses: `FTR.FMS.CAR-0001` (R3, R5)>

The size of the document matches the change: a small change has only the affected requirements, and «Users and roles» and «Success metrics» only if the change touches them. A change of a feature already handed off to development is a fix by `fix-template.md`.

If the feature changes requirements of other features, they are listed in the section «Changes to other features» (`FTR.HMR.DEV-0002`).

## Problem

Who suffers, how it shows, what confirms it.

## The solution in one phrase

## Users and roles

## Requirements

Every requirement has an ID `R<n>` (globally `<feature ID>-R<n>`, for example `FTR.FMS.CAR-0005-R3`) and acceptance criteria. A requirement inserted later between `R<n>` and `R<n+1>` gets a letter (`R26a`): the numbers of the others do not change. Refer to other features and their requirements by full key: this is how `spec_references` finds them. Split a long list into `###` subsections: a document is read by sections.

**R1.** <Requirement>

- Acceptance criteria:
  - Given <condition>, when <action>, then <result>.

## Scope

**In scope:**

**Out of scope:**

## Success metrics

The section is recommended: if there is no metric yet, for example no baseline or no real traffic, say why or delete the section.

| Metric | Source | Query | Target | Evaluated |
|---|---|---|---|---|
| <name> | <from Discovery, otherwise empty> | <from Discovery, otherwise empty> | <value> | <for example, 4 weeks after the release> |

## Changes to other features

The section is needed when the feature changes behaviour described in requirements of other features of the default branch; otherwise delete it. One row is one requirement: the full key of the feature, `R<n>` or «whole feature», what changed in one phrase; a cancelled requirement — «the requirement no longer applies». A feature still in work is changed by an assignment in its own thread, not here. Hammurapi puts the notes «*Changed in `<key of this feature>` R<n>: …*» into the changed features on save; write them by hand only when you edit a document in git past Hammurapi. Example keys go into the text, not into rows: every row of the table is applied.

| Feature | Requirement | What changed |
|---|---|---|

## Decisions

| # | Decision | Reason |
|---|---|---|
