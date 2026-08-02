# Security status

This refactor is source-complete for the documented NPS behavior, but the current
Go 1.20.14 validation toolchain is no longer suitable for a production release.

An audit on 2026-08-02 used `govulncheck v1.0.4` against the NPS server, Web UI,
and core packages. The directly reachable findings in `github.com/ulikunitz/xz`
were remediated by upgrading it to `v0.5.15`. The scan still reports 46 reachable
vulnerabilities in the Go 1.20.14 standard library. The old `golang.org/x/net`
dependency must also be modernized as part of the toolchain upgrade.

Do not deploy the sample credentials in `conf/nps.conf`. Replace all Web and API
credentials, restrict the management interface to a trusted network, and use TLS
at the deployment boundary. The legacy timestamped MD5 API authentication and
the Web UI's lack of explicit CSRF protection remain security debt.

Production release is blocked until the service is built and verified with a
supported Go toolchain and the dependency vulnerability scan is reviewed again.
