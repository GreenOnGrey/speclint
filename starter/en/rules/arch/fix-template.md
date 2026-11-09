---
parent: FTR.FMS.CAR-0000  # ID of the parent feature, handed off to development
---

# Architecture — fix: <what changes>

## What changes

A link to the section of the parent specification and a short description of the change.

## Affected requirements

Requirements of the parent that change, by full key (`FTR.FMS.CAR-0000-R<n>`: this is how `spec_references` finds them), and new requirements of this fix (`R<n>`).

## Before / after

## Changes to other features

Rows about decisions of the parent and of other features of the default branch. The section is needed when the fix changes architecture decisions of other features of the default branch; otherwise delete it. One row is one decision: the full key of the feature, `A<n>` or «whole feature», what changed in one phrase; a cancelled decision — «the decision no longer applies». Architecture changes only architecture: a change of behaviour goes to the section of `product` of the same feature. A feature still in work is changed by an assignment in its own thread, not here. Hammurapi puts the notes «*A<n> — changed in `<key of this feature>` A<m>: …*» under the table of decisions of the changed feature on save. Example keys go into the text, not into rows: every row of the table is applied.

| Feature | Decision | What changed |
|---|---|---|
## Impact on other areas
