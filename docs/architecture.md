# Architecture

This baseline describes the ticket workflow and the repository-review and
build-host features at `38c3a173fb022ea214d3ec76189f3392066a93e8`.
The current workflow contract is the [three-lane amendment](lanes.md#amendment-three-lanes).
The older tables in [design.md](design.md) describe an earlier workflow.

## Module boundaries

The workflow requirements are documented in [lanes.md](lanes.md),
[api.md](api.md) and [traceability.md](traceability.md). The dependency
directions below are inferred conventions from the current imports, ports
and wiring, rather than a separately enforced package policy.

| Module | Responsibility and dependency direction |
| --- | --- |
| `internal/domain` | Ticket state, validation, graph algorithms and host lifecycle. Keep these independent of services, protocols and database implementations. |
| `internal/app` | Coordinate use cases, permissions and transactions through application-owned persistence interfaces. Depends on domain and focused support packages, including clock, stats and secretbox; keep HTTP, MCP and SQLite implementations outside this layer. |
| `internal/store/sqlite` | Implement the application ports, SQL queries and migrations. Its dependency on `app` connects the adapter to interfaces owned by its consumer. |
| `internal/httpapi`, `internal/mcp` | Translate incoming requests into service calls and results into transport representations. Preserve service authorization and domain transitions instead of writing workflow state directly. |
| `internal/stats` | Compute metrics from supplied tickets, history and repository metadata. Persistence loading belongs to `app`. |
| `cmd/ownership/wire.go` | Assemble concrete adapters, services and event delivery. This composition root can depend on both sides of a port. |
| `web/src/api` | Client requests and transport types. The server decides workflow eligibility and permissions. |
| `web/src/lib`, feature components | Shared presentation rules and rendering. Reuse stage, relation, owner-alert, progress and PR-status helpers across views. |
| `agent/claude`, `docs/agents.md` | Client-side operating guidance. Prompts describe orchestration; they are not server authorization or executable enforcement of an external runner's behavior. |

### Existing mechanisms to reuse

- `app.Store.Write` supplies repositories bound to one transaction. SQLite
  uses `BEGIN IMMEDIATE` to serialize competing writers. `Store.Read` uses
  the connection pool without a transaction, so several reads do not promise
  a consistent snapshot ([db.go](../internal/store/sqlite/db.go)).
- `ticketWriter.change` checks an agent's lease for ordinary ticket writes;
  `apply` lets use cases with their own authorization participate in the same
  persistence/history path. `record` collects events, which callers publish
  only after the surrounding write succeeds
  ([ticket_writer.go](../internal/app/ticket_writer.go)).
- Domain workflow methods and `Stage.Lane()` define state transitions and
  lane membership. PR is a stage inside In Progress. A human reorder within
  that lane preserves the stage; an agent's completed-build handoff enters
  PR and clears the lease. The legacy agent input `review` is translated by
  `BoardService.Move`; it is not a lane accepted by filters or human moves.
- Dependency graph helpers provide cycle and capacity-stall checks. The
  application adds actor and repository checks inside the write transaction;
  SQL excludes unresolved prerequisites from claims. `WaitingOn` supplies
  the corresponding display data.
- Merge completion uses the same write transaction for sequence allocation,
  ticket/history updates, dependency-release history, generated review
  creation and checkpoint advancement. A completion sequence orders board
  events; it does not establish Git ancestry. A checkpoint commit work link
  is only a reference until its history is verified.
- Use the application clock and existing service fixtures for time-dependent
  behavior. Pairing-code plaintext uses `secretbox.Secret`; the issuance HTTP
  DTO is the explicit reveal point. Host management has its own service and
  post-commit, best-effort administrative logs, not ticket timeline events
  or a durable audit outbox ([registry amendment](amendments/118.md)).

## Health check after PR #9..#54 — selected changes

Review date: 2026-11-08. The exact selected PRs are **#9, #10, #46, #48,
#49, #50, #51, #52, #53 and #54**. This is not a review of every merge
between the endpoints. No earlier checkpoint commit was available as a
verified comparison base.

| Public PR | First-parent merge | Reviewed change |
| --- | --- | --- |
| [#9](https://github.com/rbretschneider/ownership_board/pull/9) | `5fdac8a` | Builder selection and fallback instructions |
| [#10](https://github.com/rbretschneider/ownership_board/pull/10) | `7fe15cf` | Dependencies and autonomous capacity |
| [#46](https://github.com/rbretschneider/ownership_board/pull/46) | `2190619` | Repository review interval and checkpoint fields |
| [#48](https://github.com/rbretschneider/ownership_board/pull/48) | `66c1f8e` | Automatic review scheduling on completion |
| [#49](https://github.com/rbretschneider/ownership_board/pull/49) | `65ab51a` | Review briefs and bounded related creation |
| [#50](https://github.com/rbretschneider/ownership_board/pull/50) | `a0726ed` | Review status in statistics and cards |
| [#51](https://github.com/rbretschneider/ownership_board/pull/51) | `b38aebd` | Build-host registry management |
| [#52](https://github.com/rbretschneider/ownership_board/pull/52) | `153c0fe` | Empty first-parent tree diff; provenance only |
| [#53](https://github.com/rbretschneider/ownership_board/pull/53) | `f6af549` | PR stage moved into In Progress |
| [#54](https://github.com/rbretschneider/ownership_board/pull/54) | `38c3a17` | Three-column board and PR presentation |

Each merge was resolved from local history and compared with its first
parent (`git diff <merge>^1 <merge>`). The union of changed paths bounded
the review; current implementations and their direct dependencies were
then traced for the behaviors below. Later selected changes supersede
earlier hunks, especially the lane migration. Local merge messages, parent
relationships and feature trailers support this reconstruction; they do not
authenticate external work tracking. Public PR URLs were derived from merge
messages and were not checked over the network.

### Ranked findings

No production correctness or security defect was confirmed in the inspected
changes. The findings below concern regression assurance and documentation;
P3 means low priority. Confidence describes the evidence, not the likelihood
of an undiscovered runtime failure.

1. **P3 — Missing combined rollback assertion (test coverage; high confidence).**
   In [`ticketWriter.apply`](../internal/app/ticket_writer.go), dependency
   release runs before review filing in the same transaction. The scheduling
   rollback test,
   `TestHealthCheckRollsBackMergeAndFilingTogether` in
   [health_check_test.go](../internal/app/health_check_test.go), injects
   generated-timeline and checkpoint failures and checks ticket state,
   sequence, generated tickets and emitted-event count. Its fixture has no
   dependant. The successful release tests in
   [dependency_test.go](../internal/app/dependency_test.go), including
   `TestMergeReportFreesDependantsAndNamesTheReporter` and
   `TestPersonMovingToMergedFreesDependants`, exercise the other half but
   do not inject a subsequent scheduling failure. Thus these tests would
   not detect dependency-release history accidentally surviving a failed
   filing after a future transaction-boundary refactor. Current shared
   transaction handling mitigates this; no escaped write was demonstrated.
   Extend the existing rollback fixture with a dependant, assert its history
   and waiting state are unchanged after failure, then assert one release
   entry and matching event after a successful retry. This needs only the
   application scheduling test file and existing fixtures.

2. **P3 — Conflicting workflow entry documentation (documentation drift;
   high confidence).** The Lanes and Stages tables and agent-move examples in
   [design.md](design.md#lanes) still introduce Review as a fourth lane.
   [lanes.md](lanes.md#amendment-three-lanes) explicitly supersedes that
   contract, matching `stageCatalog` in
   [stage.go](../internal/domain/stage.go) and `BoardService.Move`.
   Readers starting with the design overview can implement a filter or
   human drop target the current server rejects. Domain, service and MCP
   tests protect runtime behavior, but do not reconcile prose. Add a current
   contract notice to the design overview when updating workflow docs,
   preserving clearly marked historical material. This baseline links
   directly to the controlling amendment; it does not rewrite the older docs.

### Rubric assessment and retained boundaries

| Category | Evidence and conclusion within this scope |
| --- | --- |
| Duplication / DRY | Creation and linking share dependency insertion; related creation uses the existing creation and linking helpers in one transaction. The SQL claim predicate and domain graph validation serve different operations, and `WaitingOn` agrees on `stage != done`. Router and service human checks deliberately protect separate entry points. No actionable duplicate policy implementation was found. |
| SOLID | The merge writer now coordinates several completion effects, but those effects are already delegated to focused helpers and require atomicity. Finding 1 identifies a concrete test seam to strengthen; method size alone does not justify an event framework or service split. Registry ports widen `Repos`, but SQLite provides them through the existing unit-of-work implementation; no demonstrated interface-substitution defect or disproportionate mock burden was found. |
| Layering / dependency direction | Domain and stats imports stay independent of app and adapters. Services own persistence ports; HTTP and MCP invoke services; registry wiring sits in the composition root. Statistics load follow-up relations in the service rather than fetching inside the computation package. No violation of these dependency directions was found in scope. |
| Dead code | Retired Review-lane inputs were checked against schemas, current call paths, migration history and tests. The agent-only alias is supported compatibility, and the progress step named Review describes code review rather than a lane. Host draft/insertion and pairing usage metadata are explicitly staged protocol foundations; consumption and runner integration are not exposed by these PRs. No safe deletion was identified. |
| Tests | Existing suites cover many negative and concurrent paths, as detailed below. Finding 1 is a specific missing interaction assertion, not a claim that scheduling or dependencies lack tests. |

Two observations remain watch points, not verified architectural defects.
`BoardService.Stats` performs one relation lookup per repository with a latest
review, after loading all tickets; that is linear query growth, but no latency
or scale evidence here justifies a new repository API. Also, `Store.Read`
allows a report to span concurrent changes. Neither implies that pure
`repoHealth` should acquire persistence responsibilities. Revisit those
costs with workload evidence before introducing batching or snapshot reads.

### Behavior-to-test assessment

These references describe assertions inspected in source. Execution evidence
is listed separately; listing a test here does not imply the full suite ran.

| Behavior | Existing assertion evidence | Assessment |
| --- | --- | --- |
| Dependency graph, authorization and claims | `app/dependency_test.go` checks opposite concurrent links, cycle paths, creator/lease permissions, unlink ownership, repository changes, deletion and chain release; `app/autonomous_test.go` checks direct/transitive capacity stalls, blocked slots and expired-lease resumption. HTTP dependency tests exercise DTOs, permission errors and MCP linking. | Substantial layered coverage; no additional graph-policy gap confirmed. |
| Review cadence configuration | `app/repo_service_test.go` verifies human-only interval changes and sync preservation; SQLite migration tests verify defaults and checkpoint persistence; `RepoHealthCheckField.test.tsx` covers validation and save/error behavior. | Client and server validation have different responsibilities; no need to unify them through a new shared runtime. |
| Scheduling and generated reviews | `app/health_check_test.go` checks threshold/backlog, disabled and unassigned cases, concurrent merges, recompletion, repository isolation, sequence rollback, committed checkpoint visibility, provenance and large ranges. Follow-up tests check quota races, atomic linking and placement; `domain/health_check_test.go` checks generated text escaping. | Finding 1 is the remaining combined rollback assertion. A recorded SHA is correctly described as unverified ancestry. |
| Current review reporting | `stats/health_test.go` checks checkpoint counts, deterministic latest-review ties, disabled/idle/partial data and independence from the report period. `app/stats_service_test.go` checks direct incoming open follow-ups, ignoring reverse/indirect links and preserving state. API and `HealthTable.test.tsx` cover representation and display. | The pure calculation and service enrichment agree; no demonstrated reporting-policy divergence. |
| Registry identity and management | Domain tests check lifecycle and atomic edits; store tests cover migration round trips, revoked-key uniqueness, corrupt rows and report preservation; service tests check failed-write rollback, entropy errors and safe post-commit logs. HTTP tests reject agents, missing/foreign Origin, nulls, unknown fields and invalid types. | No consumption/replay test is expected for an unexposed enrollment operation. Issuance alone is not a working runner-pairing protocol. |
| PR stage and three-lane presentation | `domain/three_lanes_test.go`, service PR-submission guards, HTTP handoff cases and `mcp/three_lanes_test.go` cover catalogs and input contracts. SQLite migration tests compare every non-lane field across round trips. Frontend layout/card tests check the three-column catalog, PR pills, owner badges, stepper and stale/mismatched snapshot fallback. | Existing coverage addresses the changed stage predicate and common stale-state cases; no further scoped presentation gap confirmed. |
| Builder selection guidance | PR #9 changes prompt and documentation files. Current guidance chooses the builder from command input and describes resuming after fallback. | No executable runner implementation is part of this selected diff; prompt inspection cannot verify external process isolation or fallback execution. |

The recommended follow-up is the single focused scheduling rollback test in
finding 1. No broad refactor, new abstraction pattern or architecture migration
is supported by the evidence collected here. Documentation drift can be
resolved during the next workflow-documentation edit.

### Verification and limits

Static-tool discovery preceded detailed inspection. `web/package.json` and
`.github/workflows/ci.yml` configure frontend tests/build and Go formatting,
vet and race tests, but no duplication or complexity detector or separate
lint command. `command -v` found none of `golangci-lint`, `gocyclo`,
`gocognit`, `dupl`, `jscpd`, `eslint` or `staticcheck`; none was installed.
The frontend dependency directory was absent.

Checks performed on the reviewed source:

- `/usr/local/go/bin/gofmt -l cmd internal`: no output.
- `GOTOOLCHAIN=local GOMAXPROCS=3 /usr/local/go/bin/go vet -p=3 ./...`:
  passed with no diagnostics.
- The following focused, uncached test selection passed in all six packages:

  ```sh
  GOTOOLCHAIN=local GOMAXPROCS=3 /usr/local/go/bin/go test -p=3 \
    ./internal/domain ./internal/app ./internal/store/sqlite \
    ./internal/httpapi ./internal/mcp ./internal/stats \
    -run 'Test(Health|DependsOn|CreateWithDependsOn|CreateAutonomousWithDependsOn|Dependency|ConcurrentOpposite|ClaimSkips|MergeReportFrees|PersonMovingToMerged|WhoMayAdd|TheFilingAgent|AgentsRemove|RepoChangesCannotSplit|LinkRefused|DeletingAPrerequisite|AChainWaits|Autonomous|BuildHost|Pairing|ThreeLane|PRStage|LaneSchemas|StatsHealth|RelatedCreationRollsBack)' \
    -count=1
  ```

The installed toolchain was Go 1.27.3 on Linux/amd64; `go.mod` requests
Go 1.26.0. These checks did not run the complete Go suite, the race suite,
frontend tests/build, browser layout measurements, Windows verification or
hardware/network integration. CI defines the full race and frontend gates.
Manual inspection and focused passing checks do not prove the absence of
duplication, unreachable code or concurrency bugs. The documentation change
was also checked for whitespace errors and locally resolvable file/history
references; it contains no runtime changes.
