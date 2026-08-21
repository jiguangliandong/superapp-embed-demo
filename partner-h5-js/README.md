# Partner H5 JavaScript（无 SDK）

这个 H5 不依赖 `@superapp/embed-sdk`。它直接实现 SDK 原本封装的浏览器侧协议，但不会把
Partner 私钥、PKCE `code_verifier`、SuperApp Access Token 或 Refresh Token放进浏览器。

## 代码分层

| 文件 | 职责 |
| --- | --- |
| `src/app.js` | 页面状态、自动恢复 Partner Session、登录按钮和用户资料展示 |
| `src/superapp-bridge.js` | 检查并调用 `SuperappNativeBridge`，处理超时、参数和 `state` 校验 |
| `src/partner-api.js` | 调用 Partner Backend 的 `/session`、`/bootstrap`、`/complete` API |
| `src/sso-flow.js` | 显式编排 bootstrap → getAuthCode → complete 三步登录 |
| `src/sso-error.js` | H5 内部统一错误类型和稳定错误码 |

## 浏览器侧流程

```text
GET /api/sso/session
  ├─ 200：直接展示用户资料
  └─ 401 partner_session_missing
       ↓
POST /api/sso/bootstrap
       ↓ transaction_id + state + code_challenge + scopes
SuperappNativeBridge.invoke("getAuthCode", ...)
       ↓ one-time code + state
H5 校验 state
       ↓
POST /api/sso/complete
       ↓ Set-Cookie + Partner Session 用户资料
展示用户信息页
```

`/api/sso/bootstrap` 返回的数据中禁止包含 `code_verifier`。它由 Partner Backend 生成并保存在
服务端事务中，`/api/sso/complete` 根据 `transaction_id` 取回并消费。

## 本地验证

```bash
npm install
npm test
npm run build
```

构建产物位于 `dist/`，由 `partner-backend-go` 以同源方式提供。修改 H5 后需要重新构建并重启
Go 服务，因为 Demo 启动时会把 HTML 读入内存。
