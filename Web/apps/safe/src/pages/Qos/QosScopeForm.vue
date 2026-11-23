<template>
  <el-form label-width="140px">
    <el-form-item label="CPU">
      <el-input v-model="model.cpu" clearable placeholder="e.g. 64" />
    </el-form-item>
    <el-form-item label="Memory">
      <el-input v-model="model.memory" clearable placeholder="e.g. 512">
        <template #append>Gi</template>
      </el-input>
    </el-form-item>
    <el-form-item label="GPU">
      <el-input v-model="model.gpu" clearable placeholder="card count, e.g. 8" />
    </el-form-item>
    <el-form-item label="Timeout">
      <div class="limit-entries">
        <div v-for="scope in configuredScopes" :key="scope" class="limit-row">
          <el-tag closable effect="plain" @close="removeScope(scope)">{{ scope }}</el-tag>
          <el-input-number
            v-model="model.runtimeHours[scope]"
            :min="0"
            :step="1"
            :precision="2"
            controls-position="right"
            size="small"
            class="limit-input"
          />
          <span class="limit-unit">h</span>
        </div>
        <el-text v-if="configuredScopes.length" class="limit-hint" type="info">
          Close a scope to remove that timeout, then save.
        </el-text>
        <el-dropdown v-if="availableScopes.length" trigger="click" @command="addScope">
          <el-button size="small" text type="primary">+ Add scope</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="scope in availableScopes" :key="scope" :command="scope">
                {{ scope }}
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
import { QOS_SCOPES, type ScopeDraft } from '@/services/qos/type'

const model = defineModel<ScopeDraft>({ required: true })

const configuredScopes = computed(() =>
  Object.keys(model.value.runtimeHours).filter((scope) =>
    (QOS_SCOPES as readonly string[]).includes(scope),
  ),
)

const availableScopes = computed(() =>
  QOS_SCOPES.filter((scope) => !(scope in model.value.runtimeHours)),
)

/** addScope starts an empty timeout for one workspace scope. */
const addScope = (scope: string) => {
  model.value.runtimeHours[scope] = undefined
}

/** removeScope drops the timeout so that scope falls through. */
const removeScope = (scope: string) => {
  delete model.value.runtimeHours[scope]
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
.limit-hint {
  font-size: 12px;
}
</style>
