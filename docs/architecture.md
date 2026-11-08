# Architecture

This baseline describes the workflow, registered build workers and repository
verification configuration at `629528ea0e2dd7329eeb9296a7b2b1394c641c02`.
The dated reports below retain their own scope and verification evidence.
The current workflow contract is the [deployment-state amendment](lanes.md#amendment-deployment-state).
The older tables in [design.md](design.md) describe an earlier workflow.

## Module boundaries

The workflow requirements are documented in [lanes.md](lanes.md),
[api.md](api.md) and [traceability.md](traceability.md). The dependency
directions below are inferred conventions from the current imports, ports
and wiring, rather than a separately enforced package policy.

| Module | Responsibility and dependency direction |
| --- | --- |
| `internal/domain` | Workflow and completion predicates, graph algorithms, host lifecycle/facts, job targets and repository-target normalization. Keep these independent of services, transports and database implementations. |
| `internal/app` | Coordinate use cases, permissions, clocks and transactions through application-owned persistence/storage interfaces. Depends on domain and focused support packages, including auth primitives, clock, stats and secretbox; keep HTTP, MCP and SQLite implementations outside this layer. |
| `internal/store/sqlite` | Implement the application ports, SQL queries and migrations. Its dependency on `app` connects the adapter to interfaces owned by its consumer. |
| `internal/buildbundle` | Filesystem adapter for immutable opaque bundles, retained partial transfers and platform-specific upload locks. Satisfies the application `BundleStorage` interface through composition. |
| `internal/auth` | Browser/bearer identity services and machine-signature primitives. Machine identity does not inherit browser or agent-token authority. |
| `internal/httpapi`, `internal/mcp` | Translate incoming requests into service calls and results into transport representations. Preserve service authorization and domain transitions instead of writing workflow state directly. |
| `internal/install`, `agent/build_host/embed.go` | Render downloadable installers from fixed embedded templates and runtime bytes. Request values and deployment credentials do not enter the renderer. |
| `agent/build_host/build_host.py` | Separate network client: local identity/state, signing, facts, process containment, execution, assignment/result journals and scheduling. It neither imports Go services nor authorizes server operations. |
| `internal/stats` | Compute metrics from supplied tickets, history and repository metadata. Persistence loading belongs to `app`. |
| `cmd/ownership/wire.go` | Assemble concrete adapters, services and event delivery; perform bundle recovery before serving and own the reaper lifecycle. This composition root can depend on both sides of a port. |
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
- `Stage.IsCompleted()` means merge-level completion: `done`, `deploying`
  and `deployed`. Deployment can remain active without reopening dependencies
  or repeating merge-sequence allocation. Client `isCompletedStage` mirrors
  that display predicate; it does not authorize transitions.
- Reuse `NormalizeRepoBuildTargets` and `BuildJobTarget.Validate/Canonical`
  for normalized, independently owned capability lists. Repository declarations
  feed briefs; queue eligibility uses each submitted job's target, not a
  comparison against the repository's declared targets.
- `BuildJobService.Submit` reserves quota in one database transaction, streams
  outside it, then promotes the immutable bundle and publishes the job in a
  second transaction. Filesystem publication is not rolled back with SQL:
  retained orphans, digest verification and upload locks support recovery.
  `BundleStorage` is the application seam; the server does not unpack bundles.
- SQLite `eligibleQueue` serves both `Candidate` and `QueuedForHost`, keeping
  capability matching and `(created_at,id)` ordering together. Queue rows are
  eligible candidates, not promises of exclusive assignment to a host.
- `useVisiblePolling` serializes reads, pauses hidden-page polling and discards
  obsolete responses after mutations. Host settings and registered-runner
  presentation share it. Go and Python signing share deterministic vectors in
  `internal/auth/testdata/host_signature_vectors.json`; preserve that protocol
  contract when either implementation changes.

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

## Health check after PR #55..#63 — selected changes

The exact selected PRs are **#55, #57, #58, #59, #60, #61, #62 and #63**.
Review date: 2026-11-08. The source snapshot is
`629528ea0e2dd7329eeb9296a7b2b1394c641c02`. There is no verified earlier
checkpoint base. Each merge's first-parent tree diff was reconstructed
separately, using both `git diff --name-status <merge>^1 <merge>` and
`git diff <merge>^1 <merge>`; their union contains 221 paths.

| Public PR | First-parent merge | Change examined |
| --- | --- | --- |
| [#55](https://github.com/rbretschneider/ownership_board/pull/55) | `962a3729098a7026bee0ace84d9747dfe207eacc` | Enrollment, signed machine requests and heartbeat reports |
| [#57](https://github.com/rbretschneider/ownership_board/pull/57) | `3e28f5bc8754a3b587ebab37c3f418c6f8406370` | Repository deployment policy and completion accounting |
| [#58](https://github.com/rbretschneider/ownership_board/pull/58) | `e9e89dddc31852e22ab7878762e26f15d721e604` | Verification jobs, immutable uploads and assignment recovery |
| [#59](https://github.com/rbretschneider/ownership_board/pull/59) | `23be0de06462f1d307a73e9b8868dd26a4b11952` | Deployed lane and repository policy controls |
| [#60](https://github.com/rbretschneider/ownership_board/pull/60) | `cfd102e3121306b46e8d72ee060158551bc4512f` | Host management and registered-runner views |
| [#61](https://github.com/rbretschneider/ownership_board/pull/61) | `b0666427cf18a817446e646ec1c363c81093639e` | Portable worker, embedded installers and native verification harness |
| [#62](https://github.com/rbretschneider/ownership_board/pull/62) | `6fef53a3de0841e95760833c369eee2ecf755e42` | Explicit deployment request, claim and outcome reporting |
| [#63](https://github.com/rbretschneider/ownership_board/pull/63) | `629528ea0e2dd7329eeb9296a7b2b1394c641c02` | Repository verification targets and registry-backed compatibility |

PR #56 (`f5cf386`) adds the preceding architecture report and is excluded
from the selected changes. Its findings remain historical context. The
documentation branch was advanced to base `84567b9`, which also includes
PR #64. That later UI change is outside this review; snapshot versions were
used for the overlapping stage/activity helpers. The source-backed baseline
above and the conclusions here are bounded to the selected snapshot.

Merge messages, parent relationships and feature history support the mapping.
External work-link confirmation, additional private discussion and open-ticket
searches were unavailable. No external provenance or duplicate-search result
is asserted. Public PR links come from local merge messages and were not
network-verified. Direct dependencies were read to understand boundaries,
without treating unrelated modules as additional review targets.

### Ranked findings

No new finding met the evidence threshold for P1 (major correctness/security
exposure), P2 (concrete maintainability/regression risk) or P3 (specific
assurance/documentation weakness) in the inspected paths. There is therefore
no ranked remediation list. This is a bounded source review with focused
checks, not a security certification or proof that all interactions are tested.

### Rubric assessment

| Category | Evidence and conclusion |
| --- | --- |
| Duplication / DRY | Repository normalization delegates capability handling to `BuildJobTarget.Validate`; SQL `Candidate` delegates to `QueuedForHost` and its `eligibleQueue` predicate. Repeated checks in the HTTP decoder, domain validator and worker protect different trust boundaries. Go/Python signing is intentional interoperability with shared vectors. No additional duplicate business policy requiring extraction was confirmed. |
| SOLID | Application-owned `BuildHostRepository`, `BuildJobRepository` and `BundleStorage` ports keep storage replaceable at the use-case boundary. `BuildJobService` coordinates publication and recovery because filesystem and SQL failure states must be considered together. The worker separates `BoardClient`, `State`, `ProcessTree`, `JobRunner`, `FactsService` and `AgentLoop` within one distributable file. Embedded installer consumption makes a file split consequential; size alone is insufficient evidence for one. No substitution or responsibility defect was demonstrated. |
| Layering / dependency direction | New domain files use standard-library facilities. App's `internal/auth` imports use signature verification and `ErrUnauthorized`, rather than HTTP handlers or SQLite implementations. SQLite imports app to satisfy consumer-owned ports; `buildbundle` implements the storage port without importing services. Install rendering depends on embedded assets, and the composition root owns concrete wiring/recovery. These directions fit the existing conventions. |
| Dead code | Enrollment now reaches the previously staged identity path. `CompatibilityRunners`, the file reader and `LegacyRunnersStrip` remain reachable fallbacks; pending/revoked hosts do not suppress the file, while any approved registry host selects the registry representation. Migration downgrade paths and Windows-only containment have explicit platform/history roles. No safe deletion was established. |
| Test gaps | The inspected tests already cover enrollment and report rollback, live-upload recovery, stale attempts, concurrent capacity, deployment accounting across retries, settings rollback and stale polls. No additional concrete missing interaction was substantiated. Native Windows, macOS and full browser/runtime execution remain assurance limits, separately from source coverage. |

### Boundary traces and retained risks

**Enrollment and administration.** In
[`BuildHostService.Pair`](../internal/app/build_host_agent.go), identity
insertion and pairing-code consumption share `Store.Write`. Notification
follows commit and can fail without undoing enrollment. Heartbeat writes
facts/telemetry and toolchain-change history together; SQLite's
[`RecordHeartbeat`](../internal/store/sqlite/build_hosts.go) does not overwrite
administrative capability, enablement or capacity columns. Signed routes in
[`router.go`](../internal/httpapi/router.go) validate the concrete path,
single-valued headers and bounded raw body independently of browser/bearer
identity. Pending hosts can read status only; application mutations recheck
approval in their transaction. `VerifyHostSignature` accepts timestamps within
300 seconds in either direction. This is a freshness window, not nonce-based
replay prevention; the review does not infer stronger delivery guarantees.

**Queue and storage.**
[`BuildJobService`](../internal/app/build_job_service.go) rechecks repository
assignment at reservation and publication. Promotion can leave an immutable
orphan after a failed SQL insert; retry verifies and adopts matching bytes.
Recovery preserves reservations held by live upload locks and rejects corrupt
referenced bundles. Claims serialize capacity and candidate selection; a
cancelled/superseded assignment occupies capacity until acknowledgment or
timeout releases it. Attempt fencing prevents old results overwriting retries;
offline recovery stops retrying after the third attempt. Publication order is
not Git ancestry. Quota includes retained partials/orphans: retention is a
capacity tradeoff, not evidence of unused files that can simply be removed.

**Worker and distribution.** The
[`worker`](../agent/build_host/build_host.py) verifies bundle length/digest,
fetches the requested object and checks the resulting commit before running
a preset. `ProcessTree` owns platform-specific cleanup. `AgentLoop` journals
assignments/results, persists an ambiguous-claim barrier and withholds control
traffic while an unknown claim ages out. Durable result bytes support retry;
they do not establish exactly-once network delivery. `resource_limits` bounds
parallelism and respects smaller environment limits. `install.Render` consumes
fixed embedded bytes; runtime configuration enters the local installer through
arguments. Source inspection of DACLs, Job Objects, launchd and scheduled tasks
does not establish native execution success on those platforms.

**Deployment and completion.**
[`RequestDeploy` and `ReportDeploy`](../internal/app/deploy_service.go) use
`ticketWriter.change`: a person queues an eligible merged ticket, and an agent
with its current lease reports the outcome. Failed deployment returns to
blocked Merged; retry needs another explicit request. A later policy change
does not prevent reporting already queued work. `Stage.IsCompleted` includes
active deployment, so retries neither reopen prerequisites nor repeat merge
effects. Client helpers mirror this merge-level classification. The
[deployment amendment](lanes.md#amendment-deployment-state) controls the
workflow; operating guidance and an agent's report do not independently prove
that an external deployment occurred.

**Targets and compatibility.**
[`NormalizeRepoBuildTargets`](../internal/domain/repo_build_targets.go)
canonicalizes capability sets and clones their storage. Repository edits and
[`UpdateSettings`](../internal/app/settings_service.go) use transaction-local
validation; Windows compatibility fields are projections of approved hosts,
not authoritative historical setting rows. Capacity distribution sorts hosts
by ID. [`CompatibilityRunners`](../internal/app/build_host_runners.go) exposes
bounded eligible queues, which can contain the same job for multiple hosts.
The actual claim remains exclusive. Repository targets are declarations used
in briefs; `validateJobRepo` checks repository ownership but does not enforce
those declarations as a submission allowlist. No enforcement guarantee should
be inferred from the configuration UI. Runner views use several `Store.Read`
queries without a snapshot transaction; `observed_at` is not a promise of an
atomic multi-query view. Per-host queue reads can grow with registered hosts;
no workload measurement here establishes a performance defect or justifies
changing the repository interface.

### Behavior-to-test assessment

Entries below describe inspected source assertions. Only the explicitly
listed commands in the next section were executed for this report.

| Behavior | Source evidence | Assessment |
| --- | --- | --- |
| Enrollment, approval and credential isolation | `TestEnrollmentConcurrencyAndRollback` checks competing consumption and rollback; `TestHeartbeatHistoryAndManagementIsolation` injects report failure and verifies event rollback and preserved admin settings in [build_host_agent_test.go](../internal/app/build_host_agent_test.go). `TestMachineHostFlow` checks pending/approved/revoked access, duplicate headers, tampering and browser/token separation in [handlers_build_host_agent_test.go](../internal/httpapi/handlers_build_host_agent_test.go). | Existing negative-path coverage addresses identity and column-ownership boundaries. |
| Signing | `TestHostSignature`, tampering tests and shared vectors in [host_signature_test.go](../internal/auth/host_signature_test.go); Python `ProtocolTests.test_shared_vectors` in [test_protocol.py](../agent/build_host/tests/test_protocol.py). | Shared deterministic bytes cover cross-language drift; the timestamp window is explicitly tested. |
| Storage publication and recovery | `TestBuildJobsPublicationRollbackRetainsOrphan`, `TestBuildJobsRecoveryPreservesLiveReservation`, quota/corruption tests in [build_job_service_test.go](../internal/app/build_job_service_test.go); cross-process upload-lock and symlink tests in [store_test.go](../internal/buildbundle/store_test.go). | SQL rollback, filesystem retention and live-uploader ownership are tested separately rather than assumed atomic. |
| Claims, cancellation and attempts | Concurrent-claim/capacity, eligibility, foreign-host, supersession/result-race and historical-log allocation tests in [build_job_service_test.go](../internal/app/build_job_service_test.go). `TestBuildQueueHTTP` checks route separation, malformed uploads and exact result limits in [handlers_build_jobs_test.go](../internal/httpapi/handlers_build_jobs_test.go). | Capacity and stale-report protections have concrete assertions; allocation coverage is a regression bound, not a benchmark. |
| Worker execution and restart | [test_scheduler.py](../agent/build_host/tests/test_scheduler.py) checks lost-claim responses across restarts, journal-write failures, exact completion retries and connectivity cutoff. [test_execution.py](../agent/build_host/tests/test_execution.py) checks real bundles, wrong SHAs, command exits and JSON budgets; [test_process_tree.py](../agent/build_host/tests/test_process_tree.py) checks descendants and controller death. | Execution and scheduling have independent seams. Platform-gated assertions require their native environments. |
| Installers and protocol integration | `TestRenderedInstallersEmbedCanonicalPayload` in [install_test.go](../internal/install/install_test.go), platform fixtures in [test_installers.py](../agent/build_host/tests/test_installers.py), and `TestBuildHostPythonInterop` / result-budget coverage in [build_host_python_test.go](../internal/httpapi/build_host_python_test.go). | Renderer tests prove payload identity, not installation. Interop requires `BUILD_HOST_PYTHON`; generic Go jobs can skip it. |
| Deployment and merge effects | `TestDeploymentCycleAndCompletionAccounting`, `TestDeployWritesRollbackTogether`, `TestDeploymentPreservesMergeEffectsAcrossAttempts`, request/report permission and claim-recovery tests in [deploy_service_test.go](../internal/app/deploy_service_test.go). | Existing tests include dependants, generated reviews, retries and injected timeline failures; a combined rollback gap is not inferred from the older report. |
| Deployment presentation | [stages.test.ts](../web/src/lib/stages.test.ts), [flow.test.ts](../web/src/lib/flow.test.ts), board layout tests and [RepoGateToggles.test.tsx](../web/src/components/settings/RepoGateToggles.test.tsx). | Selected-snapshot tests check completion helpers, lane layout and policy save/refusal behavior. Later PR #64 UI assertions are outside scope. |
| Repository targets and compatibility | [repo_build_targets_test.go](../internal/domain/repo_build_targets_test.go) checks normalization, limits and independent storage; repository service tests inject combined-edit failures. [settings_hosts_test.go](../internal/app/settings_hosts_test.go) checks distribution/rollback and historical-setting nonauthority; [runners_registry_test.go](../internal/httpapi/runners_registry_test.go) checks fallback, redaction and database-error propagation. | Existing tests distinguish omitted/empty target edits, registry policy and compatibility presentation. |
| Host presentation and polling | `TestCompatibilityRunnersQueuesAndHeartbeat` in [build_host_runners_test.go](../internal/app/build_host_runners_test.go) checks bounded ordering, overlapping eligible queues and disabled hosts. [BuildHostsSection.test.tsx](../web/src/components/settings/BuildHostsSection.test.tsx) checks obsolete polls and mutation drafts; [useRunners.test.tsx](../web/src/hooks/useRunners.test.tsx) checks visibility, aging, serialization and effect replay. | These tests address stale client state without promising database snapshot consistency. |

### Follow-ups and verification

No new refactor or test-only follow-up is proposed or created from this scope.
The evidence supports keeping the existing ports, worker distribution model
and completion path. A broader migration or new abstraction is not recommended
without a demonstrated failure or measured cost. No open-ticket duplicate
search was performed; that limitation is not a claim that no related work exists.

Tool discovery ran before detailed inspection. `web/package.json` and
`.github/workflows/ci.yml` define Go formatting/vet/race tests, frontend
tests/build and worker unittest/interop jobs. No configured duplication or
complexity detector or separate lint command was found. `command -v` found
none of `golangci-lint`, `gocyclo`, `gocognit`, `dupl`, `jscpd`, `eslint`,
`staticcheck`, `ruff`, `pylint` or `radon`; none was installed.

Executed on Linux/amd64 with Go 1.27.3:

- `/usr/local/go/bin/gofmt -l cmd internal agent/build_host`: no output.
- `GOTOOLCHAIN=local GOMAXPROCS=3 /usr/local/go/bin/go vet -p=3 ./...`:
  passed with no diagnostics. Both checks preceded the base fast-forward;
  that update changed only frontend files, so their Go inputs are unchanged.
- The following focused command passed the eleven selected application tests.
  The other three packages reported **no tests to run** for this expression:

  ```sh
  GOTOOLCHAIN=local GOMAXPROCS=3 /usr/local/go/bin/go test -p=3 \
    ./internal/app ./internal/auth ./internal/store/sqlite ./internal/install \
    -run '^(TestEnrollmentConcurrencyAndRollback|TestHeartbeatHistoryAndManagementIsolation|TestBuildJobsPublicationRollbackRetainsOrphan|TestBuildJobsRecoveryPreservesLiveReservation|TestBuildJobsSupersessionAndResultRace|TestDeploymentCycleAndCompletionAccounting|TestDeployWritesRollbackTogether|TestDeploymentPreservesMergeEffectsAcrossAttempts|TestWindowsSettingsDistributeAndRollback|TestWindowsHistoricalSettingsAreNotAuthoritative|TestCompatibilityRunnersQueuesAndHeartbeat)$' \
    -count=1
  ```

- Signature and embedded-payload checks also passed:

  ```sh
  GOTOOLCHAIN=local GOMAXPROCS=3 /usr/local/go/bin/go test -p=3 \
    ./internal/auth ./internal/install \
    -run '^(TestHostSignature|TestHostSignatureProtocolVector|TestHostSignatureTampering|TestHostSignatureSharedVectors|TestRenderedInstallersEmbedCanonicalPayload)$' \
    -count=1
  ```

`web/node_modules` was absent. `python3 -c 'import nacl; print(nacl.__version__)'`
failed with `ModuleNotFoundError`; no dependencies were installed. Frontend,
Python/interop, full Go/race, native Windows/macOS, hardware and live-network
suites were not run for this documentation change. The checked-in
`.ownership/windows_verify.ps1 -MaxJobs 2` remains available for native worker
verification; its presence is not a passing result.

Documentation validation checked the eight merge/parent references, selected
path union, local file links and named symbols, preserved the previous report,
reviewed additions for public-safe content, and ran `git diff --check`. The
authored change is limited to `docs/architecture.md`. Earlier verification
entries above belong to the preceding report and are not current execution
claims.
