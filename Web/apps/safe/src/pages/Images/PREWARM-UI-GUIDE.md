# Prewarm（预热）前端对接指南

> 后端分支：`feature/chenyi/preheat2.0`

## 一、背景

后端对 Prewarm 功能做了增强：
1. 修复了大规模集群（如 172 节点）prewarm 虚假成功的 bug
2. 进度更新频率从 60s 提升到 10s
3. 列表 API 新增 `nodesReady`、`nodesTotal`、`jobName` 字段
4. 新增节点详情 API，可查看每个节点的预热状态

## 二、API 变更

### 2.1 列表 API（已有，字段新增）

`GET /api/v1/images/prewarm`

**新增字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `jobName` | string | OpsJob 名称，用于调用节点详情 API |
| `nodesReady` | string | 已完成节点数，如 `"3"` |
| `nodesTotal` | string | 总节点数，如 `"172"` |

原有字段（`imageName`、`workspaceId`、`workspaceName`、`status`、`prewarmProgress`、`createdTime`、`endTime`、`userName`、`errorMessage`）不变。

**响应示例：**

```json
{
  "totalCount": 1,
  "items": [
    {
      "jobName": "prewarm-202603190722-px6vl",
      "imageName": "harbor.oci-slc.primus-safe.amd.com/sync/tasimage/primus:pr-609",
      "workspaceId": "control-plane-deepseek",
      "workspaceName": "deepseek",
      "status": "Running",
      "prewarmProgress": "25%",
      "nodesReady": "3",
      "nodesTotal": "12",
      "createdTime": "2026-03-19T07:22:23Z",
      "endTime": "",
      "userName": "Zheng, Xiaofei",
      "errorMessage": ""
    }
  ]
}
```

### 2.2 节点详情 API（新增）

`GET /api/v1/images/prewarm/:name/nodes`

- `:name` = 列表 API 返回的 `jobName`
- 运行中和完成/失败后都可调用

**响应示例：**

```json
{
  "nodes": [
    { "node": "uswslocpm2m-106-2019", "status": "Ready" },
    { "node": "uswslocpm2m-106-473",  "status": "Ready" },
    { "node": "uswslocpm2m-106-218",  "status": "Pending" },
    { "node": "uswslocpm2m-106-442",  "status": "Failed", "reason": "ImagePullBackOff: back-off pulling image..." }
  ]
}
```

**status 枚举：**

| status | 含义 | 建议 Tag 颜色 |
|--------|------|---------------|
| `Ready` | 镜像拉取完成 | success (绿) |
| `Running` | Pod 运行中但容器未就绪 | warning (黄) |
| `Pending` | 等待调度或拉镜像中 | info (灰) |
| `Failed` | 失败，`reason` 有详情 | danger (红) |

## 三、前端需要改动的地方

### 3.1 新增 API 函数

文件：`src/services/images/index.ts`

```typescript
export const getPrewarmNodes = (jobName: string): Promise<any> =>
  request.get(`/images/prewarm/${jobName}/nodes`)
```

### 3.2 Progress 列显示具体数量

文件：`src/pages/Images/index.vue`，约第 196-213 行

**当前：**
```html
<span class="text-xs">{{ row.prewarmProgress || '0%' }}</span>
```

**改为：**
```html
<span class="text-xs">
  {{ row.nodesReady || '0' }}/{{ row.nodesTotal || '0' }}
  ({{ row.prewarmProgress || '0%' }})
</span>
```

效果：`3/12 (25%)` → `12/12 (100%)`

### 3.3 新增展开行查看节点详情

在 Preheat 表格的 `<el-table>` 内添加展开列：

```html
<el-table-column type="expand">
  <template #default="{ row }">
    <PrewarmNodeDetail :job-name="row.jobName" />
  </template>
</el-table-column>
```

新建组件 `src/pages/Images/Components/PrewarmNodeDetail.vue`：

```vue
<template>
  <el-card class="safe-card m-4" shadow="never">
    <el-table :data="nodes" v-loading="loading" size="small">
      <el-table-column label="Node" prop="node" min-width="250" show-overflow-tooltip />
      <el-table-column label="Status" prop="status" width="120">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Reason" prop="reason" min-width="300" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.reason || '-' }}
        </template>
      </el-table-column>
    </el-table>
    <div class="mt-2 text-xs text-gray-400">
      Total: {{ nodes.length }} nodes
      | Ready: {{ nodes.filter(n => n.status === 'Ready').length }}
      | Failed: {{ nodes.filter(n => n.status === 'Failed').length }}
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { getPrewarmNodes } from '@/services'
import { ElMessage } from 'element-plus'

const props = defineProps<{ jobName: string }>()

const loading = ref(false)
const nodes = ref<Array<{ node: string; status: string; reason?: string }>>([])

const statusTagType = (status: string) => {
  const map: Record<string, string> = {
    Ready: 'success',
    Running: 'warning',
    Pending: 'info',
    Failed: 'danger',
  }
  return map[status] || 'info'
}

onMounted(async () => {
  loading.value = true
  try {
    const res = await getPrewarmNodes(props.jobName)
    nodes.value = res.nodes || []
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to fetch node details')
  } finally {
    loading.value = false
  }
})
</script>
```

### 3.4 Running 状态自动轮询（可选）

当有任务处于 Running 状态时，每 15 秒自动刷新列表：

```typescript
import { onUnmounted } from 'vue'

const pollTimer = ref<ReturnType<typeof setInterval>>()

watch(() => prewarmState.rowData, (rows) => {
  clearInterval(pollTimer.value)
  if (rows.some((r: any) => r.status === 'Running')) {
    pollTimer.value = setInterval(getPrewarmList, 15000)
  }
}, { immediate: true })

onUnmounted(() => clearInterval(pollTimer.value))
```

### 3.5 导出新 API（如果 services/index.ts 有统一导出）

确保 `getPrewarmNodes` 在 `src/services/index.ts` 中被导出。

## 四、改动量估算

| 文件 | 改动量 | 说明 |
|------|--------|------|
| `services/images/index.ts` | +2 行 | 新增 API 函数 |
| `pages/Images/index.vue` | ~10 行 | Progress 文字 + 展开列 + 轮询 |
| `Components/PrewarmNodeDetail.vue` | ~50 行 | 新建节点详情组件 |

**总计约 60 行。**
