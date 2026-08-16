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

Restrict the management interface to a trusted network, enable TLS, keep the API
secret separate from the Web password, and rotate both credentials during upgrade.
