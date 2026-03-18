<template>
  <div class="min-h-screen flex items-center justify-center p-6">
    <!-- Background -->
    <div class="fixed inset-0 -z-1 pointer-events-none">
      <div
        class="absolute inset-0 opacity-60"
        style="
          background:
            radial-gradient(1200px 600px at 60% -10%, rgba(99, 102, 241, 0.1), transparent 70%),
            radial-gradient(800px 400px at 20% 110%, rgba(34, 197, 94, 0.08), transparent 70%);
        "
      ></div>
      <svg class="absolute inset-0 w-full h-full opacity-30" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M 40 0 L 0 0 0 40" fill="none" stroke="currentColor" stroke-width="0.6" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#grid)" />
      </svg>
    </div>

    <!-- Login card -->
    <div class="login-card w-full max-w-[480px] @1440:max-w-[520px]">
      <!-- Header -->
      <div class="px-6 py-5 header-row">
        <div class="flex items-center gapx-8">
          <div>
            <div class="textx-18 font-600 leading-tight">Register</div>
            <div class="text-[13px] opacity-80 mt-1">Create your account to get started</div>
          </div>
        </div>
      </div>

      <!-- Form body -->
      <div class="px-6 pt-4 pb-2">
        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          size="large"
          label-width="auto"
          class="login-form"
          label-position="left"
        >
          <el-form-item prop="name" label="Name">
            <el-input v-model="form.name" placeholder="Username" clearable />
          </el-form-item>

          <el-form-item prop="password" label="Password">
            <el-input
              v-model="form.password"
              placeholder="Your password"
              type="password"
              :prefix-icon="Lock"
              show-password
            />
          </el-form-item>

          <el-form-item prop="confirmPassword" label="Confirm Password">
            <el-input
              v-model="form.confirmPassword"
              placeholder="Re-enter your password"
              type="password"
              :prefix-icon="Lock"
              show-password
            />
          </el-form-item>

          <el-button
            class="w-full"
            type="primary"
            size="large"
            :loading="loading"
            @click="onSubmit"
          >
            Sign Up
          </el-button>

          <div class="mt-3 text-center text-[13px] opacity-80">
            Already have an account?
            <el-link type="primary" :underline="false" @click="onLogin">Sing in</el-link>
          </div>
        </el-form>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 footer-row">
        <div class="flex items-center justify-between">
          <div class="text-[12px] opacity-70">v{{ version }}</div>
          <div class="flex items-center gap-2">
            <el-tooltip content="Light / Dark" placement="top">
              <el-button circle size="small" @click="toggleDark()">
                <el-icon><Moon /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, toRaw } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Lock, Moon } from '@element-plus/icons-vue'
import { register } from '@/services/login'
import { useRouter } from 'vue-router'
import { toggleDark } from '@/composables'

const version = '1.0.0'

const router = useRouter()
const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  name: '',
  type: 'default',
  workspaces: [],
  password: '',
  confirmPassword: '',
})

const rules: FormRules = {
  name: [
    { required: true, message: 'Please input account', trigger: 'blur' },
    { min: 3, message: 'At least 3 characters', trigger: 'blur' },
  ],
  password: [
    { required: true, message: 'Please input password', trigger: 'blur' },
    { max: 64, message: 'Do not more than 64 characters', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: 'Please confirm password', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== form.password) {
          callback(new Error('Passwords do not match'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
}

function onLogin() {
  router.push('login')
}

async function onSubmit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      const { confirmPassword, ...payload } = toRaw(form)
      await register(payload)
      ElMessage.success('Signed up successfully')
      router.push({ path: '/login', query: { name: form.name, type: form.type } })
    } finally {
      loading.value = false
    }
  })
}
</script>

<style scoped>
/* Depth: soft shadow + translucent border + rounded corners + dividers */
.login-card {
  --dlg-radius: 16px;
  --dlg-shadow: 0 18px 50px rgba(0, 0, 0, 0.18), 0 3px 10px rgba(0, 0, 0, 0.06);
  --dlg-border: 1px solid color-mix(in oklab, var(--el-border-color) 75%, transparent);
  --dlg-bg: color-mix(in oklab, var(--el-bg-color) 96%, #fff 4%);
  border-radius: var(--dlg-radius);
  background: var(--dlg-bg);
  box-shadow: var(--dlg-shadow);
  border: var(--dlg-border);
  overflow: hidden;
  backdrop-filter: saturate(1.1) blur(2px);
}

/* Header/footer dividers and subtle background */
.header-row,
.footer-row {
  background: color-mix(in oklab, var(--el-bg-color) 98%, #fff 2%);
  border-bottom: 1px solid color-mix(in oklab, var(--el-border-color) 85%, transparent);
}
.footer-row {
  border-top: 1px solid color-mix(in oklab, var(--el-border-color) 85%, transparent);
  border-bottom: none;
}

/* Input fields with depth: border + shadow (slight lift on hover/focus) */
.login-form :deep(.el-input__wrapper) {
  border: 1px solid color-mix(in oklab, var(--el-border-color) 88%, transparent);
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.04) inset;
  transition:
    box-shadow 0.2s ease,
    border-color 0.2s ease,
    background-color 0.2s ease;
}
.login-form :deep(.el-input__wrapper:hover) {
  border-color: color-mix(
    in oklab,
    var(--el-border-color) 60%,
    var(--safe-primary, var(--el-color-primary)) 15%
  );
}
.login-form :deep(.el-input.is-focus .el-input__wrapper),
.login-form :deep(.is-focus .el-input__wrapper) {
  box-shadow: 0 0 0 2px
    color-mix(in oklab, var(--safe-primary, var(--el-color-primary)) 24%, transparent);
  border-color: color-mix(
    in oklab,
    var(--safe-primary, var(--el-color-primary)) 40%,
    var(--el-border-color) 60%
  );
}

/* Large screen: add spacing without significantly increasing font size */
@media (min-width: 1440px) {
  .header-row {
    padding: 18px 24px 14px;
  }
  .footer-row {
    padding: 14px 24px 16px;
  }
  .login-form {
    padding-top: 2px;
  }
}
</style>
