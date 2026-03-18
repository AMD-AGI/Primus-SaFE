<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { 
  alertTemplatesApi,
  logAlertRulesApi,
  type LogAlertRuleTemplate
} from '@/services/alerts'
import AlertSeverityBadge from './components/AlertSeverityBadge.vue'
import { useClusterStore } from '@/stores/cluster'

const router = useRouter()
const clusterStore = useClusterStore()

// State
const loading = ref(false)
const templates = ref<LogAlertRuleTemplate[]>([])
const showPreviewDialog = ref(false)
const previewTemplate = ref<LogAlertRuleTemplate | null>(null)
const showInstantiateDialog = ref(false)
const instantiateForm = ref({
  name: '',
  clusterName: ''
})

// Fetch templates
async function fetchTemplates() {
  loading.value = true
  try {
    templates.value = await alertTemplatesApi.list({}) || []
  } catch (error) {
    console.error('Failed to fetch templates:', error)
    ElMessage.error('Failed to fetch templates')
  } finally {
    loading.value = false
  }
}

// Group templates by category
function getTemplatesByCategory() {
  const grouped: Record<string, LogAlertRuleTemplate[]> = {}
  templates.value.forEach(t => {
    const category = t.category || 'Other'
    if (!grouped[category]) grouped[category] = []
    grouped[category].push(t)
  })
  return grouped
}

// Preview
function openPreview(template: LogAlertRuleTemplate) {
  previewTemplate.value = template
  showPreviewDialog.value = true
}

// Instantiate
function openInstantiate(template: LogAlertRuleTemplate) {
  previewTemplate.value = template
  instantiateForm.value = {
    name: template.name,
    clusterName: clusterStore.currentCluster || ''
  }
  showInstantiateDialog.value = true
}

async function instantiateTemplate() {
  if (!previewTemplate.value || !instantiateForm.value.name || !instantiateForm.value.clusterName) {
    ElMessage.warning('Please fill in all required fields')
    return
  }
  
  try {
    const rule = await alertTemplatesApi.instantiate(previewTemplate.value.id, {
      name: instantiateForm.value.name,
      clusterName: instantiateForm.value.clusterName
    })
    
    ElMessage.success('Rule created from template')
    showInstantiateDialog.value = true
    router.push('/alerts/rules/log')
  } catch (error) {
    console.error('Failed to instantiate template:', error)
    ElMessage.error('Failed to create rule from template')
  }
}

// Delete custom template
async function deleteTemplate(template: LogAlertRuleTemplate) {
  if (template.builtIn) {
    ElMessage.warning('Cannot delete built-in templates')
    return
  }
  
  try {
    await ElMessageBox.confirm(
      `Delete template "${template.name}"?`,
      'Confirm Delete',
      { type: 'warning' }
    )
    
    await alertTemplatesApi.delete(template.id)
    ElMessage.success('Template deleted')
    fetchTemplates()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to delete template')
    }
  }
}

// Utility
function getMatchTypeLabel(type: string) {
  const labels: Record<string, string> = {
    pattern: 'Pattern',
    threshold: 'Threshold',
    composite: 'Composite'
  }
  return labels[type] || type
}

onMounted(() => {
  fetchTemplates()
})
</script>

<template>
  <div class="alert-rule-templates">
    <!-- Header -->
    <div class="page-header">
      <h1 class="page-title">
        <el-icon class="title-icon"><Files /></el-icon>
        Alert Rule Templates
      </h1>
    </div>

    <div v-loading="loading" class="templates-container">
      <!-- Built-in Templates -->
      <template v-for="(categoryTemplates, category) in getTemplatesByCategory()" :key="category">
        <el-card class="category-card" shadow="hover">
          <template #header>
            <span class="category-title">{{ category }}</span>
          </template>
          
          <div class="templates-grid">
            <div 
              v-for="template in categoryTemplates"
              :key="template.id"
              class="template-item"
              :class="{ 'is-builtin': template.builtIn }"
            >
              <div class="template-header">
                <el-icon class="template-icon"><SetUp /></el-icon>
                <span class="template-name">{{ template.name }}</span>
                <el-tag v-if="template.builtIn" type="info" size="small">Built-in</el-tag>
              </div>
              
              <p class="template-description">{{ template.description }}</p>
              
              <div class="template-meta">
                <el-tag type="info" size="small">{{ getMatchTypeLabel(template.matchType) }}</el-tag>
                <AlertSeverityBadge :severity="template.severity" size="small" />
              </div>
              
              <div class="template-actions">
                <el-button type="primary" text size="small" @click="openPreview(template)">
                  Preview
                </el-button>
                <el-button type="success" text size="small" @click="openInstantiate(template)">
                  Use Template
                </el-button>
                <el-button 
                  v-if="!template.builtIn"
                  type="danger" 
                  text 
                  size="small" 
                  @click="deleteTemplate(template)"
                >
                  Delete
                </el-button>
              </div>
            </div>
          </div>
        </el-card>
      </template>
      
      <el-empty v-if="templates.length === 0" description="No templates available" />
    </div>

    <!-- Preview Dialog -->
    <el-dialog
      v-model="showPreviewDialog"
      :title="previewTemplate?.name"
      width="600px"
    >
      <template v-if="previewTemplate">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="Category">{{ previewTemplate.category }}</el-descriptions-item>
          <el-descriptions-item label="Match Type">{{ getMatchTypeLabel(previewTemplate.matchType) }}</el-descriptions-item>
          <el-descriptions-item label="Severity">
            <AlertSeverityBadge :severity="previewTemplate.severity" size="small" />
          </el-descriptions-item>
          <el-descriptions-item label="Pattern">
            <code>{{ (previewTemplate.matchConfig as any)?.pattern || '-' }}</code>
          </el-descriptions-item>
        </el-descriptions>
        
        <div v-if="previewTemplate.alertTemplate" class="template-preview">
          <h4>Alert Template</h4>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="Title">{{ previewTemplate.alertTemplate.title }}</el-descriptions-item>
            <el-descriptions-item label="Summary">{{ previewTemplate.alertTemplate.summary }}</el-descriptions-item>
          </el-descriptions>
        </div>
      </template>
      
      <template #footer>
        <el-button @click="showPreviewDialog = false">Close</el-button>
        <el-button type="primary" @click="showPreviewDialog = false; openInstantiate(previewTemplate!)">
          Use This Template
        </el-button>
      </template>
    </el-dialog>

    <!-- Instantiate Dialog -->
    <el-dialog
      v-model="showInstantiateDialog"
      title="Create Rule from Template"
      width="500px"
    >
      <el-form :model="instantiateForm" label-width="100px">
        <el-form-item label="Rule Name" required>
          <el-input v-model="instantiateForm.name" placeholder="Enter rule name" />
        </el-form-item>
        
        <el-form-item label="Cluster" required>
          <el-select v-model="instantiateForm.clusterName" placeholder="Select cluster" style="width: 100%">
            <el-option 
              v-for="cluster in clusterStore.clusters" 
              :key="cluster" 
              :label="cluster" 
              :value="cluster" 
            />
          </el-select>
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button @click="showInstantiateDialog = false">Cancel</el-button>
        <el-button type="primary" @click="instantiateTemplate">Create Rule</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
.alert-rule-templates {
  padding: 0;
}

.page-header {
  margin-bottom: 24px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  
  .title-icon {
    color: var(--el-color-primary);
  }
}

.templates-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.category-card {
  border-radius: 12px;
}

.category-title {
  font-weight: 600;
  font-size: 16px;
}

.templates-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.template-item {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  transition: all 0.2s;
  
  &:hover {
    border-color: var(--el-color-primary);
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
  }
  
  &.is-builtin {
    background: var(--el-fill-color-lighter);
  }
}

.template-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.template-icon {
  color: var(--el-color-primary);
}

.template-name {
  font-weight: 600;
  flex: 1;
}

.template-description {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin: 0 0 12px 0;
  line-height: 1.5;
}

.template-meta {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.template-actions {
  display: flex;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.template-preview {
  margin-top: 16px;
  
  h4 {
    margin: 0 0 8px 0;
    font-size: 14px;
  }
}
</style>
