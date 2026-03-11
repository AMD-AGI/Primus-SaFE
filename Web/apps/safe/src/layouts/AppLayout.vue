<template>
  <div class="h-screen w-screen overflow-hidden flex flex-col">
    <div class="flex flex-1 overflow-hidden">
      <BaseMenu class="w-55 shrink-0" />

      <main class="flex-1 overflow-y-auto p-6 main-content-with-glow">
        <router-view />
      </main>
    </div>

    <!-- Floating ChatBot -->
    <FloatingChatBot />
  </div>
</template>

<script setup lang="ts">
import BaseMenu from '@/components/layout/BaseMenu.vue'
import FloatingChatBot from '@/components/Base/FloatingChatBot.vue'
</script>

<style scoped lang="scss">
.main-content-with-glow {
  position: relative;
  background:
    radial-gradient(circle at 20% 30%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.06) 0%, transparent 50%),
    radial-gradient(circle at 40% 80%, rgba(6, 182, 212, 0.05) 0%, transparent 50%),
    linear-gradient(135deg, #f6f8fb 0%, #eef2f7 50%, #e8ecf1 100%);
  background-attachment: fixed;
}
</style>

<style lang="scss">
// Dark mode (unscoped)
html.dark .main-content-with-glow {
  background:
    radial-gradient(circle at 20% 30%, rgba(139, 92, 246, 0.12) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.1) 0%, transparent 50%),
    radial-gradient(circle at 40% 80%, rgba(6, 182, 212, 0.08) 0%, transparent 50%),
    linear-gradient(135deg, #0a0e14 0%, #0f1419 50%, #141921 100%);
  background-attachment: fixed;
}
</style>
