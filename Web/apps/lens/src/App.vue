<script setup lang="ts">
import BaseHeader from '../src/components/layout/BaseHeader.vue'
import FloatingClusterSelector from '../src/components/FloatingClusterSelector.vue'
</script>

<template>
  <div class="app-container">
    <BaseHeader />
    <!-- Floating Cluster Selector -->
    <FloatingClusterSelector />
    <!-- default page content layout -->
      <div class="p-y-5 main-content">
      <div class="page-container">
        <router-view v-slot="{ Component, route }">
          <transition name="page" mode="out-in">
            <keep-alive :include="['ClusterStats', 'NamespaceStats', 'WorkloadStats', 'LabelStats']">
              <component :is="Component" :key="route.path" />
            </keep-alive>
          </transition>
        </router-view>
      </div>
    </div>
  </div>
</template>

<style lang="scss">
.app-container {
  min-height: 100vh;
  height: 100vh;
  position: relative;
  overflow: hidden; // Prevent scrolling at app level
  display: flex;
  flex-direction: column;
  background: linear-gradient(135deg, 
    #f5f7fa 0%, 
    #e8ecf1 25%, 
    #f0f3f7 50%, 
    #e5e9f0 75%, 
    #f2f5f9 100%);
  background-attachment: fixed;
  
  // Subtle pattern overlay
  &::before {
    content: '';
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-image: 
      radial-gradient(circle at 20% 30%, rgba(64, 158, 255, 0.03) 0%, transparent 50%),
      radial-gradient(circle at 80% 70%, rgba(103, 194, 58, 0.03) 0%, transparent 50%),
      radial-gradient(circle at 40% 80%, rgba(64, 158, 255, 0.02) 0%, transparent 50%);
    pointer-events: none;
    z-index: 0;
  }
  
  .main-content {
    position: relative;
    z-index: 1;
    flex: 1;
    min-height: 0;
    min-width: 0; // Prevent content from overflowing
    overflow-y: auto; // Allow scrolling when content overflows
    overflow-x: hidden;
    padding-left: 15%;
    padding-right: 15%;
    
    // Better responsive padding
    @media (max-width: 1920px) {
      padding-left: 10%;
      padding-right: 10%;
    }
    
    @media (max-width: 1440px) {
      padding-left: 5%;
      padding-right: 5%;
    }
    
    @media (max-width: 1024px) {
      padding-left: 20px;
      padding-right: 20px;
    }
    
    @media (max-width: 768px) {
      padding-left: 12px;
      padding-right: 12px;
    }
    
    // Custom scrollbar
    &::-webkit-scrollbar {
      width: 8px;
    }
    
    &::-webkit-scrollbar-track {
      background: transparent;
    }
    
    &::-webkit-scrollbar-thumb {
      background: rgba(0, 0, 0, 0.2);
      border-radius: 4px;
      
      &:hover {
        background: rgba(0, 0, 0, 0.3);
      }
    }
  }
}

// Dark mode background
.dark .app-container {
  background: linear-gradient(135deg, 
    #1a1d23 0%, 
    #1e2229 25%, 
    #181b21 50%, 
    #1c1f26 75%, 
    #1a1e25 100%);
  
  &::before {
    background-image: 
      radial-gradient(circle at 20% 30%, rgba(64, 158, 255, 0.08) 0%, transparent 50%),
      radial-gradient(circle at 80% 70%, rgba(103, 194, 58, 0.06) 0%, transparent 50%),
      radial-gradient(circle at 40% 80%, rgba(64, 158, 255, 0.05) 0%, transparent 50%);
  }
}

// Page container to maintain layout
.page-container {
  position: relative;
  min-height: calc(100vh - 200px);
  width: 100%;
  // Prevent layout shift during transitions
  display: grid;
  grid-template-columns: 1fr;
}

// Page transition animations - optimized for speed
.page-enter-active,
.page-leave-active {
  transition: opacity 0.15s ease-out;
  grid-column: 1;
  grid-row: 1;
}

.page-enter-from {
  opacity: 0;
}

.page-leave-to {
  opacity: 0;
}

// Use grid positioning instead of absolute
.page-enter-active {
  z-index: 1;
}

.page-leave-active {
  z-index: 0;
}
/* Global date picker OK button styling */
:deep(.el-picker-panel__link-btn:last-child),
:deep(.el-date-range-picker__content .is-right .el-picker-panel__footer .el-picker-panel__link-btn:last-child) {
  background-color: #409eff !important;
  color: #fff !important;
  padding: 4px 15px !important;
  border-radius: 4px !important;
  text-decoration: none !important;
  
  &:hover {
    background-color: #66b1ff !important;
    color: #fff !important;
  }
}
</style>
