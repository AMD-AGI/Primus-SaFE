# SaFE API `/auth/verify` Endpoint Bug Report

## 问题描述 / Issue Description

`/api/v1/auth/verify` 接口无法正确验证SSO用户的token，即使token有效也返回401 Unauthorized错误。

The `/api/v1/auth/verify` endpoint fails to validate valid SSO tokens, returning 401 Unauthorized even when the token is valid.

## 影响范围 / Impact

- 影响所有通过SSO登录的用户
- 导致外部服务无法通过 `/auth/verify` 接口验证SSO用户
- Affects all users logged in via SSO
- Prevents external services from validating SSO users via the `/auth/verify` endpoint

## 根本原因 / Root Cause

Cookie名称大小写不一致：

**登录时设置的Cookie名称**（`apiserver/pkg/handlers/custom-handlers/user.go:473`）：
```go
c.SetCookie(common.UserType, string(userType), maxAge, "/", domain, false, true)
// common.UserType = "userType" (小写 u 小写 t)
```

**verify接口期望的Cookie名称**（`apiserver/pkg/handlers/authority/verify_handler.go:180`）：
```go
case CookieUserType:  // CookieUserType = "UserType" (大写 U 大写 T)
    userType = value
```

**对比正常认证流程**（`apiserver/pkg/handlers/authority/token_handler.go:81`）：
```go
userType, _ := c.Cookie(common.UserType)  // 正确使用小写 "userType"
```

### 问题流程 / Issue Flow

1. 用户SSO登录成功，浏览器收到Cookie: `Token=xxx; userId=xxx; userType=sso` (小写)
2. 外部服务调用 `/auth/verify`，传递完整cookie字符串
3. `parseCookieString()` 函数查找 `UserType` (大写) → 找不到
4. `userType` 变量为空字符串
5. 因为 `userType != "sso"`，系统使用 `DefaultTokenInstance()` 而不是 `SSOInstance()`
6. `DefaultTokenInstance()` 无法验证SSO token → 返回 "invalid token"

## 复现步骤 / Reproduction Steps

### 失败的请求 / Failed Request (original cookie)

```bash
curl -X POST "http://10.32.80.101:32495/api/v1/auth/verify" \
  -H "Content-Type: application/json" \
  -H "X-Internal-Token: <internal-token>" \
  -d '{
  "cookie": "Token=2910ea8c-6966-40b1-a310-af21efd49047; userId=5200ba27717df4828353ad51eac2dbd6; userType=sso"
}'
```

**响应 / Response:**
```json
{
  "code": 401,
  "message": "The user's token is invalid, please login first"
}
```

**日志 / Logs:**
```
E1224 06:06:12.646223  9 verify_handler.go:139] "failed to validate user token" err="invalid token"
```

### 成功的请求 / Successful Request (workaround)

将 `userType` 改为 `UserType`（大写）：

```bash
curl -X POST "http://10.32.80.101:32495/api/v1/auth/verify" \
  -H "Content-Type: application/json" \
  -H "X-Internal-Token: <internal-token>" \
  -d '{
  "cookie": "Token=2910ea8c-6966-40b1-a310-af21efd49047; userId=5200ba27717df4828353ad51eac2dbd6; UserType=sso"
}'
```

**响应 / Response:**
```json
{
  "code": 0,
  "data": {
    "id": "5200ba27717df4828353ad51eac2dbd6",
    "name": "Kong, Haishuo",
    "email": "Haishuo.Kong@amd.com",
    "exp": 1766558439,
    "type": "sso"
  }
}
```

✅ **验证成功！** / **Verification Successful!**

## 建议的修复方案 / Proposed Solutions

### 方案1：修改 verify_handler.go（推荐）/ Option 1: Fix verify_handler.go (Recommended)

在 `parseCookieString()` 函数中同时支持两种命名：

```go
// apiserver/pkg/handlers/authority/verify_handler.go:177-183
switch key {
case CookieToken:
    token = value
case CookieUserType:
    userType = value
case "userType":  // 兼容登录时设置的小写cookie名称
    if userType == "" {
        userType = value
    }
}
```

**优点 / Pros:**
- 向后兼容，不影响现有API使用者
- 修改范围最小
- Backward compatible
- Minimal code change

### 方案2：统一使用大写 UserType / Option 2: Unify to uppercase UserType

修改 `user.go:473` 使用 `authority.CookieUserType` 而不是 `common.UserType`。

**缺点 / Cons:**
- 可能影响现有使用小写cookie的客户端
- 需要更多测试验证
- May break existing clients expecting lowercase
- Requires more extensive testing

### 方案3：统一使用小写 userType / Option 3: Unify to lowercase userType

修改 `verify_handler.go` 和 `types.go` 中的 `CookieUserType` 定义。

**缺点 / Cons:**
- 与 `CookieToken` 命名风格不一致
- Inconsistent with `CookieToken` naming convention

## 临时解决方案 / Temporary Workaround

在调用 `/auth/verify` 的客户端代码中，在发送前将cookie字符串中的 `userType=` 替换为 `UserType=`：

```python
# Client-side workaround
normalized_cookie = cookie.replace("userType=", "UserType=")
```

## 相关文件 / Related Files

- `apiserver/pkg/handlers/authority/verify_handler.go:177-183`
- `apiserver/pkg/handlers/authority/types.go:9-10`
- `apiserver/pkg/handlers/custom-handlers/user.go:473`
- `apiserver/pkg/handlers/authority/token_handler.go:81`
- `common/pkg/common/constant.go:72`

## 测试环境 / Test Environment

- **Cluster:** tw-proj2
- **Namespace:** primus-safe
- **Image Version:** `harbor.tw325.primus-safe.amd.com/proxy/primussafe/apiserver:202512241022`
- **Pods:** 
  - `primus-safe-apiserver-6695564546-nkt54`
  - `primus-safe-apiserver-6695564546-spt54`

## 发现时间 / Discovery Date

2025-12-24

## 报告人 / Reported By

Lens Chat Integration Team

