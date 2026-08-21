# Partner Backend Go（无 SDK）

这个服务不依赖 `superapp-embed-go-sdk`。它直接实现 SuperApp Embed SSO 的服务端协议，并向
Partner H5 提供三个同源 API。

## API

| API | 作用 |
| --- | --- |
| `GET /api/sso/session` | 验证 Partner Session；必要时刷新 Token 并重新读取 UserInfo |
| `POST /api/sso/bootstrap` | 创建一次性 SSO 事务，返回 `state` 和 PKCE challenge |
| `POST /api/sso/complete` | 原子消费事务，用 Code 和 verifier 换 Token，查询 UserInfo 并建立 Session |

H5 只会看到事务公开数据、一次性 Authorization Code 和可以展示的用户资料。Partner 私钥、
PKCE verifier、Access Token 和 Refresh Token 始终留在服务端。

## 代码分层

| 目录 | 职责 |
| --- | --- |
| `internal/server` | HTTP API、Scope 校验、Session 恢复和安全响应 |
| `internal/sso` | 随机事务 ID、`state`、PKCE S256、浏览器 Session 绑定和一次性消费 |
| `internal/superapp` | ES256 `private_key_jwt`、Token、Refresh、Revoke 和 UserInfo HTTP 协议 |
| `internal/session` | Demo 的 Partner Session Cookie、Session ID 轮换和滑动过期 |
| `internal/config` | 环境变量、Origin、Scope、TTL 和私钥路径配置 |

## 完成登录时发生什么

```text
POST /api/sso/bootstrap
  1. 读取或创建 Partner 浏览器 Session
  2. 生成 transaction_id、state、code_verifier
  3. 计算 code_challenge = BASE64URL(SHA256(code_verifier))
  4. 只向 H5 返回 challenge，不返回 verifier

POST /api/sso/complete
  1. 原子取出并删除 transaction
  2. 常量时间校验 state 和浏览器 Session binding
  3. 生成 120 秒 ES256 private_key_jwt
  4. 使用 code + verifier + client assertion 请求 Token Endpoint
  5. 严格校验 Token 字段、Scope 和 open_id
  6. 使用 Access Token 请求 UserInfo
  7. 校验 Token open_id 与 UserInfo open_id 一致
  8. 轮换 Partner Session ID并在服务端保存凭证
```

每次 Token 或 Revoke 请求都会生成带新 `jti` 的 Client Assertion。服务不会把 Assertion、Code、
Token、verifier 或私钥写入 H5 响应。

## 本地验证

```bash
go test ./...
go vet ./...
```

服务启动仍沿用仓库根目录 README 中的环境变量和命令。

## 生产化提醒

当前 Demo 的 SSO Transaction 和 Partner Session 都保存在单进程内存中。生产多实例部署必须：

- 把 Transaction 放入加密的共享存储；
- 使用原子“读取并删除”操作确保只能消费一次；
- 把 Partner Session 放入可靠的共享 Session Store；
- 在 KMS、HSM 或 Secret Manager 中保护 Partner 私钥；
- 串行化旋转式 Refresh Token 的使用；
- 增加密钥轮换、审计、限流、监控和凭证撤销处理。
