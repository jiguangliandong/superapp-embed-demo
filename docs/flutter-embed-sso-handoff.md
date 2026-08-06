# SuperApp Flutter Embed SSO Demo 对接任务书

## 1. 文档目的

本文用于指导 Flutter 开发同事实现一个 Android-only 的 SuperApp Debug Demo，并只交付
一个可安装的 Debug APK。APK 最终由需求方在自己的电脑、Android Emulator、本地
SuperApp Backend 和现有 Partner H5 环境中完成端到端联调。

本任务目标是验证真实 Embed SSO 协议，不是实现生产版 SuperApp。

## 2. 最终验收效果

```text
打开 Flutter APP
→ Customer 使用测试账号登录
→ 首页显示一个 Partner H5 金刚位
→ 点击金刚位
→ APP 获取并验证 Launch Manifest
→ APP 创建受信 Partner WebView
→ WebView 加载 Partner H5
→ H5 检测到 SuperappNativeBridge
→ 用户点击“使用 SuperApp 登录”
→ APP 在需要时展示原生授权确认
→ APP 返回一次性 Authorization Code
→ Partner Backend 完成 Token Exchange 和 UserInfo
→ H5 显示昵称、手机号、邮箱和 KYC 状态
```

当前 Embed Client 尚未批准 `profile.avatar`，因此真实头像不属于本轮强制验收项。H5
会在没有 `avatar_url` 时显示昵称首字母占位。

## 3. 交付范围

### 3.1 必须实现

- Android-only Flutter Debug APP；
- Customer 登录页；
- 一个固定 Partner H5 金刚位；
- Customer Session Token 的内存态管理；
- Launch Manifest 获取与完整 ES256 验签；
- 独立 Partner WebView 容器；
- Native Bridge V1；
- `getContext`；
- `getAuthCode`；
- 简化的原生 Consent 确认界面；
- `openPrivacySettings` 的 Demo 页面；
- `close`；
- Android Back 关闭 Partner 容器；
- 加载、失败和可重试状态；
- 一个可安装并能完成本任务 SSO 联调的 Debug APK。

### 3.2 本轮不要求

- iOS；
- Customer Token 刷新；
- 多 Partner 动态配置；
- 生产级埋点、监控和风控；
- 生产签名；
- 进程恢复后的 WebView 恢复；
- 完整授权管理产品页面；
- 生产环境证书固定；
- 真实头像 Scope；
- Flutter 源码仓库交付；
- README、构建脚本或依赖清单交付；
- 单元测试、测试报告、静态检查报告或覆盖率报告交付；
- 日志、埋点、监控、告警和崩溃分析方案验收；
- 交付团队内部的代码规范、分支管理和发布流程验收。

## 4. 固定联调参数

```text
SUPERAPP_BASE_URL=http://localhost:8080
EMBED_ISSUER=http://localhost:8080
JWKS_URL=http://localhost:8080/.well-known/jwks.json
DISCOVERY_URL=http://localhost:8080/.well-known/superapp-embed-configuration

EMBED_CLIENT_ID=embcli_01KZAW57KCCWDTV9QXSPEZV8A7

PARTNER_ORIGIN=https://semiparochial-unpiratical-yang.ngrok-free.dev
PARTNER_LAUNCH_URL=https://semiparochial-unpiratical-yang.ngrok-free.dev/app
PARTNER_PRIVACY_URL=https://semiparochial-unpiratical-yang.ngrok-free.dev/privacy
```

当前获批 Scope：

```text
auth_base
profile.name
contact.phone
contact.email
kyc.status
```

Customer 测试账号与密码由需求方单独提供，不写入源码、Git 或构建产物。

## 5. 金刚位配置

金刚位不能把 Partner H5 当成普通外链打开。金刚位应保存：

```text
type=embed_partner
client_id=embcli_01KZAW57KCCWDTV9QXSPEZV8A7
title=SuperApp H5 SSO Demo
```

点击时使用 `client_id` 请求 Launch Manifest。APP 不应把 ngrok Launch URL 当成可信
本地配置，也不能在 Manifest 失败时降级为直接打开 URL。

## 6. Android Debug 网络配置

APK 运行在需求方的 Android Emulator 中。需求方会在安装 APK 后执行：

```bash
adb reverse tcp:8080 tcp:8080
adb reverse --list
```

因此 Flutter Debug 配置必须使用：

```text
http://localhost:8080
```

不要改成开发同事电脑的 IP，也不要使用 `10.0.2.2`。Partner H5 通过公网 ngrok HTTPS
加载，不需要映射 `3000` 端口。

必须声明网络权限：

```xml
<uses-permission android:name="android.permission.INTERNET" />
```

Debug 构建需要允许访问本地 HTTP。建议只在：

```text
android/app/src/debug/AndroidManifest.xml
```

配置：

```xml
<application android:usesCleartextTraffic="true" />
```

不得将本地明文 HTTP 配置带入生产 Manifest。

## 7. Customer 登录

登录接口：

```http
POST /api/customer/v1/auth/login
Content-Type: application/json
```

请求示例：

```json
{
  "identifier": "<测试账号>",
  "password": "<测试密码>",
  "device_id": "flutter-android-embed-demo",
  "device_name": "Android Emulator"
}
```

登录成功后，Customer Access Token 只保存在 APP 内存中。禁止写入：

- WebView；
- JavaScript；
- Cookie；
- URL；
- 日志；
- Partner H5；
- Git 配置文件。

APP 重启后允许要求用户重新登录。

## 8. Launch Manifest

点击金刚位后调用：

```http
GET /api/customer/v1/embed/apps/{client_id}/launch-manifest
Authorization: Bearer <customer-token>
Accept: application/json, application/problem+json
Accept-Language: zh-CN
```

响应字段：

```json
{
  "launch_id": "lnch_...",
  "client_id": "embcli_...",
  "display_name": "SuperApp H5 SSO Demo",
  "origin": "https://semiparochial-unpiratical-yang.ngrok-free.dev",
  "launch_url": "https://semiparochial-unpiratical-yang.ngrok-free.dev/app",
  "capabilities": ["getAuthCode", "openPrivacySettings", "close"],
  "expires_at": "...",
  "signed_payload": "eyJ..."
}
```

### 8.1 强制验签要求

从 `JWKS_URL` 取得 SuperApp 公钥，只接受：

- `kty=EC`；
- `crv=P-256`；
- `alg=ES256`；
- `use=sig`；
- 与 JWT Header 精确匹配的 `kid`。

必须验证：

1. JWS 只有一个签名；
2. `alg == ES256`；
3. 签名有效；
4. `iss == http://localhost:8080`；
5. `aud` 只包含当前 `client_id`；
6. `sub == launch_id`；
7. `iat` 不在未来；
8. `exp` 未过期；
9. JWT 内外层的 `client_id`、`display_name`、`origin`、`launch_url` 和
   `capabilities` 逐字段一致。

任何一步失败都必须拒绝启动，不能降级为普通 WebView。

### 8.2 构造启动 URL

必须使用结构化 URI API：

```dart
final launchUri = Uri.parse(manifest.launchUrl);
final target = launchUri.replace(
  queryParameters: {
    ...launchUri.queryParameters,
    'launch_id': manifest.launchId,
  },
);
```

加载前确认：

- Scheme 是 `https`；
- Origin 精确等于 Manifest Origin；
- URL 不包含 userinfo；
- Manifest 尚未过期。

## 9. Partner WebView

必须为 Partner 创建独立 WebView，不得复用 Customer 登录 WebView。

最低要求：

- 禁止 `file://`；
- 禁止 `javascript:` 导航；
- 禁止 Mixed Content；
- 只允许已验证 Manifest Origin 使用 Bridge；
- 导航到其他 Origin 后立即禁用 Bridge；
- 不向 H5 注入 Customer Token、Member ID 或设备永久标识；
- WebView 关闭时清理 Pending Bridge 请求；
- APP 进入后台或用户切换账号时取消授权流程。

## 10. Native Bridge V1

Partner JS SDK 查找：

```javascript
globalThis.SuperappNativeBridge.invoke(method, params)
```

`invoke` 必须返回 Promise。Flutter WebView Channel 通常是单向 `postMessage`，因此需要
实现 Promise Adapter、唯一 `request_id`、Pending Promise 表和固定 Native 回调。

每个请求：

- 只能 resolve 或 reject 一次；
- 默认最长 15 秒；
- WebView 销毁、导航或 APP 后台时取消；
- Native 回调参数使用安全 JSON 编码；
- 禁止把 H5 输入拼接成可执行 JavaScript。

只允许以下方法：

```text
getContext
getAuthCode
openPrivacySettings
close
```

未知方法直接拒绝。

## 11. `getContext`

请求参数必须是：

```json
{}
```

响应：

```json
{
  "sdk_version": "1.0",
  "client_id": "embcli_01KZAW57KCCWDTV9QXSPEZV8A7",
  "container": "superapp",
  "capabilities": ["getAuthCode", "openPrivacySettings", "close"],
  "locale": "zh-CN"
}
```

字段必须来自已验证 Manifest 和 APP 本地状态，不能相信 H5 自报值。

## 12. `getAuthCode`

H5 SDK 请求：

```json
{
  "transaction_id": "ptx_...",
  "client_id": "embcli_01KZAW57KCCWDTV9QXSPEZV8A7",
  "scopes": ["auth_base", "profile.name"],
  "state": "<16-512-char-base64url>",
  "code_challenge": "<43-char-base64url>",
  "code_challenge_method": "S256"
}
```

Native 在发起网络请求前必须验证：

- 顶层主文档；
- 当前真实 Origin 等于 Manifest Origin；
- 请求 Client ID 等于 Manifest Client ID；
- `transaction_id` 非空且有限长；
- State 是 16～512 字符 Base64URL；
- Scope 非空、去重且属于当前 V1 已知集合；
- `code_challenge_method == S256`；
- Challenge 是 43 字符 Base64URL，解码后 32 字节；
- Manifest、Customer Session 和 APP 前台状态有效；
- 同一 WebView 没有另一笔授权正在进行。

Native 调用：

```http
POST /api/customer/v1/embed/authorization-codes
Authorization: Bearer <customer-token>
Content-Type: application/json
Accept: application/json, application/problem+json
Accept-Language: zh-CN
```

请求：

```json
{
  "client_id": "<Manifest client_id>",
  "launch_id": "<Manifest launch_id>",
  "origin": "<Native 读取的主文档 Origin>",
  "scopes": ["auth_base", "profile.name"],
  "code_challenge": "<H5 请求并通过验证的 Challenge>",
  "code_challenge_method": "S256",
  "state": "<H5 请求并通过验证的 State>",
  "consent_approved": false
}
```

`client_id`、`launch_id` 和 `origin` 必须来自 Native 可信状态，不能直接复制 H5 自报值。

成功时 resolve：

```json
{
  "code": "embcode_...",
  "state": "<原始 state>",
  "expires_at": "<RFC3339 UTC>"
}
```

Authorization Code 只能存在于内存，并只能返回发起请求的 H5。

## 13. Consent Demo

第一次申请非 `auth_base` Scope 时，Backend 可能返回：

```text
embed.consent_required
```

APP 随后显示原生确认界面，至少列出：

- Partner 名称；
- 本次 Scope 对应的数据类型；
- Partner 隐私政策入口；
- “同意并继续”；
- “暂不同意”。

用户同意后，使用完全相同的 Client、Launch、Origin、Scope、Challenge 和 State 重新请求，
只将：

```json
{"consent_approved": true}
```

合并进原请求。

用户拒绝时 reject Bridge Promise，保持 H5 打开。Consent 显示期间如果 Origin、账号、
前后台状态或 WebView 生命周期发生变化，必须取消流程。

> 当前 Manifest 尚未返回生产 Consent UI 所需的全部法律主体和用途文案。本轮 Demo 可以
> 使用明确标注为测试用途的固定文案，但不得将其视为生产实现。

## 14. `openPrivacySettings`

调用：

```http
GET /api/customer/v1/embed/authorizations
Authorization: Bearer <customer-token>
Accept: application/json, application/problem+json
```

Demo 页面至少展示已授权应用、Scope 和隐私政策入口。若实现撤销：

```http
DELETE /api/customer/v1/embed/authorizations/{client_id}
Authorization: Bearer <customer-token>
```

撤销成功后关闭对应 Partner WebView 并清理其 WebView 状态。

## 15. `close`

关闭当前 Partner WebView，并清理：

- 当前 Manifest；
- Bridge Pending Promise；
- 当前页面内存状态。

`close` 不等于撤销 Consent，也不等于 Partner Token Revoke。

## 16. 错误处理

APP 对外只使用稳定错误分类，不向 H5暴露 Backend Detail、堆栈或内部异常。

至少区分：

```text
user_denied
not_authenticated
client_unavailable
origin_not_allowed
launch_invalid
scope_not_allowed
bridge_busy
bridge_timeout
app_not_active
internal_error
```

Authorization Code 和 Launch 都是短期一次性状态。网络结果不确定时不得盲目重放，应关闭
当前容器并重新获取 Manifest。

## 17. 工程责任边界

日志、埋点、监控、告警、崩溃分析、代码质量、测试覆盖率和团队内部发布流程由交付团队
自行设计与保障，不属于本次 APK 交付物，也不纳入需求方验收。

需求方只验收 APK 是否能按本文流程完成真实 SSO 联调。交付团队仍应自行确保其实现不会
把 Customer Token、Authorization Code、PKCE 材料或用户敏感资料泄漏到 WebView 或其他
非预期边界，但无需向需求方额外交付日志方案、安全报告或测试报告。

## 18. 交付物

只需要交付一个可安装的 Android Debug APK：

```text
app-debug.apk
```

不要求交付 Flutter 源码、README、依赖清单、构建命令、签名材料、测试代码、测试报告、
日志方案或其他工程文件。

APK 必须已经内置本文第 4 节的 Debug 环境参数，并允许通过 `adb reverse` 访问需求方本机
的 `http://localhost:8080`。

## 19. 需求方本机验收步骤

需求方会执行：

```bash
# 1. 启动 SuperApp Backend（端口 8080）

# 2. 启动 Partner H5
cd superapp-h5-sso-demo
npm run dev

# 3. 保持现有 ngrok 域名转发到 3000
ngrok http 3000

# 4. 启动 Android Emulator 后映射 Backend
adb reverse tcp:8080 tcp:8080
adb reverse --list

# 5. 安装 APK
adb install -r /path/to/app-debug.apk
```

## 20. Definition of Done

以下项目全部通过才算完成：

- Customer 能使用真实 Endpoint 登录；
- 登录 Token 未进入 WebView；
- 首页金刚位绑定 Client ID 而不是普通外链；
- Manifest 验签成功后才加载 H5；
- Manifest 签名错误、过期或 Claim 不匹配时拒绝启动；
- WebView 实际加载 URL 包含新的 `launch_id`；
- H5 能成功调用 `getContext`；
- 普通浏览器无法伪造 Bridge；
- 未登记 Origin 无法调用 Bridge；
- H5 能通过 `getAuthCode` 完成真实 SSO；
- 首次敏感 Scope 能显示原生 Consent；
- H5 最终显示昵称、手机号、邮箱和 KYC 状态；
- 重复 Code、错误 State 和非法 PKCE 被拒绝；
- `openPrivacySettings` 和 `close` 可用；
- Android Back 正确关闭 Partner 容器；
- Debug APK 可在需求方 Android Emulator 安装运行。

## 21. 权威参考

实现细节以 SuperApp Backend 仓库中的以下文档为准：

```text
docs/embed-flutter-integration.md
docs/embed-partner-integration.md
docs/embed-sso.md
```

若本文与上述协议文档冲突，以 `embed-flutter-integration.md` 的公开 Flutter 契约为准。
