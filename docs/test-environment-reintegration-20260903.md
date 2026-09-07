# 新测试环境 SSO 迁移与 Partner H5 联调交付记录

> 执行日期：2026-09-03  
> 新环境入口：`https://superapp-test.jiguang.top`  
> 参考：`eastel-backend/docs/test-environment.md`、`user-center/docs/sso/embed-h5-sso-runbook.md`  
> Demo：`superapp-h5-sso-demo`

## 1. 交付结论

本地 Demo 已切换到新的测试域名，并使用恢复自
`superapp/dist/superapp-delivery-267dfe7527f6-d30f52e1b0a7-linux-amd64/superapp_20260902.dump`
的既有 Partner Client 完成重新联调。

本次不需要重新创建 IM Service 或 Partner Client。原准入数据可以继续使用，前提是：

1. 数据库或交付 dump 已恢复；
2. IM Service 仍为 `active`；
3. Partner Client 仍为 `active`；
4. 三方登记的公钥仍有效且未撤销；
5. Partner `origin`、`launch_url` 和隐私政策地址未变化；
6. 三方仍持有与已登记公钥匹配的私钥。

域名和 Open API 路径变更属于调用配置迁移，不会自动使数据库中的 Service、Client、
公钥或既有用户授权失效。旧的短期 JWT、Client Assertion、Service Assertion、Launch
Manifest 和授权码不能跨域名复用，必须按新 URL 重新签发。

## 2. 新旧地址差异

统一基址由旧环境改为：

```text
https://superapp-test.jiguang.top
```

`/.well-known` 地址仍位于根路径：

```text
GET https://superapp-test.jiguang.top/.well-known/superapp-embed-configuration
GET https://superapp-test.jiguang.top/.well-known/jwks.json
```

Partner Embed 与 IM SSO 的三方机器接口新增 `/open` 路径段：

| 能力 | 新地址 |
| --- | --- |
| Embed Token | `/api/user/v1/open/embed/oauth/token` |
| Embed UserInfo | `/api/user/v1/open/embed/userinfo` |
| Embed Revoke | `/api/user/v1/open/embed/oauth/revoke` |
| IM Exchange | `/api/user/v1/open/im-sso/exchanges` |
| IM UserInfo | `/api/user/v1/open/im-sso/userinfo` |
| IM Profile Query | `/api/user/v1/open/im-sso/profiles/query` |
| IM Introspect | `/api/user/v1/open/im-sso/sessions/introspect` |

旧机器路径 `/api/user/v1/embed/*` 和 `/api/user/v1/im-sso/*` 已不再注册。

以下第一方接口仅替换域名，路径不增加 `/open`：

- Admin 准入：`/api/user/v1/admin/*`；
- Customer OTP、Launch Manifest、授权码与授权管理：`/api/user/v1/customer/*`。

## 3. IM SSO 应如何调整

### 3.1 管理员准入端

如果恢复后的 IM Service 已是 `active`，并且登记公钥有效，则管理员不需要重新执行
create、keys、activate。管理员侧只需确认状态和密钥有效期。

只有出现以下情况才重新操作准入：

- IM Service 不存在或不是 `active`；
- IM 服务更换了自己的签名私钥，需要登记新公钥；
- 旧 `kid` 已过期、撤销或疑似泄漏；
- 业务要求创建一个全新的 Service 身份。

管理员登录地址仍为：

```text
POST https://superapp-test.jiguang.top/api/user/v1/admin/auth/login
```

当前测试管理员账号为 `admin`，测试密码已更新为 `123456`。密码只用于 Admin 登录，
不会分发给 IM 服务或 Partner。

### 3.2 IM 服务端

IM 服务端必须同时完成两项修改：

1. 将四个机器 Endpoint 切换到新域名和 `/api/user/v1/open/im-sso/*`；
2. 为每次请求重新生成 `private_key_jwt`，并把 `aud` 精确设置成该次请求的完整新 URL。

示例：调用 Exchange 时：

```text
aud = https://superapp-test.jiguang.top/api/user/v1/open/im-sso/exchanges
```

`service_id`、IM 自己的私钥和 `kid` 可以继续使用。旧 Assertion 即使尚未到期也不能
继续使用，因为它的 `aud` 是旧域名或旧路径。

### 3.3 SuperApp APP 端

APP 申请 IM Login Code 时只替换基址，Customer 路径不变：

```text
POST https://superapp-test.jiguang.top/api/user/v1/customer/im-sso/login-codes
```

用户登录改为 OTP：先创建 challenge，再提交 verification。Customer Access Token 只应
由 APP 持有，不能交给 IM 服务或注入 H5。

## 4. Partner SSO 应如何调整

### 4.1 管理员准入端

本次恢复数据中的 Partner Client 可直接复用：

```text
client_id    = embcli_01M0HK4MQG8YBAG06K9DP44RQR
status       = active
kid          = h5-demo-es256-20260806
origin       = https://semiparochial-unpiratical-yang.ngrok-free.dev
launch_url   = https://semiparochial-unpiratical-yang.ngrok-free.dev/app
display_name = Partner No-SDK SSO Demo
```

本地 `secrets/partner-es256-private.pem` 与恢复数据中登记的公钥匹配，因此没有重新创建
Client、重新上传公钥或再次激活。若 Partner Origin 或 Launch URL 以后发生变化，应由
管理员更新或重建准入记录，不能只修改 Demo `.env`。

### 4.2 Partner Backend

Partner Backend 使用以下配置：

```text
SUPERAPP_BASE_URL=https://superapp-test.jiguang.top
SUPERAPP_CLIENT_ID=embcli_01M0HK4MQG8YBAG06K9DP44RQR
SUPERAPP_KEY_ID=h5-demo-es256-20260806
SUPERAPP_PRIVATE_KEY_PATH=secrets/partner-es256-private.pem
```

`private_key_jwt` 的 `aud` 必须是 Discovery 返回的完整 Token URL：

```text
https://superapp-test.jiguang.top/api/user/v1/open/embed/oauth/token
```

不能继续使用以旧域名或旧 `/api/user/v1/embed/oauth/token` 为 `aud` 的 Assertion。

### 4.3 Partner H5

H5 不直接访问 User Center 的 Token、UserInfo 或 Revoke。它只调用：

- Native Bridge；
- Partner 同源 `/api/sso/bootstrap`；
- Partner 同源 `/api/sso/complete`。

因此 JS SDK 不需要因 User Center endpoint 迁移而升级或修改请求路径。本 Demo 继续使用
`@superapp/embed-sdk` `v0.0.2`。

联调中发现首次原生 Consent 包含用户阅读时间，原 14–15 秒等待时间不足。两个 H5 的
SDK 调用超时已改为 121 秒，Android Native Bridge 超时改为 120 秒，保证 Native 先返回，
H5 再兜底超时。

### 4.4 Go SDK

Go SDK 负责 Partner Backend 到 User Center 的 Token、UserInfo 和 Revoke 调用，旧
`v0.0.3` 内置的是迁移前路径，因此需要升级。本 Demo 已升级到：

```text
github.com/jiguangliandong/superapp-embed-go-sdk v0.0.4
```

`v0.0.4` 使用 `/api/user/v1/open/embed/*`。Partner 只配置新的
`SUPERAPP_BASE_URL`，不要在业务代码中再次拼接旧 endpoint。

## 5. `DELIVERY_EMBED_SIGNING_PRIVATE_JWK` 变更的影响

该变量是 User Center 自己的 ES256 私钥，用于签名 Launch Manifest；对应公钥通过
`/.well-known/jwks.json` 发布。它不是 Partner 的客户端私钥，也不是 IM Service 的服务私钥。

因此这次更换不会使下面这些数据失效：

- 已登记的 Partner Client 公钥；
- 已登记的 IM Service 公钥；
- Partner 的 `client_id`、`kid`；
- IM 的 `service_id`、`kid`；
- 数据库中仍为 active 的准入记录。

需要调整的是宿主 APP 的验签缓存：

- JWKS URL 改为新域名；
- 遇到未知 `kid` 时立即刷新 JWKS 一次；
- 不能长期 Pin 旧公钥内容；
- 已获取的旧 Launch Manifest 不继续使用，重新向新环境获取并验签。

测试环境若希望容器重建后 JWKS 稳定，交付包 `.env` 中应固定填写完整私钥 JWK；不得
把私钥内容写入仓库、日志或本文档。

## 6. 本次 Demo 修改清单

| 文件 | 修改 |
| --- | --- |
| `.env.example` | 新测试域名和恢复的 Partner Client ID |
| 本地 `.env` | 新基址、Client ID 及新的 `/open/embed` endpoint |
| `README.md` | 测试环境、OTP 示例、机器路径和 Go SDK 版本 |
| `partner-backend-go/go.mod`、`go.sum` | Go SDK `v0.0.4` |
| `partner-backend-go/internal/server/server_test.go` | 测试桩迁移到 `/open/embed` |
| `partner-h5-js/src/app.js` | 交互授权超时 121 秒 |
| `partner-h5-js2/src/app.js` | 交互授权超时 121 秒 |
| 两个 H5 的 SDK 集成测试 | 增加交互授权超时保护测试 |
| `superapp-android/app/build.gradle.kts` | Base URL、Issuer、JWKS、Client ID |
| `superapp-android/.../debug_network_security_config.xml` | 删除旧 HTTP 测试域名白名单 |
| `PartnerWebView.kt` | Native Bridge 超时 120 秒 |
| `superapp-android/README.md` | 新环境配置说明 |

## 7. 本次实测记录

### 7.1 环境发现

Discovery 实测返回：

```text
issuer              = https://superapp-test.jiguang.top
token_endpoint       = https://superapp-test.jiguang.top/api/user/v1/open/embed/oauth/token
userinfo_endpoint    = https://superapp-test.jiguang.top/api/user/v1/open/embed/userinfo
revocation_endpoint  = https://superapp-test.jiguang.top/api/user/v1/open/embed/oauth/revoke
jwks_uri             = https://superapp-test.jiguang.top/.well-known/jwks.json
```

### 7.2 OTP 与 Launch Manifest

使用以下测试身份完成登录：

```text
country_code = +60
phone_number = 123456780
OTP          = 456780
```

OTP challenge、verification 和 Launch Manifest 均成功。Manifest 中的 Client ID、Origin
和 Launch URL 与恢复数据一致，签名可通过新 JWKS 验证。

### 7.3 首次 Partner 授权

测试前先撤销该用户对 Demo Client 的既有授权，并确认授权列表为空。随后执行：

1. Android APP 从新 User Center 取得 Launch Manifest；
2. WebView 打开 Partner H5；
3. H5 发起 bootstrap 和 Native Bridge `getAuthCode`；
4. 原生页面展示 Partner 名称、scope 和隐私政策；
5. 授权页停留 20 秒后点击“同意并继续”；
6. 同一笔交易成功完成 code exchange、UserInfo 和 Partner Session 创建；
7. H5 显示 `Sbuy User 6780`、`+60123456780`、OpenID、scope、KYC 和授权版本 1。

User Center 授权记录的 `granted_at` 为：

```text
2026-09-03T02:16:21.177165Z
```

该结果同时验证了 120/121 秒超时修复：授权页超过旧的 14–15 秒阈值后仍在同一笔交易
内成功，没有出现 `bridge_timeout`。

### 7.4 Partner 会话恢复

关闭 H5 后再次从金刚位进入，页面直接显示：

```text
Partner 会话自动恢复
```

用户资料、OpenID 和授权版本保持一致，证明 Partner Cookie Session 与首次 SSO 链路正常。

### 7.5 自动化回归

以下回归均通过：

- Partner Backend：`go test ./...`、`go vet ./...`；
- Partner H5 1：`npm test`、`npm run build`；
- Partner H5 2：`npm test`、`npm run build`；
- Android：`testDebugUnitTest`、`lintDebug`、`assembleDebug`；
- Go SDK `v0.0.4`：新 `/open/embed` 路径由 Demo 服务测试覆盖；
- JS SDK `v0.0.2`：Bridge 与 Partner 同源接口测试通过，无 User Center endpoint 依赖。

## 8. 三方迁移检查表

### IM 团队

- 保留有效的 `service_id`、私钥和 `kid`；
- 将四个机器 URL 改成新域名和 `/open/im-sso`；
- 每个请求用目标完整 URL 生成新的 Assertion `aud`；
- 不复用旧 JWT、Login Code 或 Federation Session 测试数据；
- 配合 APP 用新的 Customer Login Code 完成一次 Exchange 与 UserInfo。

### Partner 团队

- 保留有效的 `client_id`、私钥和 `kid`；
- Partner Backend 基址改为新域名；
- Go SDK 升级到 `v0.0.4` 或等价实现新 `/open/embed` 路径；
- JS SDK 无需因 endpoint 变化升级；
- 重新发起完整 SSO，不复用旧 Assertion、授权码、Token 或 Launch Manifest。

### SuperApp 管理员与 APP 团队

- 确认恢复后的 IM Service、Partner Client 和登记公钥仍有效；
- 无配置变化时不重复 create、keys、activate；
- APP 的 Base URL、Issuer 和 JWKS URL 改成新域名；
- APP 使用 OTP challenge/verification 登录；
- JWKS 未知 `kid` 时刷新，避免继续使用旧签名公钥缓存；
- 分别完成一次 IM SSO 和 Partner SSO 的新链路验收。

