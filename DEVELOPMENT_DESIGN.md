# NPS Refactor: Independent ID Pools

## Goal

The future management platform will use NPS as its backend. The NPS data model
must therefore provide stable, predictable IDs that do not drift upward after
delete/add cycles.

Every functional area must have its own ID pool. New objects must take the
smallest available positive integer in that pool.

Example:

```text
existing IDs: 1, 2, 3, 4
delete:       2
next add:     2

existing IDs: 1, 2, 4, 8
next add:     3
next add:     5
```

## Current NPS Problem

Upstream NPS currently has only three effective ID pools:

```text
Client.Id        client pool
Host.Id          host/domain pool
Tunnel/Task.Id   one shared tunnel pool
```

The tunnel pool is shared by these pages and modes:

```text
Port forward (TCP + UDP together)
SOCKS proxy
HTTP proxy
Secret
P2P
File
```

Internally, they all use `Tunnel` records in `tasks.json` and are stored in
`JsonDb.Tasks`.

That shared pool is not acceptable for the new platform. The target model is:

```text
Client.Id      independent pool
Host.Id        independent pool
portForward.Id independent pool, one rule listens on both TCP and UDP
socks5.Id      independent pool, bound to Client.Id
httpProxy.Id   independent pool
secret.Id      independent pool
p2p.Id         independent pool
file.Id        independent pool
```

## Target Task Key Model

Change the internal task map key from:

```text
Tasks[Id]
```

to:

```text
Tasks[mode:id]
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

This allows every user-facing mode to have its own `1, 2, 3...` sequence
without collision. TCP and UDP are deliberately not separate pools; they are one
`portForward` mode.

Current NPC clients do not need to be rebuilt. If an existing NPC config sends
`mode=tcp` or `mode=udp`, NPS canonicalizes that input into `portForward` and
uses the unchanged client protocol: TCP traffic is still sent to NPC as
`CONN_TCP`, and UDP traffic is still sent to NPC as `CONN_UDP`.

The `Tunnel.Id` field remains the user-facing ID inside that mode. The unique
runtime/storage key is the composite task key.

## ID Allocation Rule

For each pool:

1. Read all existing objects in the same pool.
2. Collect positive IDs already used.
3. Return the smallest positive integer not in use.
4. Assign the ID only after validation has passed and the object is ready to be
   saved.

Do not consume IDs on failed validation, duplicate data, occupied ports, or
permission errors.

Creation APIs must not trust caller-supplied positive IDs. `NewClient`,
`NewHost`, and `NewTask` are creation boundaries: they validate the object and
then assign the smallest available ID from the current pool. This protects the
pool from stale IDs submitted by forms, APIs, temporary config payloads, or old
clients. JSON loading and explicit edit/update paths are the only places that
may preserve an existing ID.

Hidden internal clients such as `public_vkey` clients are not part of the
visible client ID pool. They use an internal negative ID pool so they do not
consume visible client IDs like `1, 2, 3...`.

## SOCKS Binding Rule

SOCKS proxy tunnels are managed by clients.

For every normal stored/displayed client:

```text
socks5.Id        = Client.Id
socks5.Client.Id = Client.Id
socks5.Port      = 10000 + Client.Id
socks5.Remark    = Client.Remark
```

Required behavior:

- When a client is created, create or synchronize its managed SOCKS tunnel.
- Newly created managed SOCKS tunnels must be closed by default.
- When a client remark is edited, synchronize the SOCKS remark.
- When a client is deleted, delete the managed SOCKS tunnel.
- The SOCKS page is configuration read-only, but start/stop is allowed.
- Managed SOCKS tunnels cannot be manually edited from the SOCKS page.
- Managed SOCKS tunnels cannot be manually created from the SOCKS page.
- Managed SOCKS tunnels cannot be manually deleted from the SOCKS page.
- Managed SOCKS start/stop state must be persisted and must not be reset by
  client remark synchronization.
- If an opened managed SOCKS tunnel has no inlet or export flow changes for
  30 minutes, the server must automatically stop it and persist `Status=false`.

## Scope Boundaries

Do not split `Client.Id` and `Host.Id` into composite keys. They are already
separate pools.

Do split task/tunnel modes. The key change applies to `JsonDb.Tasks`, task
lookup, task deletion, task update, run-state tracking, and API/web handlers
that operate on tasks.

Internal modes such as `webServer` and `httpHostServer` must be reviewed during
implementation. If they are not user-managed records, keep them internal and
avoid exposing them as a user-facing ID pool.

## Compatibility Expectations

Existing `tasks.json` records still contain `Id` and `Mode`. On load, build the
new composite key from those fields:

```text
TaskKey(task.Mode, task.Id)
```

When writing `tasks.json`, keep the existing record shape unless a separate
migration is explicitly chosen. The composite key does not need to be written
as a field if it can be derived from `Mode` and `Id`.

If duplicate records exist for the same `mode:id`, load must not silently
overwrite data. Prefer logging a clear error and skipping the later duplicate,
or failing startup with a precise message if that is safer for the chosen
implementation.

## API Direction

The future equipment management platform will call NPS APIs directly. The API
surface may be refactored where that keeps the backend cleaner. Do not keep
old API behavior if it forces global task IDs, duplicated TCP/UDP task pools, or
extra compatibility branches that conflict with the new model.

Required rules:

- The NPC client protocol must remain compatible with existing NPC binaries.
- Existing NPC config modes `tcp` and `udp` are accepted only as server-side
  input canonicalization into `portForward`; they are not stored as independent
  task modes.
- New management-platform code should pass `type=portForward` for port
  forwarding tasks.
- Do not keep or reintroduce a separate `udp` API/task pool.
- Keep `auth_key` behavior unless the future platform deliberately replaces
  the API authentication model.

This means independent pools are the internal storage model, while TCP/UDP
port forwarding is one coherent user-facing task.

## Implementation Notes

In `workspace/nps-dev`, the current implementation uses these concrete helpers:

```text
file.TaskKey(mode, id)
file.TaskMapKey(tunnel)
file.GetTaskMapKeys(...)
file.SocksPortByClientId(clientId)
```

Task CRUD now has mode-aware helpers such as `GetTaskByMode`,
`ResolveTask`, `UpdateTaskByModeId`, and `DelTaskByMode`. Numeric-only helpers
may exist for internal transition, but new code should not rely on them when a
task mode is known.
