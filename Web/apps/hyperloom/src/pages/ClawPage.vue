<template>
  <div class="claw-page">
    <!-- Session Sidebar -->
    <div class="claw-sidebar" :class="{ collapsed: !sidebarOpen }">
      <div class="claw-sidebar-header">
        <span class="claw-sidebar-title">🤖 PrimusClaw</span>
        <button class="claw-btn-icon" @click="sidebarOpen = false">✕</button>
      </div>

      <button class="claw-new-chat-btn" @click="startNewSession">
        <span>＋</span> New Chat
      </button>

      <div class="claw-history-label" @click="historyCollapsed = !historyCollapsed">
        <span :class="{ rotated: historyCollapsed }">▾</span> History
      </div>

      <div v-if="loadingSessions && !historyCollapsed" class="claw-loading">Loading sessions...</div>

      <div v-else-if="sessions.length > 0 && !historyCollapsed" class="claw-session-list">
        <div
          v-for="s in sessions" :key="s.session_id"
          class="claw-session-item"
          :class="{ active: currentSessionId === s.session_id }"
          @click="loadSession(s)"
        >
          <span class="claw-session-name">{{ s.name || s.title || 'Untitled' }}</span>
          <span class="claw-session-delete" @click.stop="handleDeleteSession(s.session_id)">🗑</span>
        </div>
      </div>

      <div v-else-if="!historyCollapsed" class="claw-empty">No sessions yet</div>
    </div>

    <!-- Main Chat Area -->
    <div class="claw-main" :class="{ expanded: !sidebarOpen }">
      <div class="claw-topbar">
        <button v-if="!sidebarOpen" class="claw-btn-icon" @click="sidebarOpen = true">☰</button>
        <span class="claw-topbar-title">{{ currentSessionId ? 'Session Active' : 'New Chat' }}</span>
        <span class="claw-topbar-status" v-if="liveStatus">{{ liveStatus }}</span>
      </div>

      <!-- Config Summary Chips (visible after config is done and optimization started) -->
      <div v-if="e2eStarted" class="claw-config-summary">
        <span class="claw-config-chip" v-for="(val, key) in e2eConfig" :key="key">
          <span class="claw-config-dot"></span>
          {{ val }}
        </span>
        <button class="claw-config-edit-btn" @click="resetWizard">✎ Edit Config</button>
      </div>

      <div class="claw-messages" ref="messagesContainer">
        <!-- Loading state -->
        <div v-if="loadingMessages" class="claw-loading-messages">Loading messages...</div>

        <!-- Welcome / Wizard screen -->
        <div v-else-if="messages.length === 0 && !e2eStarted" class="claw-wizard-flow">

          <!-- Step 1: Model -->
          <div class="claw-wizard-msg">
            <div class="claw-wizard-avatar">🤖</div>
            <div class="claw-wizard-body">
              <div class="claw-wizard-sender">PrimusClaw</div>
              <div class="claw-wizard-content">
                Welcome! Let's set up your <strong>E2E Optimization</strong> task. I'll guide you through each step.
                <div class="claw-wizard-card">
                  <div class="claw-wizard-title">
                    <span class="claw-step-num">1</span> Select Target Model
                  </div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in modelOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.model === opt.value }"
                      @click="pickOption('model', opt.value)">
                      {{ opt.label }}<span class="claw-wizard-sub">{{ opt.sub }}</span>
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.model"
                      placeholder="Or type a custom model name..."
                      @keydown.enter="applyCustom('model')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('model')">Use Custom</button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Step 2: Framework & GPU -->
          <div v-if="wizardStep >= 2" class="claw-wizard-msg">
            <div class="claw-wizard-avatar">🤖</div>
            <div class="claw-wizard-body">
              <div class="claw-wizard-sender">PrimusClaw</div>
              <div class="claw-wizard-content">
                Great choice! Now select the serving <strong>framework</strong> and target <strong>GPU</strong>.
                <div class="claw-wizard-card">
                  <div class="claw-wizard-title">
                    <span class="claw-step-num">2</span> Framework &amp; GPU
                  </div>

                  <div class="claw-wizard-subtitle">Serving Framework:</div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in frameworkOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.framework === opt.value }"
                      @click="pickOption('framework', opt.value)">
                      {{ opt.label }}<span class="claw-wizard-sub">{{ opt.sub }}</span>
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.framework"
                      placeholder="Or type a custom framework..."
                      @keydown.enter="applyCustom('framework')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('framework')">Use Custom</button>
                  </div>

                  <div class="claw-wizard-subtitle" style="margin-top:12px;">Target GPU:</div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in gpuOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.gpu === opt.value }"
                      @click="pickOption('gpu', opt.value)">
                      {{ opt.label }}<span class="claw-wizard-sub">{{ opt.sub }}</span>
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.gpu"
                      placeholder="Or type a custom GPU..."
                      @keydown.enter="applyCustom('gpu')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('gpu')">Use Custom</button>
                  </div>

                  <div v-if="warnings.fwGpu" class="claw-wizard-warn">
                    ⚠️ <strong>{{ e2eConfig.framework }}</strong> is only supported on Nvidia GPUs. Please select B200 or H100.
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Step 3: Precision, ISL/OSL, Concurrency, Trace -->
          <div v-if="wizardStep >= 3" class="claw-wizard-msg">
            <div class="claw-wizard-avatar">🤖</div>
            <div class="claw-wizard-body">
              <div class="claw-wizard-sender">PrimusClaw</div>
              <div class="claw-wizard-content">
                Almost there! Configure the remaining parameters:
                <div class="claw-wizard-card">
                  <div class="claw-wizard-title">
                    <span class="claw-step-num">3</span> Precision, Workload, Concurrency &amp; Trace
                  </div>

                  <div class="claw-wizard-subtitle">Precision:</div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in precisionOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.precision === opt.value }"
                      @click="pickOption('precision', opt.value)">
                      {{ opt.label }}<span class="claw-wizard-sub">{{ opt.sub }}</span>
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.precision"
                      placeholder="Or type custom precision..."
                      @keydown.enter="applyCustom('precision')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('precision')">Use Custom</button>
                  </div>

                  <div class="claw-wizard-subtitle" style="margin-top:10px;">ISL / OSL (Input / Output Sequence Length):</div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in islOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.isl === opt.value }"
                      @click="pickOption('isl', opt.value)">
                      {{ opt.label }}
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.isl"
                      placeholder="Or type custom, e.g. 4K / 2K"
                      @keydown.enter="applyCustom('isl')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('isl')">Use Custom</button>
                  </div>

                  <div class="claw-wizard-subtitle" style="margin-top:10px;">Concurrency:</div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in concurrencyOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.concurrency === opt.value }"
                      @click="pickOption('concurrency', opt.value)">
                      {{ opt.label }}
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.concurrency"
                      placeholder="Or type custom number..."
                      @keydown.enter="applyCustom('concurrency')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('concurrency')">Use Custom</button>
                  </div>

                  <div class="claw-wizard-subtitle" style="margin-top:10px;">Trace Tool:</div>
                  <div class="claw-wizard-options">
                    <div v-for="opt in traceOptions" :key="opt.value"
                      class="claw-wizard-opt"
                      :class="{ selected: e2eConfig.trace === opt.value }"
                      @click="pickOption('trace', opt.value)">
                      {{ opt.label }}<span class="claw-wizard-sub">{{ opt.sub }}</span>
                    </div>
                  </div>
                  <div class="claw-wizard-custom-row">
                    <input class="claw-wizard-custom-input"
                      v-model="customInputs.trace"
                      placeholder="Or type custom tracer..."
                      @keydown.enter="applyCustom('trace')">
                    <button class="claw-wizard-custom-btn" @click="applyCustom('trace')">Use Custom</button>
                  </div>

                  <div v-if="warnings.precision" class="claw-wizard-warn">
                    ⚠️ <strong>FP4</strong> on MI300X may have reduced accuracy for large MoE models. Consider FP8.
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Confirmation Card -->
          <div v-if="wizardStep >= 4" class="claw-wizard-msg">
            <div class="claw-wizard-avatar">🤖</div>
            <div class="claw-wizard-body">
              <div class="claw-wizard-sender">PrimusClaw</div>
              <div class="claw-wizard-content">
                All configured! Review and launch:
                <div class="claw-confirm-card">
                  <div class="claw-confirm-title">✅ Ready to Optimize</div>
                  <div class="claw-confirm-grid">
                    <div class="claw-confirm-item" v-for="(val, key) in e2eConfig" :key="key">
                      <div class="claw-confirm-label">{{ configLabels[key] }}</div>
                      <div class="claw-confirm-value">{{ val }}</div>
                    </div>
                  </div>
                  <button class="claw-confirm-start-btn" @click="startE2EOptimization">
                    🚀 Start E2E Optimization
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Message list (after E2E started or existing session loaded) -->
        <div v-else-if="messages.length > 0" class="claw-message-list">
          <div v-for="(msg, idx) in messages" :key="idx"
            class="claw-message" :class="msg.role">

            <div v-if="msg.role === 'user'" class="claw-msg-user">
              <div class="claw-msg-content" v-html="formatMessage(msg.content)"></div>
            </div>

            <div v-else class="claw-msg-assistant">
              <div class="claw-msg-avatar">🤖</div>
              <div class="claw-msg-body">
                <div class="claw-msg-sender">PrimusClaw</div>

                <template v-if="!msg.segments || msg.segments.length === 0">
                  <div v-if="!msg.content && loading && idx === messages.length - 1" class="claw-typing">
                    <span></span><span></span><span></span>
                  </div>
                  <div v-else-if="msg.content" class="claw-msg-content" v-html="formatMessage(msg.content)"></div>
                </template>

                <template v-else>
                  <template v-for="(seg, si) in msg.segments" :key="si">
                    <div v-if="seg.type === 'text' && seg.text" class="claw-msg-content" v-html="formatMessage(seg.text)"></div>

                    <div v-else-if="seg.type === 'tool-execution'" class="claw-tool-group">
                      <div class="claw-tool-bar" @click="seg.expanded = !seg.expanded">
                        <span>🔧 Tool Execution ({{ seg.toolCount }})</span>
                        <span :class="{ rotated: !seg.expanded }">▾</span>
                      </div>
                      <div v-if="seg.expanded && seg.toolCalls" class="claw-tool-list">
                        <div v-for="(tc, ti) in seg.toolCalls" :key="ti" class="claw-tool-item">
                          <div class="claw-tool-header" @click="tc.expanded = !tc.expanded">
                            <span class="claw-tool-status" :class="tc.isError ? 'error' : (tc.status === 'start' || tc.status === 'running' ? 'running' : 'success')">
                              {{ tc.status === 'start' || tc.status === 'running' ? '⟳' : tc.isError ? '✕' : '✓' }}
                            </span>
                            <span class="claw-tool-name">{{ tc.name }}</span>
                            <span v-if="tc.brief" class="claw-tool-brief">{{ tc.brief }}</span>
                            <span :class="{ rotated: !tc.expanded }">▾</span>
                          </div>
                          <div v-if="tc.expanded" class="claw-tool-detail">
                            <div v-if="tc.input && Object.keys(tc.input).length" class="claw-tool-section">
                              <div class="claw-tool-section-title">INPUT</div>
                              <pre>{{ JSON.stringify(tc.input, null, 2) }}</pre>
                            </div>
                            <div v-if="tc.output || tc.description" class="claw-tool-section">
                              <div class="claw-tool-section-title">OUTPUT</div>
                              <pre>{{ tc.output || tc.description }}</pre>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Progress Bar (above input, visible during E2E optimization) -->
      <div v-if="e2eProgress.active" class="claw-progress-bar">
        <div class="claw-progress-header">
          <span class="claw-progress-label">E2E Optimization Progress</span>
          <span class="claw-progress-pct">{{ e2eProgress.percent }}%</span>
        </div>
        <div class="claw-progress-track">
          <div class="claw-progress-fill" :style="{ width: e2eProgress.percent + '%' }"></div>
        </div>
        <div class="claw-progress-steps">
          <span v-for="(step, idx) in e2eProgress.steps" :key="idx"
            class="claw-progress-step"
            :class="{ done: step.done, active: step.active }">
            {{ step.done ? '✓' : step.active ? '▶' : '○' }} {{ step.name }}
          </span>
        </div>
      </div>

      <!-- Input area -->
      <div class="claw-input-section">
        <div class="claw-input-wrapper">
          <textarea
            ref="inputRef"
            v-model="userInput"
            placeholder="Ask PrimusClaw to analyze, optimize, or profile..."
            @keydown.enter="handleEnterKey"
            rows="1"
          ></textarea>
          <div class="claw-input-controls">
            <div class="claw-input-left">
              <button class="claw-tools-btn" :class="{ active: selectedToolIds.size > 0 }"
                @click="toggleToolPicker">
                🔧 Tools
                <span v-if="selectedToolIds.size > 0" class="claw-tool-badge">{{ selectedToolIds.size }}</span>
              </button>

            </div>
            <div class="claw-input-right">
              <button v-if="loading" class="claw-stop-btn" @click="stopGeneration">⏹ Stop</button>
              <button class="claw-send-btn" :disabled="!userInput.trim()" @click="sendMessage">
                ➤ Send
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Tool Panel (slides up from bottom, overlays chat area) -->
      <transition name="claw-panel-slide">
        <div v-if="showToolPicker" class="claw-tool-panel">
          <div class="claw-tool-panel-header">
            <span class="claw-tool-panel-title">MCP / Skills</span>
            <span class="claw-tool-panel-count">{{ selectedToolIds.size }} selected / {{ filteredTools.length }} total</span>
            <button v-if="selectedToolIds.size > 0" class="claw-tool-panel-send" @click="confirmTools">
              Confirm ({{ selectedToolIds.size }})
            </button>
            <button class="claw-tool-panel-close" @click="showToolPicker = false">✕</button>
          </div>

          <div class="claw-tool-panel-filters">
            <div class="claw-tool-picker-segment">
              <button class="claw-seg-btn" :class="{ active: toolTypeFilter === 'all' }" @click="setToolTypeFilter('all')">All</button>
              <button class="claw-seg-btn" :class="{ active: toolTypeFilter === 'mcp' }" @click="setToolTypeFilter('mcp')">MCP</button>
              <button class="claw-seg-btn" :class="{ active: toolTypeFilter === 'skill' }" @click="setToolTypeFilter('skill')">Skill</button>
              <button class="claw-seg-btn" :class="{ active: toolTypeFilter === 'mine' }" @click="setToolTypeFilter('mine')">My Tools</button>
            </div>
            <div class="claw-tool-picker-search">
              <input v-model="toolSearchQuery" placeholder="Search tools..." class="claw-tool-search-input" />
            </div>
          </div>

          <div class="claw-tool-panel-body">
            <div v-if="loadingTools" class="claw-loading" style="padding:40px 0;">Loading tools...</div>
            <div v-else-if="toolsLoadError" class="claw-empty claw-empty-error" style="padding:32px 16px;">
              <div>{{ toolsLoadError }}</div>
              <button class="claw-retry-btn" @click="fetchTools">Retry</button>
            </div>
            <div v-else-if="filteredTools.length === 0 && !toolSearchQuery" class="claw-empty" style="padding:32px 16px;">
              No tools available
            </div>
            <template v-else>
              <template v-if="groupedTools.mcps.length">
                <div class="claw-tool-group-label">MCP Servers</div>
                <div v-for="tool in groupedTools.mcps" :key="tool.id"
                  class="claw-tool-picker-item" :class="{ selected: selectedToolIds.has(tool.id) }"
                  @click="toggleTool(tool.id)">
                  <span class="claw-tool-picker-icon">🔌</span>
                  <div class="claw-tool-picker-info">
                    <div class="claw-tool-picker-name">{{ tool.name }}</div>
                    <div v-if="tool.description" class="claw-tool-picker-desc">{{ tool.description }}</div>
                  </div>
                  <span class="claw-tool-picker-check">{{ selectedToolIds.has(tool.id) ? '☑' : '☐' }}</span>
                </div>
              </template>
              <template v-if="groupedTools.skills.length">
                <div class="claw-tool-group-label">Skills</div>
                <div v-for="tool in groupedTools.skills" :key="tool.id"
                  class="claw-tool-picker-item" :class="{ selected: selectedToolIds.has(tool.id) }"
                  @click="toggleTool(tool.id)">
                  <span class="claw-tool-picker-icon">⚡</span>
                  <div class="claw-tool-picker-info">
                    <div class="claw-tool-picker-name">{{ tool.name }}</div>
                    <div v-if="tool.description" class="claw-tool-picker-desc">{{ tool.description }}</div>
                  </div>
                  <span class="claw-tool-picker-check">{{ selectedToolIds.has(tool.id) ? '☑' : '☐' }}</span>
                </div>
              </template>
              <template v-if="groupedTools.other.length">
                <div class="claw-tool-group-label">Other</div>
                <div v-for="tool in groupedTools.other" :key="tool.id"
                  class="claw-tool-picker-item" :class="{ selected: selectedToolIds.has(tool.id) }"
                  @click="toggleTool(tool.id)">
                  <span class="claw-tool-picker-icon">📦</span>
                  <div class="claw-tool-picker-info">
                    <div class="claw-tool-picker-name">{{ tool.name }}</div>
                    <div v-if="tool.description" class="claw-tool-picker-desc">{{ tool.description }}</div>
                  </div>
                  <span class="claw-tool-picker-check">{{ selectedToolIds.has(tool.id) ? '☑' : '☐' }}</span>
                </div>
              </template>
              <div v-if="filteredTools.length === 0 && toolSearchQuery" class="claw-empty" style="padding:16px;">No tools match "{{ toolSearchQuery }}"</div>
            </template>
          </div>
        </div>
      </transition>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { marked } from 'marked';
import {
  getSessions, createSession, deleteSession,
  getSessionMessages, clawChat, processHistoryEvents, getTools,
} from '@/services/claw.js';

defineOptions({ name: 'ClawPage' });

// ========== Config Options ==========

const modelOptions = [
  { value: 'gpt-oss 120B', label: 'gpt-oss 120B', sub: 'MoE · 120B params' },
  { value: 'DeepSeek R1 0528', label: 'DeepSeek R1 0528', sub: 'MoE · 671B params' },
  { value: 'Llama 3.1 405B', label: 'Llama 3.1 405B', sub: 'Dense · 405B params' },
];
const frameworkOptions = [
  { value: 'SGLang', label: 'SGLang', sub: 'Recommended for AMD' },
  { value: 'vLLM', label: 'vLLM', sub: 'General purpose' },
  { value: 'TRT-LLM', label: 'TRT-LLM', sub: 'Nvidia only' },
];
const gpuOptions = [
  { value: 'MI355X', label: 'MI355X', sub: 'CDNA4 · 288GB HBM3e' },
  { value: 'MI300X', label: 'MI300X', sub: 'CDNA3 · 192GB HBM3' },
  { value: 'B200', label: 'B200', sub: 'Blackwell · 192GB' },
  { value: 'H100', label: 'H100', sub: 'Hopper · 80GB' },
];
const precisionOptions = [
  { value: 'FP4', label: 'FP4', sub: 'Higher throughput' },
  { value: 'FP8', label: 'FP8', sub: 'Balanced' },
  { value: 'FP16', label: 'FP16', sub: 'Full precision' },
];
const islOptions = [
  { value: '1K / 1K', label: '1K / 1K' },
  { value: '1K / 8K', label: '1K / 8K' },
  { value: '8K / 1K', label: '8K / 1K' },
  { value: '8K / 8K', label: '8K / 8K' },
];
const concurrencyOptions = [
  { value: '1', label: '1' },
  { value: '4', label: '4' },
  { value: '8', label: '8' },
  { value: '16', label: '16' },
  { value: '32', label: '32' },
];
const traceOptions = [
  { value: 'PyTorch', label: 'PyTorch', sub: 'torch.profiler' },
  { value: 'ROCm', label: 'ROCm', sub: 'rocprof' },
  { value: 'Nsight', label: 'Nsight', sub: 'Nvidia only' },
];

const configLabels = {
  model: 'Model',
  framework: 'Framework',
  gpu: 'GPU',
  precision: 'Precision',
  isl: 'ISL / OSL',
  concurrency: 'Concurrency',
  trace: 'Trace',
};

// ========== E2E Config State ==========

const e2eConfig = reactive({
  model: '',
  framework: '',
  gpu: '',
  precision: '',
  isl: '',
  concurrency: '',
  trace: '',
});

const customInputs = reactive({
  model: '', framework: '', gpu: '', precision: '',
  isl: '', concurrency: '', trace: '',
});

const e2eStarted = ref(false);
const wizardStep = ref(1);

const warnings = computed(() => ({
  fwGpu: e2eConfig.framework === 'TRT-LLM' && (e2eConfig.gpu === 'MI355X' || e2eConfig.gpu === 'MI300X'),
  precision: e2eConfig.precision === 'FP4' && e2eConfig.gpu === 'MI300X',
}));

const e2eProgress = reactive({
  active: false,
  percent: 0,
  steps: [
    { name: 'Profiling', done: false, active: false },
    { name: 'TraceLens', done: false, active: false },
    { name: 'GEAK Optimize', done: false, active: false },
    { name: 'Validate', done: false, active: false },
    { name: 'Report', done: false, active: false },
  ],
});

// ========== Wizard Logic ==========

function pickOption(key, value) {
  e2eConfig[key] = value;
  advanceWizard();
}

function applyCustom(key) {
  const val = customInputs[key]?.trim();
  if (!val) return;
  e2eConfig[key] = val;
  customInputs[key] = '';
  advanceWizard();
}

function advanceWizard() {
  nextTick(() => {
    if (e2eConfig.model && wizardStep.value < 2) {
      wizardStep.value = 2;
    }
    if (e2eConfig.framework && e2eConfig.gpu && wizardStep.value < 3) {
      wizardStep.value = 3;
    }
    if (e2eConfig.precision && e2eConfig.isl && e2eConfig.concurrency && e2eConfig.trace && wizardStep.value < 4) {
      wizardStep.value = 4;
    }
    nextTick(scrollToBottom);
  });
}

function resetWizard() {
  e2eStarted.value = false;
  e2eProgress.active = false;
  e2eProgress.percent = 0;
  e2eProgress.steps.forEach(s => { s.done = false; s.active = false; });
  messages.value = [];
  wizardStep.value = 1;
  if (e2eConfig.model) wizardStep.value = 2;
  if (e2eConfig.framework && e2eConfig.gpu) wizardStep.value = 3;
  if (e2eConfig.precision && e2eConfig.isl && e2eConfig.concurrency && e2eConfig.trace) wizardStep.value = 4;
}

function startE2EOptimization() {
  e2eStarted.value = true;
  const configStr = `Start E2E optimization: **${e2eConfig.model}** on **${e2eConfig.gpu}**, ${e2eConfig.framework}, ${e2eConfig.precision}, ISL/OSL ${e2eConfig.isl}, concurrency ${e2eConfig.concurrency}, trace ${e2eConfig.trace}`;
  userInput.value = configStr;
  nextTick(() => {
    sendMessage();
    e2eProgress.active = true;
    simulateProgress();
  });
}

function simulateProgress() {
  const vals = [20, 45, 70, 90, 100];
  let i = 0;
  function tick() {
    if (i >= vals.length) return;
    e2eProgress.percent = vals[i];
    for (let j = 0; j <= i; j++) {
      e2eProgress.steps[j].done = true;
      e2eProgress.steps[j].active = false;
    }
    if (i + 1 < vals.length) {
      e2eProgress.steps[i + 1].active = true;
    }
    i++;
    if (i < vals.length) setTimeout(tick, 3000);
  }
  e2eProgress.steps[0].active = true;
  setTimeout(tick, 1500);
}

// ========== UI State ==========

const sidebarOpen = ref(true);
const historyCollapsed = ref(false);
const showToolPicker = ref(false);
const userInput = ref('');
const messages = ref([]);
const loading = ref(false);
const loadingMessages = ref(false);
const liveStatus = ref('');
const messagesContainer = ref(null);
const inputRef = ref(null);

const sessions = ref([]);
const loadingSessions = ref(false);
const currentSessionId = ref('');

const tools = ref([]);
const loadingTools = ref(false);
const selectedToolIds = ref(new Set());
const toolTypeFilter = ref('all');
const toolsLoadError = ref('');

let abortController = null;

// ========== Tools Filtering ==========

const toolSearchQuery = ref('');

const normalizeToolType = (tool) => {
  if (!tool || typeof tool !== 'object') return tool;
  const normalized = { ...tool };
  if (typeof normalized.type === 'string') {
    normalized.type = normalized.type.toLowerCase();
  }
  return normalized;
};

const typeFilteredTools = computed(() => {
  const type = toolTypeFilter.value;
  if (type === 'mcp') return tools.value.filter(t => String(t.type || '').toLowerCase() === 'mcp');
  if (type === 'skill') return tools.value.filter(t => String(t.type || '').toLowerCase() === 'skill');
  return tools.value;
});

const filteredTools = computed(() => {
  const q = toolSearchQuery.value.toLowerCase().trim();
  const list = typeFilteredTools.value;
  if (!q) return list;
  return list.filter(t =>
    t.name?.toLowerCase().includes(q) ||
    t.description?.toLowerCase().includes(q)
  );
});

const groupedTools = computed(() => {
  const list = filteredTools.value;
  const skills = list.filter(t => String(t.type || '').toLowerCase() === 'skill');
  const mcps = list.filter(t => String(t.type || '').toLowerCase() === 'mcp');
  const other = list.filter(t => {
    const tp = String(t.type || '').toLowerCase();
    return tp !== 'skill' && tp !== 'mcp';
  });
  return { skills, mcps, other };
});

// ========== Helpers ==========

function fixTokenSpacing(text) {
  if (!text) return text;
  const codeBlocks = [];
  let out = text.replace(/```[\s\S]*?```|`[^`]+`/g, (m) => {
    codeBlocks.push(m);
    return `\x00CB${codeBlocks.length - 1}\x00`;
  });

  out = out
    .replace(/([.!?])([A-Za-z])/g, '$1 $2')
    .replace(/([,;:])([A-Za-z])/g, '$1 $2')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/([)\]}>])([A-Za-z])/g, '$1 $2');

  out = out.replace(/\x00CB(\d+)\x00/g, (_, i) => codeBlocks[parseInt(i)]);
  return out;
}

const formatMessage = (content) => {
  if (!content) return '';
  try {
    const fixed = fixTokenSpacing(content);
    return marked.parse(fixed, { breaks: true, gfm: true });
  } catch {
    const escaped = content
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/\n/g, '<br>');
    return `<p>${escaped}</p>`;
  }
};

const setInput = (text) => {
  userInput.value = text;
  nextTick(() => inputRef.value?.focus());
};

const scrollToBottom = () => {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight;
  }
};

const autoScroll = () => {
  nextTick(() => {
    if (messagesContainer.value) {
      const c = messagesContainer.value;
      if (c.scrollHeight - c.scrollTop - c.clientHeight < 200) c.scrollTop = c.scrollHeight;
    }
  });
};

const toggleTool = (id) => {
  const s = new Set(selectedToolIds.value);
  if (s.has(id)) s.delete(id); else s.add(id);
  selectedToolIds.value = s;
};

const setToolTypeFilter = (filterType) => {
  if (toolTypeFilter.value === filterType) return;
  toolTypeFilter.value = filterType;
  fetchTools();
};

const toggleToolPicker = () => {
  showToolPicker.value = !showToolPicker.value;
  if (showToolPicker.value && (tools.value.length === 0 || toolsLoadError.value)) {
    fetchTools();
  }
};

const confirmTools = () => {
  showToolPicker.value = false;
  const names = tools.value
    .filter(t => selectedToolIds.value.has(t.id))
    .map(t => t.display_name || t.name);
  if (names.length && !userInput.value.trim()) {
    userInput.value = `Use tools: ${names.join(', ')}`;
  }
  nextTick(() => {
    const el = document.querySelector('.claw-input-wrapper textarea');
    if (el) el.focus();
  });
};

watch(userInput, () => {
  nextTick(() => {
    if (inputRef.value) {
      inputRef.value.style.height = 'auto';
      inputRef.value.style.height = inputRef.value.scrollHeight + 'px';
    }
  });
});

// ========== API Calls ==========

const fetchSessions = async () => {
  loadingSessions.value = true;
  try {
    const res = await getSessions();
    sessions.value = res.data || [];
  } catch (error) {
    console.error('Failed to fetch sessions:', error);
  } finally {
    loadingSessions.value = false;
  }
};

const fetchTools = async () => {
  loadingTools.value = true;
  toolsLoadError.value = '';
  try {
    const pageSize = 100;
    const maxPages = 30;
    let offset = 0;
    let total = Number.POSITIVE_INFINITY;
    let page = 0;
    const allTools = [];

    while (page < maxPages && offset < total) {
      const params = { offset, limit: pageSize, order: 'desc' };
      if (toolTypeFilter.value === 'mcp' || toolTypeFilter.value === 'skill') {
        params.type = toolTypeFilter.value;
      } else if (toolTypeFilter.value === 'mine') {
        params.owner = 'me';
      }

      const res = await getTools(params);
      const batch = Array.isArray(res?.tools) ? res.tools : [];
      const totalFromApi = Number(res?.total);
      if (Number.isFinite(totalFromApi) && totalFromApi >= 0) {
        total = totalFromApi;
      }

      if (batch.length === 0) break;

      allTools.push(...batch.map(normalizeToolType));
      offset += batch.length;
      page += 1;

      if (batch.length < pageSize) break;
    }

    // Dedupe by id (keep latest record)
    const dedupMap = new Map();
    allTools.forEach((tool) => {
      if (!tool) return;
      const key = tool.id != null ? String(tool.id) : `${tool.name || 'unknown'}-${tool.type || 'unknown'}`;
      dedupMap.set(key, tool);
    });

    tools.value = Array.from(dedupMap.values());

    // Clear selections for removed tools
    const validIds = new Set(tools.value.map(t => t.id));
    selectedToolIds.value = new Set(
      [...selectedToolIds.value].filter(id => validIds.has(id))
    );
  } catch (error) {
    console.error('Failed to fetch tools:', error);
    const msg = String(error?.message || error || '');
    if (msg.includes('401')) {
      toolsLoadError.value = 'Session expired. Redirecting to login...';
      localStorage.removeItem('hl-user');
      setTimeout(() => { window.location.href = '/hyperloom/login'; }, 1500);
    } else if (msg.includes('403')) {
      toolsLoadError.value = 'Access denied — your account does not have permission to list tools.';
    } else if (msg.includes('Unexpected token') || msg.includes('JSON')) {
      toolsLoadError.value = 'Tools API returned non-JSON data. Check proxy configuration.';
    } else {
      toolsLoadError.value = 'Failed to fetch tools — please try again.';
    }
    tools.value = [];
  } finally {
    loadingTools.value = false;
  }
};

// ========== Session Management ==========

const loadSession = async (session) => {
  currentSessionId.value = session.session_id;
  loadingMessages.value = true;
  showToolPicker.value = false;
  e2eStarted.value = true;

  try {
    const res = await getSessionMessages(session.session_id);
    if (res.data && res.data.length > 0) {
      messages.value = processHistoryEvents(res.data);
      await nextTick();
      scrollToBottom();
    } else {
      messages.value = [];
    }
  } catch (error) {
    console.error('Failed to load session messages:', error);
    messages.value = [];
  } finally {
    loadingMessages.value = false;
  }
};

const startNewSession = () => {
  currentSessionId.value = '';
  messages.value = [];
  loadingMessages.value = false;
  liveStatus.value = '';
  e2eStarted.value = false;
  e2eProgress.active = false;
  e2eProgress.percent = 0;
  e2eProgress.steps.forEach(s => { s.done = false; s.active = false; });
  Object.keys(e2eConfig).forEach(k => { e2eConfig[k] = ''; });
  wizardStep.value = 1;
};

const handleDeleteSession = async (sessionId) => {
  if (!confirm('Delete this session?')) return;
  try {
    await deleteSession(sessionId);
    if (currentSessionId.value === sessionId) startNewSession();
    await fetchSessions();
  } catch (error) {
    console.error('Failed to delete session:', error);
  }
};

// ========== Chat ==========

const handleEnterKey = (event) => {
  if (event.shiftKey) return;
  event.preventDefault();
  sendMessage();
};

const sendMessage = async () => {
  const query = userInput.value.trim();
  if (!query) return;

  if (!e2eStarted.value) e2eStarted.value = true;

  messages.value.push({ role: 'user', content: query });
  userInput.value = '';
  nextTick(() => { if (inputRef.value) inputRef.value.style.height = 'auto'; });

  const assistantIndex = messages.value.length;
  messages.value.push({ role: 'assistant', content: '', segments: [] });

  await nextTick();
  scrollToBottom();

  loading.value = true;
  liveStatus.value = '';
  abortController = new AbortController();

  if (!currentSessionId.value) {
    try {
      const res = await createSession({
        name: query.slice(0, 50),
        agent_id: 'agent_default',
      });
      currentSessionId.value = res.data.session_id;
      fetchSessions();
    } catch (error) {
      console.error('Failed to create session:', error);
      messages.value[assistantIndex].content = 'Failed to create session. Please try again.';
      messages.value[assistantIndex].segments = undefined;
      loading.value = false;
      return;
    }
  }

  const msg = () => messages.value[assistantIndex];

  const appendText = (content) => {
    const m = msg();

    if (m.content.length > 0 && content.length > 0) {
      const last = m.content[m.content.length - 1];
      const first = content[0];
      if (last !== ' ' && last !== '\n' && first !== ' ' && first !== '\n' && !/[*_#`~\[(<>|{/\\]/.test(first)) {
        const needsSpace =
          (/[a-zA-Z0-9]/.test(last) && /[a-zA-Z]/.test(first)) ||
          (/[.!?;,:]/.test(last) && /[a-zA-Z]/.test(first)) ||
          (/[)\]}>]/.test(last) && /[a-zA-Z]/.test(first));
        if (needsSpace) content = ' ' + content;
      }
    }

    m.content += content;
    const segs = m.segments;
    const lastSeg = segs[segs.length - 1];
    if (lastSeg?.type === 'text') {
      lastSeg.text = (lastSeg.text || '') + content;
    } else {
      segs.push({ type: 'text', text: content });
    }
    autoScroll();
  };

  const handleToolUsed = (data) => {
    if (data.tool === 'suggestion') return;
    const m = msg();
    const segs = m.segments;

    if (data.status === 'start' || data.status === 'running') {
      const last = segs[segs.length - 1];
      let toolSeg = last?.type === 'tool-execution' ? last : null;
      if (!toolSeg) {
        toolSeg = { type: 'tool-execution', toolCalls: [], toolCount: 0, expanded: true };
        segs.push(toolSeg);
      }
      const existing = toolSeg.toolCalls.find(t => t.toolUseId === data.actionId);
      if (existing) {
        if (data.brief) existing.brief = data.brief;
        if (data.description) existing.description = data.description;
      } else {
        toolSeg.toolCalls.push({
          toolUseId: data.actionId || '',
          name: data.tool || data.brief || 'Unknown',
          tool: data.tool || '',
          status: data.status,
          brief: data.brief || '',
          description: data.description || '',
          input: data.argumentsDetail || undefined,
          isError: false,
          expanded: false,
        });
        toolSeg.toolCount = toolSeg.toolCalls.length;
      }
    } else if (data.status === 'success' || data.status === 'error') {
      for (const seg of segs) {
        if (seg.type !== 'tool-execution') continue;
        const tc = seg.toolCalls?.find(t => t.toolUseId === data.actionId);
        if (tc) {
          tc.status = data.status;
          tc.isError = data.status === 'error';
          if (data.tool) { tc.name = data.tool; tc.tool = data.tool; }
          if (data.brief) tc.brief = data.brief;
          if (data.description) { tc.description = data.description; tc.output = data.description; }
          break;
        }
      }
    }
    autoScroll();
  };

  try {
    await clawChat(
      { query, session_id: currentSessionId.value, tools: [...selectedToolIds.value] },
      appendText,
      (error) => {
        console.error('Chat error:', error);
        const m = msg();
        m.content = 'Sorry, an error occurred. Please try again.';
        m.segments = undefined;
        loading.value = false;
        liveStatus.value = '';
      },
      () => { loading.value = false; liveStatus.value = ''; },
      abortController.signal,
      {
        onToolUsed: handleToolUsed,
        onLiveStatus: (data) => { liveStatus.value = data.text || ''; },
      },
    );
  } catch (err) {
    console.error('Send message error:', err);
    const m = msg();
    m.content = 'Sorry, an error occurred. Please try again.';
    m.segments = undefined;
    loading.value = false;
    liveStatus.value = '';
  }
};

const stopGeneration = () => {
  if (abortController) {
    abortController.abort();
    loading.value = false;
    liveStatus.value = '';
  }
};

// Lifecycle
onMounted(async () => {
  await Promise.all([fetchTools(), fetchSessions()]);
});

onUnmounted(() => {
  if (abortController) abortController.abort();
});
</script>
