<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Workspace access</el-text>
    <el-text type="info" class="block mt-1">
      Joining a team gives access to every workspace bound to it. Request a workspace directly only
      when no team fits.
    </el-text>
    <el-card class="safe-card mt-4" shadow="never">
      <el-tabs :model-value="tab" @update:model-value="onTabChange">
        <el-tab-pane label="Catalog" name="catalog">
          <CatalogTab v-if="tab === 'catalog'" />
        </el-tab-pane>
        <el-tab-pane label="My requests" name="mine">
          <MyRequestsTab v-if="tab === 'mine'" />
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CatalogTab from './Components/CatalogTab.vue'
import MyRequestsTab from './Components/MyRequestsTab.vue'

const route = useRoute()
const router = useRouter()

// The open tab is kept in the query so a link can open "My requests" directly.
const tab = computed(() => (route.query.tab === 'mine' ? 'mine' : 'catalog'))

// Switches the tab by rewriting the query.
const onTabChange = (name: string | number) => {
  router.replace({ query: { ...route.query, tab: name === 'mine' ? 'mine' : undefined } })
}
defineOptions({
  name: 'WorkspaceAccessPage',
})
</script>
