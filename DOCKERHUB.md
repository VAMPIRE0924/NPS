# NPS Server

NPS server image maintained at `vampirerune/nps`. It keeps existing NPC binaries and configuration
compatible while adding independent tunnel ID pools, managed SOCKS5 tunnels, tenant-isolated Web
sessions, encrypted credential storage, and hardened API authentication.

## Image tags

| Tag | Purpose |
| --- | --- |
| `main` | Latest accepted stable build from the protected `main` branch |
| `<VERSION>` | Immutable release channel, for example `2.0.0` |
| `dev` | Development build; use only for testing |

Supported platforms: `linux/amd64` and `linux/arm64`.

## Before starting

Download the public configuration template and replace the placeholder management password with a
unique password of at least 12 characters:

```bash
mkdir -p ./conf
curl -fsSL \
  https://raw.githubusercontent.com/VAMPIRE0924/NPS/main/conf/nps.conf \
  -o ./conf/nps.conf
chmod 600 ./conf/nps.conf
```

Keep `public_vkey` and `auth_key` empty unless those features are required. Never publish real keys,
passwords, certificates, persisted JSON data, or `credential.key`.

The first successful start creates `conf/credential.key` and rewrites credential fields in
`nps.conf`, `clients.json`, `tasks.json`, and `hosts.json` as `npsenc:v1:` ciphertext. The files stay
encrypted after NPS stops; Web, API, and NPC behavior still use the in-memory plaintext values.

## Docker Compose (Linux host network)

```yaml
services:
  nps:
    image: vampirerune/nps:main
    container_name: nps
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./conf:/nps/conf
```

```bash
docker compose up -d
docker compose logs -f nps
```

Host networking is recommended on Linux because NPS may listen on dynamically configured tunnel
ports. With the public template, the main ports are HTTP `80`, HTTPS `443`, Web `8080`, and NPC
bridge `8024`.

## Docker bridge network

If host networking is unavailable, publish every configured service and tunnel port explicitly:

```bash
docker run -d \
  --name nps \
  --restart unless-stopped \
  -p 80:80/tcp \
  -p 443:443/tcp \
  -p 8080:8080/tcp \
  -p 8024:8024/tcp \
  -v "$PWD/conf:/nps/conf" \
  vampirerune/nps:main
```

Add both TCP and UDP mappings for every port-forward rule when bridge networking is used.

## Upgrade

Back up the complete `conf/` directory, then pull and recreate the container:

```bash
tar -czf nps-conf-before-upgrade.tar.gz conf
```

Do not omit `credential.key`: encrypted configuration cannot be restored with a different key.
Then upgrade:

```bash
docker compose pull
docker compose up -d
```

For reproducible production deployment, pin a numbered version instead of `main`. NPS and its
matching `/nps/web` assets are shipped together in the image. Existing NPC installations do not need
to be replaced. Rolling back to a build that predates encrypted credential storage also requires the
complete pre-upgrade `conf/` backup.

## Links

- Source and documentation: https://github.com/VAMPIRE0924/NPS
- Releases and checksums: https://github.com/VAMPIRE0924/NPS/releases
- API authentication: https://github.com/VAMPIRE0924/NPS/blob/main/API.md
- Upgrade and rollback: https://github.com/VAMPIRE0924/NPS/blob/main/UPGRADING.md
- Changes: https://github.com/VAMPIRE0924/NPS/blob/main/CHANGELOG.md
