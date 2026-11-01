<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">API Keys</el-text>
    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center">
        <el-button
          v-if="creatable"
          type="primary"
          round
          :icon="Plus"
          class="text-black"
          @click="
            () => {
              addVisible = true
            }
          "
        >
          Create API Key
        </el-button>
        <el-text v-else type="info">
          New API keys are issued by the external identity provider. A key issued before the
          switch can still be deleted here.
        </el-text>
      </div>
      <div class="flex flex-wrap items-center">
        <el-input
          v-model="searchName"
          placeholder="Search by name"
          clearable
          :prefix-icon="Search"
          style="max-width: 300px"
          @input="debouncedSearch"
          @clear="appliedName = ''"
        />
      </div>
    </div>
  </div>
  <KeyList
    ref="primary"
    :provider-url="providerUrl"
    :name="appliedName"
    :height="legacyPresent ? '320px' : 'calc(100vh / var(--zoom) - 240px)'"
  />
  <!-- With an identity provider that issues keys, the keys this deployment issued before the switch
       still authenticate and are not the provider's to list: they stay here, to be revoked. -->
  <KeyList
    v-if="legacy"
    hide-until-present
    :name="appliedName"
    height="240px"
    title="Keys issued before the switch"
    hint="These keys were issued by this deployment before the identity provider took over. They still work until they expire or are deleted."
    @present="legacyPresent = $event"
  />
  <AddDialog
    v-model:visible="addVisible"
    :provider-url="providerUrl"
    @success="primary?.refetch()"
  />
</template>

<script lang="ts" setup>
import { ref, computed } from 'vue'
import { Plus, Search } from '@element-plus/icons-vue'
import { debounce } from 'lodash'
import AddDialog from './Components/AddDialog.vue'
import KeyList from './Components/KeyList.vue'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()
// With an external identity provider the apiserver refuses to issue keys: a key is created at the
// provider's route when the deployment has one (GET /envs apiKeysUrl), and there is nothing to create
// with otherwise. Without a route the list is the apiserver's, as before. See apiKeyRouting.
const providerUrl = computed(() => userStore.apiKeyRouting.providerUrl)
const creatable = computed(() => userStore.apiKeyRouting.creatable)
const legacy = computed(() => userStore.apiKeyRouting.legacy)
const addVisible = ref(false)
const primary = ref<InstanceType<typeof KeyList> | null>(null)
const legacyPresent = ref(false)
const searchName = ref('')
const appliedName = ref('')

const debouncedSearch = debounce((name: string) => {
  appliedName.value = name.trim()
}, 300)

defineOptions({
  name: 'APIKeysPage',
})
</script>

<style></style>
