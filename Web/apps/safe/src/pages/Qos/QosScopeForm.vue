<template>
  <el-form label-width="140px">
    <el-form-item label="CPU">
      <el-input v-model="model.cpu" clearable placeholder="e.g. 64" />
    </el-form-item>
    <el-form-item label="Memory">
      <el-input v-model="model.memory" clearable placeholder="e.g. 512Gi" />
    </el-form-item>
    <el-form-item label="GPU">
      <el-input v-model="model.gpu" clearable placeholder="card count, e.g. 8" />
    </el-form-item>
    <el-form-item label="Timeout">
      <div class="limit-entries">
        <div v-for="kind in configuredKinds" :key="kind" class="limit-row">
          <el-tag closable effect="plain" @close="removeKind(kind)">{{ kind }}</el-tag>
          <el-input-number
            v-model="model.runtimeHours[kind]"
            :min="0"
            :step="1"
            :precision="2"
            controls-position="right"
            size="small"
            class="limit-input"
          />
          <span class="limit-unit">h</span>
        </div>
        <el-dropdown v-if="availableKinds.length" trigger="click" @command="addKind">
          <el-button size="small" text type="primary">+ Add kind</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="kind in availableKinds" :key="kind" :command="kind">
                {{ kind }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </el-form-item>
  </el-form>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { WORKLOAD_KINDS, type ScopeDraft } from '@/services/qos/type'

const model = defineModel<ScopeDraft>({ required: true })

const configuredKinds = computed(() =>
  Object.keys(model.value.runtimeHours).filter((kind) => WORKLOAD_KINDS.includes(kind)),
)

const availableKinds = computed(() =>
  WORKLOAD_KINDS.filter((kind) => !(kind in model.value.runtimeHours)),
)

/** addKind starts an empty timeout for one workload kind. */
const addKind = (kind: string) => {
  model.value.runtimeHours[kind] = undefined
}

/** removeKind drops the timeout so that kind falls through. */
const removeKind = (kind: string) => {
  delete model.value.runtimeHours[kind]
}
</script>

<style scoped>
.limit-entries {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.limit-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.limit-input {
  width: 140px;
}
.limit-unit {
  color: var(--el-text-color-secondary);
}
</style>
