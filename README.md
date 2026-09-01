# SuperApp Embed SSO 端到端 Demo

本仓库用三个彼此隔离的目录演示完整 SSO：

- `superapp-android`：原生 Kotlin/Compose 宿主客户端；
- `partner-h5-js`：使用本地 `@superapp/embed-sdk`（`file:../../superapp-embed-js-sdk`）的 H5；
- `partner-backend-go`：使用本地 `superapp-embed-go-sdk`（`replace => ../../superapp-embed-go-sdk`）的 Partner Backend。

身份与 Embed 权威在同级 `user-center`（当前联调 Issuer 为
`http://superapp-dev.jiguang.top`），不再走业务单体的
旧 `/api/customer/v1`、`/api/embed/v1` 路径。按步骤的联调走查见
[eastel-backend/docs/user/embed-h5-sso-runbook.md](../eastel-backend/docs/user/embed-h5-sso-runbook.md)。

最终路径是：Customer OTP 登录 Android App → 点击金刚位 → App 获取并验签 Launch
Manifest → H5 请求 Native Bridge 授权 → Go Backend 兑换 Code、查询 UserInfo → H5
展示昵称、OpenID、手机号、邮箱和 KYC 状态。

如果 Partner 不引入 JavaScript SDK 和服务端 SDK，可直接参考
[`SuperApp Embed SSO 三方接入指南（无 SDK 版）`](docs/superapp-embed-sso-partner-integration-no-sdk.md)，
按照 Native Bridge、PKCE、`private_key_jwt` 和 OAuth Endpoint 契约自行实现。

SuperApp Android 客户端团队可参考
[`SuperApp Embed SSO Android 接入指南`](docs/superapp-embed-sso-android-integration-guide.md)，
实现 Launch Manifest 验签、受控 WebView、Native Bridge、原生 Consent 和授权管理。

WeTix 的前期资料收集、Sandbox Client 创建、公钥登记和激活流程见
[`WeTix Partner Embed SSO 前期入驻与联调准备`](docs/wetix-partner-onboarding-guide.md)。

## 1. 一次性配置

除非代码块另有说明，下面的项目命令都从仓库根目录执行：

```bash
cd /path/to/superapp-h5-sso-demo
```

User Center 通过 `http://superapp-dev.jiguang.top` 访问。Embed Client 的 `origin` 和 `launch_url`
必须配置为 Partner H5 的公网 HTTPS 地址，例如：

```text
origin:     https://<your-domain>
launch_url: https://<your-domain>/app
```

Android App 不写死 H5 地址；它只接受 User Center 返回并通过 ES256 验签的地址。
`client_id` 在 Admin 准入后写入 `superapp-android/app/build.gradle.kts` 的
`EMBED_CLIENT_ID`。

复制环境变量模板并确认私钥已放入被 Git 忽略的 `secrets/`：

```bash
cp .env.example .env
test -f secrets/partner-es256-private.pem
```

Partner `.env` 的 `SUPERAPP_BASE_URL` 必须等于 User Center Issuer origin
（当前为 `http://superapp-dev.jiguang.top`），否则 `private_key_jwt` 的 `aud` 会校验失败。

### 1.1 Android 命令行环境

下面三个 `export` 只对当前 Shell 有效。可以每次打开新终端后执行，也可以一次性写入
`~/.zshrc`：

```bash
export JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export PATH="$ANDROID_HOME/platform-tools:$PATH"
```

确认工具可用：

```bash
java -version
adb version
android emulator list
```

## 2. 第一次构建

### 2.1 Partner H5

```bash
cd /path/to/superapp-h5-sso-demo
npm --prefix partner-h5-js install
npm --prefix partner-h5-js test
npm --prefix partner-h5-js run build
```

H5 产物输出到 `partner-h5-js/dist/`。修改 H5 后需要重新执行 `npm run build`，并重启
Partner Go Backend，因为 Go 服务启动时会把 `app.html` 读入内存。

### 2.2 Android APK

```bash
cd /path/to/superapp-h5-sso-demo/superapp-android
./gradlew testDebugUnitTest lintDebug assembleDebug
```

APK 输出位置：

```text
superapp-android/app/build/outputs/apk/debug/app-debug.apk
```

第一次安装或 Android 代码重新构建后执行：

```bash
cd /path/to/superapp-h5-sso-demo/superapp-android
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

## 3. 日常启动

### 3.1 启动 User Center

先在同级 `eastel-backend` 准备 PostgreSQL / Redis，再启动 `user-center`：

```bash
cd /path/to/eastel-backend && make infra
cd /path/to/user-center
make build && ./bin/user-center
```

```bash
curl --fail-with-body --silent http://localhost:8081/healthz
curl --fail-with-body --silent http://localhost:8081/.well-known/superapp-embed-configuration | jq .
```

Client 准入（Admin 登录、创建、登记公钥、激活）按联调走查第 2 步执行。

### 3.2 启动 Partner Go Backend

Partner Go Backend 不会自动读取 `.env`：

```bash
cd /path/to/superapp-h5-sso-demo
set -a
source .env
set +a
cd partner-backend-go
go run ./cmd/server
```

Partner 服务监听 `http://localhost:3000`。

### 3.3 启动 ngrok

```bash
ngrok http --url=<your-domain> 3000
```

固定域名必须和 User Center 中 Embed Client 的 `origin`、`launch_url` 完全一致。

### 3.4 启动 Android 模拟器

```bash
adb devices
```

没有设备时启动模拟器。当前构建直接访问 `superapp-dev.jiguang.top`，模拟器必须能解析并
访问该域名；只有把 Issuer 改回 `http://localhost:8081` 做纯本地联调时才需要
`adb reverse tcp:8081 tcp:8081`。

### 3.5 启动已经安装的 App

```bash
adb shell am start -W -n \
  com.jiguangliandong.superapp.embeddemo.debug/com.jiguangliandong.superapp.embeddemo.MainActivity
```

## 4. 联调行为

使用 +60 手机号 OTP 登录。local/dev 验证码是规范化 E.164 的后六位，例如
`123456789` → `+60123456789` → `345678`。然后点击“Partner SSO Demo”金刚位。

第一次申请资料会展示原生授权弹窗；同意后 H5 切换到独立的用户信息界面。再次打开时：

- Partner Session 有效：直接显示用户信息；
- Partner Session 丢失但 SuperApp Consent 有效：自动走一次授权码流程，静默恢复登录；
- Consent 不存在、已撤销、已过期、版本变化或新增 Scope：再次展示原生授权界面。

H5 不使用 LocalStorage 判断用户是否已授权。

## 5. 代理与模拟器排障

如果 macOS 正在使用本机代理，而模拟器访问 ngrok 报 `ERR_CONNECTION_CLOSED`，可以让
模拟器的公网请求使用宿主机代理，同时让 `localhost:8081` 继续通过 `adb reverse` 直连：

```bash
adb shell settings put global http_proxy 10.0.2.2:7893
adb shell settings put global global_http_proxy_exclusion_list 'localhost,127.0.0.1'
```

不再需要代理时恢复：

```bash
adb shell settings put global http_proxy :0
adb shell settings delete global global_http_proxy_exclusion_list
```

## 6. 哪些操作需要重复

| 操作 | 什么时候执行 |
| --- | --- |
| 配置 `JAVA_HOME`、`ANDROID_HOME`、`PATH` | 每个新 Shell；写入 `~/.zshrc` 后无需手动重复 |
| `npm install` | 第一次或 H5 / JS SDK 依赖变化 |
| `npm run build` | H5 代码变化 |
| 加载根目录 `.env` | 每次在新 Shell 启动 Partner Go Backend |
| 启动 Go Backend | 进程未运行，或 Go/H5 页面代码变化 |
| 启动 ngrok | 隧道未运行 |
| 启动模拟器 | `adb devices` 没有目标设备 |
| `adb reverse tcp:8081 tcp:8081` | 仅使用 localhost Issuer 时；每次模拟器/设备或 ADB 重启后 |
| `./gradlew ... assembleDebug` | Android 代码变化或首次构建 |
| `adb install -r ...` | 第一次安装，或 APK 重新构建后 |

## 7. 安全边界

- Customer Token 只存在 Android App 内存，不进入 URL、WebView 或 H5；
- PKCE verifier 只存在 Partner Backend；
- Partner Token 只存在 Partner Backend；
- WebView Bridge 只注入验签 Manifest 指定的 HTTPS Origin；
- `openPrivacySettings` 打开原生授权列表，可查看和撤销 Partner Consent。

更详细的 Android 说明见 [`superapp-android/README.md`](superapp-android/README.md)。
