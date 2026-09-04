# Partner Embed SSO 技术准入说明

> 测试环境：`https://superapp-test.jiguang.top`  
> 用途：说明 Partner 需要提交的技术参数、SuperApp Admin API 入驻操作，以及如何确认准入完成。

## 1. Partner 需要提交的技术参数

每个环境分别提交一份。Sandbox 和 Production 使用不同的 Client ID 和密钥。

| 参数 | 示例 | 说明 |
| --- | --- | --- |
| `display_name` | `Example Partner` | APP、WebView 和授权页展示名称 |
| `legal_entity` | Partner 注册主体 | Admin 创建 Client 的必填字段 |
| `allowed_scopes` | `auth_base`、`profile.name` | Partner 实际需要的用户资料范围 |
| `account_mode` | `auto_provision` | 可选；默认 `auto_provision` |
| `risk_tier` | `standard` | 可选；默认 `standard` |
| `origin` | `https://partner-sandbox.example.com` | 精确 Origin，不能带尾部 `/` 或 Path |
| `launch_url` | `https://partner-sandbox.example.com/app` | APP 点击入口后加载的完整 H5 地址 |
| `privacy_policy_url` | `https://partner-sandbox.example.com/privacy` | 用户授权页展示的 HTTPS 隐私政策 |
| `kid` | `partner-sandbox-es256-2026-01` | Partner 签名密钥标识 |
| `alg` | `ES256` | 推荐 ES256 / P-256 |
| `public_jwk` | JWK JSON | 只能提交公钥，不能包含私钥参数 `d` |
| `not_before` | UTC 时间 | 公钥开始生效时间 |
| `expires_at` | UTC 时间 | 公钥失效时间 |

地址要求：

- `origin` 只能是 `scheme + host + port`，不能包含 Path、Query 或 Fragment；
- `launch_url` 必须使用公网 HTTPS，并属于登记的 Origin；
- `launch_url` 应返回 HTTP 200，且能在 APP WebView 中正常加载；
- `privacy_policy_url` 应能在未登录状态下打开。

Partner 资料提交模板：

```text
Partner 名称：
环境：Sandbox / Production
Legal Entity：
Allowed Scopes：
Account Mode：auto_provision
Risk Tier：standard
Origin：
Launch URL：
Privacy Policy URL：
Key ID（kid）：
Algorithm：ES256
Public JWK：附件或 JSON
Not Before（UTC）：
Expires At（UTC）：
```

## 2. Partner 生成 ES256 密钥

以下脚本用于生成 Partner Sandbox 联调密钥。接入方应按实际项目替换目录和 `kid`。

```bash
umask 077
KEY_OUTPUT_DIR='partner-sandbox-keys' \
KEY_ID='partner-sandbox-es256-2026-01' \
node --input-type=module <<'NODE'
import { generateKeyPairSync } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const outputDir = process.env.KEY_OUTPUT_DIR;
const kid = process.env.KEY_ID;
if (!outputDir || !kid) {
  throw new Error("KEY_OUTPUT_DIR and KEY_ID are required");
}

mkdirSync(outputDir, {
  recursive: true,
  mode: 0o700,
});

const { privateKey, publicKey } = generateKeyPairSync("ec", {
  namedCurve: "P-256",
});

const publicJwk = {
  ...publicKey.export({ format: "jwk" }),
  use: "sig",
  alg: "ES256",
  kid,
};

writeFileSync(
  join(outputDir, "partner-es256-private.pem"),
  privateKey.export({
    format: "pem",
    type: "pkcs8",
  }),
  {
    mode: 0o600,
    flag: "wx",
  },
);

writeFileSync(
  join(outputDir, "partner-es256-public.pem"),
  publicKey.export({
    format: "pem",
    type: "spki",
  }),
  {
    mode: 0o644,
    flag: "wx",
  },
);

writeFileSync(
  join(outputDir, "partner-es256-public.jwk.json"),
  `${JSON.stringify(publicJwk, null, 2)}\n`,
  {
    mode: 0o644,
    flag: "wx",
  },
);

console.log(`Keys generated in ${outputDir}`);
NODE
```

检查 Public JWK：

```bash
jq -e '
  .kty == "EC" and
  .crv == "P-256" and
  .use == "sig" and
  .alg == "ES256" and
  .kid == "partner-sandbox-es256-2026-01" and
  (has("d") | not)
' partner-sandbox-keys/partner-es256-public.jwk.json
```

生成的文件用途：

| 文件 | 用途 | 是否提交给 SuperApp |
| --- | --- | --- |
| `partner-es256-private.pem` | Partner Backend 生成 `private_key_jwt` | 否 |
| `partner-es256-public.pem` | Partner 本地检查 | 不需要 |
| `partner-es256-public.jwk.json` | SuperApp Admin 登记公钥 | 是 |

私钥由 Partner 自己保管，不能发送给 SuperApp、提交到 Git 或放进 H5 前端。

## 3. SuperApp Admin 入驻操作

以下接口仅由 SuperApp 管理员调用，不提供 Admin Token 给 Partner。

### 3.1 Admin 登录

测试环境管理员接口：

```text
POST /api/user/v1/admin/auth/login
```

```bash
export BASE='https://superapp-test.jiguang.top'

curl --fail-with-body --silent --show-error \
  --request POST \
  "$BASE/api/user/v1/admin/auth/login" \
  --header 'Content-Type: application/json' \
  --data-binary @- <<'JSON' > /tmp/partner-admin-login.json
{
  "username": "admin",
  "password": "123456",
  "device_id": "partner-onboarding-admin-1",
  "device_name": "Partner onboarding"
}
JSON

export ADMIN_TOKEN="$(jq -er '.access_token' /tmp/partner-admin-login.json)"
```

Admin 凭据和 Token 只用于我方测试环境，不放入 Partner 回执。

### 3.2 创建 draft Client

接口：

```text
POST /api/user/v1/admin/embed/clients
```

请求示例：

```bash
curl --fail-with-body --silent --show-error \
  --request POST \
  "$BASE/api/user/v1/admin/embed/clients" \
  --header "Authorization: Bearer $ADMIN_TOKEN" \
  --header 'Content-Type: application/json' \
  --data-binary @- <<'JSON' | tee /tmp/partner-client.json | jq .
{
  "display_name": "Example Partner",
  "legal_entity": "<PARTNER_LEGAL_ENTITY>",
  "risk_tier": "standard",
  "account_mode": "auto_provision",
  "allowed_scopes": [
    "auth_base",
    "profile.name"
  ],
  "privacy_policy_url": "https://partner-sandbox.example.com/privacy",
  "origin": "https://partner-sandbox.example.com",
  "launch_url": "https://partner-sandbox.example.com/app"
}
JSON

export CLIENT_ID="$(jq -er '.client_id' /tmp/partner-client.json)"
```

成功响应：

```json
{
  "client_id": "embcli_...",
  "status": "draft",
  "consent_version": 1
}
```

### 3.3 登记 Partner 公钥

接口：

```text
POST /api/user/v1/admin/embed/clients/{client_id}/keys
```

将 Partner 提供的 Public JWK 保存到本地临时文件，并确认不含 `d`：

```bash
export PARTNER_PUBLIC_JWK_FILE='partner-es256-public.jwk.json'
export PARTNER_KID='partner-sandbox-es256-2026-01'

jq -e 'has("d") | not' "$PARTNER_PUBLIC_JWK_FILE"

jq -n \
  --arg kid "$PARTNER_KID" \
  --arg not_before '<PARTNER_NOT_BEFORE_UTC>' \
  --arg expires_at '2027-12-31T00:00:00Z' \
  --slurpfile public_jwk "$PARTNER_PUBLIC_JWK_FILE" \
  '{
    kid: $kid,
    alg: "ES256",
    public_jwk: $public_jwk[0],
    not_before: $not_before,
    expires_at: $expires_at
  }' |
curl --fail-with-body --silent --show-error \
  --request POST \
  "$BASE/api/user/v1/admin/embed/clients/$CLIENT_ID/keys" \
  --header "Authorization: Bearer $ADMIN_TOKEN" \
  --header 'Content-Type: application/json' \
  --data-binary @- | tee /tmp/partner-key.json | jq .
```

执行前将 `not_before` 和 `expires_at` 替换为 Partner 实际提交并确认的 UTC 时间。

成功响应：

```json
{
  "key_id": "embkey_...",
  "kid": "partner-sandbox-es256-2026-01",
  "expires_at": "2027-12-31T00:00:00Z"
}
```

说明：

- `kid` 是 Partner 签名时使用的 Key ID；
- `key_id` 是 SuperApp 内部的公钥记录 ID；
- Partner Backend 使用 `kid`，不使用 `embkey_...`。

### 3.4 激活 Client

接口：

```text
POST /api/user/v1/admin/embed/clients/{client_id}/activate
```

```bash
curl --fail-with-body --silent --show-error \
  --request POST \
  "$BASE/api/user/v1/admin/embed/clients/$CLIENT_ID/activate" \
  --header "Authorization: Bearer $ADMIN_TOKEN" \
  --output /tmp/partner-activate.json \
  --write-out 'activate_http=%{http_code}\n'
```

返回 2xx 表示激活成功。Client 激活后才能签发 Launch Manifest。

## 4. 如何确认准入完成

### 4.1 当前没有单独的 Admin 查询接口

截至 2026-09-03，当前 User Center 没有：

```text
GET /api/user/v1/admin/embed/clients/{client_id}
```

因此不能用一个 Admin GET 请求重新查询 Client、Origin、Launch URL、Key 和状态。当前通过
三次 Admin 操作结果加运行时检查确认准入：

1. create 响应取得 `client_id`；
2. keys 响应取得 `key_id`、`kid` 和 `expires_at`；
3. activate 返回 2xx；
4. Customer Launch Manifest 返回 HTTP 200；
5. Partner Launch URL 返回 HTTP 200。

如管理后台需要单接口展示完整准入结果，需要新增上述 GET 接口。

### 4.2 验证 Launch Manifest

使用我方 Customer Token 调用：

```bash
: "${CUSTOMER_TOKEN:?CUSTOMER_TOKEN is required}"

curl --fail-with-body --silent --show-error \
  "$BASE/api/user/v1/customer/embed/apps/$CLIENT_ID/launch-manifest" \
  --header "Authorization: Bearer $CUSTOMER_TOKEN" |
jq '{client_id, display_name, origin, launch_url, capabilities, expires_at}'
```

返回 HTTP 200，并且 `client_id`、`origin`、`launch_url` 与准入资料一致，说明 Client 已可用于
APP 启动。Customer Token 不能提供给 Partner。

### 4.3 验证 H5 落地页

```bash
export PARTNER_LAUNCH_URL='https://partner-sandbox.example.com/app'

curl --location --silent --show-error \
  --output /dev/null \
  --write-out 'http_code=%{http_code}\nfinal_url=%{url_effective}\n' \
  "$PARTNER_LAUNCH_URL"
```

期望 `http_code=200`，并且 `final_url` 仍属于登记的 Origin。

## 5. 准入完成后返回给 Partner 的信息

```text
Client ID：
Origin：
Launch URL：
已批准 Scope：
Key ID（kid）：
公钥记录 ID（SuperApp key_id）：
公钥有效期：
隐私政策：
状态：已激活
Launch Manifest 验证：HTTP 200
H5 地址验证：HTTP 200

SuperApp Base URL：https://superapp-test.jiguang.top
Discovery：https://superapp-test.jiguang.top/.well-known/superapp-embed-configuration
JWKS：https://superapp-test.jiguang.top/.well-known/jwks.json
Token/UserInfo/Revoke Endpoint：以 Discovery 当前返回值为准
```

不能返回：Admin Token、Customer Token、SuperApp 签名私钥或 Partner 私钥。

## 6. Sandbox 准入结果示例

```text
Client ID：embcli_...
Origin：https://partner-sandbox.example.com
Launch URL：https://partner-sandbox.example.com/app
Key ID（kid）：partner-sandbox-es256-2026-01
公钥记录 ID（SuperApp key_id）：embkey_...
公钥有效期：2027-12-31T00:00:00Z
隐私政策：https://partner-sandbox.example.com/privacy
状态：已激活
H5 地址验证：HTTP 200
```

如果需要把该结果作为完整准入回执发送给 Partner，还应补充最终批准的 Scope 和 Launch
Manifest 验证结果。
