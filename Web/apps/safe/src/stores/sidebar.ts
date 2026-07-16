import { defineStore } from 'pinia'

export const SIDEBAR_MIN_WIDTH = 180
export const SIDEBAR_MAX_WIDTH = 320
export const SIDEBAR_DEFAULT_WIDTH = 220
export const SIDEBAR_COLLAPSED_WIDTH = 64

export interface SidebarState {
  width: number
  collapsed: boolean
}

export const useSidebarStore = defineStore('sidebar', {
  state: (): SidebarState => ({
    width: SIDEBAR_DEFAULT_WIDTH,
    collapsed: false,
  }),

  persist: {
    key: 'sidebar',
    storage: localStorage,
    paths: ['width', 'collapsed'],
  } as any,

  getters: {
    // Effective pixel width used by the layout.
    effectiveWidth(state): number {
      return state.collapsed ? SIDEBAR_COLLAPSED_WIDTH : state.width
    },
  },

  actions: {
    setWidth(width: number) {
      this.width = Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)))
    },
    toggleCollapsed() {
      this.collapsed = !this.collapsed
    },
    setCollapsed(collapsed: boolean) {
      this.collapsed = collapsed
    },
  },
})
