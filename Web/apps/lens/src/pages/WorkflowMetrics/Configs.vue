<template>
  <div class="configs-page">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">Workflow Metrics Configurations</h2>
      <el-button type="primary" :icon="Plus" @click="showCreateDialog = true">
        New Config
      </el-button>
    </div>

    <!-- Filters -->
    <div class="filter-section">
      <el-input
        v-model="filters.name"
        placeholder="Search by name"
        clearable
        class="filter-input"
        :prefix-icon="Search"
        @keyup.enter="onSearch"
      />
      <el-input
        v-model="filters.githubOwner"
        placeholder="GitHub Owner"
        clearable
        class="filter-input"
        @keyup.enter="onSearch"
      />
      <el-input
        v-model="filters.githubRepo"
        placeholder="GitHub Repo"
        clearable
        class="filter-input"
        @keyup.enter="onSearch"
      />
      <el-select v-model="filters.enabled" placeholder="Status" clearable class="filter-select">
        <el-option label="Enabled" :value="true" />
        <el-option label="Disabled" :value="false" />
      </el-select>
      <el-button :icon="Search" @click="onSearch">Search</el-button>
      <el-button :icon="Refresh" @click="resetFilters">Reset</el-button>
    </div>

    <!-- Table -->
    <el-card class="table-card">
      <el-table
        v-loading="loading"
        :data="tableData"
        style="width: 100%"
        @row-click="goToDetail"
      >
        <el-table-column prop="name" label="Name" min-width="180">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" class="config-link">
              {{ row.name }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="Repository" min-width="200">
          <template #default="{ row }">
            <span class="repo-text">{{ row.githubOwner }}/{{ row.githubRepo }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="runnerSetName" label="Runner Set" min-width="200">
          <template #default="{ row }">
            <el-tooltip :content="row.runnerSetName" placement="top">
              <span class="runner-text">{{ row.runnerSetName }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="runnerSetNamespace" label="Namespace" min-width="180" />
        <el-table-column label="File Patterns" min-width="200">
          <template #default="{ row }">
            <el-tooltip v-if="row.filePatterns?.length" :content="row.filePatterns.join(', ')" placement="top">
              <el-tag size="small">{{ row.filePatterns.length }} pattern(s)</el-tag>
            </el-tooltip>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="Status" width="100" align="center">
          <template #default="{ row }">
            <el-switch
              v-model="row.enabled"
              :loading="row._updating"
              @click.stop
              @change="(val: boolean) => toggleEnabled(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="lastCheckedAt" label="Last Checked" width="180">
          <template #default="{ row }">
            {{ row.lastCheckedAt ? formatDate(row.lastCheckedAt) : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="120" fixed="right" align="center">
          <template #default="{ row }">
            <el-button-group>
              <el-tooltip content="Edit">
                <el-button link :icon="Edit" @click.stop="editConfig(row)" />
              </el-tooltip>
              <el-tooltip content="Delete">
                <el-button link :icon="Delete" type="danger" @click.stop="confirmDelete(row)" />
              </el-tooltip>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-if="pagination.total > 0"
        v-model:current-page="pagination.pageNum"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next"
        @current-change="fetchData"
        @size-change="fetchData"
        class="mt-4"
      />
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog
      v-model="showCreateDialog"
      :title="editingConfig ? 'Edit Configuration' : 'Create New Configuration'"
      width="640px"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="formData"
        :rules="formRules"
        label-width="140px"
        label-position="right"
      >
        <el-divider content-position="left">Basic Information</el-divider>
        <el-form-item label="Name" prop="name">
          <el-input v-model="formData.name" placeholder="e.g. MI325 Benchmark Collection" />
        </el-form-item>
        <el-form-item label="Description" prop="description">
          <el-input
            v-model="formData.description"
            type="textarea"
            :rows="2"
            placeholder="Optional description"
          />
        </el-form-item>

        <el-divider content-position="left">GitHub Repository</el-divider>
        <el-form-item label="Owner" prop="githubOwner">
          <el-input v-model="formData.githubOwner" placeholder="e.g. AMD-AGI" />
        </el-form-item>
        <el-form-item label="Repository" prop="githubRepo">
          <el-input v-model="formData.githubRepo" placeholder="e.g. Primus-Turbo" />
        </el-form-item>
        <el-form-item label="Workflow Filter" prop="workflowFilter">
          <el-input v-model="formData.workflowFilter" placeholder="Optional: benchmark*.yml" />
        </el-form-item>
        <el-form-item label="Branch Filter" prop="branchFilter">
          <el-input v-model="formData.branchFilter" placeholder="Optional: main" />
        </el-form-item>

        <el-divider content-position="left">Runner Configuration</el-divider>
        <el-form-item label="Namespace" prop="runnerSetNamespace">
          <el-input v-model="formData.runnerSetNamespace" placeholder="e.g. tw-project2-control-plane" />
        </el-form-item>
        <el-form-item label="Runner Set Name" prop="runnerSetName">
          <el-input v-model="formData.runnerSetName" placeholder="e.g. turbo-pt-bench-gfx942" />
        </el-form-item>

        <el-divider content-position="left">File Patterns</el-divider>
        <el-form-item label="Patterns" prop="filePatterns">
          <div class="patterns-editor">
            <div v-for="(pattern, index) in formData.filePatterns" :key="index" class="pattern-row">
              <el-input v-model="formData.filePatterns[index]" placeholder="e.g. **/summary.csv" />
              <el-button :icon="Delete" circle @click="removePattern(index)" />
            </div>
            <el-button type="dashed" :icon="Plus" @click="addPattern" class="add-pattern-btn">
              Add Pattern
            </el-button>
          </div>
        </el-form-item>

        <el-form-item label="Enable" prop="enabled">
          <el-switch v-model="formData.enabled" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showCreateDialog = false">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">
          {{ editingConfig ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Search, Refresh, Edit, Delete } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  getConfigs,
  createConfig,
  updateConfig,
  deleteConfig,
  type WorkflowConfig,
  type CreateConfigRequest
} from '@/services/workflow-metrics'

const router = useRouter()

// State
const loading = ref(false)
const tableData = ref<(WorkflowConfig & { _updating?: boolean })[]>([])
const pagination = reactive({
  pageNum: 1,
  pageSize: 20,
  total: 0
})
const filters = reactive({
  name: '',
  githubOwner: '',
  githubRepo: '',
  enabled: undefined as boolean | undefined
})

// Dialog state
const showCreateDialog = ref(false)
const editingConfig = ref<WorkflowConfig | null>(null)
const formRef = ref<FormInstance>()
const submitting = ref(false)

const defaultFormData = (): CreateConfigRequest & { enabled: boolean } => ({
  name: '',
  description: '',
  runnerSetNamespace: '',
  runnerSetName: '',
  githubOwner: '',
  githubRepo: '',
  workflowFilter: '',
  branchFilter: '',
  filePatterns: [''],
  enabled: true
})

const formData = reactive(defaultFormData())

const formRules: FormRules = {
  name: [{ required: true, message: 'Name is required', trigger: 'blur' }],
  runnerSetNamespace: [{ required: true, message: 'Namespace is required', trigger: 'blur' }],
  runnerSetName: [{ required: true, message: 'Runner Set Name is required', trigger: 'blur' }],
  githubOwner: [{ required: true, message: 'GitHub Owner is required', trigger: 'blur' }],
  githubRepo: [{ required: true, message: 'GitHub Repo is required', trigger: 'blur' }],
  filePatterns: [{ required: true, message: 'At least one pattern is required', trigger: 'blur' }]
}

// Methods
const fetchData = async () => {
  loading.value = true
  try {
    const res = await getConfigs({
      offset: (pagination.pageNum - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      name: filters.name || undefined,
      githubOwner: filters.githubOwner || undefined,
      githubRepo: filters.githubRepo || undefined,
      enabled: filters.enabled
    })
    tableData.value = res.configs || []
    pagination.total = res.total || 0
  } catch (error) {
    console.error('Failed to fetch configs:', error)
    ElMessage.error('Failed to fetch configurations')
  } finally {
    loading.value = false
  }
}

const onSearch = () => {
  pagination.pageNum = 1
  fetchData()
}

const resetFilters = () => {
  filters.name = ''
  filters.githubOwner = ''
  filters.githubRepo = ''
  filters.enabled = undefined
  pagination.pageNum = 1
  fetchData()
}

const formatDate = (date: string) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm:ss')
}

const goToDetail = (row: WorkflowConfig) => {
  router.push(`/workflow-metrics/configs/${row.id}`)
}

const toggleEnabled = async (row: WorkflowConfig & { _updating?: boolean }, enabled: boolean) => {
  row._updating = true
  try {
    await updateConfig(row.id, { ...row, enabled })
    ElMessage.success(`Config ${enabled ? 'enabled' : 'disabled'}`)
  } catch (error) {
    console.error('Failed to update config:', error)
    ElMessage.error('Failed to update config')
    row.enabled = !enabled // Revert
  } finally {
    row._updating = false
  }
}

const editConfig = (row: WorkflowConfig) => {
  editingConfig.value = row
  Object.assign(formData, {
    name: row.name,
    description: row.description || '',
    runnerSetNamespace: row.runnerSetNamespace,
    runnerSetName: row.runnerSetName,
    githubOwner: row.githubOwner,
    githubRepo: row.githubRepo,
    workflowFilter: row.workflowFilter || '',
    branchFilter: row.branchFilter || '',
    filePatterns: row.filePatterns?.length ? [...row.filePatterns] : [''],
    enabled: row.enabled
  })
  showCreateDialog.value = true
}

const confirmDelete = (row: WorkflowConfig) => {
  ElMessageBox.confirm(
    `Are you sure you want to delete "${row.name}"? This action cannot be undone.`,
    'Delete Configuration',
    {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning'
    }
  ).then(async () => {
    try {
      await deleteConfig(row.id)
      ElMessage.success('Config deleted')
      fetchData()
    } catch (error) {
      console.error('Failed to delete config:', error)
      ElMessage.error('Failed to delete config')
    }
  }).catch(() => {})
}

const addPattern = () => {
  formData.filePatterns.push('')
}

const removePattern = (index: number) => {
  if (formData.filePatterns.length > 1) {
    formData.filePatterns.splice(index, 1)
  }
}

const submitForm = async () => {
  if (!formRef.value) return
  
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    
    submitting.value = true
    try {
      const data = {
        ...formData,
        filePatterns: formData.filePatterns.filter(p => p.trim())
      }
      
      if (editingConfig.value) {
        await updateConfig(editingConfig.value.id, data)
        ElMessage.success('Config updated')
      } else {
        await createConfig(data)
        ElMessage.success('Config created')
      }
      
      showCreateDialog.value = false
      editingConfig.value = null
      Object.assign(formData, defaultFormData())
      fetchData()
    } catch (error) {
      console.error('Failed to save config:', error)
      ElMessage.error('Failed to save config')
    } finally {
      submitting.value = false
    }
  })
}

// Watch dialog close to reset form
const handleDialogClose = () => {
  editingConfig.value = null
  Object.assign(formData, defaultFormData())
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped lang="scss">
.configs-page {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;

    .page-title {
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin: 0;
    }
  }

  .filter-section {
    display: flex;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;

    .filter-input {
      width: 180px;
    }

    .filter-select {
      width: 140px;
    }
  }

  .table-card {
    border-radius: 12px;

    :deep(.el-card__body) {
      padding: 20px;
    }

    .config-link {
      font-weight: 500;
    }

    .repo-text {
      color: var(--el-text-color-secondary);
      font-family: monospace;
    }

    .runner-text {
      display: inline-block;
      max-width: 180px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .patterns-editor {
    width: 100%;

    .pattern-row {
      display: flex;
      gap: 8px;
      margin-bottom: 8px;

      .el-input {
        flex: 1;
      }
    }

    .add-pattern-btn {
      width: 100%;
      border-style: dashed;
    }
  }
}
</style>

