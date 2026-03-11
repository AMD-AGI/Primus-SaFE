<template>
  <el-tooltip v-if="showTooltip" :content="item.tooltip" placement="right">
    <el-menu-item :index="item.index" :disabled="true" :data-tour="item.dataTour">
      <MenuItemIcon
        v-if="item.icon"
        :index="item.index"
        :light="item.icon.light"
        :dark="item.icon.dark"
        :active="item.icon.active"
        :size="16"
        match="prefix"
      />
      <el-icon v-else-if="item.elIcon">
        <component :is="item.elIcon" />
      </el-icon>
      {{ item.name }}
    </el-menu-item>
  </el-tooltip>
  <el-menu-item v-else :index="item.index" :disabled="!enabled" :data-tour="item.dataTour">
    <MenuItemIcon
      v-if="item.icon"
      :index="item.index"
      :light="item.icon.light"
      :dark="item.icon.dark"
      :active="item.icon.active"
      :size="16"
      match="prefix"
    />
    <el-icon v-else-if="item.elIcon">
      <component :is="item.elIcon" />
    </el-icon>
    {{ item.name }}
  </el-menu-item>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import MenuItemIcon from '@/components/Base/MenuItemIcon.vue'

interface MenuItem {
  index: string
  name: string
  icon?: {
    light: string
    dark: string
    active: string
  }
  elIcon?: Component
  canAccess?: boolean
  tooltip?: string
  dataTour?: string
}

const props = defineProps<{
  item: MenuItem
}>()

// Use computed to ensure reactive updates
const enabled = computed(() => props.item.canAccess !== false)
const showTooltip = computed(() => !enabled.value && props.item.tooltip)
</script>
