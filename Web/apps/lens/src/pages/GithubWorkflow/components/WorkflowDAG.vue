<template>
  <div class="workflow-dag" v-if="jobs.length > 0">
    <!-- Jobs grouped by dependency level -->
    <div 
      v-for="(level, levelIndex) in jobLevels" 
      :key="levelIndex" 
      class="dag-level"
    >
      <div 
        v-for="job in level" 
        :key="job.id" 
        class="job-node"
        :class="getJobClass(job)"
        @click="$emit('job-click', job)"
      >
        <div class="job-header">
          <span class="job-icon">
            <el-icon v-if="job.status === 'completed' && job.conclusion === 'success'"><Check /></el-icon>
            <el-icon v-else-if="job.status === 'completed' && job.conclusion === 'failure'"><Close /></el-icon>
            <el-icon v-else-if="job.status === 'in_progress'" class="spinning"><Loading /></el-icon>
            <el-icon v-else-if="job.status === 'queued'"><Clock /></el-icon>
            <el-icon v-else><More /></el-icon>
          </span>
          <span class="job-name" :title="job.name">{{ job.name }}</span>
        </div>
        <div class="job-meta">
          <span v-if="job.durationSeconds > 0" class="duration">
            {{ formatDuration(job.durationSeconds) }}
          </span>
          <span v-if="job.stepsCount > 0" class="steps">
            {{ job.stepsCompleted }}/{{ job.stepsCount }} steps
          </span>
        </div>
        <!-- Connector lines to dependencies -->
        <div 
          v-if="job.needs && job.needs.length > 0" 
          class="connectors"
        >
          <div 
            v-for="need in job.needs" 
            :key="need" 
            class="connector-line"
            :style="getConnectorStyle(levelIndex, job, need)"
          ></div>
        </div>
      </div>
    </div>
  </div>
  <el-empty v-else description="No jobs found" :image-size="80" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Check, Close, Loading, Clock, More } from '@element-plus/icons-vue'
import type { GithubJobNode } from '@/services/workflow-metrics'

const props = defineProps<{
  jobs: GithubJobNode[]
}>()

defineEmits<{
  (e: 'job-click', job: GithubJobNode): void
}>()

// Build job levels based on dependencies
const jobLevels = computed(() => {
  if (!props.jobs.length) return []
  
  const jobMap = new Map<string, GithubJobNode>()
  props.jobs.forEach(job => jobMap.set(job.name, job))
  
  const levels: GithubJobNode[][] = []
  const placed = new Set<string>()
  
  // First pass: jobs with no dependencies go to level 0
  const level0 = props.jobs.filter(job => !job.needs || job.needs.length === 0)
  if (level0.length > 0) {
    levels.push(level0)
    level0.forEach(job => placed.add(job.name))
  }
  
  // Subsequent passes: place jobs whose dependencies are all placed
  let maxIterations = props.jobs.length
  while (placed.size < props.jobs.length && maxIterations > 0) {
    const nextLevel: GithubJobNode[] = []
    
    for (const job of props.jobs) {
      if (placed.has(job.name)) continue
      
      const depsPlaced = !job.needs || job.needs.every(dep => placed.has(dep))
      if (depsPlaced) {
        nextLevel.push(job)
      }
    }
    
    if (nextLevel.length > 0) {
      levels.push(nextLevel)
      nextLevel.forEach(job => placed.add(job.name))
    }
    
    maxIterations--
  }
  
  // Add any remaining jobs (circular dependencies)
  const remaining = props.jobs.filter(job => !placed.has(job.name))
  if (remaining.length > 0) {
    levels.push(remaining)
  }
  
  return levels
})

const getJobClass = (job: GithubJobNode) => {
  const classes: string[] = []
  
  if (job.status === 'completed') {
    if (job.conclusion === 'success') classes.push('job-success')
    else if (job.conclusion === 'failure') classes.push('job-failure')
    else if (job.conclusion === 'cancelled') classes.push('job-cancelled')
    else if (job.conclusion === 'skipped') classes.push('job-skipped')
  } else if (job.status === 'in_progress') {
    classes.push('job-running')
  } else if (job.status === 'queued') {
    classes.push('job-queued')
  }
  
  return classes
}

const formatDuration = (seconds: number) => {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
}

const getConnectorStyle = (_levelIndex: number, _job: GithubJobNode, _need: string) => {
  // Simple vertical connector style
  return {}
}
</script>

<style scoped lang="scss">
.workflow-dag {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 16px;
  background: var(--el-bg-color-page);
  border-radius: 8px;
  overflow-x: auto;
  
  .dag-level {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    justify-content: flex-start;
    position: relative;
    
    &::before {
      content: '';
      position: absolute;
      left: 50%;
      top: -12px;
      height: 12px;
      width: 2px;
      background: var(--el-border-color);
    }
    
    &:first-child::before {
      display: none;
    }
  }
  
  .job-node {
    min-width: 200px;
    max-width: 280px;
    padding: 12px 16px;
    background: var(--el-bg-color);
    border: 2px solid var(--el-border-color);
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s ease;
    position: relative;
    
    &:hover {
      border-color: var(--el-color-primary);
      box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
    }
    
    &.job-success {
      border-color: var(--el-color-success);
      background: var(--el-color-success-light-9);
      
      .job-icon {
        color: var(--el-color-success);
      }
    }
    
    &.job-failure {
      border-color: var(--el-color-danger);
      background: var(--el-color-danger-light-9);
      
      .job-icon {
        color: var(--el-color-danger);
      }
    }
    
    &.job-running {
      border-color: var(--el-color-warning);
      background: var(--el-color-warning-light-9);
      
      .job-icon {
        color: var(--el-color-warning);
      }
    }
    
    &.job-queued {
      border-color: var(--el-color-info);
      background: var(--el-color-info-light-9);
      
      .job-icon {
        color: var(--el-color-info);
      }
    }
    
    &.job-cancelled,
    &.job-skipped {
      border-color: var(--el-text-color-disabled);
      background: var(--el-fill-color-light);
      opacity: 0.7;
    }
    
    .job-header {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 8px;
      
      .job-icon {
        font-size: 18px;
        display: flex;
        align-items: center;
        
        .spinning {
          animation: spin 1s linear infinite;
        }
      }
      
      .job-name {
        font-weight: 500;
        font-size: 14px;
        color: var(--el-text-color-primary);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
    
    .job-meta {
      display: flex;
      gap: 12px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      
      .duration {
        font-family: monospace;
      }
    }
  }
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
