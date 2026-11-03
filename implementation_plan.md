# Implementation Plan: Matrix Frequency Integration & Widget

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. On commit steps, invoke the `git-committer` subagent.

**Goal:** Build a Matrix protocol integration and Element Web widget in Go (`mautrix-go`, pure-Go SQLite, HTMX, Excelize) for course frequency tracking and XLSX export, restricted to teachers based on Matrix room power levels ($\ge 50$).

**Architecture:** A standalone Go service that serves an embedded HTMX/Tailwind widget for Element Web and connects to Matrix (`matrix.dc.ufc.br` or local homeserver) via `mautrix-go`. Uses pure-Go SQLite (`modernc.org/sqlite` in WAL mode) for persistence and `excelize` for multi-tab end-of-semester exports. Teacher authorization is enforced via Matrix OpenID token exchange and `m.room.power_levels` inspection.

**Tech Stack:**
- Language: Go 1.27
- Database: SQLite (via `modernc.org/sqlite`, CGO-free, WAL mode)
- Matrix SDK: `maunium.net/go/mautrix`
- Spreadsheet: `github.com/xuri/excelize/v2`
- UI: Go `html/template` + HTMX + Tailwind CSS CDN (embedded with `//go:embed`)
- Container: Multi-stage Docker build targeting `FROM scratch` (non-root `10001:10001`)

---

## Global Constraints

- **Language & Runtime:** Go 1.27 with `CGO_ENABLED=0` static compilation.
- **Pure Go SQLite:** `modernc.org/sqlite` with WAL mode pragmas (`journal_mode=WAL`, `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout=5000`).
- **Code Style (RULE[user_global]):**
  - Functions: 4–20 lines (50 lines hard ceiling). Flat control flow, guard clauses, no deep nesting.
  - Files: under 500 lines (target 200–300).
  - Immutability: create new values/structs rather than mutating in place.
  - Error messages: include offending value and expected shape.
  - Tests: Single command `go test -v -race -coverprofile=coverage.out ./...` with $\ge 80\%$ coverage.
  - External I/O: mock external I/O with named fake classes (`FakeMatrixClient`), not inline stubs.
  - Commands: always prefix bash commands with `rtk`.
- **Security & Authorization:** Matrix power level $\ge 50$ required for all management operations. Non-teachers or power level $< 50$ receive `403 Forbidden` / read-only notification.

---

## Review Focus

1. **Unauthenticated or forged widget requests:** Element passes `$matrix_room_id` and OpenID token; backend must verify token against homeserver before checking power levels. A `DEV_MODE=true` toggle provides local bypassing only when explicitly configured.
2. **Special characters & accents in student names:** Portuguese names must sort A–Z with accent folding/normalization (e.g. `Érika` is sorted as `Erika`, before `Fabio` and after `Daniel`).
3. **Mid-semester roster changes:** Adding or removing students mid-semester must not corrupt previous class attendance records. Soft deletion (`active = false`) preserves historical attendance while excluding inactive students from future roll calls.
4. **Attendance percentage calculation against configured total periods:** Absent periods must be calculated against classes held, while overall progress is measured against `TotalPeriods`. Edge case where `TotalPeriods == 0` must return 0% and avoid division by zero.
5. **SQLite concurrency in container:** Non-root user `10001:10001` in `FROM scratch` must have write permissions to `/app/data` to create `.db`, `-wal`, and `-shm` files without runtime permission denied errors.

---

## Task Breakdown

### Task 1: Project Scaffolding & Go Module Initialization

**Files:**
- Create: `go.mod`
- Create: `Taskfile.yml`
- Test: `rtk go test ./...`

- [ ] **Step 1.1**: Initialize Go module `frequencia-matrix-integration` and add required dependencies:
  - `modernc.org/sqlite`
  - `maunium.net/go/mautrix`
  - `github.com/xuri/excelize/v2`
  - `golang.org/x/text` (for accent-insensitive / folded collation)
- [ ] **Step 1.2**: Create `Taskfile.yml` running `rtk go test -v -race -coverprofile=coverage.out ./...`.
- [ ] **Step 1.3**: Verify `go mod tidy` builds cleanly without errors.
- [ ] **Step 1.4**: Call `git-committer` subagent to commit scaffolding: `chore: initialize go module and dependencies`.

---

### Task 2: Domain Layer — Models, Validation & Scheduling Logic

**Files:**
- Create: `internal/domain/models.go`
- Create: `internal/domain/schedule.go`
- Test: `internal/domain/schedule_test.go`
- Test: `internal/domain/models_test.go`

- [ ] **Step 2.1**: Write unit tests in `internal/domain/schedule_test.go` for:
  - Date generation given `StartDate`, `Weekdays` (e.g. Tue/Thu), and `TotalPeriods`.
  - Student sorting A–Z across Graduation and Pos-Graduation groups with accent folding (e.g., `Érika` treated as `Erika`, sorting between `Daniel` and `Fabio`).
  - Attendance summary calculations (percentage, presences, absences, $\ge 75\%$ threshold) including zero-total guard.
- [ ] **Step 2.2**: Run `rtk go test ./internal/domain/...` to confirm tests fail (RED).
- [ ] **Step 2.3**: Implement `internal/domain/models.go`:
  - Enums: `StudentLevel` (`Graduation`, `PosGraduation`).
  - Structs: `CourseConfig`, `Student`, `AttendanceEntry`, `AttendanceSession`, `StudentSummary`.
  - Boundary validation functions (non-empty names, valid weekdays, positive period counts).
- [ ] **Step 2.4**: Implement `internal/domain/schedule.go`:
  - `CalculateClassDates(start time.Time, weekdays []time.Weekday, totalPeriods int) []time.Time`
  - `SortAndGroupStudents(students []Student) (grad []Student, posGrad []Student)` with accent folding normalization.
  - `CalculateStudentSummary(student Student, sessions []AttendanceSession, totalCoursePeriods int) StudentSummary`
- [ ] **Step 2.5**: Run `rtk go test -v -race -cover ./internal/domain/...` to confirm 100% pass (GREEN).
- [ ] **Step 2.6**: Call `git-committer` subagent to commit domain layer: `feat(domain): add course, student models, scheduling, and attendance math`.

---

### Task 3: Persistence Layer — SQLite Store & Migrations

**Files:**
- Create: `internal/store/migrations/001_initial.sql`
- Create: `internal/store/migrations.go`
- Create: `internal/store/sqlite.go`
- Test: `internal/store/sqlite_test.go`

- [ ] **Step 3.1**: Create DDL migration `internal/store/migrations/001_initial.sql`:
  - `schema_migrations` (`version INTEGER PRIMARY KEY, applied_at DATETIME`)
  - `courses` (`room_id TEXT PRIMARY KEY, course_name TEXT, weekdays_json TEXT, total_periods INTEGER, start_date TEXT, created_at DATETIME`)
  - `students` (`id TEXT PRIMARY KEY, room_id TEXT, full_name TEXT, registration_number TEXT, level TEXT, active BOOLEAN, created_at DATETIME`)
  - `attendance_sessions` (`id TEXT PRIMARY KEY, room_id TEXT, class_date TEXT, period_count INTEGER, created_at DATETIME, UNIQUE(room_id, class_date)`)
  - `attendance_entries` (`session_id TEXT, student_id TEXT, is_present BOOLEAN, PRIMARY KEY(session_id, student_id)`)
- [ ] **Step 3.2**: Write tests in `internal/store/sqlite_test.go` using in-memory SQLite:
  - Auto-running migrations on database init.
  - Upserting and fetching `CourseConfig`.
  - Adding, listing active, and deactivating students (preserving historical records).
  - Recording attendance sessions and reading back student attendance history.
- [ ] **Step 3.3**: Run `rtk go test ./internal/store/...` to confirm tests fail (RED).
- [ ] **Step 3.4**: Implement `internal/store/migrations.go` using `//go:embed migrations/*.sql`.
- [ ] **Step 3.5**: Implement `internal/store/sqlite.go`:
  - Open connection with WAL pragmas.
  - Implement CRUD methods adhering to single-responsibility and immutability rules.
- [ ] **Step 3.6**: Run `rtk go test -v -race -cover ./internal/store/...` to verify all store operations pass cleanly (GREEN).
- [ ] **Step 3.7**: Call `git-committer` subagent to commit persistence layer: `feat(store): implement pure-Go sqlite store with embedded migrations`.

---

### Task 4: Spreadsheet Exporter — Two-Sheet XLSX Generation

**Files:**
- Create: `internal/export/xlsx.go`
- Test: `internal/export/xlsx_test.go`

- [ ] **Step 4.1**: Write tests in `internal/export/xlsx_test.go`:
  - Export generates valid `.xlsx` bytes parseable by `excelize`.
  - Sheet `Graduacao` and Sheet `Pos-Graduacao` both exist.
  - Students are sorted alphabetically A–Z in both sheets.
  - Dynamic date columns (`DD/MM`) match session dates with `P` / `F`.
  - Summary columns (`Presenças`, `Faltas`, `% Frequência`, `Situação`) match calculated attendance.
- [ ] **Step 4.2**: Run `rtk go test ./internal/export/...` to confirm test failure (RED).
- [ ] **Step 4.3**: Implement `internal/export/xlsx.go`:
  - Define `ExportColumnConfig` for easy adaptability.
  - Build two sheets with styled headers, alternating row colors or clear borders, and auto-fitted column widths.
  - Populate data rows from domain summaries.
- [ ] **Step 4.4**: Run `rtk go test -v -race -cover ./internal/export/...` (GREEN).
- [ ] **Step 4.5**: Call `git-committer` subagent to commit export module: `feat(export): generate two-sheet xlsx with adaptable column configuration`.

---

### Task 5: Matrix Integration & Role Verification Gateway

**Files:**
- Create: `internal/matrix/gateway.go`
- Create: `internal/matrix/fake_client.go`
- Create: `internal/matrix/mautrix_client.go`
- Test: `internal/matrix/gateway_test.go`

- [ ] **Step 5.1**: Define `MatrixGateway` interface in `internal/matrix/gateway.go`:
  - `VerifyUser(ctx context.Context, roomID string, openIDToken string) (UserIdentity, error)`
  - `IsTeacher(ctx context.Context, roomID string, userID string) (bool, error)`
  - `SendNotice(ctx context.Context, roomID string, message string) error`
- [ ] **Step 5.2**: Implement `internal/matrix/fake_client.go` for tests and local development mode with configurable user power levels and in-memory message history.
- [ ] **Step 5.3**: Write tests in `internal/matrix/gateway_test.go` verifying role inference (power level $\ge 50$ is teacher, $< 50$ is student, unauthenticated returns error).
- [ ] **Step 5.4**: Implement `internal/matrix/mautrix_client.go`:
  - OpenID token exchange endpoint `/openid/userinfo` with homeserver.
  - Power level resolution via `m.room.power_levels` state event.
  - Message sending via `maunium.net/go/mautrix`.
- [ ] **Step 5.5**: Run `rtk go test -v -race -cover ./internal/matrix/...` (GREEN).
- [ ] **Step 5.6**: Call `git-committer` subagent to commit Matrix gateway: `feat(matrix): add matrix gateway with power level verification and fake client`.

---

### Task 6: Web UI & HTTP Handlers (HTMX + Go Templates)

**Files:**
- Create: `internal/web/templates/layout.html`
- Create: `internal/web/templates/setup.html`
- Create: `internal/web/templates/roster.html`
- Create: `internal/web/templates/attendance.html`
- Create: `internal/web/templates/summary.html`
- Create: `internal/web/templates/unauthorized.html`
- Create: `internal/web/handler.go`
- Test: `internal/web/handler_test.go`

- [ ] **Step 6.1**: Write HTTP handler tests in `internal/web/handler_test.go`:
  - Teacher (power level $\ge 50$) can view dashboard, update course setup, add/remove students, record attendance, and download XLSX.
  - Non-teacher (power level $< 50$) gets `403 Forbidden` / unauthorized view.
  - HTMX partials return proper HTML snippets for tab switches and dynamic updates.
- [ ] **Step 6.2**: Run `rtk go test ./internal/web/...` to confirm tests fail (RED).
- [ ] **Step 6.3**: Implement embedded HTML templates using HTMX and Tailwind CSS:
  - `layout.html`: Tabbed navigation (Setup, Roster, Chamada, Resumo).
  - `setup.html`: Course title, days of week checkboxes, total periods.
  - `roster.html`: Student list by level with quick add and remove buttons.
  - `attendance.html`: Date picker + student checklist + "Salvar Chamada" button.
  - `summary.html`: Summary table + "Baixar Planilha XLSX" link.
  - `unauthorized.html`: Informational page for students ("Acesso restrito ao professor").
- [ ] **Step 6.4**: Implement `internal/web/handler.go`:
  - Middleware for auth check using `MatrixGateway`.
  - HTTP routing for all actions and XLSX stream download.
- [ ] **Step 6.5**: Run `rtk go test -v -race -cover ./internal/web/...` (GREEN).
- [ ] **Step 6.6**: Call `git-committer` subagent to commit web layer: `feat(web): add htmx-driven widget handlers and embedded templates`.

---

### Task 7: Application Server Entrypoint (`cmd/server`)

**Files:**
- Create: `cmd/server/main.go`
- Create: `cmd/server/config.go`
- Test: `cmd/server/config_test.go`

- [ ] **Step 7.1**: Implement `cmd/server/config.go` with environment variable loading:
  - `PORT` (default `8080`)
  - `DB_PATH` (default `data/frequencia.db`)
  - `MATRIX_HOMESERVER_URL`
  - `MATRIX_USER_ID`, `MATRIX_ACCESS_TOKEN`
  - `DEV_MODE` (default `false`)
- [ ] **Step 7.2**: Write test for configuration loading and validation in `cmd/server/config_test.go`.
- [ ] **Step 7.3**: Implement `cmd/server/main.go`:
  - Wire SQLite store, run embedded migrations on startup.
  - Initialize Matrix client (or Fake client if `DEV_MODE=true`).
  - Wire HTTP server with graceful shutdown (`os.Interrupt`, `syscall.SIGTERM`).
- [ ] **Step 7.4**: Verify application builds with `rtk go build -o /dev/null ./cmd/server`.
- [ ] **Step 7.5**: Call `git-committer` subagent to commit server entrypoint: `feat(server): wire application entrypoint with config and graceful shutdown`.

---

### Task 8: Dockerization & Local Integration Stack

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `README.md`

- [ ] **Step 8.1**: Create `Dockerfile`:
  - Multi-stage build based on `golang:1.27-alpine` builder.
  - `CGO_ENABLED=0` static build.
  - Target `FROM scratch` with `USER 10001:10001` and pre-created `/app/data` directory for SQLite.
- [ ] **Step 8.2**: Create `docker-compose.yml`:
  - `frequencia-app`: The Go application.
  - `conduit`: Lightweight Matrix homeserver for full local testing.
  - `element-web`: Element web client with pre-configured homeserver.
- [ ] **Step 8.3**: Create comprehensive `README.md`:
  - Project architecture & features.
  - Local dev testing guide (standalone dev mode and docker compose stack).
  - Production deployment guide for `matrix.dc.ufc.br` (widget URL, permissions).
- [ ] **Step 8.4**: Call `git-committer` subagent to commit docker and docs: `chore(deploy): add scratch dockerfile, docker-compose local stack, and docs`.

---

### Task 9: Full Verification Gate & Test Coverage Audit

- [ ] **Step 9.1**: Run full test suite with coverage:
  ```bash
  rtk go test -v -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
  ```
  Verify total test coverage $\ge 80\%$.
- [ ] **Step 9.2**: Run linter:
  ```bash
  rtk golangci-lint run
  ```
  Ensure zero lint warnings.
- [ ] **Step 9.3**: Test local binary startup in dev mode:
  ```bash
  DEV_MODE=true DB_PATH=:memory: rtk go run ./cmd/server
  ```
- [ ] **Step 9.4**: Call `code-auditor` subagent to audit code quality, blast radius, and rule compliance.
- [ ] **Step 9.5**: Final commit via `git-committer` and walkthrough artifact generation.
