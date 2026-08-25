# 升级与回滚

本文适用于升级到 `v2.0.2`。从 `v2.0.1` 升级不改变凭据密文格式；从 `v2.0.0` 或更早
版本升级时，首次启动会迁移 NPS 本地凭据的落盘格式。所有路径都不要求替换 NPC。

`v2.0.2` 会清理已废弃的每 Client Web 用户名/密码字段；客户端 Web 登录统一使用
`user` + Client VerifyKey。如果历史 Client 的 VerifyKey 为空，启动时会自动生成新密钥，部署后应
在 Client 列表中核对并更新对应 NPC 配置。

## 升级前

1. 记录当前 NPS 版本或镜像摘要。
2. 停止会修改配置的运维操作。
3. 备份完整 `conf/`，不要只备份三份 JSON。
4. 同时保留当前 NPS 二进制和完整 `web/`，用于回滚。

```bash
tar -C /path/to/nps -czf nps-conf-before-upgrade.tar.gz conf
```

生产配置、JSON、TLS 私钥和 API 密钥不得上传到 GitHub、工单或聊天记录。

## 升级

### Docker

镜像内已经包含匹配的 NPS 与 Web 资源；Compose 只挂载配置目录：

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
docker compose pull
docker compose up -d
docker compose logs --tail=200 nps
```

不要再用宿主机旧二进制或旧 `web/` 覆盖镜像内文件。

### 二进制包

同时替换 NPS 二进制和完整 `web/`：

```text
nps（Windows 为 nps.exe）
web/static/
web/views/
```

保留原 `conf/`，然后启动新版本。

## 凭据迁移

第一次成功启动时，NPS 会：

1. 在 `conf/credential.key` 创建随机 AES-256 主密钥并设置 `0600`；
2. 将 `nps.conf` 和三份持久化 JSON 中的凭据写回为 `npsenc:v1:` 密文；
3. 仅在进程内保留解密值，因此 Web、API、VerifyKey 登录和 NPC 连接继续使用原值。

服务停止后配置仍保持密文，不会恢复明文。以后备份或迁移必须复制完整 `conf/`。如果密文存在但
`credential.key` 缺失、损坏或来自另一套配置，NPS 会拒绝启动。

需要修改 `web_password`、`auth_key` 或 `public_vkey` 时，可先停止 NPS，把对应配置项替换为新的
明文值，再启动 NPS；启动成功后新值会再次写回密文。不要删除已有 `credential.key`。

## 升级后检查

- Web 管理端可以登录，Session 与 CSRF 正常。
- 现有 Client 重新上线，Client/Host/Tunnel 数量与升级前一致。
- `ConfigConnAllow=false` 的配置文件 NPC 仍能上线，但其配置不会新增服务端规则。
- 历史 TCP/UDP、HTTP/HTTPS、SOCKS、P2P 和 file 规则状态正确。
- 普通客户只能看到自己的对象；管理 API 仍只接受 HMAC 管理员身份。
- `conf/credential.key` 和持久化文件权限为 `0600`，文件中不再出现已知明文凭据。
- Docker 部署的 NPS 与 `/nps/web` 来自同一镜像版本。

## API 调用方

API 仍使用 `X-NPS-Timestamp`、`X-NPS-Nonce`、`X-NPS-Signature`。签名覆盖实际发送的原始正文，
旧 MD5 查询参数认证不可用。轮换 `auth_key` 后必须同步更新调用方，并用一次只读签名请求验证。

完整合约见 [API.md](API.md)。

## 回滚

凭据迁移后，旧版本不能识别 `npsenc:v1:`。回滚必须同时恢复：

1. 升级前的 NPS 二进制与完整 `web/`；
2. 升级前的完整 `conf/` 备份。

不要只替换旧二进制后继续使用已迁移配置，也不要通过删除 `credential.key` 强制启动。
