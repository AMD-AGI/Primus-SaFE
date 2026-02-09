# Tools API 统一错误响应格式

本文档定义了 Tools API 的统一错误响应格式，方便前端进行统一的错误处理。

---

## 📐 错误响应结构

### TypeScript 类型定义

```typescript
interface ErrorResponse {
  errorCode: string;     // 错误码，用于程序判断
  errorMessage: string;  // 错误消息，可直接展示或用于日志
}
```

### 响应示例

```json
{
  "errorCode": "INVALID_PARAMETER",
  "errorMessage": "Invalid parameter: id. ID must be a valid integer"
}
```

---

## 🔢 错误码列表

### 通用错误 (4xx)

| HTTP Status | `errorCode`             | 说明                     |
|-------------|-------------------------|------------------------|
| `400`       | `BAD_REQUEST`           | 请求格式错误              |
| `400`       | `INVALID_PARAMETER`     | 参数验证失败              |
| `401`       | `UNAUTHORIZED`          | 未授权/未登录            |
| `403`       | `FORBIDDEN`             | 权限不足                 |
| `404`       | `NOT_FOUND`             | 资源不存在               |
| `409`       | `CONFLICT`              | 资源冲突                 |
| `413`       | `PAYLOAD_TOO_LARGE`     | 请求体过大               |

### Tool 业务错误

| HTTP Status | `errorCode`             | 说明                     |
|-------------|-------------------------|------------------------|
| `404`       | `TOOL_NOT_FOUND`        | Tool 不存在              |
| `409`       | `TOOL_ALREADY_LIKED`    | 已经点赞过               |
| `403`       | `ACCESS_DENIED`         | 无权访问私有 Tool        |

### 文件上传错误

| HTTP Status | `errorCode`             | 说明                          |
|-------------|-------------------------|------------------------------|
| `400`       | `FILE_REQUIRED`         | 缺少文件                      |
| `400`       | `FILE_TOO_LARGE`        | 文件过大（超过 2MB）           |
| `400`       | `INVALID_FILE_TYPE`     | 文件类型不支持                |

### Skill 导入错误

| HTTP Status | `errorCode`             | 说明                          |
|-------------|-------------------------|------------------------------|
| `400`       | `SELECTION_EMPTY`       | 未选择任何 Skill              |
| `400`       | `MISSING_FILE_OR_URL`   | 缺少文件或 GitHub URL         |
| `400`       | `BOTH_FILE_AND_URL`     | 同时提供了文件和 GitHub URL   |

### 搜索错误

| HTTP Status | `errorCode`             | 说明                          |
|-------------|-------------------------|------------------------------|
| `400`       | `QUERY_REQUIRED`        | 缺少搜索关键词                |
| `400`       | `INVALID_SEARCH_MODE`   | 不支持的搜索模式              |

### 服务端错误 (5xx)

| HTTP Status | `errorCode`             | 说明                     |
|-------------|-------------------------|------------------------|
| `500`       | `INTERNAL_ERROR`        | 服务器内部错误           |
| `503`       | `SERVICE_NOT_CONFIGURED`| 服务未配置               |
| `503`       | `SERVICE_UNAVAILABLE`   | 服务不可用               |

---

## 📝 API 示例

### 示例 1: 创建 MCP Tool (参数错误)

**请求:**
```bash
POST /api/tools/mcp
Content-Type: application/json

{
  "description": "A test tool"
  # 缺少 name 字段
}
```

**响应 (400):**
```json
{
  "errorCode": "BAD_REQUEST",
  "errorMessage": "Invalid request body: Key: 'CreateMCPRequest.Name' Error:Field validation for 'Name' failed on the 'required' tag"
}
```

---

### 示例 2: 更新私有 Tool (权限不足)

**请求:**
```bash
PUT /api/tools/123
```

**响应 (403):**
```json
{
  "errorCode": "ACCESS_DENIED",
  "errorMessage": "Access denied"
}
```

---

### 示例 3: 点赞已点赞的 Tool

**请求:**
```bash
POST /api/tools/456/like
```

**响应 (409):**
```json
{
  "errorCode": "TOOL_ALREADY_LIKED",
  "errorMessage": "Tool already liked"
}
```

---

### 示例 4: 上传图标 (文件过大)

**请求:**
```bash
POST /api/tools/icon
Content-Type: multipart/form-data

file: (3MB 图片)
```

**响应 (400):**
```json
{
  "errorCode": "FILE_TOO_LARGE",
  "errorMessage": "File size exceeds 2MB limit"
}
```

---

### 示例 5: 搜索 Tool (参数错误)

**请求:**
```bash
GET /api/tools?mode=fuzzy
```

**响应 (400):**
```json
{
  "errorCode": "INVALID_SEARCH_MODE",
  "errorMessage": "Invalid search mode. Supported modes: keyword, semantic, hybrid"
}
```

---

## 🎯 前端集成指南

### 1. 定义类型

```typescript
// types/api.ts
export type ErrorCode =
  | 'BAD_REQUEST'
  | 'INVALID_PARAMETER'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'CONFLICT'
  | 'PAYLOAD_TOO_LARGE'
  | 'TOOL_NOT_FOUND'
  | 'TOOL_ALREADY_LIKED'
  | 'ACCESS_DENIED'
  | 'FILE_REQUIRED'
  | 'FILE_TOO_LARGE'
  | 'INVALID_FILE_TYPE'
  | 'SELECTION_EMPTY'
  | 'MISSING_FILE_OR_URL'
  | 'BOTH_FILE_AND_URL'
  | 'QUERY_REQUIRED'
  | 'INVALID_SEARCH_MODE'
  | 'INTERNAL_ERROR'
  | 'SERVICE_NOT_CONFIGURED'
  | 'SERVICE_UNAVAILABLE';

export interface ErrorResponse {
  errorCode: ErrorCode;
  errorMessage: string;
}
```

---

### 2. Axios 全局拦截器

```typescript
// utils/axios.ts
import axios from 'axios';
import { toast } from 'sonner'; // 或其他 Toast 库
import { router } from '@/router';

// 错误码到中文提示的映射
const ERROR_MESSAGES: Record<string, string> = {
  BAD_REQUEST: '请求格式错误',
  INVALID_PARAMETER: '参数错误，请检查输入',
  UNAUTHORIZED: '请先登录',
  FORBIDDEN: '您没有权限执行此操作',
  NOT_FOUND: '资源不存在',
  TOOL_NOT_FOUND: 'Tool 不存在',
  TOOL_ALREADY_LIKED: '您已经点赞过了',
  ACCESS_DENIED: '无权访问该 Tool',
  FILE_REQUIRED: '请选择文件',
  FILE_TOO_LARGE: '文件过大，最大支持 2MB',
  INVALID_FILE_TYPE: '文件类型不支持，仅支持 png/jpg/svg/webp',
  SELECTION_EMPTY: '请至少选择一个 Skill',
  INVALID_SEARCH_MODE: '搜索模式不支持',
  INTERNAL_ERROR: '服务器错误，请稍后重试',
  SERVICE_NOT_CONFIGURED: '服务暂不可用',
};

const apiClient = axios.create({
  baseURL: '/api',
  timeout: 30000,
});

// 响应拦截器
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    const errorData = error.response?.data;

    if (errorData?.errorCode) {
      const { errorCode, errorMessage } = errorData;

      // 优先使用预定义的中文提示，否则使用服务端返回的消息
      const userMessage = ERROR_MESSAGES[errorCode] || errorMessage;

      // 显示错误提示
      toast.error(userMessage);

      // 开发环境下打印详细信息
      if (import.meta.env.DEV) {
        console.error(`[API Error] ${errorCode}:`, errorMessage);
      }

      // 特殊处理：未授权跳转登录
      if (errorCode === 'UNAUTHORIZED') {
        router.push('/login');
      }
    } else {
      // 非标准错误响应
      toast.error('网络请求失败，请稍后重试');
    }

    return Promise.reject(error);
  }
);

export default apiClient;
```

---

### 3. React Query 使用示例

```typescript
import { useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import type { ErrorResponse } from '@/types/api';

interface CreateToolData {
  name: string;
  description: string;
  // ...
}

const useCreateTool = () => {
  return useMutation({
    mutationFn: (data: CreateToolData) => 
      apiClient.post('/tools/mcp', data),
    
    onSuccess: () => {
      toast.success('创建成功');
    },
    
    onError: (error: any) => {
      const errorCode = error.response?.data?.errorCode;
      const errorMessage = error.response?.data?.errorMessage;

      // 可以根据错误码进行特殊处理
      if (errorCode === 'TOOL_ALREADY_LIKED') {
        toast.info('您已经点赞过了');
        return;
      }

      // 全局拦截器已经显示了通用提示
      // 这里可以做额外的业务逻辑处理
      console.error('创建失败:', errorMessage);
    },
  });
};

export default useCreateTool;
```

---

### 4. 错误处理 Hook (可选)

```typescript
// hooks/useApiError.ts
import { useCallback } from 'react';
import { toast } from 'sonner';
import { useNavigate } from 'react-router-dom';
import type { ErrorResponse, ErrorCode } from '@/types/api';

export const useApiError = () => {
  const navigate = useNavigate();

  const handleError = useCallback((error: any) => {
    const errorData: ErrorResponse | undefined = error.response?.data;

    if (!errorData?.errorCode) {
      toast.error('网络错误，请稍后重试');
      return;
    }

    const { errorCode, errorMessage } = errorData;

    // 自定义错误处理
    const handlers: Partial<Record<ErrorCode, () => void>> = {
      UNAUTHORIZED: () => {
        toast.error('请先登录');
        navigate('/login');
      },
      ACCESS_DENIED: () => {
        toast.error('您没有权限访问该资源');
      },
      TOOL_ALREADY_LIKED: () => {
        toast.info('您已点赞过此工具');
      },
      FILE_TOO_LARGE: () => {
        toast.error('文件过大，最大支持 2MB');
      },
      INVALID_FILE_TYPE: () => {
        toast.error('仅支持 png/jpg/svg/webp 格式');
      },
    };

    const handler = handlers[errorCode];
    if (handler) {
      handler();
    } else {
      toast.error(errorMessage || '操作失败');
    }
  }, [navigate]);

  return { handleError };
};
```

**使用:**
```typescript
const { handleError } = useApiError();

const mutation = useMutation({
  mutationFn: createTool,
  onError: handleError,
});
```

---

## ✅ 最佳实践

### 1. 使用 errorCode 判断错误类型
```typescript
// ✅ 正确
if (error.response?.data?.errorCode === 'UNAUTHORIZED') {
  // 跳转登录
}

// ❌ 错误 - 不要依赖 errorMessage 字符串匹配
if (error.response?.data?.errorMessage?.includes('unauthorized')) {
  // errorMessage 可能会变化
}
```

---

### 2. 提供友好的中文提示
```typescript
// 根据 errorCode 显示用户友好的中文提示
const userMessage = ERROR_MESSAGES[errorCode] || errorMessage;
toast.error(userMessage);
```

---

### 3. 开发环境打印详细信息
```typescript
if (import.meta.env.DEV) {
  console.error(`[${errorCode}]`, errorMessage);
}
```

---

### 4. 特殊错误码的特殊处理
```typescript
switch (errorCode) {
  case 'UNAUTHORIZED':
    // 跳转登录页
    navigate('/login');
    break;
  case 'TOOL_ALREADY_LIKED':
    // 更新 UI 状态，显示已点赞
    setLiked(true);
    break;
  default:
    toast.error(errorMessage);
}
```

---

### 5. 优雅降级处理
```typescript
// 对于未知的 errorCode，显示通用提示
const userMessage = ERROR_MESSAGES[errorCode] || errorMessage || '操作失败';
toast.error(userMessage);
```

---

## 🔄 与旧格式的兼容

如果需要同时支持新旧格式，可以这样处理：

```typescript
const errorCode = error.response?.data?.errorCode;
const errorMessage = error.response?.data?.errorMessage 
  || error.response?.data?.error  // 兼容旧格式
  || '操作失败';

if (errorCode) {
  // 使用新格式处理
  handleNewFormatError(errorCode, errorMessage);
} else {
  // 使用旧格式处理
  toast.error(errorMessage);
}
```

---

## 📞 联系与反馈

如有任何问题或建议，请联系后端团队或在项目中提交 Issue。

---

**版本**: v1.0.0  
**最后更新**: 2026-02-09
