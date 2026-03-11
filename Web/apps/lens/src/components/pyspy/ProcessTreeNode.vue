<template>
  <div class="process-tree-node">
    <div
      class="process-item"
      :class="{
        'process-item--selected': isSelected,
        'process-item--python': process.isPython
      }"
      @click="handleClick"
    >
      <div class="process-info">
        <el-icon
          v-if="hasChildren"
          class="expand-icon"
          :class="{ 'is-expanded': expanded }"
          @click.stop="toggleExpand"
        >
          <CaretRight />
        </el-icon>
        <el-icon v-if="process.isPython" class="python-icon">
          <ChromeFilled />
        </el-icon>
        <span class="process-command">{{ process.command }}</span>
        <el-tag size="small" type="info" class="pid-tag">
          PID: {{ process.hostPid }}
        </el-tag>
      </div>
      <div class="process-meta">
        <span v-if="process.containerName" class="container-name">{{ process.containerName }}</span>
        <span class="state">{{ process.state }}</span>
        <span v-if="process.threads > 1" class="threads">{{ process.threads }} threads</span>
      </div>
    </div>

    <div v-if="hasChildren && expanded" class="process-children">
      <ProcessTreeNode
        v-for="child in process.children"
        :key="child.hostPid"
        :process="child"
        :selected-pid="selectedPid"
        @select="$emit('select', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { CaretRight, ChromeFilled } from '@element-plus/icons-vue'
import type { NormalizedProcessInfo } from '@/services/pyspy'

interface Props {
  process: NormalizedProcessInfo
  selectedPid?: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  select: [process: NormalizedProcessInfo]
}>()

const expanded = ref(true)

const hasChildren = computed(() => 
  props.process.children && props.process.children.length > 0
)

const isSelected = computed(() => 
  props.selectedPid === props.process.hostPid
)

const toggleExpand = () => {
  expanded.value = !expanded.value
}

const handleClick = () => {
  if (props.process.isPython) {
    emit('select', props.process)
  }
}
</script>

<style scoped lang="scss">
.process-tree-node {
  .process-item {
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s;
    margin-bottom: 4px;

    &:hover {
      background-color: var(--el-fill-color-light);
    }

    &--selected {
      background-color: var(--el-color-primary-light-9);
      border-left: 3px solid var(--el-color-primary);
    }

    &--python {
      border-left: 2px solid var(--el-color-success);
    }

    .process-info {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 4px;

      .expand-icon {
        cursor: pointer;
        transition: transform 0.2s;
        flex-shrink: 0;

        &.is-expanded {
          transform: rotate(90deg);
        }
      }

      .python-icon {
        color: var(--el-color-success);
        flex-shrink: 0;
      }

      .process-command {
        font-weight: 500;
        flex: 1;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .pid-tag {
        flex-shrink: 0;
      }
    }

    .process-meta {
      display: flex;
      gap: 12px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      margin-left: 28px;

      .container-name {
        color: var(--el-color-primary);
      }

      .state,
      .threads {
        font-family: 'Consolas', 'Monaco', monospace;
      }
    }
  }

  .process-children {
    margin-left: 24px;
  }
}
</style>
