<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">User Group</el-text>
  </div>

  <el-card class="mt-6 safe-card relative" shadow="never">
    <div class="absolute right-16px top-16px z-1">
      <el-tooltip content="Reset MCP Session" placement="top">
        <el-button circle size="small" :icon="RefreshRight" @click="handleResetSession" />
      </el-tooltip>
    </div>
    <el-tabs v-model="activeTab">
      <!-- Check User -->
      <el-tab-pane label="Check User" name="checkUser">
        <el-form
          :model="checkForm"
          label-width="140px"
          class="max-w-600px mt-4"
        >
          <el-form-item label="User Identifier">
            <el-input
              v-model="checkForm.userIdentifier"
              placeholder="NTID, email or employee ID"
              clearable
            />
          </el-form-item>
          <el-form-item label="Group CN">
            <el-input
              v-model="checkForm.groupCn"
              placeholder="e.g. dl.primus-safe-users"
              clearable
            />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" @click="handleCheckUser">
              Check
            </el-button>
          </el-form-item>
        </el-form>
      </el-tab-pane>

      <!-- Get User NTID -->
      <el-tab-pane label="Get NTID" name="getNTID">
        <el-form
          :model="ntidForm"
          label-width="140px"
          class="max-w-600px mt-4"
        >
          <el-form-item label="User Identifier">
            <el-input
              v-model="ntidForm.userIdentifier"
              placeholder="Email or employee ID"
              clearable
            />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" @click="handleGetNTID">
              Query
            </el-button>
          </el-form-item>
        </el-form>
      </el-tab-pane>

      <!-- List Allowed Groups -->
      <el-tab-pane label="Allowed Groups" name="listGroups">
        <div class="mt-4">
          <el-button type="primary" :loading="loading" @click="handleListGroups">
            Fetch Groups
          </el-button>
        </div>
      </el-tab-pane>

      <!-- List Group Members -->
      <el-tab-pane label="Group Members" name="listMembers">
        <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
          <el-form :model="membersForm" inline class="flex items-center gap-2">
            <el-form-item label="Group CN" class="mb-0">
              <el-input
                v-model="membersForm.groupCn"
                placeholder="e.g. dl.primus-safe-users"
                clearable
                style="width: 280px"
              />
            </el-form-item>
            <el-form-item class="mb-0">
              <el-button type="primary" :loading="loading" @click="handleListMembers">
                Query Members
              </el-button>
            </el-form-item>
          </el-form>
          <el-input
            v-if="membersList.length"
            v-model="memberSearch"
            placeholder="Filter members"
            clearable
            :prefix-icon="Search"
            style="width: 220px"
          />
        </div>
        <el-table
          v-if="membersList.length"
          :data="filteredMembers"
          :height="'calc(100vh - 340px)'"
          size="large"
          class="mt-4"
        >
          <el-table-column type="index" label="#" width="60" />
          <el-table-column prop="ntid" label="NTID" />
        </el-table>
        <div v-if="membersList.length" class="mt-2 text-13px text-gray-400">
          Total: {{ membersList.length }} members
          <template v-if="memberSearch"> · Showing: {{ filteredMembers.length }}</template>
        </div>
      </el-tab-pane>

      <!-- Add User to Group -->
      <el-tab-pane label="Add User" name="addUser">
        <el-form
          :model="addForm"
          label-width="140px"
          class="max-w-600px mt-4"
        >
          <el-form-item label="User Identifier">
            <el-input
              v-model="addForm.userIdentifier"
              placeholder="NTID, email or employee ID"
              clearable
            />
          </el-form-item>
          <el-form-item label="Group CN">
            <el-input
              v-model="addForm.groupCn"
              placeholder="e.g. dl.primus-safe-users"
              clearable
            />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" @click="handleAddUser">
              Add
            </el-button>
          </el-form-item>
        </el-form>
      </el-tab-pane>

      <!-- Remove User from Group -->
      <el-tab-pane label="Remove User" name="removeUser">
        <el-form
          :model="removeForm"
          label-width="140px"
          class="max-w-600px mt-4"
        >
          <el-form-item label="User Identifier">
            <el-input
              v-model="removeForm.userIdentifier"
              placeholder="NTID, email or employee ID"
              clearable
            />
          </el-form-item>
          <el-form-item label="Group CN">
            <el-input
              v-model="removeForm.groupCn"
              placeholder="e.g. dl.primus-safe-users"
              clearable
            />
          </el-form-item>
          <el-form-item>
            <el-button type="danger" :loading="loading" @click="handleRemoveUser">
              Remove
            </el-button>
          </el-form-item>
        </el-form>
      </el-tab-pane>
    </el-tabs>

    <!-- Result -->
    <div v-if="result !== null" class="mt-4">
      <el-alert :type="resultType" :closable="false" show-icon>
        <template #title>
          <span class="font-500">{{ resultTitle }}</span>
        </template>
        <pre class="m-0 whitespace-pre-wrap font-mono text-13px leading-relaxed">{{ formatResult(result) }}</pre>
      </el-alert>
    </div>

    <!-- Error -->
    <div v-if="error !== null" class="mt-4">
      <el-alert type="error" :closable="false" show-icon>
        <template #title>
          <span class="font-500">Error</span>
        </template>
        <pre class="m-0 whitespace-pre-wrap font-mono text-13px leading-relaxed">{{ formatError(error) }}</pre>
      </el-alert>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Search, RefreshRight } from '@element-plus/icons-vue'
import {
  getUserNTID,
  listAllowedGroups,
  listGroupMembers,
  addUserToGroup,
  checkUserInGroup,
  removeUserFromGroup,
  resetSession,
} from '@/services/mcp'

defineOptions({ name: 'UserGroupPage' })

const activeTab = ref('checkUser')
const loading = ref(false)
const result = ref<any>(null)
const error = ref<any>(null)
const resultType = ref<'success' | 'info' | 'warning'>('success')
const resultTitle = ref('Success')

const DEFAULT_GROUP = 'dl.primus-safe-users'

const checkForm = reactive({ userIdentifier: '', groupCn: DEFAULT_GROUP })
const ntidForm = reactive({ userIdentifier: '' })
const membersForm = reactive({ groupCn: DEFAULT_GROUP })
const addForm = reactive({ userIdentifier: '', groupCn: DEFAULT_GROUP })
const removeForm = reactive({ userIdentifier: '', groupCn: DEFAULT_GROUP })

const membersList = ref<{ ntid: string }[]>([])
const memberSearch = ref('')
const filteredMembers = computed(() => {
  if (!memberSearch.value) return membersList.value
  const keyword = memberSearch.value.toLowerCase()
  return membersList.value.filter((m) => m.ntid.toLowerCase().includes(keyword))
})

watch(activeTab, () => {
  clearResults()
  membersList.value = []
  memberSearch.value = ''
})

function clearResults() {
  result.value = null
  error.value = null
}

function formatResult(data: any) {
  return typeof data === 'string' ? data : JSON.stringify(data, null, 2)
}

function formatError(err: any) {
  if (typeof err === 'string') return err
  if (err?.message) return err.message
  return JSON.stringify(err, null, 2)
}

async function handleAPICall(apiFunc: () => Promise<any>, successMsg: string) {
  clearResults()
  loading.value = true
  try {
    const res = await apiFunc()
    result.value = res
    resultType.value = 'success'
    resultTitle.value = successMsg
    ElMessage.success(successMsg)
  } catch (err: any) {
    error.value = err
    ElMessage.error(`Failed: ${err?.message || 'Unknown error'}`)
  } finally {
    loading.value = false
  }
}

function handleCheckUser() {
  if (!checkForm.userIdentifier || !checkForm.groupCn) {
    return ElMessage.warning('Please fill in User Identifier and Group CN')
  }
  handleAPICall(() => checkUserInGroup(checkForm.userIdentifier, checkForm.groupCn), 'Check completed')
}

function handleGetNTID() {
  if (!ntidForm.userIdentifier) {
    return ElMessage.warning('Please fill in User Identifier')
  }
  handleAPICall(() => getUserNTID(ntidForm.userIdentifier), 'Query succeeded')
}

function handleListGroups() {
  handleAPICall(() => listAllowedGroups(), 'Groups fetched')
}

async function handleListMembers() {
  if (!membersForm.groupCn) {
    return ElMessage.warning('Please fill in Group CN')
  }
  clearResults()
  loading.value = true
  memberSearch.value = ''
  try {
    const res = await listGroupMembers(membersForm.groupCn)
    // Parse "  - ntid" lines from the result string
    const lines = typeof res === 'string' ? res.split('\n') : []
    membersList.value = lines
      .map((l: string) => l.trim())
      .filter((l: string) => l.startsWith('- '))
      .map((l: string) => ({ ntid: l.substring(2).trim() }))
    ElMessage.success('Members fetched')
  } catch (err: any) {
    error.value = err
    membersList.value = []
    ElMessage.error(`Failed: ${err?.message || 'Unknown error'}`)
  } finally {
    loading.value = false
  }
}

function handleAddUser() {
  if (!addForm.userIdentifier || !addForm.groupCn) {
    return ElMessage.warning('Please fill in User Identifier and Group CN')
  }
  handleAPICall(() => addUserToGroup(addForm.userIdentifier, addForm.groupCn), 'User added')
}

function handleRemoveUser() {
  if (!removeForm.userIdentifier || !removeForm.groupCn) {
    return ElMessage.warning('Please fill in User Identifier and Group CN')
  }
  handleAPICall(() => removeUserFromGroup(removeForm.userIdentifier, removeForm.groupCn), 'User removed')
}

function handleResetSession() {
  resetSession()
  clearResults()
  ElMessage.info('MCP session has been reset')
}
</script>
