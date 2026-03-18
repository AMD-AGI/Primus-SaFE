<template>
  <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4 mt-4" v-loading="loading">
    <el-card
      v-for="agent in parsedAgents"
      :key="agent.id"
      class="safe-card agent-card"
      shadow="never"
    >
      <div class="flex items-start justify-between">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <el-text class="font-600 text-base" tag="b" truncated>
              {{ agent.displayName || agent.serviceName }}
            </el-text>
            <el-tag v-if="agent.cardVersion" size="small" type="info">
              v{{ agent.cardVersion }}
            </el-tag>
          </div>
          <el-text class="block mt-1 text-sm" type="info" truncated>
            {{ agent.description || 'No description' }}
          </el-text>
        </div>
        <el-tooltip content="Delete" placement="top">
          <el-button
            circle
            size="small"
            class="btn-danger-plain"
            :icon="Delete"
            @click="$emit('delete', agent.serviceName)"
          />
        </el-tooltip>
      </div>

      <el-divider style="margin: 12px 0" />

      <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
        <span class="flex items-center gap-1">
          <el-icon :color="healthColor[agent.a2aHealth] || '#9ca3af'" :size="14">
            <component :is="agent.a2aHealth === 'healthy' ? SuccessFilled : CircleCloseFilled" />
          </el-icon>
          {{ agent.a2aHealth || 'unknown' }}
        </span>
        <span>
          <el-text type="info">Skills:</el-text>
          {{ agent.parsedSkills.length }}
        </span>
        <span v-if="agent.cardProvider">
          <el-text type="info">Provider:</el-text>
          {{ agent.cardProvider }}
        </span>
        <el-tag size="small" :type="agent.discoverySource === 'k8s-scanner' ? '' : 'warning'">
          {{ agent.discoverySource }}
        </el-tag>
      </div>

      <div v-if="agent.parsedSkills.length" class="flex flex-wrap gap-1 mt-3">
        <el-tag
          v-for="skill in agent.parsedSkills.slice(0, 6)"
          :key="skill.id || skill.name"
          size="small"
          effect="plain"
        >
          {{ skill.name || skill.id }}
        </el-tag>
        <el-tag v-if="agent.parsedSkills.length > 6" size="small" type="info" effect="plain">
          +{{ agent.parsedSkills.length - 6 }} more
        </el-tag>
      </div>

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
</style>
