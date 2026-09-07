# SuperApp Android Client

原生 Kotlin + Jetpack Compose 内部 H5 联调客户端。它不是任意 URL 浏览器：点击金刚位时先向
User Center 获取短期 Launch Manifest，使用 JWKS 验证 ES256 签名及全部关键字段，
验证通过后才创建受控 WebView。

## 已实现

- Customer +60 手机号 OTP 登录，Token 仅保存在进程内存；
- 金刚位入口与短期 Launch Manifest；
- ES256、`kid`、P-256、issuer、audience、subject、时效和外层字段一致性校验；
- 精确 Origin 导航限制和 document-start Promise Bridge；
- `getContext`、`getAuthCode`、`openPrivacySettings`、`close`；
- PKCE、State、Client ID、Scope、前台、锁屏、时效和单并发授权校验；
- 首次/增量 Scope 的原生 Consent 弹窗；
- 原生应用数据访问记录、数据说明入口和当前访问清除；
- 撤销或退出登录后清理当前 Demo 的 WebView Cookie 与 Web Storage。

## 本地配置

联调常量在 `app/build.gradle.kts`：

```kotlin
SUPERAPP_BASE_URL = "http://localhost:8081"
EMBED_ISSUER = "http://superapp-dev.jiguang.top"
EMBED_JWKS_URL = "http://localhost:8081/.well-known/jwks.json"
EMBED_CLIENT_ID = "embcli_01M1WVWT0PXG93RD6MN4KN7RF7"
```

User Center 的本地 Docker 端口是宿主机 `9111`。启动或重启模拟器后执行：

```bash
adb reverse tcp:8081 tcp:9111
```

APP 通过 `localhost:8081` 调用 API 和读取 JWKS，但 Launch Manifest 中的 Issuer 仍按
`http://superapp-dev.jiguang.top` 验证。正式环境不使用 ADB 映射，API 和 Issuer 都必须
替换为同一个公网 HTTPS Origin。

真实测试/生产包应把 SuperApp API、Issuer 和 JWKS 全部替换为公网 HTTPS 地址，并保留
release Manifest 的明文流量禁用策略。

## 验证与 APK

```bash
export JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
./gradlew testDebugUnitTest lintDebug assembleDebug
```

APK 输出：

```text
app/build/outputs/apk/debug/app-debug.apk
```
