# SuperApp Embed SSO 端到端 Demo

本分支用三个彼此隔离的目录演示完整 SSO：

- `superapp-android`：原生 Kotlin/Compose SuperApp 客户端；
- `partner-h5-js`：使用 `@superapp/embed-sdk v0.0.1` 的真实 H5 页面；
- `partner-backend-go`：使用 `superapp-embed-go-sdk v0.0.2` 的 Partner Backend。

最终路径是：Customer 登录 Android App → 点击金刚位 → App 获取并验签 Launch
Manifest → H5 请求 Native Bridge 授权 → Go Backend 兑换 Code、查询 UserInfo → H5
展示昵称、OpenID、手机号、邮箱和 KYC 状态。

如果 Partner 不引入 JavaScript SDK 和服务端 SDK，可直接参考
[`SuperApp Embed SSO 三方接入指南（无 SDK 版）`](docs/superapp-embed-sso-partner-integration-no-sdk.md)，
按照 Native Bridge、PKCE、`private_key_jwt` 和 OAuth Endpoint 契约自行实现。

## 1. 一次性配置

除非代码块另有说明，下面的项目命令都从仓库根目录执行：

```bash
cd /path/to/superapp-h5-sso-demo
```

SuperApp Backend 运行在 `http://localhost:8080`。Embed Client 使用：

```text
embcli_01KZAW57KCCWDTV9QXSPEZV8A7
```

SuperApp Backend 中该 Client 的 `origin` 和 `launch_url` 必须配置为 Partner H5 的公网
HTTPS 地址，例如：

```text
origin:     https://<your-domain>
launch_url: https://<your-domain>/app
```

Android App 不写死 H5 地址；它只接受 SuperApp Backend 返回并通过 ES256 验签的地址。

复制环境变量模板并确认私钥已放入被 Git 忽略的 `secrets/`：

```bash
cp .env.example .env
test -f secrets/partner-es256-private.pem
```

### 1.1 Android 命令行环境

下面三个 `export` 只对当前 Shell 有效。可以每次打开新终端后执行，也可以一次性写入
`~/.zshrc`，以后不再重复输入：

```bash
export JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export PATH="$ANDROID_HOME/platform-tools:$PATH"
```

写入 `~/.zshrc` 后，重新打开终端，或者执行一次：

```bash
source ~/.zshrc
```

确认工具可用：

```bash
java -version
adb version
android emulator list
```

这里的 `JAVA_HOME` 主要供 Gradle 构建 Android 使用，`ANDROID_HOME` 和 `PATH` 让当前
终端可以直接调用 `adb`。如果本机安装路径不同，应使用本机实际路径。

## 2. 第一次构建

### 2.1 Partner H5

`npm install` 只需在第一次使用或依赖变化时执行：

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

构建只会生成 APK，不会自动安装到模拟器。第一次安装或 Android 代码重新构建后执行：

```bash
cd /path/to/superapp-h5-sso-demo/superapp-android
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

`-r` 表示覆盖安装并保留 App 数据。安装会重启 App 进程，因此只存在内存中的 Customer
Token 会丢失，需要重新登录；SuperApp Backend 中的 Consent 不会因此丢失。

## 3. 日常启动

下面是已经完成第一次构建后的日常联调顺序。

### 3.1 启动 SuperApp Backend

先确认 SuperApp Backend 在电脑的 `8080` 端口运行：

```bash
curl --fail-with-body --silent http://localhost:8080/healthz
```

预期输出：

```text
ok
```

### 3.2 启动 Partner Go Backend

Partner Go Backend 不会自动读取 `.env`，所以每个新终端都要先把 `.env` 加载到当前进程
环境。如果一直使用同一个终端，则不需要重复加载：

```bash
cd /path/to/superapp-h5-sso-demo
set -a
source .env
set +a
cd partner-backend-go
go run ./cmd/server
```

Partner 服务监听 `http://localhost:3000`。可在另一个终端验证：

```bash
curl --fail-with-body --silent http://localhost:3000/healthz | jq .
```

### 3.3 启动 ngrok

另开终端，把 Partner 服务暴露为 SuperApp Embed Client 已登记的公网 HTTPS Origin：

```bash
ngrok http --url=<your-domain>.ngrok-free.dev 3000
```

ngrok 已经运行时不需要重复启动。验证公网地址：

```bash
curl --noproxy '*' --fail-with-body --silent \
  https://<your-domain>.ngrok-free.dev/healthz | jq .
```

固定域名必须和 SuperApp Backend 中 Embed Client 的 `origin`、`launch_url` 完全一致。

### 3.4 启动 Android 模拟器

如果 `adb devices` 已经显示正在运行的设备，就不需要再次启动模拟器：

```bash
adb devices
```

没有设备时启动：

```bash
android emulator start medium_phone
```

如果模拟器快照出现黑屏、网络栈异常或 TLS 连接反复关闭，可以做一次不清数据的冷启动：

```bash
android emulator stop medium_phone
android emulator start --cold medium_phone
```

### 3.5 建立端口映射

Android 模拟器或 USB 设备里的 `localhost` 指向设备自身，因此先把设备的 8080 端口
反向映射到电脑上的 SuperApp Backend。这个映射不会跨模拟器重启保留，所以每次启动模拟器
后都应重新执行；重复执行是安全的：

```bash
adb reverse tcp:8080 tcp:8080
adb reverse --list
```

预期列表中包含：

```text
tcp:8080 tcp:8080
```

### 3.6 启动已经安装的 App

如果 APK 已经安装且 Android 代码没有变化，不需要重新构建或安装，直接启动：

```bash
adb shell am start -W -n \
  com.jiguangliandong.superapp.embeddemo.debug/com.jiguangliandong.superapp.embeddemo.MainActivity
```

需要重新安装 APK 的情况：

- 第一次安装；
- 修改了 `superapp-android/` 代码并重新构建；
- 当前模拟器中的 APK 不是最新版本。

只修改 `partner-h5-js/` 或 `partner-backend-go/` 时，不需要重新安装 APK。

## 4. 联调行为

使用 Customer 测试账号登录，然后点击“Partner SSO Demo”金刚位。第一次申请资料会展示
原生授权弹窗；同意后 H5 切换到独立的用户信息界面。再次打开时：

- Partner Session 有效：直接显示用户信息；
- Partner Session 丢失但 SuperApp Consent 有效：自动走一次授权码流程，静默恢复登录；
- Consent 不存在、已撤销、已过期、版本变化或新增 Scope：再次展示原生授权界面。

H5 不使用 LocalStorage 判断用户是否已授权，最终状态始终由 Partner Session 和 SuperApp
Backend 中的 Consent 决定。每次打开仍会使用新的短期 Launch Manifest；需要重建 Partner
Session 时也会使用新的一次性 Authorization Code。

Demo 的 `PARTNER_SESSION_TTL_SECONDS` 默认是 43200 秒（12 小时滑动过期）。Access Token
到期前由 Partner Backend 使用 Go SDK 和 Refresh Token 刷新；刷新或 UserInfo 校验发现
授权已撤销时，Partner Session 会立即失效。当前 SuperApp Backend 的 Consent 默认无固定
期限，持续到用户撤销、Partner 提升 Consent Version 或申请新增 Scope。生产环境可根据数据
敏感度在 SuperApp 侧设置例如 180 天的 Consent 期限；Consent 期限由 SuperApp 的授权策略
决定，不能由 Partner H5 自行延长。

## 5. 代理与模拟器排障

如果 macOS 正在使用 WandaCloud 或其他本机代理，而模拟器访问 ngrok 报
`ERR_CONNECTION_CLOSED`，可以让模拟器的公网请求使用宿主机代理，同时让
`localhost:8080` 继续通过 `adb reverse` 直连：

```bash
adb shell settings put global http_proxy 10.0.2.2:7893
adb shell settings put global global_http_proxy_exclusion_list 'localhost,127.0.0.1'
```

其中 `10.0.2.2` 是 Android Emulator 访问宿主机的特殊地址，`7893` 应替换为本机代理实际
端口。检查设置：

```bash
adb shell settings get global http_proxy
adb shell settings get global global_http_proxy_exclusion_list
```

不再需要代理时恢复：

```bash
adb shell settings put global http_proxy :0
adb shell settings delete global global_http_proxy_exclusion_list
```

模拟器窗口全黑但 `adb devices` 仍显示在线时，可以唤醒并解锁：

```bash
adb shell input keyevent KEYCODE_WAKEUP
adb shell wm dismiss-keyguard
```

## 6. 哪些操作需要重复

| 操作 | 什么时候执行 |
| --- | --- |
| 配置 `JAVA_HOME`、`ANDROID_HOME`、`PATH` | 每个新 Shell；写入 `~/.zshrc` 后无需手动重复 |
| `npm install` | 第一次或 H5 依赖变化 |
| `npm run build` | H5 代码变化 |
| 加载根目录 `.env` | 每次在新 Shell 启动 Partner Go Backend |
| 启动 Go Backend | 进程未运行，或 Go/H5 页面代码变化 |
| 启动 ngrok | 隧道未运行 |
| 启动模拟器 | `adb devices` 没有目标设备 |
| `adb reverse tcp:8080 tcp:8080` | 每次模拟器/设备或 ADB 重启后 |
| `./gradlew ... assembleDebug` | Android 代码变化或首次构建 |
| `adb install -r ...` | 第一次安装，或 APK 重新构建后 |
| `adb shell am start ...` | App 未运行或需要重新打开 |

## 7. 安全边界

- Customer Token 只存在 Android App 内存，不进入 URL、WebView 或 H5；
- PKCE verifier 只存在 Partner Backend；
- Partner Token 只存在 Partner Backend；
- WebView Bridge 只注入验签 Manifest 指定的 HTTPS Origin；
- release 包禁止明文流量，debug 包只为 `localhost` 联调开放；
- `openPrivacySettings` 打开原生授权列表，可查看和撤销 Partner Consent；
- 撤销 Consent 会同步使 Access/Refresh Token 失效，并由 Android 清理对应 Partner Cookie
  与 Web Storage，所以下次进入必须重新授权；
- 当前 Partner Session 与 SSO 事务是内存实现，服务重启后失效，适合联调 Demo。

更详细的 Android 说明见 [`superapp-android/README.md`](superapp-android/README.md)。
