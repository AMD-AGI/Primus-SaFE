import { defineStore } from 'pinia'

export const useFlavorStore = defineStore('flavor', {
  state: () => ({ map: {} as Record<string, any> }),
  actions: {
    set(item: any) {
      this.map[item.flavorId] = item
    },
    get(id: string) {
      return this.map[id]
    },
    clear() {
      this.map = {}
    },
  },
})
