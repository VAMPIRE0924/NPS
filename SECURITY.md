# 安全说明

## 支持范围与版本边界

安全修复首先进入 `dev` 验收，通过受保护 Pull Request 后合入 `main`。正式生产应使用
最新的固定版本标签和对应校验和，不应长期跟随 `dev` 或浮动的 `main` 标签。

- `v2.0.0` 是首个 `v2.x` 正式基线：已使用 Go 1.26.6 验证，并已包含 HMAC-SHA256 API
  鉴权、CSRF 与会话固化防护。
- `v2.0.1` 是当前已发布生产版本，在上述基线上又加入普通客户端控制面隔离、凭据
  AES-256-GCM 非明文落盘以及其他完整安全加固。新部署和从 `v2.0.0` 升级的环境应优先使用
  `v2.0.1` 或更新固定版本。

本仓库只交付 NPS 服务端。原版或第三方 NPC 二进制与配置的兼容是硬边界；升级 NPS 不要求
重建、替换或改写 NPC。

## `v2.0.0` 安全验证基线

- 项目声明 Go 1.25 及以上，并固定 `toolchain go1.26.6`；实际验证环境为
  `go version go1.26.6 darwin/arm64`。
- 标准 NPS/service `go test` 和 `go vet`、关键服务端包 `go test -race` 以及 Linux/Windows
  amd64 NPS-only 交叉构建通过。
- `govulncheck v1.7.0` 对 NPS 服务端、Web 和核心包报告 **0 个可达漏洞**。模块级不可达
  记录不应写成服务端可利用漏洞。
- 完整 `go test ./...` 仍会被 `cmd/npc/npc.go` 和 `cmd/npc/sdk.go` 在同一包中各自声明
  `main` 阻断；完整 `go vet ./...` 还会报告 NPC 上游非具名 `net.TCPAddr` 字面量。这两项是
  NPS-only 发布范围外的 NPC 构建/兼容例外，不是未修复的 NPS 可达漏洞。

## API 鉴权

管理 API 只映射为管理员身份，使用 HMAC-SHA256 鉴权。请求必须携带：

```text
X-NPS-Timestamp
X-NPS-Nonce
X-NPS-Signature
```

签名覆盖请求方法、路径与原始查询串、时间戳、nonce 和实际发送的原始正文 SHA-256。服务端允许
正负 30 秒时钟偏差，nonce 只能使用一次，并在 Beego 解析表单之前按 1 MiB 上限捕获原始正文。空正文
伪签名、过期时间戳、nonce 重放、错误签名和未鉴权的受保护 POST 均返回 HTTP 401。

旧 `MD5(auth_key + timestamp)` 查询参数鉴权已移除，不属于 NPC 兼容边界。调用方必须对完全相同的路径、
查询串和正文字节进行签名与发送，每次重试都要重新生成时间戳、nonce 和签名。

## Web 会话与 CSRF

- 浏览器写操作只允许 POST，并必须通过 CSRF 校验。
- 管理员和 VerifyKey 客户端登录成功后均轮换 Session ID；注销和无效会话会销毁服务端会话。
- 未建立 Web 会话的受保护 POST 返回 HTTP 401；通过鉴权但无权访问的对象操作返回 HTTP 403。
- 自 `v2.0.1` 起，普通客户 Web 会话强制使用 Session 中的 Client ID，忽略请求伪造的
  `client_id`，并在列表、详情和写操作层重新校验对象归属。

## 仍保留的 NPC 兼容性例外

- NPC 验证握手继续使用旧协议要求的 MD5 派生值。该 MD5 只用于 NPC 线协议兼容，不用于 Web
  密码存储、管理 API 或新的鉴权设计。
- NPS 为 NPC 协议生成临时自签名证书，旧协议无法传递可验证的信任锚，因此 NPC 侧仍跳过证书链
  验证；双方最低 TLS 版本已设为 1.2。这不等同于经公开 CA 验证的现代双向身份认证。
- NPS 不默认拦截 NPC 访问客户端本机或客户端内网目标，并保留 `LocalProxy` 行为；这是数据面
  功能边界，部署者必须通过网络分段和访问策略限制可达目标。
- `public_vkey` 保留旧 NPC 共享临时配置入口，不等同于正式 Client 身份，不提供单设备吊销和持久
  设备审计语义。

删除这些例外或改变握手字节会使现有 NPC 失联，必须通过新协议版本和对应迁移方案处理。

## 生产部署指南

- 将 Web 管理端限制在受信网络，并在部署边界使用可验证证书的 TLS；不要将明文 HTTP 或
  仅依赖自签名证书的管理端暴露到不受信网络。
- 将 NPC Bridge 监听端口置于受控网络或可验证的外层安全传输之后，不要把旧 NPC 握手当作
  现代互联网边界身份认证。
- `web_password`、`auth_key`、`public_vkey` 和各 Client VerifyKey 必须相互独立、使用强随机值并
  定期轮换。不使用管理 API 或公共配置模式时，保持 `auth_key=` 或 `public_vkey=` 为空。
- 使用固定版本标签和 Release `SHA256SUMS`。NPS 不嵌入页面资源，二进制与完整
  `web/static/`、`web/views/` 必须来自同一版本；不需要同步替换 NPC。
- 自 `v2.0.1` 起，首次启动会生成 `conf/credential.key` 并迁移凭据为版本化 AES-256-GCM
  密文。备份、迁移和回滚必须整体保护并复制 `conf/`；遗漏或错配密钥会导致启动拒绝。
  这项措施不能防御已能同时读取整个 `conf/` 的本机入侵者，仍需目录权限、磁盘加密和加密备份。
- 仓库和发布物不包含 TLS 私钥。启用 TLS 终止时使用部署者自有证书与私钥；如果历史环境
  曾使用公开的示例 `server.key`，必须更换证书和私钥。
- 不要记录 API 密钥、完整签名请求、生产配置或包含 VerifyKey/密码的历史 API 响应。平台
  后端应映射 DTO 并遮罩敏感字段，不得把 NPS 原始对象直接透传到前端。

## 历史安全发现（已取代）

> **历史状态，不代表 `v2.0.0` 或更新版本的当前安全姿态。**

2026-08-02 对 Go 1.20.14 旧基线执行的 `govulncheck v1.0.4` 曾报告 46 个可达的标准库漏洞，
并指出旧 `golang.org/x/net`、时间戳 MD5 API 鉴权和 Web 缺少显式 CSRF 防护的问题。
`github.com/ulikunitz/xz` 的可达问题当时已通过升级到 `v0.5.15` 解决；其余上述基线问题随
Go 1.26.6/依赖升级、HMAC-SHA256 和 CSRF/会话加固而在 `v2.0.0` 前解决。不应继续引用该次
46 项结果作为 `v2.0.0` 的漏洞扫描结论。

## 报告漏洞

请优先使用 GitHub 仓库的私有 Security Advisory 报告可复现的安全问题，不要在公开 Issue 中附带
真实密钥、生产配置、客户数据或可直接利用的未修复细节。报告应包含受影响版本、入口、复现条件、
预期影响和最小化日志。

部署与迁移细节见 [UPGRADING.md](UPGRADING.md)，API 鉴权合约见 [API.md](API.md) 和
[API_SIGNING_EXAMPLES.md](API_SIGNING_EXAMPLES.md)。
