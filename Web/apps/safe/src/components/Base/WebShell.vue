<template>
  <el-dialog
    :model-value="visible"
    :destroy-on-close="false"
    @opened="onDialogOpened"
    @close="onCloseDialog"
    class="ws-resizable-dialog"
    :show-close="true"
    :close-on-click-modal="false"
  >
    <template #title>
      <div style="display: flex; align-items: center; justify-content: space-between; width: 100%">
        <span>WebShell — {{ podName }}</span>
        <div style="display: flex; gap: 8px; align-items: center">
          <el-button size="mini" @click="reconnect">Reconnect</el-button>
          <span style="color: #909399; font-size: 12px">{{ statusText }}</span>
        </div>
      </div>
    </template>

    <div style="display: flex; flex-direction: column; gap: 8px">
      <div ref="termEl" class="term-container"></div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, nextTick } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'
import { useWorkspaceStore } from '@/stores/workspace'

interface Props {
  visible: boolean
  podName: string
  container?: string
  cmd?: string
  wsHost?: string
  protocol?: 'ws' | 'wss'
  wlid?: string
  sendResizeOnOpen?: boolean
}
const props = defineProps<Props>()
const emit = defineEmits(['update:visible'])
const wsStore = useWorkspaceStore()

function getApiHost(): string {
  const base = import.meta.env.VITE_API_BASE_URL || '/api'
  if (import.meta.env.DEV) {
    try {
      return new URL(base).host
    } catch {
      return base.replace(/^https?:\/\//, '').replace(/\/.*$/, '')
    }
  } else {
    return window.location.host
  }
}
const podName = computed(() => props.podName || '')
const container = computed(() => props.container ?? '')
const cmd = computed(() => props.cmd ?? 'bash')
const WS_HOST = getApiHost()
const PROTOCOL = props.protocol ?? 'ws'
const sendOnOpen = props.sendResizeOnOpen ?? false

// xterm & ws
const termEl = ref<HTMLDivElement | null>(null)
let term: Terminal | null = null
let fit: FitAddon | null = null
let ws: WebSocket | null = null
let ro: ResizeObserver | null = null
let dialogEl: HTMLElement | null = null

const statusText = ref('idle')
let lastCols = 80
let lastRows = 24

function buildWsUrl(cols = lastCols, rows = lastRows) {
  const base = `${PROTOCOL}://${WS_HOST}/api/v1/workloads/${encodeURIComponent(props.wlid ?? '')}/pods/${encodeURIComponent(podName.value)}/webshell`
  const qs = new URLSearchParams({
    namespace: wsStore.currentWorkspaceId ?? '',
    rows: String(rows),
    cols: String(cols),
    container: container.value,
    cmd: cmd.value,
  })
  return `${base}?${qs.toString()}`
}

function closeWs() {
  stopHeartbeat()
  if (ws) {
    try {
      ws.close()
    } catch {}
    ws = null
  }
}

function sendResize(cols: number, rows: number) {
  lastCols = cols
  lastRows = rows
  if (!ws || ws.readyState !== WebSocket.OPEN) return
  try {
    ws.send(JSON.stringify({ type: 'resize', cols, rows }))
  } catch {}
}

function reconnect() {
  connect()
}

function onCloseDialog() {
  emit('update:visible', false)
  ro?.disconnect()
  ro = null
  closeWs()
  if (term) {
    term.dispose()
    term = null
  }
  fit = null
}

// Heartbeat: application-level ping/pong keepalive
const HEARTBEAT_INTERVAL = 25000
const HEARTBEAT_TIMEOUT = 8000
let heartbeatTimer: number | null = null
let heartbeatWaitTimer: number | null = null

function startHeartbeat() {
  stopHeartbeat()
  heartbeatTimer = window.setInterval(() => {
    if (!ws || ws.readyState !== WebSocket.OPEN) return
    try {
      ws.send('')
      if (HEARTBEAT_TIMEOUT > 0) {
        if (heartbeatWaitTimer) clearTimeout(heartbeatWaitTimer)
        heartbeatWaitTimer = window.setTimeout(() => {
          try {
            ws?.close()
          } catch {}
        }, HEARTBEAT_TIMEOUT)
      }
    } catch (e) {
      console.warn('heartbeat send error', e)
    }
  }, HEARTBEAT_INTERVAL)
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
  if (heartbeatWaitTimer) {
    clearTimeout(heartbeatWaitTimer)
    heartbeatWaitTimer = null
  }
}

function handlePong() {
  if (heartbeatWaitTimer) {
    clearTimeout(heartbeatWaitTimer)
    heartbeatWaitTimer = null
  }
}

function connect() {
  closeWs()
  const url = buildWsUrl()
  statusText.value = 'connecting...'
  try {
    ws = new WebSocket(url)
  } catch (e) {
    statusText.value = 'ws create error'
    console.error(e)
    return
  }
  ws.binaryType = 'arraybuffer'

  ws.onopen = () => {
    statusText.value = 'connected'
    term?.writeln('\x1b[32m[Connected]\x1b[0m\r\n')
    if (sendOnOpen) sendResize(lastCols, lastRows)
    startHeartbeat()
  }
  ws.onmessage = (ev) => {
    try {
      if (typeof ev.data === 'string') {
        const txt = ev.data.trim()
        if (txt === 'pong') {
          handlePong()
          return
        }
        if (txt.startsWith('{')) {
          try {
            const obj = JSON.parse(txt)
            if (obj?.type === 'pong') {
              handlePong()
              return
            }
          } catch {}
        }
        term?.write(txt)
      } else {
        term?.write(new TextDecoder().decode(ev.data as ArrayBuffer))
      }
    } catch (e) {
      console.error('ws.onmessage error', e)
    }
  }
  ws.onclose = (ev) => {
    statusText.value = `closed (${ev.code})`
    term?.writeln('\r\n\x1b[31m[Disconnected]\x1b[0m')
    stopHeartbeat()
  }
  ws.onerror = (err) => {
    console.error('websocket error', err)
    statusText.value = 'ws error'
    stopHeartbeat()
  }
}

// Wait for dialog to fully open before init/open/fit/connect
async function onDialogOpened() {
  if (term) {
    try {
      term.dispose()
      fit = null
      ro?.disconnect()
      ro = null
    } catch (e) {
      console.warn('dispose error', e)
    }
    term = null
  }

  term = new Terminal({ cursorBlink: true, scrollback: 5000, convertEol: true })
  fit = new FitAddon()
  term.loadAddon(fit)

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(data)
  })

  await nextTick()
  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))

  term.open(termEl.value!)
  if ('fonts' in document && (document as any).fonts?.ready) {
    try {
      await (document as any).fonts.ready
    } catch {}
  }
  fit.fit()
  lastCols = term.cols
  lastRows = term.rows
  // Do not send resize here

  connect()

  // Observe size changes of the outer el-dialog element
  dialogEl = termEl.value!.closest('.el-dialog.ws-resizable-dialog') as HTMLElement
  if (dialogEl && !dialogEl.style.height) {
    dialogEl.style.width = '80vw'
    dialogEl.style.height = '70vh'
  }

  // === Throttled ResizeObserver ===
  const THROTTLE_MS = 200
  let rafId = 0
  let lastRan = 0
  let trailingTimer: number | null = null

  function runFitAndResize() {
    if (!term || !fit) return
    fit.fit()
    if (term.cols !== lastCols || term.rows !== lastRows) {
      lastCols = term.cols
      lastRows = term.rows
      sendResize(lastCols, lastRows)
    }
  }

  ro?.disconnect()
  ro = new ResizeObserver(() => {
    // Merge multiple callbacks within the same frame
    cancelAnimationFrame(rafId)
    rafId = requestAnimationFrame(() => {
      const now = performance.now()
      if (now - lastRan >= THROTTLE_MS) {
        lastRan = now
        runFitAndResize()
      } else {
        // Trailing call, ensure fit is called once more after dragging stops
        const wait = THROTTLE_MS - (now - lastRan)
        if (trailingTimer) clearTimeout(trailingTimer)
        trailingTimer = window.setTimeout(() => {
          lastRan = performance.now()
          runFitAndResize()
        }, wait)
      }
    })
  })
  ro.observe(dialogEl || termEl.value!)
}

onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
  closeWs()
  term?.dispose()
  term = null
  fit = null
})

// Reduce heartbeat frequency when page is hidden
document.addEventListener('visibilitychange', () => {
  if (document.hidden) {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer)
      heartbeatTimer = null
    }
    heartbeatTimer = window.setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send('')
    }, 60000)
  } else {
    startHeartbeat()
  }
})
</script>
<style>
.el-dialog.ws-resizable-dialog {
  width: 80vw;
  height: 70vh;
  display: flex;
  flex-direction: column;
  resize: both;
  overflow: hidden;
  max-width: 90vw;
  max-height: calc(100vh - 32px);
  min-width: 560px;
  min-height: 40vh;
}

.el-dialog.ws-resizable-dialog .el-dialog__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 12px;
  min-height: 0;
}

.term-container {
  flex: 1;
  min-height: 0;
  border: 1px solid #2a2a2a;
  background: #000;
  border-radius: 6px;
  overflow: hidden;
}

.xterm,
.xterm-helpers,
.xterm-screen {
  height: 100%;
}
</style>
