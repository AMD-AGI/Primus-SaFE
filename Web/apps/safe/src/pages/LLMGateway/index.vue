<template>
  <el-text class="block textx-18 font-500" tag="b">LLM Gateway</el-text>
  <p class="mt-2 text-gray-500 text-sm">
    Manage your Azure APIM Key binding to enable LLM services.
  </p>

  <el-card class="mt-4 safe-card gateway-card" shadow="never" v-loading="pageLoading">
    <div class="gateway-center">
      <div class="gateway-content">
        <!-- Bound state -->
        <template v-if="binding?.has_apim_key">
          <div class="status-banner status-bound">
            <el-icon :size="20"><CircleCheckFilled /></el-icon>
            <span>APIM Key is bound</span>
          </div>

          <el-descriptions :column="2" border class="mt-6">
            <el-descriptions-item label="Email">
              {{ binding.user_email }}
            </el-descriptions-item>
            <el-descriptions-item label="Key Alias">
              {{ binding.key_alias || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Created At">
              {{ formatTimeStr(binding.created_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Updated At">
              {{ formatTimeStr(binding.updated_at) }}
            </el-descriptions-item>
          </el-descriptions>

          <el-divider />

          <el-text class="block font-500 mb-4" tag="b">Update APIM Key</el-text>
          <div class="key-input-row">
            <el-input
              v-model="apimKeyInput"
              placeholder="Enter new APIM Key"
              show-password
              clearable
              class="key-input"
            />
            <el-button
              type="primary"
              :loading="submitLoading"
              :disabled="!apimKeyInput.trim()"
              @click="handleUpdate"
            >
              Update
            </el-button>
          </div>

          <el-divider />

          <el-text class="block font-500 mb-2" tag="b">Usage</el-text>
          <el-text class="block mb-4 text-sm text-gray-500">
            Use any SaFE API Key to call the LLM:
          </el-text>
          <div class="code-block">
            <pre><code>from openai import OpenAI

client = OpenAI(
    api_key="ak-&lt;your-safe-key&gt;",
    base_url="{{ llmGatewayBaseUrl }}"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)</code></pre>
          </div>
        </template>

        <!-- Unbound / error fallback state -->
        <template v-else-if="!pageLoading">
          <div v-if="binding" class="status-banner status-unbound">
            <el-icon :size="20"><WarningFilled /></el-icon>
            <span>APIM Key is not bound</span>
          </div>

          <el-descriptions v-if="binding" :column="1" border class="mt-6">
            <el-descriptions-item label="Email">
              {{ binding.user_email }}
            </el-descriptions-item>
          </el-descriptions>

          <el-empty v-else description="Unable to load binding status" :image-size="80" />

          <el-divider />

          <el-text class="block font-500 mb-4" tag="b">Bind APIM Key</el-text>
          <el-text class="block mb-4 text-sm text-gray-500">
            Please upload your Azure APIM Subscription Key to enable LLM services.
          </el-text>
          <div class="key-input-row">
            <el-input
              v-model="apimKeyInput"
              placeholder="Enter your APIM Key"
              show-password
              clearable
              class="key-input"
            />
            <el-button
              type="primary"
              :loading="submitLoading"
              :disabled="!apimKeyInput.trim()"
              @click="handleBind"
            >
              Bind
            </el-button>
          </div>
        </template>
      </div>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import {
  getLLMGatewayBinding,
  createLLMGatewayBinding,
  updateLLMGatewayBinding,
} from '@/services'
import type { LLMGatewayBinding } from '@/services'
import { formatTimeStr } from '@/utils/index'
import { ElMessage } from 'element-plus'
import { CircleCheckFilled, WarningFilled } from '@element-plus/icons-vue'

defineOptions({ name: 'LLMGatewayPage' })

const pageLoading = ref(false)
const submitLoading = ref(false)
const binding = ref<LLMGatewayBinding | null>(null)
const apimKeyInput = ref('')

const llmGatewayBaseUrl = `${location.origin}/llm-gateway/v1`

const fetchBinding = async () => {
  try {
    pageLoading.value = true
    binding.value = await getLLMGatewayBinding()
  } catch {
    binding.value = null
  } finally {
    pageLoading.value = false
  }
}

const handleBind = async () => {
  const key = apimKeyInput.value.trim()
  if (!key) return

  try {
    submitLoading.value = true
    await createLLMGatewayBinding({ apim_key: key })
    ElMessage.success('APIM Key bound successfully')
    apimKeyInput.value = ''
    await fetchBinding()
  } catch (err: unknown) {
    if (typeof err === 'string' && err.includes('already exists')) {
      ElMessage.warning('Already bound. Please use the Update function.')
    }
  } finally {
    submitLoading.value = false
  }
}

const handleUpdate = async () => {
  const key = apimKeyInput.value.trim()
  if (!key) return

  try {
    submitLoading.value = true
    await updateLLMGatewayBinding({ apim_key: key })
    ElMessage.success('APIM Key updated successfully')
    apimKeyInput.value = ''
    await fetchBinding()
  } catch (err: unknown) {
    if (typeof err === 'string' && err.includes('no binding found')) {
      ElMessage.warning('Not bound yet. Please bind first.')
    }
  } finally {
    submitLoading.value = false
  }
}

onMounted(() => {
  fetchBinding()
})
</script>

<style scoped>
.gateway-card {
  min-height: calc(100vh - 130px);
}
.gateway-card :deep(.el-card__body) {
  height: 100%;
  display: flex;
}
.gateway-center {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 0;
}
.gateway-content {
  width: 100%;
  max-width: 720px;
}
.status-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 20px;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 500;
}
.status-bound {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
}
.status-unbound {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning-dark-2);
}
.key-input-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.key-input {
  flex: 1;
  max-width: 480px;
}
.code-block {
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  overflow-x: auto;
}
.code-block pre {
  margin: 0;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
}
</style>
