<template>
  <div class="ws-fullpage">
    <header class="ws-bar">
      <span>WebShell — {{ podName }}</span>
      <div class="ws-actions">
        <el-button @click="reconnect">Reconnect</el-button>
        <span class="ws-status">{{ statusText }}</span>
      </div>
    </header>
    <div ref="termEl" class="term-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'

async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false
  // Prefer modern API (requires HTTPS or localhost)
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      /* fallthrough */
    }
  }
  // Fallback: execCommand (works on HTTP too with user gesture)
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '-9999px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
function canProgramReadClipboard() {
  return window.isSecureContext && 'clipboard' in navigator && !!navigator.clipboard?.readText
}

const route = useRoute()

// Clipboard capability
let onMouseUp: ((e: MouseEvent) => void) | null = null
let onContextMenu: ((e: MouseEvent) => void) | null = null

const podName = computed(() => String(route.query.pod ?? ''))
const containerQ = computed(() => String(route.query.container ?? ''))
const cmdQ = computed(() => String(route.query.cmd ?? 'bash'))
const workloadIdQ = computed(() => String(route.query.workloadId ?? ''))
const namespaceQ = computed(() => String(route.query.namespace ?? ''))
function getWsProtocol() {
  return window.location.protocol === 'https:' ? 'wss' : 'ws'
}
const protocolQ = computed(() => String(route.query.protocol ?? getWsProtocol()))

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
const WS_HOST = getApiHost()

const termEl = ref<HTMLDivElement | null>(null)
let term: Terminal | null = null
let fit: FitAddon | null = null
let ws: WebSocket | null = null
let ro: ResizeObserver | null = null

const statusText = ref('idle')
let lastCols = 80
let lastRows = 24

function buildWsUrl(cols = lastCols, rows = lastRows) {
  const base = `${protocolQ.value}://${WS_HOST}/api/v1/workloads/${encodeURIComponent(workloadIdQ.value)}/pods/${encodeURIComponent(podName.value)}/webshell`
  const qs = new URLSearchParams({
    namespace: namespaceQ.value,
    rows: String(rows),
    cols: String(cols),
    container: containerQ.value,
    cmd: cmdQ.value,
  })
  return `${base}?${qs.toString()}`
}

function sendResize(cols: number, rows: number) {
  lastCols = cols
  lastRows = rows
  if (!ws || ws.readyState !== WebSocket.OPEN) return
  try {
    ws.send(`RESIZE ${cols} ${rows}`)
  } catch {}
}

function closeWs() {
  if (ws) {
    try {
      ws.onclose = null
      ws.close()
    } catch {}
    ws = null
  }
}
function reconnect() {
  connect()
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
  }
  ws.onmessage = (ev) => {
    try {
      if (typeof ev.data === 'string') {
        const txt = ev.data.trim()
        if (txt.startsWith('{')) {
          try {
            const obj = JSON.parse(txt)
            const controlTypes = ['pong', 'ping', 'keepalive', 'resize_ack']
            if (controlTypes.includes(obj?.type)) return
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
  }
  ws.onerror = (err) => {
    console.error('websocket error', err)
    statusText.value = 'ws error'
  }
}

function getCurrentLineText(): string {
  try {
    // xterm buffer API: get the line at cursor position
    const y = (term as any)?.buffer?.active?.cursorY ?? 0
    const line = (term as any)?.buffer?.active?.getLine?.(y)
    return line?.translateToString?.(true)?.trim?.() || ''
  } catch {
    return ''
  }
}

// 2) Status hint (optional)
let flashTimer: number | null = null
function flashStatusOnce(msg: string, ms = 900) {
  if (flashTimer) clearTimeout(flashTimer)
  const prev = statusText.value
  statusText.value = msg
  flashTimer = window.setTimeout(() => {
    statusText.value = prev
  }, ms)
}

onMounted(async () => {
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

  /** Copy on selection (works on HTTP too, with execCommand fallback) */
  onMouseUp = async () => {
    if (!term?.hasSelection()) return
    const text = term.getSelection()
    const ok = await copyToClipboard(text)
    if (!ok) {
      console.warn('[WebShell] copy failed; use Ctrl/Cmd+Shift+C as fallback')
    }
  }
  term.element?.addEventListener('mouseup', onMouseUp)

  /** Right-click paste (intercept menu; Shift+right-click keeps native menu) */
  onContextMenu = async (e: MouseEvent) => {
    // Browser allows readText (https/localhost) → intercept menu and paste directly
    if (canProgramReadClipboard()) {
      e.preventDefault()
      term?.focus()
      term?.clearSelection() // Clear selection before paste for a more native terminal feel
      try {
        const text = await navigator.clipboard.readText()
        if (text) term!.paste(text)
        flashStatusOnce('pasted')
      } catch (err) {
        console.warn('[WebShell] clipboard-read denied', err)
      }
      return
    }
    // HTTP does not support programmatic paste: don't intercept, keep native "Paste" menu
    term?.focus()
  }
  term.element?.addEventListener('contextmenu', onContextMenu)

  /** Keyboard: Ctrl/Cmd + C / V  (with or without Shift) */
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.type !== 'keydown') return true
    const isMac = /Mac|iPod|iPhone|iPad/.test(navigator.platform)
    const ctrlOrCmd = isMac ? ev.metaKey : ev.ctrlKey
    if (!ctrlOrCmd) return true

    // Copy — Ctrl+C or Ctrl+Shift+C
    if (ev.code === 'KeyC') {
      if (term?.hasSelection()) {
        copyToClipboard(term.getSelection()).catch(() => {})
        term.clearSelection()
        return false
      }
      // No selection: Ctrl+C → pass through as SIGINT; Ctrl+Shift+C → swallow
      return !ev.shiftKey
    }

    // Paste — return false so xterm won't interpret Ctrl+V as control char \x16.
    // The browser's native paste event then fires and xterm's paste listener handles it.
    // Ctrl+Shift+V: browser won't fire paste event, so read clipboard manually.
    if (ev.code === 'KeyV') {
      if (!ev.shiftKey) return false
      if (canProgramReadClipboard()) {
        navigator.clipboard
          .readText()
          .then((t) => t && term?.paste(t))
          .catch(() => {})
      }
      return false
    }
    return true
  })

  connect()

  // Fullscreen page: observe terminal container and window resize
  const THROTTLE_MS = 120
  let rafId = 0,
    lastRan = 0,
    trailing: number | null = null
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
    cancelAnimationFrame(rafId)
    rafId = requestAnimationFrame(() => {
      const now = performance.now()
      if (now - lastRan >= THROTTLE_MS) {
        lastRan = now
        runFitAndResize()
      } else {
        if (trailing) clearTimeout(trailing)
        trailing = window.setTimeout(
          () => {
            lastRan = performance.now()
            runFitAndResize()
          },
          THROTTLE_MS - (now - lastRan),
        )
      }
    })
  })
  if (termEl.value) ro.observe(termEl.value)

  window.addEventListener('resize', runFitAndResize)
})
onBeforeUnmount(() => {
  if (onMouseUp) term?.element?.removeEventListener('mouseup', onMouseUp)
  if (onContextMenu) term?.element?.removeEventListener('contextmenu', onContextMenu)

  ro?.disconnect()
  ro = null
  window.removeEventListener('resize', () => {})
  closeWs()
  term?.dispose()
  term = null
  fit = null
})
</script>

<style>
html,
body,
#app {
  height: 100%;
}
.ws-fullpage {
  height: calc(100vh - 60px);
  display: flex;
  flex-direction: column;
  background: #0b0b0b;
}
.ws-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px 20px 12px;
  color: #cfd3dc;
  border-bottom: 1px solid #1f1f1f;
}
.ws-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}
.ws-status {
  color: #909399;
  font-size: 12px;
}
.term-container {
  flex: 1;
  min-height: 0;
  height: 100%;
  border-top: 0;
  background: #000;
  overflow: hidden;
}
.xterm,
.xterm-helpers,
.xterm-screen {
  height: 100%;
}
</style>
