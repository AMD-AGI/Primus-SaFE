<template>
  <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4 mt-4" v-loading="loading">
    <el-card
      v-for="agent in parsedAgents"
      :key="agent.id"
      class="safe-card agent-card"
      shadow="never"
    >
      <!-- Header: name + version + delete -->
      <div class="flex items-center justify-between mb-2">
        <div class="flex items-center gap-2 min-w-0 flex-1">
          <span class="font-600 text-base truncate">
            {{ agent.displayName || agent.serviceName }}
          </span>
          <el-tag v-if="agent.cardVersion" size="small" effect="light" class="version-tag" round>
            v{{ agent.cardVersion }}
          </el-tag>
        </div>
        <el-tooltip content="Delete" placement="top">
          <el-button
            circle
            size="small"
            class="btn-danger-plain ml-2"
            :icon="Delete"
            @click="$emit('delete', agent.serviceName)"
          />
        </el-tooltip>
      </div>

      <!-- Description -->
      <el-text class="block text-sm mb-3" type="info" line-clamp="2">
        {{ agent.description || 'No description' }}
      </el-text>

      <!-- Meta row -->
      <div class="meta-row">
        <span class="meta-item" :style="{ color: healthColor[agent.a2aHealth] || '#9ca3af' }">
          <el-icon :size="14">
            <component :is="agent.a2aHealth === 'healthy' ? SuccessFilled : CircleCloseFilled" />
          </el-icon>
          {{ agent.a2aHealth || 'unknown' }}
        </span>
        <span class="meta-item">
          <el-text type="info" size="small">Skills</el-text>
          <b>{{ agent.parsedSkills.length }}</b>
        </span>
        <span v-if="agent.cardProvider" class="meta-item">
          <el-text type="info" size="small">Provider</el-text>
          <b>{{ agent.cardProvider }}</b>
        </span>
        <el-tag size="small" effect="dark" round :type="agent.discoverySource === 'k8s-scanner' ? 'info' : 'warning'">
          {{ agent.discoverySource }}
        </el-tag>
      </div>

      <!-- Skills -->
      <div v-if="agent.parsedSkills.length" class="skill-list">
        <span
          v-for="skill in agent.parsedSkills.slice(0, 5)"
          :key="skill.id || skill.name"
          class="skill-chip"
        >
          {{ skill.name || skill.id }}
        </span>
        <span v-if="agent.parsedSkills.length > 5" class="skill-chip skill-chip--more">
          +{{ agent.parsedSkills.length - 5 }}
        </span>
      </div>

      <!-- Footer -->
      <div class="mt-3 text-xs">
        <el-text type="info">Last seen: {{ formatTimeStr(agent.a2aLastSeen) }}</el-text>
      </div>
    </el-card>
  </div>

  <el-empty v-if="!loading && !services.length" description="No agents registered" class="mt-8" />
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { Delete, SuccessFilled, CircleCloseFilled } from '@element-plus/icons-vue'
import { formatTimeStr } from '@/utils'
import type { A2AService } from '@/services'

const props = defineProps<{
  services: A2AService[]
  loading: boolean
}>()

defineEmits<{
  delete: [serviceName: string]
}>()

interface ParsedSkill {
  id?: string
  name?: string
}

const healthColor: Record<string, string> = {
  healthy: '#10b981',
  unhealthy: '#ef4444',
  unknown: '#9ca3af',
}

const parsedAgents = computed(() =>
  props.services.map((s) => {
    let cardVersion = ''
    let cardProvider = ''
    let parsedSkills: ParsedSkill[] = []

    try {
      if (s.a2aAgentCard) {
        const card = JSON.parse(s.a2aAgentCard)
        cardVersion = card.version || ''
        cardProvider = card.provider?.organization || ''
      }
    } catch { /* ignore parse errors */ }

    try {
      if (s.a2aSkills) {
        parsedSkills = JSON.parse(s.a2aSkills) || []
      }
    } catch { /* ignore parse errors */ }

    return { ...s, cardVersion, cardProvider, parsedSkills }
  }),
)
</script>

<style scoped>
.agent-card {
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}
.agent-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
}

.meta-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--el-border-color-lighter);
  border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
}
.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.skill-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}
.skill-chip {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  line-height: 20px;
  background: var(--el-fill-color);
  color: var(--el-text-color-regular);
}
.skill-chip--more {
  background: transparent;
  color: var(--el-text-color-secondary);
}

.version-tag {
  background: rgba(0, 229, 229, 0.12) !important;
  border-color: rgba(0, 229, 229, 0.3) !important;
  color: #00a3a3 !important;
  font-weight: 500;
}
</style>
