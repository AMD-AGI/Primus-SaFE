<template>
  <Transition name="slash-menu">
    <div v-if="visible && hasContent" class="slash-command-menu">
      <!-- Searching / subcommand mode: flat display items -->
      <template v-if="isSearching">
        <div class="slash-menu-scroll">
          <div
            v-for="(item, i) in displayItems"
            :key="item.id"
            class="slash-menu-item"
            :class="{ active: i === activeIndex }"
            :ref="(el) => setItemRef(el as HTMLElement | null, i)"
            @click="$emit('select', item)"
            @mouseenter="$emit('update:activeIndex', i)"
          >
            <el-icon class="slash-menu-icon">
              <component :is="iconMap[item.icon || ''] || ChatDotRound" />
            </el-icon>
            <span class="slash-menu-name">/{{ item.displayCommand }}</span>
            <span class="slash-menu-desc">{{ item.description }}</span>
            <el-icon v-if="item.parentCommand.action === 'navigate'" class="slash-menu-arrow">
              <Right />
            </el-icon>
          </div>
        </div>
      </template>

      <!-- Idle: grouped view with submenu hover -->
      <template v-else>
        <!-- Submenu groups: outside scroll to avoid overflow clipping -->
        <template v-for="group in submenuGroupList" :key="group.label">
          <div
            class="slash-submenu-wrapper"
            @mouseenter="onSubmenuEnter(group.label)"
            @mouseleave="hoveredGroup = ''"
          >
            <div class="slash-menu-item submenu-trigger" :class="{ active: hoveredGroup === group.label }">
              <el-icon class="slash-menu-icon"><Menu /></el-icon>
              <span class="slash-menu-name">{{ group.label }}</span>
              <span class="slash-menu-desc">{{ subItemCount(group) }} items</span>
              <el-icon class="slash-menu-arrow"><Right /></el-icon>
            </div>

            <Transition name="slash-submenu">
              <div v-if="hoveredGroup === group.label" class="slash-submenu-panel">
                <div class="slash-submenu-inner">
                  <div class="slash-menu-group-label">{{ group.label }}</div>
                  <template v-for="cmd in group.commands" :key="cmd.id">
                    <!-- Subcommand options -->
                    <template v-if="cmd.subcommands?.length">
                      <div
                        v-for="sub in cmd.subcommands"
                        :key="sub.value"
                        class="slash-menu-item"
                        :class="{ active: hoveredSubItem === sub.value }"
                        @click="onSubcommandClick(cmd, sub)"
                        @mouseenter="hoveredSubItem = sub.value"
                        @mouseleave="hoveredSubItem = ''"
                      >
                        <el-icon class="slash-menu-icon">
                          <component :is="iconMap[sub.icon || ''] || Box" />
                        </el-icon>
                        <span class="slash-menu-name">/{{ cmd.command }} {{ sub.value }}</span>
                        <span class="slash-menu-desc">{{ sub.description }}</span>
                      </div>
                    </template>
                    <!-- Fallback: commands without subcommands -->
                    <div
                      v-else
                      class="slash-menu-item"
                      :class="{ active: hoveredSubItem === cmd.id }"
                      @click="onCommandClick(cmd)"
                      @mouseenter="hoveredSubItem = cmd.id"
                      @mouseleave="hoveredSubItem = ''"
                    >
                      <el-icon class="slash-menu-icon">
                        <component :is="iconMap[cmd.icon || ''] || ChatDotRound" />
                      </el-icon>
                      <span class="slash-menu-name">/{{ cmd.command }}</span>
                      <span class="slash-menu-desc">{{ cmd.description }}</span>
                    </div>
                  </template>
                </div>
              </div>
            </Transition>
          </div>
          <div class="slash-menu-divider" />
        </template>

        <!-- Regular groups: inside scroll container -->
        <div class="slash-menu-scroll">
          <template v-for="(group, gi) in regularGroupList" :key="group.label">
            <div v-if="gi > 0" class="slash-menu-divider" />
            <div class="slash-menu-group-label">{{ group.label }}</div>
            <div
              v-for="(cmd, ci) in group.commands"
              :key="cmd.id"
              class="slash-menu-item"
              :class="{ active: getRegularFlatIndex(gi, ci) === activeIndex }"
              :ref="(el) => setItemRef(el as HTMLElement | null, getRegularFlatIndex(gi, ci))"
              @click="onCommandClick(cmd)"
              @mouseenter="onRegularItemEnter(gi, ci)"
            >
              <el-icon class="slash-menu-icon">
                <component :is="iconMap[cmd.icon || ''] || ChatDotRound" />
              </el-icon>
              <span class="slash-menu-name">/{{ cmd.command }}</span>
              <span class="slash-menu-desc">{{ cmd.description }}</span>
            </div>
          </template>
        </div>
      </template>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { watch, ref, computed } from 'vue'
import {
  Delete,
  Edit,
  ChatDotRound,
  MagicStick,
  View,
  QuestionFilled,
  Box,
  Cpu,
  TrendCharts,
  Refresh,
  Lightning,
  EditPen,
  Right,
  Menu,
} from '@element-plus/icons-vue'
import { submenuCategories, type SlashCommand, type SubcommandOption } from '../constants/slashCommands'
import type { MenuDisplayItem } from '../composables/slashCommandExecutor'
import type { SlashCommandGroup } from '../composables/useSlashCommands'

const props = defineProps<{
  groups: SlashCommandGroup[]
  displayItems: MenuDisplayItem[]
  activeIndex: number
  visible: boolean
  isSearching: boolean
}>()

const emit = defineEmits<{
  select: [item: MenuDisplayItem]
  'update:activeIndex': [index: number]
}>()

const iconMap: Record<string, unknown> = {
  Delete, Edit, ChatDotRound, MagicStick, View, QuestionFilled,
  Box, Cpu, TrendCharts, Refresh, Lightning, EditPen,
}

const hoveredGroup = ref('')
const hoveredSubItem = ref('')
const itemRefs = ref<(HTMLElement | null)[]>([])

const submenuGroupList = computed(() =>
  props.groups.filter((g) => submenuCategories.has(g.label)),
)
const regularGroupList = computed(() =>
  props.groups.filter((g) => !submenuCategories.has(g.label)),
)

const hasContent = computed(() => {
  if (props.isSearching) return props.displayItems.length > 0
  return props.groups.length > 0
})

const subItemCount = (group: SlashCommandGroup) =>
  group.commands.reduce((sum, cmd) => sum + (cmd.subcommands?.length ?? 1), 0)

// Flat index for regular (non-submenu) groups in idle mode
const getRegularFlatIndex = (groupIdx: number, cmdIdx: number): number => {
  let idx = 0
  for (let g = 0; g < groupIdx; g++) {
    idx += regularGroupList.value[g].commands.length
  }
  return idx + cmdIdx
}

const setItemRef = (el: HTMLElement | null, index: number) => {
  if (index >= 0) itemRefs.value[index] = el
}

const onSubmenuEnter = (label: string) => {
  hoveredGroup.value = label
  emit('update:activeIndex', -1)
}

const onRegularItemEnter = (groupIdx: number, cmdIdx: number) => {
  hoveredGroup.value = ''
  emit('update:activeIndex', getRegularFlatIndex(groupIdx, cmdIdx))
}

/** Click a subcommand option in the hover submenu → construct MenuDisplayItem directly. */
const onSubcommandClick = (cmd: SlashCommand, sub: SubcommandOption) => {
  emit('select', {
    id: `${cmd.id}:${sub.value}`,
    displayCommand: `${cmd.command} ${sub.value}`,
    title: sub.title,
    description: sub.description,
    icon: sub.icon,
    parentCommand: cmd,
    subValue: sub.value,
  })
}

/** Click a regular command → construct MenuDisplayItem directly. */
const onCommandClick = (cmd: SlashCommand) => {
  emit('select', {
    id: cmd.id,
    displayCommand: cmd.command,
    title: cmd.title,
    description: cmd.description,
    icon: cmd.icon,
    parentCommand: cmd,
  })
}

watch(
  () => props.activeIndex,
  (index) => {
    if (index >= 0) {
      const el = itemRefs.value[index]
      if (el) el.scrollIntoView({ block: 'nearest' })
    }
  },
)

watch(
  () => props.visible,
  (v) => {
    if (!v) {
      hoveredGroup.value = ''
      hoveredSubItem.value = ''
    }
  },
)
</script>

<style scoped lang="scss">
.slash-command-menu {
  position: absolute;
  bottom: 100%;
  left: 0;
  right: 0;
  margin-bottom: 6px;
  background: rgba(30, 41, 59, 0.96);
  backdrop-filter: blur(16px) saturate(180%);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35);
  z-index: 100;
  padding: 6px;
}

.slash-menu-scroll {
  max-height: 280px;
  overflow-y: auto;
}

.slash-menu-group-label {
  padding: 6px 10px 4px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: rgba(255, 255, 255, 0.35);
  user-select: none;
}

.slash-menu-divider {
  height: 1px;
  background: rgba(255, 255, 255, 0.08);
  margin: 4px 8px;
}

.slash-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.12s ease;

  &:hover,
  &.active {
    background: rgba(20, 184, 166, 0.15);
  }
}

.slash-menu-icon {
  flex-shrink: 0;
  font-size: 14px;
  color: #14b8a6;
}

.slash-menu-name {
  font-size: 13px;
  font-weight: 600;
  color: #f1f5f9;
  white-space: nowrap;
}

.slash-menu-desc {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.slash-menu-arrow {
  flex-shrink: 0;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.25);
  margin-left: auto;
}

// --- Submenu ---
.slash-submenu-wrapper {
  position: relative;
}

.slash-submenu-panel {
  position: absolute;
  left: 100%;
  top: -6px;
  padding-left: 6px;
  z-index: 10;
}

.slash-submenu-inner {
  background: rgba(30, 41, 59, 0.96);
  backdrop-filter: blur(16px) saturate(180%);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35);
  padding: 6px;
  min-width: 220px;
}

// --- Light mode ---
:root:not(.dark) {
  .slash-command-menu {
    background: rgba(255, 255, 255, 0.96);
    border-color: rgba(226, 232, 240, 0.8);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12);
  }

  .slash-menu-group-label {
    color: rgba(0, 0, 0, 0.35);
  }

  .slash-menu-divider {
    background: rgba(0, 0, 0, 0.06);
  }

  .slash-menu-item {
    &:hover,
    &.active {
      background: rgba(20, 184, 166, 0.1);
    }
  }

  .slash-menu-name {
    color: #1e293b;
  }

  .slash-menu-desc {
    color: rgba(0, 0, 0, 0.4);
  }

  .slash-menu-arrow {
    color: rgba(0, 0, 0, 0.2);
  }

  .slash-submenu-inner {
    background: rgba(255, 255, 255, 0.96);
    border-color: rgba(226, 232, 240, 0.8);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12);
  }
}

// --- Transitions ---
.slash-menu-enter-active {
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}
.slash-menu-leave-active {
  transition: all 0.15s ease-in;
}
.slash-menu-enter-from,
.slash-menu-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.slash-submenu-enter-active {
  transition: all 0.15s cubic-bezier(0.16, 1, 0.3, 1);
}
.slash-submenu-leave-active {
  transition: all 0.1s ease-in;
}
.slash-submenu-enter-from,
.slash-submenu-leave-to {
  opacity: 0;
  transform: translateX(-4px);
}
</style>
