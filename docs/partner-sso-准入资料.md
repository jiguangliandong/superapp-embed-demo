# Partner Embed SSO 技术准入说明

> 测试环境：`https://superapp-test.jiguang.top`  
> 用途：说明 Partner 需要提交的技术参数

## 1. Partner 需要提交的技术参数

每个环境分别提交一份。Sandbox 和 Production 使用不同的 Client ID 和密钥。


| 参数                   | 示例                                            | 说明                           |
| -------------------- | --------------------------------------------- | ---------------------------- |
| `display_name`       | `Example Partner`                             | APP、WebView 和授权页展示名称         |
| `legal_entity`       | `Example Partner Sdn. Bhd.`                  |  公司主体        |
| `origin`             | `https://partner-sandbox.example.com`         | 精确 Origin，不能带尾部 `/` 或 Path   |
| `launch_url`         | `https://partner-sandbox.example.com/app`     | APP 点击入口后加载的完整 H5 地址         |
| `privacy_policy_url` | `https://partner-sandbox.example.com/privacy` | 用户授权页展示的 HTTPS 隐私政策          |
| `public_jwk`         | JWK JSON                                      | 使用下方脚本生成；只能提交公钥，不能包含私钥参数 `d` |


地址要求：

- `origin` 只能是 `scheme + host + port`，不能包含 Path、Query 或 Fragment；
- `launch_url` 必须使用公网 HTTPS，并属于登记的 Origin；
- `launch_url` 应返回 HTTP 200，且能在 APP WebView 中正常加载；
- `privacy_policy_url` 应能在未登录状态下打开。

Partner 资料提交模板（示例）：

```text
Partner 名称：Example Partner
环境：Sandbox
Legal Entity：Example Partner Sdn. Bhd.
Origin：https://partner-sandbox.example.com
Launch URL：https://partner-sandbox.example.com/app
Privacy Policy URL：https://partner-sandbox.example.com/privacy
Public JWK：partner-es256-public.jwk.json
```

三方不需要另外填写算法和密钥有效期：

- 算法由我方固定为 `ES256`，三方按下方脚本生成 P-256 密钥；
- `not_before` 和 `expires_at` 由我方管理员按环境策略填写；
- `kid` 已包含在 Public JWK 中，我方直接读取，不要求三方在表单里重复填写。

注意：当前 Admin 公钥登记接口要求请求中包含 `kid`、`alg`、`not_before` 和
`expires_at`。接口登记后生成的是 SuperApp 内部 `key_id`（`embkey_...`），不是 `kid`。

## 2. Partner 生成 ES256 密钥

以下脚本用于生成 Partner Sandbox 联调密钥。脚本会把 `kid` 写入 Public JWK，三方只需
提交生成的 `partner-es256-public.jwk.json`。

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


| 文件                              | 用途                                   | 是否提交给 SuperApp |
| ------------------------------- | ------------------------------------ | -------------- |
| `partner-es256-private.pem`     | Partner Backend 生成 `private_key_jwt` | 否              |
| `partner-es256-public.pem`      | Partner 本地检查                         | 不需要            |
| `partner-es256-public.jwk.json` | SuperApp Admin 登记公钥                  | 是              |


私钥由 Partner 自己保管，不能发送给 SuperApp、提交到 Git 或放进 H5 前端。