# Security status

This release is built with Go 1.26.6 and current `golang.org/x/net`,
`golang.org/x/crypto`, and `golang.org/x/sys` dependencies.

An audit on 2026-08-17 used `govulncheck v1.7.0` against the NPS server, Web UI,
and core packages and found zero reachable vulnerabilities. The scanner retains
a module-level advisory for the unmaintained `x/crypto/openpgp` package, which
this NPS server does not import or call.

The sample configuration contains a deliberately invalid Web password placeholder.
NPS refuses to start the Web manager until it is replaced with a unique password
of at least 12 characters. API authentication now uses HMAC-SHA256 with a body
digest, timestamp window, and nonce replay protection. Browser mutations require
POST plus CSRF validation, and successful login rotates the session identifier.

Management API authentication is administrator-only. A client Web session is
always scoped to its session Client ID and cannot select another client through
request parameters. Clients retain VerifyKey Web login and their compatible NPS
features, while dedicated NPS listener-port management remains administrator-only.

Configuration credentials are encrypted at rest with AES-256-GCM. NPS creates
`conf/credential.key` on first start and automatically migrates plaintext fields
in `nps.conf`, `clients.json`, `tasks.json`, and `hosts.json`. Back up or migrate
the complete `conf/` directory; encrypted data without its matching key fails
closed. This protects individual files and ordinary backups from plaintext
disclosure, but it does not protect against an attacker who can read the entire
runtime directory, so filesystem and backup encryption remain required.

Restrict the management interface to a trusted network, enable TLS, keep the API
secret separate from the Web password, and rotate both credentials during upgrade.
The legacy NPC handshake remains wire-compatible and therefore retains its MD5-
derived authentication behavior; protect the Bridge listener with network controls.
