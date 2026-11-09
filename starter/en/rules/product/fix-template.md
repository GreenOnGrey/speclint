---
parent: FTR.FMS.CAR-0000  # ID of the parent feature, handed off to development
---

# Product specification — fix: <what changes>

## What changes

A link to the section of the parent specification and a short description of the change.

## Affected requirements

New requirements of this fix (`R<n>`). The requirements of the parent that change are listed once — as rows of the section «Changes to other features» below: by it Hammurapi puts the notes «*Changed in `<key of the fix>` R<n>: …*» at them. In the text name the requirements of the parent by full key (`FTR.FMS.CAR-0000-R<n>`): this is how `spec_references` finds them.

## Before / after

## Changes to other features

The section is needed when the feature changes behaviour described in requirements of other features of the default branch; otherwise delete it. One row is one requirement: the full key of the feature, `R<n>` or «whole feature», what changed in one phrase; a cancelled requirement — «the requirement no longer applies». A feature still in work is changed by an assignment in its own thread, not here. Hammurapi puts the notes «*Changed in `<key of this feature>` R<n>: …*» into the changed features on save; write them by hand only when you edit a document in git past Hammurapi. Example keys go into the text, not into rows: every row of the table is applied.

| Feature | Requirement | What changed |
|---|---|---|

## Impact on other areas
