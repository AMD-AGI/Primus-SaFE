import { defineStore } from 'pinia'

/**
 * UI bridge store:
 * - Allows any page to request opening FloatingChatBot and pre-filling input text.
 * - Uses requestId to ensure watchers trigger even if the same text is sent repeatedly.
 */
export const useChatbotUIStore = defineStore('chatbotUI', {
  state: () => ({
    requestId: 0,
    prefillText: '' as string,
  }),
  actions: {
    openAndPrefill(text: string) {
      this.prefillText = text
      this.requestId += 1
    },
    clearPrefill() {
      this.prefillText = ''
    },
  },
})
