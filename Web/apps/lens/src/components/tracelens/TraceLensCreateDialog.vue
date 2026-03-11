<template>
  <el-dialog
    v-model="dialogVisible"
    title="Start TraceLens Analysis"
    width="700px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    @closed="handleClosed"
  >
    <!-- Dialog Content -->
    <div class="dialog-content">
      <!-- File Info -->
      <div class="info-section mt-4">
        <h4>File Information</h4>
        <div class="info-grid">
          <div class="info-item">
            <label>File Name:</label>
            <span class="file-name">{{ file?.fileName }}</span>
          </div>
          <div class="info-item">
            <label>Size:</label>
            <span>{{ formatFileSize(file?.fileSize) }}</span>
          </div>
          <div class="info-item">
            <label>Cluster:</label>
            <span>{{ selectedCluster }}</span>
          </div>
        </div>
      </div>

      <!-- Resource Configuration -->
      <div class="config-section">
        <h4>Resource Configuration</h4>
        <el-radio-group v-model="resourceProfile" class="resource-radio-group">
          <el-radio 
            v-for="profile in resourceProfiles" 
            :key="profile.value"
            :label="profile.value"
            :border="true"
            class="resource-radio"
          >
            <div class="profile-content">
              <div class="profile-header">
                <span class="profile-name">{{ profile.label }}</span>
                <el-tag 
                  v-if="profile.isDefault" 
                  type="success" 
                  size="small"
                  effect="plain"
                >
                  Recommended
                </el-tag>
              </div>
              <div class="profile-desc">{{ profile.description }}</div>
            </div>
          </el-radio>
        </el-radio-group>
      </div>

      <!-- Session Duration -->
      <div class="duration-section">
        <h4>Session Duration</h4>
        <div class="duration-input">
          <el-input-number
            v-model="ttlMinutes"
            :min="1"
            :max="240"
            :step="30"
            controls-position="right"
          />
          <span class="duration-unit">minutes</span>
          <span class="duration-hint">(Default: 1 hour, Max: 4 hours)</span>
        </div>
      </div>

      <!-- Warning Message -->
      <el-alert
        title="Notice"
        type="warning"
        :closable="false"
        show-icon
      >
        <ul class="warning-list">
          <li>Analysis environment will consume cluster resources, please choose resource configuration wisely</li>
          <li>Resources will be released automatically after session expires, you can extend time on analysis page</li>
          <li>It's recommended to choose resource configuration based on file size</li>
        </ul>
      </el-alert>
    </div>

    <!-- Dialog Footer -->
    <template #footer>
      <el-button @click="handleCancel" :disabled="creating">Cancel</el-button>
      <el-button 
        type="primary" 
        @click="handleCreate"
        :loading="creating"
      >
        Start Analysis
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { createSession, listWorkloadSessions, getResourceProfiles, DEFAULT_RESOURCE_PROFILES } from '@/services/tracelens'
import type { ResourceProfile } from '@/services/tracelens'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// Get cluster from global state
const { selectedCluster } = useGlobalCluster()

// Resource profiles loaded from backend
const resourceProfiles = ref<ResourceProfile[]>(DEFAULT_RESOURCE_PROFILES)

// Load resource profiles from backend
const loadResourceProfiles = async () => {
  try {
    const profiles = await getResourceProfiles()
    resourceProfiles.value = profiles
  } catch (error) {
    console.warn('Failed to load resource profiles, using defaults:', error)
    resourceProfiles.value = DEFAULT_RESOURCE_PROFILES
  }
}

onMounted(() => {
  loadResourceProfiles()
})

// Props & Emits
const props = defineProps<{
  visible: boolean
  file: any
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  'created': [sessionId: string]
}>()

// Dialog visibility
const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

// Form data
const resourceProfile = ref('medium')
const ttlMinutes = ref(60)
const creating = ref(false)

// Handle create session
const handleCreate = async () => {
  if (!props.file) {
    ElMessage.error('File information missing')
    return
  }

  creating.value = true
  try {
    // Check for existing active session first
    try {
      const sessions = await listWorkloadSessions(props.file.workloadUid, selectedCluster.value)
      const activeSession = sessions.find(
        session => session.profilerFileId === props.file.id && 
                  session.status === 'ready'
      )
      
      if (activeSession) {
        // If there's an active session, just navigate to it
        ElMessage.success('Active session found, opening...')
        emit('created', activeSession.sessionId)
        dialogVisible.value = false
        return
      }
    } catch (error) {
    }

    // Create new session
    const session = await createSession(
      {
        workloadUid: props.file.workloadUid,
        profilerFileId: props.file.id,
        resourceProfile: resourceProfile.value,
        ttlMinutes: ttlMinutes.value
      },
      selectedCluster.value
    )

    ElMessage.success('Session created successfully, starting analysis environment...')
    emit('created', session.sessionId)
    dialogVisible.value = false
  } catch (error: any) {
    console.error('Failed to create session:', error)
    
    // Handle specific error codes
    if (error.response?.data?.meta?.code === 4004) {
      ElMessage.error('Profiler file does not exist or has been deleted')
    } else if (error.response?.data?.meta?.code === 5003) {
      ElMessage.error('Insufficient cluster resources, please try again later or choose a smaller resource configuration')
    } else {
      ElMessage.error(error.message || 'Failed to create session, please try again')
    }
  } finally {
    creating.value = false
  }
}

// Handle cancel
const handleCancel = () => {
  dialogVisible.value = false
}

// Handle dialog closed
const handleClosed = () => {
  // Reset form
  resourceProfile.value = 'medium'
  ttlMinutes.value = 60
}

// Format file size
const formatFileSize = (bytes: number) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// Watch file changes and suggest resource profile based on file size
watch(() => props.file, (newFile) => {
  if (newFile?.file_size) {
    const sizeMB = newFile.file_size / (1024 * 1024)
    if (sizeMB < 5) {
      resourceProfile.value = 'small'
    } else if (sizeMB < 20) {
      resourceProfile.value = 'medium'
    } else {
      resourceProfile.value = 'large'
    }
  }
}, { immediate: true })
</script>

<style scoped lang="scss">
.dialog-content {
  .info-section,
  .config-section,
  .duration-section {
    margin-bottom: 24px;
    
    h4 {
      margin: 0 0 12px 0;
      font-size: 14px;
      font-weight: 500;
      color: #303133;
    }
  }
  
  .info-grid {
    .info-item {
      display: flex;
      align-items: center;
      margin-bottom: 8px;
      
      label {
        width: 80px;
        color: #606266;
        font-size: 14px;
      }
      
      span {
        flex: 1;
        color: #303133;
        font-size: 14px;
      }
      
      .file-name {
        font-family: 'Courier New', monospace;
        font-size: 13px;
        word-break: break-all;
      }
    }
  }
  
  .resource-radio-group {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 12px;
    
    :deep(.resource-radio) {
      width: 100% !important;
      margin: 0 !important;
      height: auto;
      box-sizing: border-box;
      display: block !important;
      
      .el-radio__input {
        display: none;
      }
      
      .el-radio__label {
        width: 100%;
        padding: 0;
      }
      
      &.is-checked {
        border-color: var(--el-color-primary);
        background-color: var(--el-color-primary-light-9);
      }
      
      .profile-content {
        padding: 12px;
        
        .profile-header {
          display: flex;
          align-items: center;
          gap: 8px;
          margin-bottom: 4px;
          
          .profile-name {
            font-weight: 500;
            font-size: 14px;
          }
        }
        
        .profile-desc {
          color: #909399;
          font-size: 13px;
        }
      }
    }
  }
  
  .duration-input {
    display: flex;
    align-items: center;
    gap: 8px;
    
    .duration-unit {
      color: #606266;
      font-size: 14px;
    }
    
    .duration-hint {
      color: #909399;
      font-size: 13px;
    }
  }
  
  .warning-list {
    margin: 0;
    padding-left: 20px;
    
    li {
      line-height: 1.8;
      font-size: 13px;
    }
  }
}

// Dark theme support
.dark {
  .dialog-content {
    h4 {
      color: var(--el-text-color-primary);
    }
    
    .info-grid {
      label {
        color: var(--el-text-color-regular);
      }
      
      span {
        color: var(--el-text-color-primary);
      }
    }
  }
}
</style>
