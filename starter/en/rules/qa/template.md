# Test specification: <feature title>

## Principles and levels of tests

U — unit, I — integration, E — end-to-end. The ID of a test case is in the name or tag of the automated test — this is how CI results are linked to the specification. If the same ID is a test case of another feature too, the test file names the key of its feature, for example with the comment `// FTR.HMR.DEV-0001 qa`: otherwise `speclint -code` counts the test for neither.

## Test cases

«Services» are keys of the table of services of `arch`, comma-separated: the case is needed in the code of these services only. An empty cell — every service of the feature.

| ID | Requirements | Services | Scenario | Expected result | Lvl |
|---|---|---|---|---|---|
| `AREA-01` | R1 | `<service>` | <scenario> | <result> | I |

## Decisions

| # | Decision | Reason |
|---|---|---|

## Open questions
