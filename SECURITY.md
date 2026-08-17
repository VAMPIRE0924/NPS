# 安全说明

## 支持范围

安全修复首先进入 `dev` 验收，通过受保护 Pull Request 后合入 `main`。正式生产建议使用最新固定
版本标签；`dev` 只用于测试。

本仓库只交付 NPS 服务端。原版 NPC 兼容是硬边界，旧 NPC 握手仍保留协议要求的 MD5 派生行为，
因此 Bridge 监听端口应置于受控网络或额外安全传输之后。

## 当前安全基线

- 使用 Go 1.26.6 和当前 `golang.org/x/net`、`x/crypto`、`x/sys` 构建。
- `govulncheck v1.7.0` 对 NPS 服务端、Web 和核心包报告 0 个可达漏洞。
- 管理 API 使用 HMAC-SHA256，签名覆盖方法、路径与原始查询串、时间戳、nonce 和原始正文摘要。
- Web 写操作要求 POST 与 CSRF，登录成功轮换 Session ID；越权对象访问返回 HTTP 403。
- HMAC API 只映射为管理员。普通客户 Web 会话强制绑定本 Client，不能通过 `client_id`、mode 或
  对象 ID 读取、修改其他账号。
- `nps.conf` 与持久化 JSON 中的凭据使用 AES-256-GCM 非明文落盘。
- 网络帧、SOCKS/P2P、HTTP/TLS/NPC 握手具有长度、状态或超时边界；共享缓存和代理认证头已隔离。
- 示例配置使用不可启动的管理密码占位符，仓库与发布物不包含 TLS 私钥。

## 部署责任

- 将 Web 管理端限制在受信网络并启用 TLS；不要把自签名或明文 HTTP 暴露到不受信网络。
- `web_password`、`auth_key`、`public_vkey` 和 Client VerifyKey 必须相互独立并定期轮换。
- 不使用管理 API 或公共配置模式时，保持 `auth_key=`、`public_vkey=` 为空。
- 完整保护并备份 `conf/`。`credential.key` 与密文同目录是为了可迁移性，不能抵御已经能读取整个
  运行目录的本机攻击者；仍需使用文件权限、磁盘加密和加密备份。
- 不要记录 API 密钥、完整签名请求或包含密码的历史 API 响应。
- 使用固定版本标签和校验和；NPS 二进制与完整 `web/` 必须来自同一版本。

## 兼容性例外

- NPC 线协议和配置保持兼容，不默认拦截 NPC 访问客户端本机或客户端内网目标。
- NPC 临时自签名 TLS 无法在不升级协议的情况下提供现代信任锚；最低 TLS 版本为 1.2。
- `public_vkey` 保留旧的共享临时配置入口，不等同于正式 Client 身份，也不提供单设备吊销语义。
- 历史管理 API 对象可能包含 VerifyKey 或密码字段；平台后端必须映射 DTO 并遮罩，不得直接透传。

## 报告漏洞

请优先使用 GitHub 仓库的私有 Security Advisory 报告可复现的安全问题，不要在公开 Issue 中附带
真实密钥、生产配置、客户数据或可直接利用的未修复细节。报告应包含受影响版本、入口、复现条件、
预期影响和最小化日志。

部署与迁移细节见 [UPGRADING.md](UPGRADING.md)，API 鉴权见 [API.md](API.md)。
