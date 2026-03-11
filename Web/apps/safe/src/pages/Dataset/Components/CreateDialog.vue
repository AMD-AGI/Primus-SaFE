<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="Create Dataset"
    width="650px"
    :close-on-click-modal="false"
    :before-close="handleClose"
    destroy-on-close
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="auto" class="p-y-3 p-x-5">
      <!-- Dataset source selection -->
      <div class="flex items-center justify-between m-b-4">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Dataset Source</span>
        </div>
        <el-segmented v-model="sourceType" :options="sourceOptions" size="default" />
      </div>

      <div class="flex items-center m-b-4 m-t-6">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Basic Information</span>
      </div>

      <!-- Form fields for upload mode -->
      <el-form-item v-if="sourceType === 'upload'" label="Name" prop="displayName">
        <el-input
          v-model="form.displayName"
          placeholder="Enter dataset name"
          maxlength="64"
          show-word-limit
        />
      </el-form-item>

      <!-- Form fields for HuggingFace mode -->
      <el-form-item v-if="sourceType === 'huggingface'" label="HuggingFace URL or Repo ID" prop="url">
        <el-input
          v-model="form.url"
          placeholder="e.g. HuggingFaceH4/MATH-500 or https://huggingface.co/datasets/gsm8k"
          maxlength="256"
        />
        <div class="text-[12px] text-gray-400 mt-1">
          Supports repo ID (gsm8k) or full URL
        </div>
      </el-form-item>

      <el-form-item label="Type" prop="datasetType">
        <el-select
          v-model="form.datasetType"
          placeholder="Select dataset type"
          class="w-full"
          :loading="loadingTypes"
        >
          <el-option
            v-for="type in datasetTypes"
            :key="type.name"
            :label="type.name"
            :value="type.name"
          >
            <div class="flex items-center justify-between w-full">
              <span>{{ type.name }}</span>
              <el-tooltip :content="formatSchema(type.schema)" placement="right" raw-content>
                <el-icon class="ml-2 text-gray-400 cursor-help">
                  <QuestionFilled />
                </el-icon>
              </el-tooltip>
            </div>
          </el-option>
        </el-select>
      </el-form-item>

      <el-form-item label="Workspace">
        <el-select
          v-model="form.workspace"
          :placeholder="sourceType === 'upload' ? 'Select workspace (optional)' : 'All Workspaces (Public)'"
          class="w-full"
          clearable
        >
          <el-option
            v-for="ws in store.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
        <div v-if="sourceType === 'huggingface'" class="text-[12px] text-gray-400 mt-1">
          Leave empty for public access
        </div>
      </el-form-item>

      <el-form-item v-if="sourceType === 'upload'" label="Description">
        <el-input
          v-model="form.description"
          type="textarea"
          :rows="3"
          placeholder="Enter dataset description"
          maxlength="500"
          show-word-limit
        />
      </el-form-item>

      <el-form-item v-if="sourceType === 'huggingface'" label="Token (Optional)">
        <el-input
          v-model="form.token"
          type="password"
          placeholder="For private datasets only"
          show-password
        />
        <div class="text-[12px] text-gray-400 mt-1">
          Required only for private datasets
        </div>
      </el-form-item>

      <!-- File upload for upload mode -->
      <template v-if="sourceType === 'upload'">
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Upload Files</span>
          <span class="text-gray-400 text-[12px] ml-2">(Optional)</span>
        </div>

        <el-form-item label="Files">
          <div class="flex gap-2 mb-2">
            <el-button @click="triggerFolderUpload" :icon="FolderOpened" size="small">
              Upload Folder
            </el-button>
            <input
              ref="folderInputRef"
              type="file"
              webkitdirectory
              multiple
              style="display: none"
              @change="handleFolderSelect"
            />
          </div>
          <el-upload
            ref="uploadRef"
            v-model:file-list="fileList"
            class="w-full"
            drag
            multiple
            :auto-upload="false"
            :on-remove="handleFileRemove"
            :on-change="handleFileChange"
            :accept="acceptTypes"
          >
            <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
            <div class="el-upload__text">Drop files here or <em>click to upload</em></div>
            <template #tip>
              <div class="el-upload__tip">
                Supported formats: JSON, JSONL, CSV, TXT, Parquet. Max 10GB total.
              </div>
            </template>
          </el-upload>
        </el-form-item>
      </template>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">Cancel</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="loading">
          {{ sourceType === 'upload' ? 'Create' : 'Import' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, reactive, watch, computed, onMounted } from 'vue'
import {
  ElMessage,
  ElMessageBox,
  type FormInstance,
  type FormRules,
  type UploadInstance,
  type UploadFile,
  type UploadFiles,
} from 'element-plus'
import { UploadFilled, QuestionFilled, FolderOpened } from '@element-plus/icons-vue'
import { createDataset, getDatasetTypes, importHFDataset } from '@/services/dataset'
import { useWorkspaceStore } from '@/stores/workspace'
import type { DatasetType } from '@/services/dataset/type'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits(['update:visible', 'success'])

const store = useWorkspaceStore()
const formRef = ref<FormInstance>()
const uploadRef = ref<UploadInstance>()
const folderInputRef = ref<HTMLInputElement>()
const loading = ref(false)
const loadingTypes = ref(false)
const fileList = ref<UploadFile[]>([])
const datasetTypes = ref<DatasetType[]>([])

const acceptTypes = '.json,.jsonl,.csv,.txt,.parquet'
const maxTotalSize = 10 * 1024 * 1024 * 1024 // 10GB total

const sourceType = ref<'upload' | 'huggingface'>('upload')
const sourceOptions = [
  { label: 'Upload Files', value: 'upload' },
  { label: 'Import from HuggingFace', value: 'huggingface' },
]

const initialForm = () => ({
  displayName: '',
  datasetType: '',
  workspace: '',
  description: '',
  url: '',
  token: '',
})

const form = reactive({ ...initialForm() })

const rules = computed<FormRules>(() => {
  if (sourceType.value === 'upload') {
    return {
      displayName: [
        { required: true, message: 'Please enter dataset name' },
        { min: 1, max: 64, message: 'Name should be 1-64 characters' },
      ],
      datasetType: [{ required: true, message: 'Please select dataset type' }],
    }
  } else {
    return {
      url: [
        { required: true, message: 'Please enter HuggingFace URL or Repo ID' },
        { min: 1, max: 256, message: 'URL should be 1-256 characters' },
      ],
      datasetType: [{ required: true, message: 'Please select dataset type' }],
    }
  }
})

const handleFileChange = (file: UploadFile, files: UploadFiles) => {
  // Validate total file size
  const totalSize = files.reduce((sum, f) => sum + (f.raw?.size || 0), 0)
  if (totalSize > maxTotalSize) {
    ElMessage.warning(`Total file size exceeds 10GB limit`)
    fileList.value = files.filter((f) => f.uid !== file.uid)
    return
  }
}

const handleFileRemove = (file: UploadFile, files: UploadFiles) => {
  fileList.value = files
}

const triggerFolderUpload = () => {
  folderInputRef.value?.click()
}

const handleFolderSelect = (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = input.files
  if (!files || files.length === 0) return

  // Filter files by accepted types
  const acceptedExtensions = acceptTypes.split(',').map((ext) => ext.trim())
  const validFiles: File[] = []
  let totalSize = fileList.value.reduce((sum, f) => sum + (f.raw?.size || 0), 0)

  for (let i = 0; i < files.length; i++) {
    const file = files[i]
    const fileExtension = '.' + file.name.split('.').pop()?.toLowerCase()

    if (acceptedExtensions.includes(fileExtension)) {
      totalSize += file.size
      if (totalSize > maxTotalSize) {
        ElMessage.warning(`Total file size exceeds 10GB limit`)
        break
      }
      validFiles.push(file)
    }
  }

  // Add files to upload list
  validFiles.forEach((file) => {
    // Add uid to File object to make it compatible with UploadRawFile
    const rawFile = Object.assign(file, { uid: Date.now() + Math.random() })
    const uploadFile: UploadFile = {
      name: file.name,
      size: file.size,
      raw: rawFile,
      status: 'ready',
      uid: rawFile.uid,
    }
    fileList.value.push(uploadFile)
  })

  if (validFiles.length > 0) {
    ElMessage.success(`Successfully added ${validFiles.length} file(s) from folder`)
  }

  // Reset input
  input.value = ''
}

const formatSchema = (schema: Record<string, string>) => {
  return Object.entries(schema)
    .map(([key, value]) => `<strong>${key}</strong>: ${value}`)
    .join('<br/>')
}

const fetchDatasetTypes = async () => {
  try {
    loadingTypes.value = true
    const response = await getDatasetTypes()
    datasetTypes.value = response.types || []
  } catch (error) {
    console.error('Failed to fetch dataset types:', error)
    ElMessage.error('Failed to load dataset types')
  } finally {
    loadingTypes.value = false
  }
}

const hasFormData = computed(() => {
  if (sourceType.value === 'upload') {
    return (
      form.displayName ||
      form.datasetType ||
      form.workspace ||
      form.description ||
      fileList.value.length > 0
    )
  } else {
    return form.url || form.datasetType || form.workspace || form.token
  }
})

const handleSubmit = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
    loading.value = true

    if (sourceType.value === 'upload') {
      // Upload mode
      const files = fileList.value.map((f) => f.raw).filter(Boolean) as File[]

      await createDataset({
        displayName: form.displayName,
        datasetType: form.datasetType,
        description: form.description || undefined,
        workspace: form.workspace,
        files: files.length > 0 ? files : undefined,
      })

      ElMessage.success('Dataset created successfully')
    } else {
      // HuggingFace import mode
      await importHFDataset({
        url: form.url,
        datasetType: form.datasetType,
        workspace: form.workspace || undefined,
        token: form.token || undefined,
      })

      ElMessage.success('Dataset import started')
    }

    emit('update:visible', false)
    emit('success')
  } catch (error: unknown) {
    console.error('Failed to create/import dataset:', error)
    
    if (sourceType.value === 'huggingface') {
      // User-friendly error message
      const err = error as { response?: { data?: { errorMessage?: string } }; message?: string }
      const errorMsg = err?.response?.data?.errorMessage || err?.message || 'Unknown error'
      let userFriendlyMsg = 'Failed to import dataset'
      
      if (errorMsg.includes('already exists')) {
        userFriendlyMsg = 'This dataset has already been imported'
      } else if (errorMsg.includes('invalid dataset type')) {
        userFriendlyMsg = 'Please select a valid dataset type'
      } else if (errorMsg.includes('failed to fetch HF dataset info')) {
        userFriendlyMsg = 'Could not find this dataset on HuggingFace. Please check the URL'
      } else if (errorMsg.includes("validation for 'URL' failed")) {
        userFriendlyMsg = 'URL is required'
      } else if (errorMsg.includes("validation for 'DatasetType' failed")) {
        userFriendlyMsg = 'Dataset type is required'
      }
      
      ElMessage.error(userFriendlyMsg)
    }
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  if (hasFormData.value) {
    ElMessageBox.confirm('All fields will be cleared.', 'Clear form & close?', {
      confirmButtonText: 'OK',
      cancelButtonText: 'Cancel',
      type: 'warning',
    }).then(() => {
      emit('update:visible', false)
      resetForm()
    })
  } else {
    emit('update:visible', false)
  }
}

const resetForm = () => {
  Object.assign(form, initialForm())
  fileList.value = []
  uploadRef.value?.clearFiles()
  sourceType.value = 'upload'
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      resetForm()
      formRef.value?.clearValidate()
    }
  },
)

onMounted(() => {
  fetchDatasetTypes()
})
</script>

<style scoped>
:deep(.el-upload-dragger) {
  padding: 20px;
}
</style>
