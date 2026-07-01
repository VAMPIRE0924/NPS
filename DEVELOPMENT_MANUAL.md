# NPS Refactor Development Manual

This workspace refactors NPS as the backend for a future equipment management
platform. Development happens in `workspace/nps-dev`; keep `source/nps` as a
clean upstream reference.

## Current Rules

- Every visible object pool allocates the smallest available positive integer.
- `Client.Id` and `Host.Id` are independent pools.
- Tunnel IDs are independent per user-facing mode.
- TCP and UDP are one user-facing mode: `portForward`.
- One `portForward` task listens on both TCP and UDP on the same server port.
- Existing NPC binaries and config files do not need changes. Server-side
  `mode=tcp` and `mode=udp` inputs are stored as `portForward`; runtime traffic
  still uses NPC `CONN_TCP` and `CONN_UDP`.
- SOCKS is managed by clients:

```text
socks5.Id        = Client.Id
socks5.Client.Id = Client.Id
socks5.Port      = 10000 + Client.Id
socks5.Remark    = Client.Remark
```

- Managed SOCKS is created closed by default.
- Managed SOCKS can be started or stopped, but cannot be manually added,
  edited, or deleted from the SOCKS page.
- A running managed SOCKS task is stopped and persisted closed after 30 minutes
  without inlet/export flow changes.
- Hidden internal clients (`NoStore && NoDisplay`) use negative IDs and do not
  consume visible client IDs.
- Client Basic auth username/password are server-managed. Web/API may set
  client `u` / `p`; NPS clears `basic_username` / `basic_password` when they
  are reported by NPC-side config during client registration, so original or
  third-party NPC binaries remain compatible but cannot override server-side
  Basic auth.
- New web/API code should pass `type` or `mode` with task IDs.
- Do not recreate a separate UDP task pool.

## Key Implementation Points

Task storage now uses composite string keys:

```text
file.TaskKey(mode, id)
file.TaskMapKey(tunnel)
RunList["mode:id"]
```

Examples:

```text
portForward:1
socks5:1
httpProxy:1
secret:1
p2p:1
file:1
```

Creation methods are the allocation boundary:

```text
DbUtils.NewClient
DbUtils.NewHost
DbUtils.NewTask
```

They ignore caller-supplied positive IDs and allocate after validation. Web
port-forward creation checks port availability before saving, and rolls back
the just-created task if server startup returns an immediate error.

## Files To Review First

```text
lib/file/file.go
lib/file/db.go
lib/file/obj.go
server/server.go
server/proxy/port_forward.go
server/proxy/tcp.go
server/proxy/udp.go
bridge/bridge.go
lib/conn/conn.go
lib/goroutine/pool.go
web/controllers/base.go
web/controllers/client.go
web/controllers/index.go
web/views/public/layout.html
web/views/index/list.html
web/views/index/add.html
web/views/index/edit.html
web/static/page/languages.xml
docs/webapi.md
```

## Web Deployment Rule

NPS does not embed `web/views` or `web/static` into the binary. Deployment
artifacts must include the full `web` directory beside `nps` / `nps.exe`.

Correct runtime shape:

```text
nps
conf/
web/
  static/
  views/
```

Copying only `nps` breaks template changes. Copying only `languages.xml` breaks
CSS/JS loading. Always deploy the full `web` directory from the matching build
output.

Build outputs:

```text
workspace/build/windows_amd64
workspace/build/linux_amd64
```

## Validation Commands

Use the bundled Go toolchain when needed:

```powershell
$repo = (Resolve-Path '.').Path
$env:Path = (Join-Path $repo 'tools\go\bin') + ';' + $env:Path
Set-Location (Join-Path $repo 'workspace\nps-dev')
```

Core validation:

```powershell
go test -v ./lib/file ./server ./server/proxy ./server/tool ./server/test ./web/controllers ./web/routers
```

Broad service validation:

```powershell
go test ./bridge ./client ./cmd/npc ./cmd/nps ./lib/cache ./lib/common ./lib/conn ./lib/crypt ./lib/daemon ./lib/file ./lib/goroutine ./lib/install ./lib/rate ./lib/sheap ./lib/version ./server ./server/connection ./server/proxy ./server/test ./server/tool ./web/controllers ./web/routers
```

Vet NPS/service packages. NPC source is intentionally excluded from this vet
command because the compatibility boundary is "do not change NPC":

```powershell
go vet ./bridge ./cmd/nps ./lib/cache ./lib/common ./lib/conn ./lib/crypt ./lib/daemon ./lib/file ./lib/goroutine ./lib/install ./lib/rate ./lib/sheap ./lib/version ./server ./server/connection ./server/proxy ./server/test ./server/tool ./web/controllers ./web/routers
```

Release build:

```powershell
$repo = (Resolve-Path '..\..').Path
go build -trimpath -ldflags='-s -w' -o (Join-Path $repo 'workspace\build\windows_amd64\nps.exe') ./cmd/nps

$env:GOOS='linux'
$env:GOARCH='amd64'
go build -trimpath -ldflags='-s -w' -o (Join-Path $repo 'workspace\build\linux_amd64\nps') ./cmd/nps
Remove-Item Env:\GOOS
Remove-Item Env:\GOARCH
```

Only the NPS server binary is a deployment artifact for this refactor. Existing
original or third-party NPC binaries remain compatible and do not need to be
rebuilt or replaced.

Default `go build` keeps symbol/debug metadata and produces much larger
binaries. Release artifacts should use `-trimpath -ldflags='-s -w'`.

After build, sync the full web directory:

```powershell
$repo = (Resolve-Path '..\..').Path
$targets = @(
  (Join-Path $repo 'workspace\build\windows_amd64'),
  (Join-Path $repo 'workspace\build\linux_amd64')
)
foreach ($target in $targets) {
  New-Item -ItemType Directory -Force -Path (Join-Path $target 'web') | Out-Null
  Copy-Item -Path web\static -Destination (Join-Path $target 'web') -Recurse -Force
  Copy-Item -Path web\views -Destination (Join-Path $target 'web') -Recurse -Force
}
```

## Current Audit Notes

- `TaskIncreaseId` / `GetTaskId` are not used by the refactored creation paths.
- `JsonDb.Tasks` and `server.RunList` use composite task keys for runtime and
  storage.
- When loading existing `tasks.json`, duplicate keys inside the same pool are
  not silently dropped; the later record is reassigned to the current mode's
  smallest available positive ID and persisted immediately.
- `GetTunnel` supports all-task listing when both `type` and `client_id` are
  empty.
- Login and layout templates quote `window.nps.web_base_url`, so empty
  `web_base_url` does not break JavaScript.
- `portForward` language keys are lower-case in `languages.xml`, matching the
  front-end language loader.
- The public header welcome text, help menu item, footer copyright, and footer
  read-more link have been removed from the shared layout.
- Manual task add/edit forms no longer expose `socks5` as a selectable mode;
  managed SOCKS can only be viewed and started/stopped from the SOCKS list.
- Client list verify keys are masked as `********` by default. The eye button
  reveals the key only in the current row; API response fields are unchanged.
- Binary-size audit showed no new dependency bloat: `go.mod` / `go.sum` are
  unchanged and the NPS dependency graph is unchanged versus upstream. The
  large size jump came from default unstripped `go build` output.
- Current release build sizes after `-trimpath -ldflags='-s -w'`:
  Windows `nps.exe` 13,979,648 bytes (13.33 MiB), Linux `nps` 13,553,826
  bytes (12.93 MiB).
- `server.AddTask` starts the flow snapshot goroutine only once and logs async
  startup errors without assuming every task has a client pointer.
- NPC-provided Basic auth credentials are invalidated before client records are
  saved. Server-side Web/API Basic auth settings remain valid.
- `go vet` service packages are clean after small upstream-quality fixes:
  keyed TCP/UDP address literals, no unreachable KCP listener return, and
  `WaitGroup.Add` before launching the HTTP proxy goroutine.

## Known Upstream/Environment Test Blockers

Full `go test ./...` can still hit upstream or environment issues:

- `gui/npc` can fail on old OpenGL/Fyne build constraints.
- `lib/pmux` can fail or panic in local environment tests around port `8888`
  and nil/invalid local connections.
- `lib/config` has old test fixtures using legacy keys.
- Full `go vet ./...` can also report an upstream NPC/client unkeyed
  `net.TCPAddr` literal. That file is outside the NPS-only modification scope.
- `go test -race` requires cgo and a local C compiler; this Windows workspace
  currently has `CGO_ENABLED=0` by default and no `gcc` in PATH.

Use the broad service validation package list above for this refactor unless
you are intentionally working on those upstream test areas.

## Do Not Do

- Do not restore global task ID allocation.
- Do not use `max(id) + 1`.
- Do not split `portForward` back into independent TCP and UDP task pools.
- Do not change the NPC wire protocol.
- Do not allow managed SOCKS manual add/edit/delete.
- Do not deploy only the binary when templates or static files changed.
