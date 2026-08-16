# HMAC 签名与调用示例

本文配套[API 说明](API说明.md)，示例中的密钥均为测试值，不得用于生产。

## 固定测试向量

输入：

```text
secret    = test-secret
method    = POST
path      = /index/socksstatus/
timestamp = 1800000000
nonce     = nonce-0123456789
body      = id=3
```

请求体 SHA-256：

```text
3a61b75b7aa22faa41b273a0d680ce23b19cb09a910b5ba52be4f3e0bd2e27e2
```

规范字符串：

```text
POST
/index/socksstatus/
1800000000
nonce-0123456789
3a61b75b7aa22faa41b273a0d680ce23b19cb09a910b5ba52be4f3e0bd2e27e2
```

期望签名：

```text
98c00fd4ee715fe8388b8144e9ca60f5c7fe791934cf9a201d4caad8dba32212
```

接入方应先用该向量验证自己的实现，再连接真实 NPS。

## Python 示例

依赖 `requests`，签名后直接发送同一份请求体字节：

```python
import hashlib
import hmac
import secrets
import time
from urllib.parse import urlencode, urlsplit

import requests


def nps_post(base_url: str, path: str, params: dict[str, object], secret: str):
    url = base_url.rstrip("/") + "/" + path.lstrip("/")
    body = urlencode(params).encode("utf-8")
    timestamp = str(int(time.time()))
    nonce = secrets.token_urlsafe(18)  # 仅包含签名协议允许的字符

    parsed = urlsplit(url)
    path_with_query = parsed.path or "/"
    if parsed.query:
        path_with_query += "?" + parsed.query

    body_hash = hashlib.sha256(body).hexdigest()
    canonical = "\n".join([
        "POST",
        path_with_query,
        timestamp,
        nonce,
        body_hash,
    ])
    signature = hmac.new(
        secret.encode("utf-8"),
        canonical.encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()

    response = requests.post(
        url,
        data=body,
        headers={
            "Content-Type": "application/x-www-form-urlencoded",
            "X-NPS-Timestamp": timestamp,
            "X-NPS-Nonce": nonce,
            "X-NPS-Signature": signature,
        },
        timeout=10,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("status") == 0 or payload.get("code") == 0:
        raise RuntimeError(payload.get("msg", "NPS business error"))
    return payload


result = nps_post(
    "https://nps.example.com:6443",
    "/index/socksstatus/",
    {"id": 3},
    "replace-with-secret-from-a-secret-manager",
)
print(result["data"])
```

如果配置了 `web_base_url=/nps`，传入路径应为 `/nps/index/socksstatus/`；签名中的路径也必须
包含 `/nps`。

## JavaScript / TypeScript 示例

适用于 Node.js 18 及以上。`URLSearchParams.toString()` 的结果同时用于签名和发送：

```typescript
import { createHash, createHmac, randomBytes } from "node:crypto";

async function npsPost(
  baseUrl: string,
  path: string,
  params: Record<string, string | number | boolean>,
  secret: string,
) {
  const url = new URL(path.replace(/^\//, ""), baseUrl.replace(/\/?$/, "/"));
  const body = new URLSearchParams(
    Object.entries(params).map(([key, value]) => [key, String(value)]),
  ).toString();
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const nonce = randomBytes(18).toString("base64url");
  const bodyHash = createHash("sha256").update(body, "utf8").digest("hex");
  const pathWithQuery = url.pathname + url.search;
  const canonical = [
    "POST",
    pathWithQuery,
    timestamp,
    nonce,
    bodyHash,
  ].join("\n");
  const signature = createHmac("sha256", secret)
    .update(canonical, "utf8")
    .digest("hex");

  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-NPS-Timestamp": timestamp,
      "X-NPS-Nonce": nonce,
      "X-NPS-Signature": signature,
    },
    body,
    signal: AbortSignal.timeout(10_000),
  });
  const payload = await response.json();
  if (!response.ok || payload.status === 0 || payload.code === 0) {
    throw new Error(payload.msg ?? `NPS HTTP ${response.status}`);
  }
  return payload;
}

const result = await npsPost(
  "https://nps.example.com:6443/",
  "/index/socksstatus/",
  { id: 3 },
  process.env.NPS_AUTH_KEY!,
);
console.log(result.data);
```

## 常见签名错误

| 现象 | 常见原因 |
| --- | --- |
| `401 missing API authentication headers` | 三个签名请求头未全部发送 |
| `401 request timestamp is outside allowed window` | 调用方时钟偏差超过 30 秒 |
| `401 nonce has already been used` | 重试时复用了 nonce |
| `401 invalid API signature` | 路径前缀、查询串、表单编码或请求体字节与签名时不一致 |
| 本地签名正确、经过网关后失败 | 网关改写了路径、查询参数或请求体 |

失败重试必须重新生成时间戳、nonce 和签名。网络超时后若业务操作是否成功不确定，应先查询目标
状态再决定是否重试创建或修改操作。
