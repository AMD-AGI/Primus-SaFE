<template>
  <div class="skills-repository">
    <!-- Main Tabs -->
    <el-tabs v-model="activeTab" class="main-tabs">
      <el-tab-pane label="Skills" name="skills">
        <!-- Skills Header with actions -->
        <div class="skills-header">
          <div class="search-section">
            <el-select
              v-model="selectedSkillset"
              placeholder="All Skillsets"
              clearable
              style="width: 180px; margin-right: 12px"
              @change="loadSkills"
            >
              <el-option
                v-for="ss in skillsets"
                :key="ss.name"
                :label="ss.name"
                :value="ss.name"
              >
                <span>{{ ss.name }}</span>
                <el-tag v-if="ss.is_default" size="small" type="success" style="margin-left: 8px">default</el-tag>
              </el-option>
            </el-select>

            <el-input
              v-model="searchQuery"
              placeholder="Search skills by name or description..."
              clearable
              style="width: 400px"
              @keyup.enter="handleSearch"
            >
              <template #prefix>
                <i i="ep-search" />
              </template>
              <template #append>
                <el-button @click="handleSearch">
                  <i i="ep-search" />
                </el-button>
              </template>
            </el-input>

            <el-select
              v-model="filterCategory"
              placeholder="Category"
              clearable
              style="width: 150px; margin-left: 12px"
              @change="loadSkills"
            >
              <el-option
                v-for="cat in SKILL_CATEGORIES"
                :key="cat.value"
                :label="cat.label"
                :value="cat.value"
              />
            </el-select>

            <el-select
              v-model="filterSource"
              placeholder="Source"
              clearable
              style="width: 150px; margin-left: 12px"
              @change="loadSkills"
            >
              <el-option
                v-for="src in SKILL_SOURCES"
                :key="src.value"
                :label="src.label"
                :value="src.value"
              />
            </el-select>
          </div>

          <div class="action-buttons">
            <el-button @click="showImportDialog">
              <i i="ep-upload" class="mr-1" />
              Import
            </el-button>
            <el-button type="primary" @click="showCreateDialog">
              <i i="ep-plus" class="mr-1" />
              Create Skill
            </el-button>
          </div>
        </div>

    <!-- Skills Table -->
    <el-card class="skills-table-card">
      <el-table
        v-loading="loading"
        :data="skills"
        style="width: 100%"
        stripe
      >
        <el-table-column prop="name" label="Name" min-width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewSkill(row)">
              {{ row.name }}
            </el-button>
          </template>
        </el-table-column>

        <el-table-column prop="description" label="Description" min-width="300" show-overflow-tooltip />

        <el-table-column prop="category" label="Category" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.category" size="small" type="info">
              {{ row.category }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="source" label="Source" width="100">
          <template #default="{ row }">
            <el-tag
              :type="getSourceTagType(row.source)"
              size="small"
            >
              {{ row.source }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="version" label="Version" width="100" />

        <el-table-column prop="updated_at" label="Updated" width="160">
          <template #default="{ row }">
            {{ formatDate(row.updated_at) }}
          </template>
        </el-table-column>

        <el-table-column label="Actions" width="150" fixed="right">
          <template #default="{ row }">
            <el-button-group>
              <el-tooltip content="Edit">
                <el-button size="small" @click="editSkill(row)">
                  <i i="ep-edit" />
                </el-button>
              </el-tooltip>
              <el-tooltip content="Delete">
                <el-button size="small" type="danger" @click="confirmDelete(row)">
                  <i i="ep-delete" />
                </el-button>
              </el-tooltip>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>

      <!-- Pagination -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadSkills"
          @current-change="loadSkills"
        />
      </div>
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? 'Edit Skill' : 'Create Skill'"
      width="800px"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="formData"
        :rules="formRules"
        label-width="120px"
      >
        <el-form-item label="Name" prop="name">
          <el-input
            v-model="formData.name"
            :disabled="isEditing"
            placeholder="Unique skill identifier (e.g., k8s-oom-diagnose)"
          />
        </el-form-item>

        <el-form-item label="Description" prop="description">
          <el-input
            v-model="formData.description"
            type="textarea"
            :rows="3"
            placeholder="Brief description of what this skill does"
          />
        </el-form-item>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Category" prop="category">
              <el-select v-model="formData.category" placeholder="Select category" style="width: 100%">
                <el-option
                  v-for="cat in SKILL_CATEGORIES"
                  :key="cat.value"
                  :label="cat.label"
                  :value="cat.value"
                />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Source" prop="source">
              <el-select v-model="formData.source" placeholder="Select source" style="width: 100%">
                <el-option
                  v-for="src in SKILL_SOURCES"
                  :key="src.value"
                  :label="src.label"
                  :value="src.value"
                />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Version" prop="version">
              <el-input v-model="formData.version" placeholder="e.g., 1.0.0" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="License" prop="license">
              <el-input v-model="formData.license" placeholder="e.g., MIT, Apache-2.0" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="Content" prop="content">
          <el-input
            v-model="formData.content"
            type="textarea"
            :rows="15"
            placeholder="SKILL.md content (Markdown format)"
            class="skill-content-editor"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="saveSkill">
          {{ isEditing ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- View Skill Dialog -->
    <el-dialog
      v-model="viewDialogVisible"
      :title="viewingSkill?.name || 'Skill Details'"
      width="900px"
      destroy-on-close
    >
      <div v-if="viewingSkill" class="skill-view">
        <el-descriptions :column="3" border>
          <el-descriptions-item label="Name">{{ viewingSkill.name }}</el-descriptions-item>
          <el-descriptions-item label="Category">
            <el-tag v-if="viewingSkill.category" size="small">{{ viewingSkill.category }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Source">
            <el-tag :type="getSourceTagType(viewingSkill.source)" size="small">
              {{ viewingSkill.source }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Version">{{ viewingSkill.version || '-' }}</el-descriptions-item>
          <el-descriptions-item label="License">{{ viewingSkill.license || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Updated">{{ formatDate(viewingSkill.updated_at) }}</el-descriptions-item>
          <el-descriptions-item label="Description" :span="3">
            {{ viewingSkill.description }}
          </el-descriptions-item>
        </el-descriptions>

        <el-divider>Content</el-divider>

        <div v-loading="loadingContent" class="skill-content-view">
          <div v-if="viewingContent" class="markdown-content" v-html="renderedContent" />
          <el-empty v-else description="No content available" />
        </div>
      </div>

      <template #footer>
        <el-button @click="viewDialogVisible = false">Close</el-button>
        <el-button type="primary" @click="editSkill(viewingSkill!)">
          <i i="ep-edit" class="mr-1" />
          Edit
        </el-button>
      </template>
    </el-dialog>

      </el-tab-pane>

      <!-- Skillsets Tab -->
      <el-tab-pane label="Skillsets test" name="skillsets">
        <div class="skills-header">
          <div class="search-section">
            <el-input
              v-model="skillsetSearchQuery"
              placeholder="Search skillsets..."
              clearable
              style="width: 300px"
            >
              <template #prefix>
                <i i="ep-search" />
              </template>
            </el-input>
          </div>

          <div class="action-buttons">
            <el-button type="primary" @click="showCreateSkillsetDialog">
              <i i="ep-plus" class="mr-1" />
              Create Skillset
            </el-button>
          </div>
        </div>

        <el-card class="skills-table-card">
          <el-table
            v-loading="loadingSkillsets"
            :data="filteredSkillsets"
            style="width: 100%"
            stripe
          >
            <el-table-column prop="name" label="Name" min-width="150">
              <template #default="{ row }">
                <el-button link type="primary" @click="viewSkillset(row)">
                  {{ row.name }}
                </el-button>
                <el-tag v-if="row.is_default" size="small" type="success" style="margin-left: 8px">default</el-tag>
              </template>
            </el-table-column>

            <el-table-column prop="description" label="Description" min-width="250" show-overflow-tooltip />

            <el-table-column prop="owner" label="Owner" width="120" />

            <el-table-column label="Skills" width="100">
              <template #default="{ row }">
                <el-tag size="small">{{ skillsetSkillCounts[row.name] || 0 }}</el-tag>
              </template>
            </el-table-column>

            <el-table-column prop="updated_at" label="Updated" width="160">
              <template #default="{ row }">
                {{ formatDate(row.updated_at) }}
              </template>
            </el-table-column>

            <el-table-column label="Actions" width="200" fixed="right">
              <template #default="{ row }">
                <el-button-group>
                  <el-tooltip content="Manage Skills">
                    <el-button size="small" @click="manageSkillsetSkills(row)">
                      <i i="ep-folder" />
                    </el-button>
                  </el-tooltip>
                  <el-tooltip content="Set as Default" v-if="!row.is_default">
                    <el-button size="small" @click="setDefaultSkillset(row)">
                      <i i="ep-star" />
                    </el-button>
                  </el-tooltip>
                  <el-tooltip content="Edit">
                    <el-button size="small" @click="editSkillset(row)">
                      <i i="ep-edit" />
                    </el-button>
                  </el-tooltip>
                  <el-tooltip content="Delete">
                    <el-button size="small" type="danger" @click="confirmDeleteSkillset(row)">
                      <i i="ep-delete" />
                    </el-button>
                  </el-tooltip>
                </el-button-group>
              </template>
            </el-table-column>
          </el-table>

          <div class="pagination-wrapper">
            <el-pagination
              v-model:current-page="skillsetCurrentPage"
              v-model:page-size="skillsetPageSize"
              :page-sizes="[10, 20, 50]"
              :total="skillsetTotal"
              layout="total, sizes, prev, pager, next"
              @size-change="loadSkillsets"
              @current-change="loadSkillsets"
            />
          </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- Create/Edit Skillset Dialog -->
    <el-dialog
      v-model="skillsetDialogVisible"
      :title="isEditingSkillset ? 'Edit Skillset' : 'Create Skillset'"
      width="500px"
      destroy-on-close
    >
      <el-form
        ref="skillsetFormRef"
        :model="skillsetFormData"
        :rules="skillsetFormRules"
        label-width="100px"
      >
        <el-form-item label="Name" prop="name">
          <el-input
            v-model="skillsetFormData.name"
            :disabled="isEditingSkillset"
            placeholder="Unique skillset name (e.g., cicd-skillset)"
          />
        </el-form-item>

        <el-form-item label="Description" prop="description">
          <el-input
            v-model="skillsetFormData.description"
            type="textarea"
            :rows="3"
            placeholder="Description of this skillset"
          />
        </el-form-item>

        <el-form-item label="Owner" prop="owner">
          <el-input v-model="skillsetFormData.owner" placeholder="e.g., team-name or user@email.com" />
        </el-form-item>

        <el-form-item label="Default">
          <el-switch v-model="skillsetFormData.is_default" />
          <span style="margin-left: 8px; color: var(--el-text-color-secondary); font-size: 12px">
            Set as the default skillset
          </span>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="skillsetDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="savingSkillset" @click="saveSkillset">
          {{ isEditingSkillset ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Manage Skillset Skills Dialog -->
    <el-dialog
      v-model="manageSkillsDialogVisible"
      :title="`Manage Skills - ${managingSkillset?.name || ''}`"
      width="900px"
      destroy-on-close
    >
      <div class="manage-skills-container">
        <el-row :gutter="20">
          <!-- Available Skills -->
          <el-col :span="11">
            <div class="skills-panel">
              <div class="panel-header">
                <span>Available Skills</span>
                <el-input
                  v-model="availableSkillsSearch"
                  placeholder="Search..."
                  size="small"
                  style="width: 150px"
                  clearable
                />
              </div>
              <div class="skills-list" v-loading="loadingAvailableSkills">
                <div
                  v-for="skill in filteredAvailableSkills"
                  :key="skill.name"
                  class="skill-item"
                  :class="{ selected: selectedAvailableSkills.includes(skill.name) }"
                  @click="toggleAvailableSkill(skill.name)"
                >
                  <span class="skill-name">{{ skill.name }}</span>
                  <el-tag v-if="skill.category" size="small" type="info">{{ skill.category }}</el-tag>
                </div>
                <el-empty v-if="filteredAvailableSkills.length === 0" description="No available skills" />
              </div>
            </div>
          </el-col>

          <!-- Action Buttons -->
          <el-col :span="2" class="action-col">
            <el-button
              :disabled="selectedAvailableSkills.length === 0"
              @click="addSelectedSkills"
            >
              <i i="ep-arrow-right" />
            </el-button>
            <el-button
              :disabled="selectedSkillsetSkills.length === 0"
              @click="removeSelectedSkills"
            >
              <i i="ep-arrow-left" />
            </el-button>
          </el-col>

          <!-- Skillset Skills -->
          <el-col :span="11">
            <div class="skills-panel">
              <div class="panel-header">
                <span>Skillset Skills</span>
                <el-input
                  v-model="skillsetSkillsSearch"
                  placeholder="Search..."
                  size="small"
                  style="width: 150px"
                  clearable
                />
              </div>
              <div class="skills-list" v-loading="loadingSkillsetSkills">
                <div
                  v-for="skill in filteredSkillsetSkillsList"
                  :key="skill.name"
                  class="skill-item"
                  :class="{ selected: selectedSkillsetSkills.includes(skill.name) }"
                  @click="toggleSkillsetSkill(skill.name)"
                >
                  <span class="skill-name">{{ skill.name }}</span>
                  <el-tag v-if="skill.category" size="small" type="info">{{ skill.category }}</el-tag>
                </div>
                <el-empty v-if="filteredSkillsetSkillsList.length === 0" description="No skills in skillset" />
              </div>
            </div>
          </el-col>
        </el-row>
      </div>

      <template #footer>
        <el-button @click="manageSkillsDialogVisible = false">Close</el-button>
      </template>
    </el-dialog>

    <!-- Import Dialog -->
    <el-dialog
      v-model="importDialogVisible"
      title="Import Skills"
      width="600px"
      destroy-on-close
    >
      <el-tabs v-model="importTab">
        <!-- GitHub Import Tab -->
        <el-tab-pane label="From GitHub" name="github">
          <el-form :model="importGitHubForm" label-width="120px">
            <el-form-item label="Repository URL" required>
              <el-input
                v-model="importGitHubForm.url"
                placeholder="https://github.com/owner/repo or https://github.com/owner/repo/tree/main/skills"
              />
            </el-form-item>
            <el-form-item label="GitHub Token">
              <el-input
                v-model="importGitHubForm.github_token"
                type="password"
                placeholder="Personal access token (required for private repos)"
                show-password
              />
            </el-form-item>
            <el-alert
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 16px"
            >
              <template #title>Supported URL formats:</template>
              <ul style="margin: 8px 0 0 0; padding-left: 20px">
                <li><code>https://github.com/owner/repo</code> - Import all SKILL.md files</li>
                <li><code>https://github.com/owner/repo/tree/branch/path</code> - Import from specific path</li>
                <li><code>https://github.com/owner/repo/blob/branch/file.md</code> - Import single file</li>
              </ul>
            </el-alert>
          </el-form>
        </el-tab-pane>

        <!-- File Upload Tab -->
        <el-tab-pane label="Upload File" name="file">
          <el-upload
            ref="uploadRef"
            class="upload-area"
            drag
            :auto-upload="false"
            :limit="1"
            accept=".md,.zip"
            :on-change="handleFileChange"
            :on-exceed="handleExceed"
          >
            <i i="ep-upload-filled" style="font-size: 48px; color: var(--el-color-primary)" />
            <div class="el-upload__text">
              Drop file here or <em>click to upload</em>
            </div>
            <template #tip>
              <div class="el-upload__tip">
                Supported formats: SKILL.md file or ZIP archive containing multiple skills
              </div>
            </template>
          </el-upload>
        </el-tab-pane>
      </el-tabs>

      <!-- Import Results -->
      <div v-if="importResult" class="import-result">
        <el-divider>Import Results</el-divider>
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item label="Status">
            <el-tag :type="importResult.errors.length > 0 ? 'warning' : 'success'">
              {{ importResult.message }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item v-if="importResult.imported.length > 0" label="Imported">
            <el-tag
              v-for="name in importResult.imported"
              :key="name"
              type="success"
              size="small"
              style="margin-right: 4px; margin-bottom: 4px"
            >
              {{ name }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item v-if="importResult.errors.length > 0" label="Errors">
            <div v-for="(error, idx) in importResult.errors" :key="idx" class="error-item">
              <el-text type="danger" size="small">{{ error }}</el-text>
            </div>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <template #footer>
        <el-button @click="importDialogVisible = false">Close</el-button>
        <el-button
          type="primary"
          :loading="importing"
          :disabled="!canImport"
          @click="handleImport"
        >
          Import
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { marked } from 'marked'
import type { UploadInstance, UploadFile, UploadRawFile } from 'element-plus'
import {
  listSkills,
  getSkill,
  getSkillContent,
  createSkill,
  updateSkill,
  deleteSkill,
  searchSkills,
  importFromGitHub,
  importFromFile,
  listSkillsets,
  createSkillset,
  updateSkillset,
  deleteSkillset,
  listSkillsetSkills,
  addSkillsToSkillset,
  removeSkillsFromSkillset,
  searchSkillsInSkillset,
  SKILL_CATEGORIES,
  SKILL_SOURCES,
  type Skill,
  type Skillset,
  type CreateSkillRequest,
  type CreateSkillsetRequest,
  type ImportResult
} from '@/services/skills'

// Main tab state
const activeTab = ref('skills')

// Skills State
const loading = ref(false)
const saving = ref(false)
const skills = ref<Skill[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const selectedSkillset = ref('')

// Filters
const searchQuery = ref('')
const filterCategory = ref('')
const filterSource = ref('')

// Dialog state
const dialogVisible = ref(false)
const isEditing = ref(false)
const formRef = ref<FormInstance>()
const formData = ref<CreateSkillRequest>({
  name: '',
  description: '',
  category: '',
  version: '1.0.0',
  source: 'user',
  license: '',
  content: ''
})

// View dialog
const viewDialogVisible = ref(false)
const viewingSkill = ref<Skill | null>(null)
const viewingContent = ref('')
const loadingContent = ref(false)

// Import dialog
const importDialogVisible = ref(false)
const importTab = ref('github')
const importing = ref(false)
const importResult = ref<ImportResult | null>(null)
const importGitHubForm = ref({
  url: '',
  github_token: ''
})
const uploadRef = ref<UploadInstance>()
const uploadFile = ref<UploadRawFile | null>(null)

// Skillset state
const skillsets = ref<Skillset[]>([])
const skillsetTotal = ref(0)
const skillsetCurrentPage = ref(1)
const skillsetPageSize = ref(20)
const loadingSkillsets = ref(false)
const skillsetSearchQuery = ref('')
const skillsetSkillCounts = ref<Record<string, number>>({})

// Skillset dialog
const skillsetDialogVisible = ref(false)
const isEditingSkillset = ref(false)
const savingSkillset = ref(false)
const skillsetFormRef = ref<FormInstance>()
const skillsetFormData = ref<CreateSkillsetRequest>({
  name: '',
  description: '',
  owner: '',
  is_default: false
})

// Manage skills dialog
const manageSkillsDialogVisible = ref(false)
const managingSkillset = ref<Skillset | null>(null)
const loadingAvailableSkills = ref(false)
const loadingSkillsetSkills = ref(false)
const allSkillsList = ref<Skill[]>([])
const skillsetSkillsList = ref<Skill[]>([])
const selectedAvailableSkills = ref<string[]>([])
const selectedSkillsetSkills = ref<string[]>([])
const availableSkillsSearch = ref('')
const skillsetSkillsSearch = ref('')

// Form rules
const formRules: FormRules = {
  name: [
    { required: true, message: 'Name is required', trigger: 'blur' },
    { pattern: /^[a-z0-9-]+$/, message: 'Name must be lowercase letters, numbers, and hyphens only', trigger: 'blur' }
  ],
  description: [
    { required: true, message: 'Description is required', trigger: 'blur' },
    { min: 10, message: 'Description must be at least 10 characters', trigger: 'blur' }
  ]
}

const skillsetFormRules: FormRules = {
  name: [
    { required: true, message: 'Name is required', trigger: 'blur' },
    { pattern: /^[a-z0-9-]+$/, message: 'Name must be lowercase letters, numbers, and hyphens only', trigger: 'blur' }
  ]
}

// Computed
const renderedContent = computed(() => {
  if (!viewingContent.value) return ''
  return marked(viewingContent.value)
})

const canImport = computed(() => {
  if (importTab.value === 'github') {
    return importGitHubForm.value.url.trim() !== ''
  }
  return uploadFile.value !== null
})

const filteredSkillsets = computed(() => {
  if (!skillsetSearchQuery.value) return skillsets.value
  const query = skillsetSearchQuery.value.toLowerCase()
  return skillsets.value.filter(ss =>
    ss.name.toLowerCase().includes(query) ||
    ss.description?.toLowerCase().includes(query)
  )
})

const filteredAvailableSkills = computed(() => {
  const skillsetSkillNames = skillsetSkillsList.value.map(s => s.name)
  let available = allSkillsList.value.filter(s => !skillsetSkillNames.includes(s.name))
  if (availableSkillsSearch.value) {
    const query = availableSkillsSearch.value.toLowerCase()
    available = available.filter(s =>
      s.name.toLowerCase().includes(query) ||
      s.description?.toLowerCase().includes(query)
    )
  }
  return available
})

const filteredSkillsetSkillsList = computed(() => {
  if (!skillsetSkillsSearch.value) return skillsetSkillsList.value
  const query = skillsetSkillsSearch.value.toLowerCase()
  return skillsetSkillsList.value.filter(s =>
    s.name.toLowerCase().includes(query) ||
    s.description?.toLowerCase().includes(query)
  )
})

// Methods
const loadSkills = async () => {
  loading.value = true
  try {
    const offset = (currentPage.value - 1) * pageSize.value

    if (selectedSkillset.value) {
      // Load skills from skillset
      const res = await listSkillsetSkills(selectedSkillset.value, { offset, limit: pageSize.value })
      skills.value = res.skills || []
      total.value = res.total || 0
    } else {
      // Load all skills
      const params: Record<string, any> = {
        offset,
        limit: pageSize.value
      }
      if (filterCategory.value) params.category = filterCategory.value
      if (filterSource.value) params.source = filterSource.value

      const res = await listSkills(params)
      skills.value = res.skills || []
      total.value = res.total || 0
    }
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to load skills')
  } finally {
    loading.value = false
  }
}

const handleSearch = async () => {
  if (!searchQuery.value.trim()) {
    loadSkills()
    return
  }

  loading.value = true
  try {
    let res
    if (selectedSkillset.value) {
      res = await searchSkillsInSkillset(selectedSkillset.value, { query: searchQuery.value, limit: pageSize.value })
    } else {
      res = await searchSkills({ query: searchQuery.value, limit: pageSize.value })
    }
    // Map search results to skill format for display
    skills.value = res.skills.map(s => ({
      id: 0,
      name: s.name,
      description: s.description,
      category: s.category,
      version: '',
      source: '',
      license: '',
      content: '',
      file_path: '',
      metadata: {},
      created_at: '',
      updated_at: ''
    }))
    total.value = res.total
  } catch (err: any) {
    ElMessage.error(err.message || 'Search failed')
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  isEditing.value = false
  formData.value = {
    name: '',
    description: '',
    category: '',
    version: '1.0.0',
    source: 'user',
    license: '',
    content: ''
  }
  dialogVisible.value = true
}

const viewSkill = async (skill: Skill) => {
  viewingSkill.value = skill
  viewingContent.value = ''
  viewDialogVisible.value = true

  loadingContent.value = true
  try {
    // First get full skill details
    const fullSkill = await getSkill(skill.name)
    viewingSkill.value = fullSkill
    viewingContent.value = fullSkill.content || ''

    // If no content in skill, try to get from content endpoint
    if (!viewingContent.value) {
      try {
        const content = await getSkillContent(skill.name)
        viewingContent.value = content
      } catch {
        // Content might not be available
      }
    }
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to load skill details')
  } finally {
    loadingContent.value = false
  }
}

const editSkill = async (skill: Skill) => {
  viewDialogVisible.value = false
  isEditing.value = true

  try {
    const fullSkill = await getSkill(skill.name)
    formData.value = {
      name: fullSkill.name,
      description: fullSkill.description,
      category: fullSkill.category,
      version: fullSkill.version,
      source: fullSkill.source,
      license: fullSkill.license,
      content: fullSkill.content
    }
    dialogVisible.value = true
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to load skill')
  }
}

const saveSkill = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
  } catch {
    return
  }

  saving.value = true
  try {
    if (isEditing.value) {
      await updateSkill(formData.value.name!, {
        description: formData.value.description,
        category: formData.value.category,
        version: formData.value.version,
        license: formData.value.license,
        content: formData.value.content
      })
      ElMessage.success('Skill updated successfully')
    } else {
      await createSkill(formData.value)
      ElMessage.success('Skill created successfully')
    }
    dialogVisible.value = false
    loadSkills()
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to save skill')
  } finally {
    saving.value = false
  }
}

const confirmDelete = (skill: Skill) => {
  ElMessageBox.confirm(
    `Are you sure you want to delete skill "${skill.name}"? This action cannot be undone.`,
    'Delete Skill',
    {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning'
    }
  ).then(async () => {
    try {
      await deleteSkill(skill.name)
      ElMessage.success('Skill deleted successfully')
      loadSkills()
    } catch (err: any) {
      ElMessage.error(err.message || 'Failed to delete skill')
    }
  }).catch(() => {
    // Cancelled
  })
}

const getSourceTagType = (source: string) => {
  switch (source) {
    case 'platform': return 'success'
    case 'team': return 'warning'
    case 'user': return 'info'
    default: return 'info'
  }
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString()
}

// Import methods
const showImportDialog = () => {
  importResult.value = null
  importGitHubForm.value = { url: '', github_token: '' }
  uploadFile.value = null
  importDialogVisible.value = true
}

const handleFileChange = (file: UploadFile) => {
  uploadFile.value = file.raw || null
}

const handleExceed = () => {
  ElMessage.warning('Only one file can be uploaded at a time')
}

const handleImport = async () => {
  importing.value = true
  importResult.value = null

  try {
    if (importTab.value === 'github') {
      const data: { url: string; github_token?: string } = {
        url: importGitHubForm.value.url
      }
      if (importGitHubForm.value.github_token) {
        data.github_token = importGitHubForm.value.github_token
      }
      importResult.value = await importFromGitHub(data)
    } else if (uploadFile.value) {
      importResult.value = await importFromFile(uploadFile.value)
    }

    if (importResult.value && importResult.value.imported.length > 0) {
      ElMessage.success(`Successfully imported ${importResult.value.imported.length} skill(s)`)
      loadSkills()
    }
  } catch (err: any) {
    ElMessage.error(err.message || 'Import failed')
    importResult.value = {
      message: 'Import failed',
      imported: [],
      skipped: [],
      errors: [err.message || 'Unknown error']
    }
  } finally {
    importing.value = false
  }
}

// ======================== Skillset Methods ========================

const loadSkillsets = async () => {
  loadingSkillsets.value = true
  try {
    const offset = (skillsetCurrentPage.value - 1) * skillsetPageSize.value
    const res = await listSkillsets({ offset, limit: skillsetPageSize.value })
    skillsets.value = res.skillsets || []
    skillsetTotal.value = res.total || 0

    // Load skill counts for each skillset
    for (const ss of skillsets.value) {
      try {
        const skillsRes = await listSkillsetSkills(ss.name, { limit: 1 })
        skillsetSkillCounts.value[ss.name] = skillsRes.total
      } catch {
        skillsetSkillCounts.value[ss.name] = 0
      }
    }
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to load skillsets')
  } finally {
    loadingSkillsets.value = false
  }
}

const showCreateSkillsetDialog = () => {
  isEditingSkillset.value = false
  skillsetFormData.value = {
    name: '',
    description: '',
    owner: '',
    is_default: false
  }
  skillsetDialogVisible.value = true
}

const editSkillset = (skillset: Skillset) => {
  isEditingSkillset.value = true
  skillsetFormData.value = {
    name: skillset.name,
    description: skillset.description,
    owner: skillset.owner,
    is_default: skillset.is_default
  }
  skillsetDialogVisible.value = true
}

const saveSkillset = async () => {
  if (!skillsetFormRef.value) return

  try {
    await skillsetFormRef.value.validate()
  } catch {
    return
  }

  savingSkillset.value = true
  try {
    if (isEditingSkillset.value) {
      await updateSkillset(skillsetFormData.value.name!, {
        description: skillsetFormData.value.description,
        owner: skillsetFormData.value.owner,
        is_default: skillsetFormData.value.is_default
      })
      ElMessage.success('Skillset updated successfully')
    } else {
      await createSkillset(skillsetFormData.value)
      ElMessage.success('Skillset created successfully')
    }
    skillsetDialogVisible.value = false
    loadSkillsets()
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to save skillset')
  } finally {
    savingSkillset.value = false
  }
}

const confirmDeleteSkillset = (skillset: Skillset) => {
  ElMessageBox.confirm(
    `Are you sure you want to delete skillset "${skillset.name}"? This will not delete the skills, only the skillset.`,
    'Delete Skillset',
    {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning'
    }
  ).then(async () => {
    try {
      await deleteSkillset(skillset.name)
      ElMessage.success('Skillset deleted successfully')
      loadSkillsets()
    } catch (err: any) {
      ElMessage.error(err.message || 'Failed to delete skillset')
    }
  }).catch(() => {})
}

const setDefaultSkillset = async (skillset: Skillset) => {
  try {
    await updateSkillset(skillset.name, { is_default: true })
    ElMessage.success(`"${skillset.name}" is now the default skillset`)
    loadSkillsets()
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to set default skillset')
  }
}

const viewSkillset = (skillset: Skillset) => {
  selectedSkillset.value = skillset.name
  activeTab.value = 'skills'
  loadSkills()
}

// Manage skillset skills
const manageSkillsetSkills = async (skillset: Skillset) => {
  managingSkillset.value = skillset
  selectedAvailableSkills.value = []
  selectedSkillsetSkills.value = []
  availableSkillsSearch.value = ''
  skillsetSkillsSearch.value = ''
  manageSkillsDialogVisible.value = true

  // Load all skills and skillset skills
  loadingAvailableSkills.value = true
  loadingSkillsetSkills.value = true

  try {
    const [allRes, ssRes] = await Promise.all([
      listSkills({ limit: 1000 }),
      listSkillsetSkills(skillset.name, { limit: 1000 })
    ])
    allSkillsList.value = allRes.skills || []
    skillsetSkillsList.value = ssRes.skills || []
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to load skills')
  } finally {
    loadingAvailableSkills.value = false
    loadingSkillsetSkills.value = false
  }
}

const toggleAvailableSkill = (name: string) => {
  const idx = selectedAvailableSkills.value.indexOf(name)
  if (idx >= 0) {
    selectedAvailableSkills.value.splice(idx, 1)
  } else {
    selectedAvailableSkills.value.push(name)
  }
}

const toggleSkillsetSkill = (name: string) => {
  const idx = selectedSkillsetSkills.value.indexOf(name)
  if (idx >= 0) {
    selectedSkillsetSkills.value.splice(idx, 1)
  } else {
    selectedSkillsetSkills.value.push(name)
  }
}

const addSelectedSkills = async () => {
  if (!managingSkillset.value || selectedAvailableSkills.value.length === 0) return

  loadingSkillsetSkills.value = true
  try {
    await addSkillsToSkillset(managingSkillset.value.name, { skills: selectedAvailableSkills.value })
    ElMessage.success('Skills added to skillset')

    // Refresh lists
    const ssRes = await listSkillsetSkills(managingSkillset.value.name, { limit: 1000 })
    skillsetSkillsList.value = ssRes.skills || []
    selectedAvailableSkills.value = []

    // Update skill count
    skillsetSkillCounts.value[managingSkillset.value.name] = ssRes.total
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to add skills')
  } finally {
    loadingSkillsetSkills.value = false
  }
}

const removeSelectedSkills = async () => {
  if (!managingSkillset.value || selectedSkillsetSkills.value.length === 0) return

  loadingSkillsetSkills.value = true
  try {
    await removeSkillsFromSkillset(managingSkillset.value.name, { skills: selectedSkillsetSkills.value })
    ElMessage.success('Skills removed from skillset')

    // Refresh lists
    const ssRes = await listSkillsetSkills(managingSkillset.value.name, { limit: 1000 })
    skillsetSkillsList.value = ssRes.skills || []
    selectedSkillsetSkills.value = []

    // Update skill count
    skillsetSkillCounts.value[managingSkillset.value.name] = ssRes.total
  } catch (err: any) {
    ElMessage.error(err.message || 'Failed to remove skills')
  } finally {
    loadingSkillsetSkills.value = false
  }
}

// Lifecycle
onMounted(() => {
  loadSkills()
  loadSkillsets()
})

// Watch tab changes
watch(activeTab, (tab) => {
  if (tab === 'skillsets') {
    loadSkillsets()
  }
})
</script>

<style scoped>
.skills-repository {
  padding: 0;
}

.skills-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.search-section {
  display: flex;
  align-items: center;
}

.skills-table-card {
  margin-bottom: 20px;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

.skill-content-editor :deep(textarea) {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 13px;
  line-height: 1.5;
}

.skill-view {
  max-height: 70vh;
  overflow-y: auto;
}

.skill-content-view {
  padding: 16px;
  background: var(--el-bg-color-page);
  border-radius: 8px;
  max-height: 400px;
  overflow-y: auto;
}

.markdown-content {
  line-height: 1.8;
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3) {
  margin-top: 1em;
  margin-bottom: 0.5em;
}

.markdown-content :deep(pre) {
  background: var(--el-fill-color-light);
  padding: 12px;
  border-radius: 6px;
  overflow-x: auto;
}

.markdown-content :deep(code) {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  padding-left: 20px;
}

.markdown-content :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 1em 0;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  border: 1px solid var(--el-border-color);
  padding: 8px 12px;
}

.markdown-content :deep(th) {
  background: var(--el-fill-color-light);
}

.action-buttons {
  display: flex;
  gap: 12px;
}

.upload-area {
  width: 100%;
}

.upload-area :deep(.el-upload-dragger) {
  width: 100%;
  height: 200px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
}

.import-result {
  margin-top: 16px;
}

.error-item {
  margin-bottom: 4px;
}

.error-item:last-child {
  margin-bottom: 0;
}

/* Workspace Tabs */
.main-tabs {
  width: 100%;
}

.main-tabs :deep(.el-tabs__content) {
  padding: 0;
}

/* Manage Skills Dialog */
.manage-skills-container {
  min-height: 400px;
}

.skills-panel {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  height: 400px;
  display: flex;
  flex-direction: column;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--el-border-color);
  background: var(--el-fill-color-light);
  border-radius: 8px 8px 0 0;
  font-weight: 500;
}

.skills-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.skill-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
  margin-bottom: 4px;
}

.skill-item:hover {
  background: var(--el-fill-color-light);
}

.skill-item.selected {
  background: var(--el-color-primary-light-9);
  border: 1px solid var(--el-color-primary-light-5);
}

.skill-name {
  font-size: 13px;
}

.action-col {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 12px;
}

.action-col .el-button {
  width: 40px;
  height: 40px;
  padding: 0;
}
</style>
