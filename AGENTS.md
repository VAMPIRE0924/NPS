# NPS Refactor Workspace Handoff

This workspace is being used to refactor NPS as the backend for a future custom
equipment management platform.

Directory layout:

- `source/nps/`: freshly cloned upstream NPS source. Keep this clean as the
  reference copy.
- `workspace/nps-dev/`: development clone. Make source-code changes here.
- `conf/`: the user's current normalized NPS configuration. Do not overwrite
  backups unless explicitly asked.
- `workspace/`: development workspace. Keep generated checks and temporary
  artifacts outside `workspace/nps-dev/` when possible.
- `AGENTS.md`: this handoff file.
- `DEVELOPMENT_DESIGN.md`: agreed design.
- `DEVELOPMENT_MANUAL.md`: implementation manual.
- `代码说明.md`: source-code change map and current implementation notes.

Before changing source code, read these root-level documents in order:

1. `DEVELOPMENT_DESIGN.md`
2. `DEVELOPMENT_MANUAL.md`
3. `代码说明.md`

Current agreed direction:

- Use independent ID pools for every functional surface.
- `Client.Id` is an independent pool.
- `Host.Id` is an independent pool.
- Tunnel IDs are independent per user-facing mode, not shared globally.
- TCP and UDP are not separate user-facing tunnel pools anymore. They are one
  `portForward` pool: one port-forward rule listens on both TCP and UDP.
- Supported user-facing tunnel pools include `portForward`, `socks5`,
  `httpProxy`, `secret`, `p2p`, and `file`.
- Every new object must use the smallest available positive integer in its own
  pool.
- Allocation must happen only when the object is actually saved successfully.
- Creation paths must ignore caller-supplied positive IDs and allocate from the
  current pool after validation.
- Hidden internal clients (`NoStore && NoDisplay`, such as public-vkey clients)
  use negative internal IDs and must not consume visible client IDs.
- SOCKS is a managed client-bound tunnel:
  - `socks5.Id == Client.Id`
  - `socks5.Client.Id == Client.Id`
  - `socks5.Port == 10000 + Client.Id`
  - `socks5.Remark == Client.Remark`
  - Managed SOCKS is created closed by default.
  - Running managed SOCKS auto-stops after 30 minutes without flow changes.
  - The SOCKS page is configuration read-only. It may start/stop managed SOCKS,
    but must not allow editing, deletion, or manual creation of managed SOCKS
    tunnels.
- Do not revive the old global `TaskIncreaseId` behavior.
- Do not implement a partial "global task ID plus display ID" workaround unless
  the user explicitly changes direction. The agreed direction is composite task
  keys by mode and ID.
- The hard compatibility boundary is NPC, not the old web API. Existing NPC
  binaries and configs must not need to be rebuilt or rewritten. Server-side
  input with `mode=tcp` or `mode=udp` is canonicalized into `portForward`, and
  runtime traffic still uses the unchanged NPC `CONN_TCP` / `CONN_UDP`
  protocol.
- The web/API surface may be refactored for the future management platform.
  Do not let old API compatibility reintroduce global task IDs or a separate
  UDP task pool.
- Release/deployment artifacts are NPS-only. Existing original or third-party
  NPC binaries remain compatible and do not need to be rebuilt or replaced.

Important implementation note:

NPS source currently stores all tunnels in `JsonDb.Tasks` with a single integer
`Task.Id` key. The refactor must change the task map key to a composite key,
for example `mode:id`, so that `portForward:1`, `socks5:1`, and
`httpProxy:1` can coexist. `tcp` and `udp` are canonicalized into
`portForward` at the server boundary so current NPC clients do not need to be
rebuilt.

API direction note:

Task APIs may be refactored as needed for the future management platform. New
code should pass the real mode (`portForward`, `socks5`, `httpProxy`, `secret`,
`p2p`, or `file`) when addressing tasks. Do not preserve a separate UDP task
surface. NPC compatibility remains mandatory.

Current implementation note:

The development clone has begun this refactor. The core changes are in
`workspace/nps-dev/lib/file/file.go`, `workspace/nps-dev/lib/file/db.go`,
`workspace/nps-dev/server/server.go`, `workspace/nps-dev/bridge/bridge.go`,
`workspace/nps-dev/lib/conn/conn.go`, `workspace/nps-dev/web/controllers/*`,
and `workspace/nps-dev/web/views/index/*`.

Loading an existing `tasks.json` now protects against duplicate composite task
keys inside the same pool: the later record is reassigned to that mode's
smallest available positive ID instead of being silently dropped.

Go is installed locally at `tools/go`; add `tools/go/bin` to PATH in older
shells before running Go commands. The core service/API package set has already
passed `gofmt`, `go test`, and NPS/service-package `go vet`. Full `go test ./...`
still has upstream/environment blockers in `gui/npc`, `lib/pmux`, and
`lib/config`; full `go vet ./...` also reports an upstream NPC/client unkeyed
literal outside the NPS-only modification scope. See
`DEVELOPMENT_MANUAL.md` for details.
