# Technical specification: <feature title>

## Changes by service

Only the services of the table «Affected services and contracts» of the architecture; when the gate of the architecture is skipped, this table names the services of the feature itself.

| Service | Repository | What changes | Requirements |
|---|---|---|---|
| `booking` | `org/booking-svc` | <briefly> | R1, R2 |

## API

A contract of a service can go next to the document as a file: `tech/<service key>.openapi.yaml` (OpenAPI 3) or `tech/<service key>.proto` (proto3). The key is a service of the table of the architecture. Hammurapi shows the contract at the gate and passes it to code generation.

Endpoints, request bodies, error codes, access rights. Paths and names literally: the search finds the specification by them.

## Database schema

DDL, indexes, key queries.

## Decisions

| # | Decision | Reason |
|---|---|---|

## Open questions
