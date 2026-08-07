# SuperApp Android Client

原生 Kotlin + Jetpack Compose 联调客户端。它不是任意 URL 浏览器：点击金刚位时先向
SuperApp Backend 获取短期 Launch Manifest，使用 JWKS 验证 ES256 签名及全部关键字段，
验证通过后才创建受控 WebView。

## 已实现

- Customer 手机号/邮箱和密码登录，Token 仅保存在进程内存；
- 金刚位入口与短期 Launch Manifest；
- ES256、`kid`、P-256、issuer、audience、subject、时效和外层字段一致性校验；
- 精确 Origin 导航限制和 document-start Promise Bridge；
- `getContext`、`getAuthCode`、`openPrivacySettings`、`close`；
- PKCE、State、Client ID、Scope、前台、锁屏、时效和单并发授权校验；
- 首次/增量 Scope 的原生 Consent 弹窗；
- 原生授权列表、隐私政策入口和撤销授权；
- 撤销或退出登录后清理当前 Demo 的 WebView Cookie 与 Web Storage。

## 本地配置

联调常量在 `app/build.gradle.kts`：

```kotlin
SUPERAPP_BASE_URL = "http://localhost:8080"
EMBED_ISSUER = "http://localhost:8080"
EMBED_JWKS_URL = "http://localhost:8080/.well-known/jwks.json"
EMBED_CLIENT_ID = "embcli_01KZAW57KCCWDTV9QXSPEZV8A7"
```

模拟器或 USB 设备联调前执行：

```bash
adb reverse tcp:8080 tcp:8080
```

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
