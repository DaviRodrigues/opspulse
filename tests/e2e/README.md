# OpsPulse End-to-End (E2E) Test Suite

This directory contains the 4-tier opaque-box E2E verification suite for **OpsPulse**, covering backend persistence/API/checker behavior, SDD documentation (`specs/`), frontend build & quality gates (`web/opspulse-frontend`), and repository-wide code quality rules.

## 4-Tier Opaque-Box Architecture

1. **Tier 1 — Feature Coverage**: Verifies each core requirement independently (SDD docs structure & OpenAPI spec, Go unit tests & binary builds, MongoDB repositories & REST/SSE/CORS/jitter endpoints, frontend `axios`/env/lint/build/R3 UI features, and R4 quality/dependency rules).
2. **Tier 2 — Boundary & Corner Cases**: Tests validation errors, invalid ObjectIDs, duplicate URLs (`409 Conflict`), empty/stub doc guards, hardcoded URL/credential/wildcard CORS guards, bundle hygiene, and `vite preview` asset serving.
3. **Tier 3 — Cross-Feature Combinations**: Exercises multi-module interactions such as OpenAPI ↔ Go router ↔ frontend route alignment, CORS origin pairing with `vite preview`, and backend target CRUD + monitoring loop + check-log persistence.
4. **Tier 4 — Real-World Application Scenarios**: Runs full end-to-end flows including cross-stack frontend build with sentinel `VITE_API_BASE_URL` served via `vite preview` and the full workspace production readiness gate (`specs/` + `go vet` + `go test -race` + `go build` + `oxlint` + `vite build`).

## Running the Suites

Run both the Node.js and Go E2E suites using the unified runner:

```bash
./tests/e2e/run_e2e.sh
```

Run a single track:

```bash
# Node.js SDD Docs, Frontend R3, Go build gates & Quality R4 suite
./tests/e2e/run_e2e.sh --frontend-docs-only

# Go Backend E2E suite only
./tests/e2e/run_e2e.sh --backend-only
```

Or invoke each runner directly:

```bash
node --test tests/e2e/docs_frontend_quality.test.mjs
PATH="/usr/local/go/bin:$PATH" go test -v -race -tags=e2e ./tests/e2e/...
```

## In-Process MongoDB Fake (`tests/e2e/mongofake`)

The Go E2E suite uses `tests/e2e/mongofake`, an in-process MongoDB wire-protocol fake speaking `OP_MSG` (`hello`, `ping`, `insert`, `find`, `update`, `delete`, `createIndexes`, `aggregate`, etc.) over an ephemeral loopback TCP socket. This lets the real `go.mongodb.org/mongo-driver/v2` client, `cmd/api`, and `cmd/opspulse` binaries run full opaque-box tests without requiring an external `mongod` daemon.

## Browser Testing Scope Note

Interactive browser DOM click/navigation tests (e.g., Playwright/Puppeteer) are **omitted** on this host because no Chrome, Chromium, Firefox, or Playwright browser binaries are installed locally. Frontend verification instead exercises `oxlint`, `tsc -b`, `vite build` with sentinel `VITE_*` environment injection into temporary `--outDir` directories, source/bundle no-hardcode assertions, R3 feature presence checks, and live `vite preview` HTTP asset serving.
