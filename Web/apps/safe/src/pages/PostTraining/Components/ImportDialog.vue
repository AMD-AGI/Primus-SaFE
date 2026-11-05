<template>
  <el-dialog
    :model-value="modelValue"
    title="Import a Run from Spur"
    width="680px"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @open="onOpen"
    @closed="onClosed"
  >
    <div v-loading="!config" class="min-h-[120px]">
      <template v-if="config">
        <el-radio-group v-model="mode" :disabled="!!result" @change="onModeChange">
          <el-radio-button value="mount" :disabled="!config.mountAvailable">
            <el-tooltip
              :content="config.mountReason"
              :disabled="config.mountAvailable || !config.mountReason"
              placement="top"
            >
              <span>Mounted path</span>
            </el-tooltip>
          </el-radio-button>
          <el-radio-button value="cli">Upload from Spur</el-radio-button>
        </el-radio-group>

        <div class="pt-import-intro">
          <template v-if="mode === 'mount'">
            SaFE reads the directory through {{ config.mountRoot }}, mounted read-only on cluster
            {{ config.mountCluster }}. Nothing is copied.
            <template v-if="config.mountPrefixes?.length">
              You can scan directories under {{ config.mountPrefixes.join(' or ') }}.
            </template>
          </template>
          <template v-else>
            Run one command on a Spur login node. It parses the directory there and uploads a
            summary of a few hundred KB, so no mount is needed.
          </template>
        </div>

        <el-form label-position="top" @submit.prevent>
          <el-form-item label="Run directory" required class="!mb-2">
            <div class="flex w-full gap-2">
              <el-input
                v-model="dir"
                clearable
                :disabled="!!result || scanning"
                :placeholder="EXAMPLE_DIR"
                @keyup.enter="mode === 'mount' && scan()"
              />
              <el-button
                v-if="mode === 'mount'"
                :loading="scanning"
                :disabled="!dir.trim() || !!result"
                @click="scan"
              >
                {{ scanning ? 'Scanning…' : 'Scan' }}
              </el-button>
            </div>
            <div v-if="mode === 'cli'" class="pt-import-hint">
              A slime-rocm run directory, such as {{ EXAMPLE_DIR }}.
            </div>
          </el-form-item>
        </el-form>

        <div v-if="mode === 'mount' && scanning" class="pt-import-hint">
          Reading node-*.log, results/, markers/, mon/, ckpt/ and the main log…
        </div>

        <template v-if="mode === 'cli' && !result">
          <div class="pt-import-label">Run this on a Spur login node</div>
          <pre class="pt-import-command">{{ command }}</pre>
          <div class="flex items-center gap-3 text-[12px]">
            <el-button size="small" :disabled="!token || tokenExpired" @click="copyText(command)">
              Copy Command
            </el-button>
            <template v-if="tokenExpired">
              <span class="text-[var(--el-color-danger)]">Token expired</span>
              <el-button size="small" link type="primary" :loading="issuing" @click="issueToken">
                New Command
              </el-button>
            </template>
            <span v-else-if="token" class="text-[var(--safe-muted)]">
              Single-use token for {{ workspace }} · expires at {{ expiresAt }}
            </span>
          </div>
          <div v-if="token && !tokenExpired" class="pt-import-waiting">
            <span class="pt-dot" />Waiting for the upload…
          </div>
        </template>

        <el-alert v-if="error" class="mt-3" type="error" :closable="false" :title="error" />
        <el-alert
          v-if="result"
          class="mt-3"
          type="success"
          :closable="false"
          :title="resultTitle"
        />
        <el-alert v-else-if="existingRunId" class="mt-3" type="warning" :closable="false">
          <template #title>
            This directory is already imported.
            <el-link type="primary" @click="openRun(existingRunId)">
              Open {{ shortRunId(existingRunId) }}
            </el-link>
          </template>
        </el-alert>

        <template v-if="preview">
          <el-descriptions class="mt-3" :column="1" border size="small">
            <el-descriptions-item
              v-for="r in rows"
              :key="r.label"
              :label="r.label"
              label-class-name="pt-import-row-label"
            >
              {{ r.value }}
            </el-descriptions-item>
          </el-descriptions>
          <el-alert
            v-for="h in hints"
            :key="h.text"
            class="mt-2"
            :type="h.type"
            :closable="false"
            :title="h.text"
          />
        </template>

        <div class="pt-import-hint mt-3">
          Imported runs are read-only and don't count toward Verified. SaFE keeps the parsed summary
          and a reference to the directory.
        </div>
      </template>
    </div>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">{{
        result ? 'Close' : 'Cancel'
      }}</el-button>
      <el-button v-if="result" type="primary" @click="openRun(result.runId)">Open Run</el-button>
      <el-button
        v-else-if="mode === 'mount' && canCommit"
        type="primary"
        :loading="committing"
        @click="commit"
      >
        Import
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useIntervalFn, useNow } from '@vueuse/core'
import dayjs from 'dayjs'
import { copyText } from '@/utils'
import {
  commitImportScan,
  getImportConfig,
  getImportScan,
  getImportToken,
  importCommandFor,
  importHints,
  importPreviewRows,
  issueImportToken,
  startImportScan,
  type ImportConfig,
  type ImportPreview,
  type ImportResult,
  type ImportScanStatus,
  type ImportToken,
  type ImportTokenStatus,
} from '@/services/postTraining'

const props = defineProps<{
  modelValue: boolean
  workspace: string
  /** The way to open with, when it is available. */
  initialMode?: 'mount' | 'cli'
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; imported: [runId: string] }>()

const EXAMPLE_DIR = '/shared_nfs/zhanglei/glm53-kernel/slime-rocm/runs/j2r7-242118'
const POLL_MS = 3000

const router = useRouter()
const config = ref<ImportConfig>()
const mode = ref<'mount' | 'cli'>('cli')
const dir = ref('')
const error = ref('')
const preview = ref<ImportPreview>()
const result = ref<{
  runId: string
  displayName: string
  uploadedAt?: string
  via: 'cli' | 'scan'
}>()

const token = ref<ImportToken>()
const issuing = ref(false)
const tokenState = ref<ImportTokenStatus>()
const now = useNow({ interval: 1000 })
const tokenExpired = computed(
  () =>
    !!token.value &&
    (tokenState.value?.state === 'expired' ||
      now.value.getTime() >= dayjs(token.value.expiresAt).valueOf()),
)
const command = computed(() =>
  importCommandFor(
    token.value?.command ?? 'SAFE_IMPORT_TOKEN=… safe-post-training-import <run-dir>',
    dir.value,
  ),
)
const expiresAt = computed(() => (token.value ? dayjs(token.value.expiresAt).format('HH:mm') : ''))

const scanId = ref('')
const scanning = ref(false)
const scanState = ref<ImportScanStatus>()
const committing = ref(false)
const existingRunId = computed(() =>
  scanState.value?.state === 'ready' ? (scanState.value.existingRunId ?? '') : '',
)
const canCommit = computed(() => scanState.value?.state === 'ready' && !existingRunId.value)

const rows = computed(() => (preview.value ? importPreviewRows(preview.value) : []))
const hints = computed(() => (preview.value ? importHints(preview.value) : []))
const resultTitle = computed(() => {
  const r = result.value
  if (!r) return ''
  if (r.via === 'scan') return `Imported as ${r.displayName}.`
  const at = r.uploadedAt ? ` · ${dayjs(r.uploadedAt).format('MM-DD HH:mm')}` : ''
  return `Imported as ${r.displayName}. Uploaded by safe-post-training-import${at}.`
})

/** Clears the preview, the scan, the result and the errors; the directory and the token stay. */
const reset = () => {
  error.value = ''
  preview.value = undefined
  result.value = undefined
  scanId.value = ''
  scanning.value = false
  scanState.value = undefined
  tokenState.value = undefined
}

/** The API's reason for a refused request, or what failed. */
const reasonOf = (err: unknown, what: string) =>
  (err as { response?: { data?: { errorMessage?: string } } })?.response?.data?.errorMessage ??
  `${what}: ${String(err)}`

/** Loads the workspace's import settings and opens the requested way, or the mounted path when it works. */
const onOpen = async () => {
  reset()
  config.value = undefined
  token.value = undefined
  let cfg: ImportConfig
  try {
    cfg = (await getImportConfig(props.workspace)) as unknown as ImportConfig
  } catch (err) {
    config.value = { mountAvailable: false, canImport: false }
    error.value = `Couldn't load the import settings: ${String(err)}`
    return
  }
  config.value = cfg
  const wanted = props.initialMode ?? (cfg.mountAvailable ? 'mount' : 'cli')
  mode.value = wanted === 'mount' && !cfg.mountAvailable ? 'cli' : wanted
  if (mode.value === 'cli') await issueToken()
}

/** Stops polling once the dialog is closed; an issued token can still be used until it expires. */
const onClosed = () => {
  scanning.value = false
  scanId.value = ''
}

/** Switching the way clears the preview and the errors and keeps the directory. */
const onModeChange = () => {
  reset()
  if (mode.value === 'cli' && !token.value) issueToken()
}

/** Issues a token, shown in the command; it lives 30 minutes whether or not the dialog stays open. */
const issueToken = async () => {
  if (!config.value?.canImport) {
    error.value = 'Read-only access'
    return
  }
  issuing.value = true
  try {
    token.value = (await issueImportToken(props.workspace)) as unknown as ImportToken
    tokenState.value = undefined
    error.value = ''
  } catch (err) {
    error.value = reasonOf(err, "Couldn't issue a command")
  } finally {
    issuing.value = false
  }
}

/** Reads whether the command used the token; a refused upload shows its reason, and the token still works. */
const pollToken = async () => {
  const t = token.value
  if (!t || mode.value !== 'cli' || result.value || tokenExpired.value) return
  try {
    const s = (await getImportToken(t.tokenId)) as unknown as ImportTokenStatus
    if (token.value?.tokenId !== t.tokenId) return
    tokenState.value = s
    error.value =
      s.state === 'waiting' && s.lastError
        ? `The upload was refused: ${s.lastError} Run the same command again once it is fixed.`
        : ''
    if (s.state === 'used' && s.runId) {
      preview.value = s.preview
      result.value = {
        runId: s.runId,
        displayName: s.displayName || s.runId,
        uploadedAt: s.uploadedAt,
        via: 'cli',
      }
      emit('imported', s.runId)
    }
  } catch {
    /* the next poll tries again */
  }
}

/** Starts a scan of the directory; polling shows it once it is ready, failed or imported. */
const scan = async () => {
  const path = dir.value.trim()
  if (!path || scanning.value || result.value) return
  reset()
  scanning.value = true
  try {
    const res = (await startImportScan(props.workspace, path)) as unknown as { scanId: string }
    scanId.value = res.scanId
  } catch (err) {
    scanning.value = false
    error.value = reasonOf(err, "Couldn't start the scan")
  }
}

/** Reads the scan until it leaves running. */
const pollScan = async () => {
  const id = scanId.value
  if (!id || !scanning.value) return
  try {
    const s = (await getImportScan(id)) as unknown as ImportScanStatus
    if (scanId.value !== id || s.state === 'running') return
    scanState.value = s
    scanning.value = false
    preview.value = s.preview
    if (s.state === 'failed') error.value = s.message || 'The scan failed.'
    if (s.state === 'imported' && s.runId)
      result.value = { runId: s.runId, displayName: s.preview?.name || s.runId, via: 'scan' }
  } catch {
    /* the next poll tries again */
  }
}

/** Writes the scanned directory as a run; a directory imported meanwhile shows that run instead. */
const commit = async () => {
  if (!scanId.value) return
  committing.value = true
  try {
    const r = (await commitImportScan(scanId.value)) as unknown as ImportResult
    result.value = { runId: r.runId, displayName: r.displayName, via: 'scan' }
    emit('imported', r.runId)
  } catch (err) {
    const resp = (
      err as { response?: { status?: number; data?: { errorMessage?: string; runId?: string } } }
    )?.response
    if (resp?.status === 409 && resp.data?.runId && scanState.value) {
      scanState.value = { ...scanState.value, existingRunId: resp.data.runId }
      error.value = ''
    } else {
      error.value = reasonOf(err, "Couldn't import")
    }
  } finally {
    committing.value = false
  }
}

useIntervalFn(() => {
  if (!props.modelValue) return
  pollToken()
  pollScan()
}, POLL_MS)

/** A new directory needs a new scan: its preview no longer applies. */
watch(dir, () => {
  if (mode.value === 'mount' && !scanning.value && !result.value) {
    scanState.value = undefined
    preview.value = undefined
    scanId.value = ''
    error.value = ''
  }
})

/** The directory name of an imported run's id. */
const shortRunId = (runId: string) => runId.replace(/^spur-/, '')

const openRun = (runId: string) => {
  emit('update:modelValue', false)
  router.push({ path: '/post-training/detail', query: { runId } })
}
</script>

<style scoped>
.pt-import-intro {
  margin: 12px 0 8px;
  font-size: 13px;
  color: var(--safe-muted);
}
.pt-import-hint {
  margin-top: 4px;
  font-size: 12px;
  line-height: 18px;
  color: var(--safe-muted);
}
.pt-import-label {
  margin: 4px 0 6px;
  font-size: 13px;
  font-weight: 500;
}
.pt-import-command {
  margin: 0 0 8px;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.pt-import-waiting {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
  font-size: 13px;
  color: var(--safe-muted);
}
.pt-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  animation: pt-dot-breathe 1.6s ease-in-out infinite;
}
@keyframes pt-dot-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}
:deep(.pt-import-row-label) {
  width: 110px;
}
</style>
