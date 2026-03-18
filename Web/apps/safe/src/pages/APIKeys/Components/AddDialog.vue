<template>
  <el-dialog
    :model-value="visible"
    title="Create API Key"
    width="600"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
      :rules="rules"
    >
      <el-form-item label="Name" prop="name">
        <el-input
          v-model="form.name"
          maxlength="100"
          show-word-limit
          placeholder="Enter API Key name"
        />
      </el-form-item>

      <el-form-item label="TTL (Days)" prop="ttlDays">
        <el-input-number
          v-model="form.ttlDays"
          :min="1"
          :max="366"
          controls-position="right"
          class="w-full"
        />
        <div class="text-gray-400 text-xs mt-1">Valid period in days (1-366)</div>
      </el-form-item>

      <el-form-item label="IP Whitelist" prop="whitelistText">
        <el-input
          v-model="whitelistText"
          type="textarea"
          :rows="6"
          placeholder="Enter IP addresses or CIDR, one per line&#10;&#10;Example:&#10;192.168.1.100&#10;10.0.0.0/24&#10;2001:db8::1"
          class="w-full"
        />
        <div class="text-gray-400 text-xs mt-1">
          Optional: Leave empty for no restriction. Supports IPv4, IPv6, CIDR format (one per line)
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit(ruleFormRef)">
          Confirm
        </el-button>
      </div>
    </template>
  </el-dialog>

  <!-- Success Dialog with API Key Display -->
  <el-dialog
    v-model="successVisible"
    title="API Key Created Successfully"
    width="600"
    :close-on-click-modal="false"
  >
    <el-alert
      title="Important: Save your API Key"
      type="warning"
      :closable="false"
      show-icon
      class="mb-4"
    >
      <template #default>
        <div class="text-sm">
          This is the only time you will see this API Key. Please save it securely.
        </div>
      </template>
    </el-alert>

    <el-form label-width="auto" class="p-3">
      <el-form-item label="Name">
        <el-text>{{ createdKey?.name }}</el-text>
      </el-form-item>

      <el-form-item label="API Key">
        <div class="flex items-center gap-2 w-full">
          <el-input :model-value="createdKey?.apiKey" readonly class="font-mono">
            <template #append>
              <el-button :icon="CopyDocument" @click="copyApiKey" />
            </template>
          </el-input>
        </div>
      </el-form-item>

      <el-form-item label="Expiration">
        <el-text>{{ formatTimeStr(createdKey?.expirationTime) }}</el-text>
      </el-form-item>

      <el-form-item
        label="Whitelist"
        v-if="createdKey?.whitelist && createdKey.whitelist.length > 0"
      >
        <el-tag v-for="ip in createdKey.whitelist" :key="ip" effect="plain" class="mr-2">{{
          ip
        }}</el-tag>
      </el-form-item>
      <el-form-item label="Whitelist" v-else>
        <el-text type="info">No restriction</el-text>
      </el-form-item>
    </el-form>

    <!-- <template #footer>
      <el-button type="primary" @click="closeSuccessDialog">Close</el-button>
    </template> -->
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, ref, computed, nextTick } from 'vue'
import { createAPIKey } from '@/services/apikeys'
import type { CreateAPIKeyResponse } from '@/services/apikeys/type'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import { copyText, formatTimeStr } from '@/utils/index'

defineProps<{
  visible: boolean
}>()
const emit = defineEmits(['update:visible', 'success'])

const initialForm = () => ({
  name: '',
  ttlDays: 90,
})
const form = reactive({ ...initialForm() })
const whitelistText = ref('')
const submitting = ref(false)
const successVisible = ref(false)
const createdKey = ref<CreateAPIKeyResponse | null>(null)

const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: 'Please input API Key name', trigger: 'change' }],
  ttlDays: [
    { required: true, message: 'Please input TTL days', trigger: 'change' },
    {
      type: 'number',
      min: 1,
      max: 366,
      message: 'TTL must be between 1 and 366 days',
      trigger: 'change',
    },
  ],
}))

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()
    submitting.value = true

    // Parse whitelist from textarea (one IP per line)
    const whitelist = whitelistText.value
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0)

    const response = await createAPIKey({
      name: form.name,
      ttlDays: form.ttlDays,
      whitelist: whitelist.length > 0 ? whitelist : undefined,
    })

    createdKey.value = response
    emit('update:visible', false)
    successVisible.value = true
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey as any)
      ElMessage.error(firstMsg)
    }
  } finally {
    submitting.value = false
  }
}

const copyApiKey = () => {
  if (createdKey.value?.apiKey) {
    copyText(createdKey.value.apiKey)
  }
}

const closeSuccessDialog = () => {
  successVisible.value = false
  createdKey.value = null
}

const onOpen = async () => {
  ruleFormRef.value?.resetFields()
  Object.assign(form, initialForm())
  whitelistText.value = ''
  await nextTick()
}
</script>
