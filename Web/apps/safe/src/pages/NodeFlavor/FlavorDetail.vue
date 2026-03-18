<template>
  <!-- Header -->
  <div v-if="detailData" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
          Back
        </el-button>
        <h1 class="w-name">{{ detailData.flavorId }}</h1>
        <el-tag class="ml-2" type="success">NodeFlavor</el-tag>
      </div>
    </div>
  </div>

  <!-- Body -->
  <el-card class="mt-4 safe-card" shadow="never">
    <el-descriptions v-if="detailData" :column="2" border>
      <!-- CPU -->
      <el-descriptions-item label="CPU Product">
        {{ detailData.cpu?.product || '-' }}
      </el-descriptions-item>
      <el-descriptions-item label="CPU Quantity">
        {{ detailData.cpu?.quantity ?? '-' }} core
      </el-descriptions-item>

      <!-- Memory -->
      <el-descriptions-item label="Memory" :span="2">
        <el-tag type="info">{{ formatCapacity(detailData.memory) }}</el-tag>
        <span class="ml-2 text-[var(--el-text-color-secondary)]">({{ detailData.memory }})</span>
      </el-descriptions-item>

      <!-- GPU -->
      <el-descriptions-item label="GPU Product">
        {{ detailData.gpu?.product || '-' }}
      </el-descriptions-item>
      <el-descriptions-item label="GPU Quantity">
        {{ detailData.gpu?.quantity ?? '-' }} card
      </el-descriptions-item>
      <el-descriptions-item label="GPU Resource Name" :span="2">
        <code class="code">{{ detailData.gpu?.resourceName || '-' }}</code>
      </el-descriptions-item>

      <!-- Root Disk -->
      <el-descriptions-item label="RootDisk Type">
        {{ detailData.rootDisk?.type || '-' }}
      </el-descriptions-item>
      <el-descriptions-item label="RootDisk Capacity / Count">
        <el-space>
          <el-tag>{{ formatCapacity(detailData.rootDisk?.quantity) }}</el-tag>
          <span>× {{ detailData.rootDisk?.count ?? '-' }}</span>
        </el-space>
      </el-descriptions-item>

      <!-- Data Disk -->
      <el-descriptions-item label="DataDisk Type">
        {{ detailData.dataDisk?.type || '-' }}
      </el-descriptions-item>
      <el-descriptions-item label="DataDisk Capacity / Count">
        <el-space>
          <el-tag>{{ formatCapacity(detailData.dataDisk?.quantity) }}</el-tag>
          <span>× {{ detailData.dataDisk?.count ?? '-' }}</span>
        </el-space>
      </el-descriptions-item>

      <!-- Extended Resources -->
      <el-descriptions-item label="ephemeral-storage">
        <template v-if="detailData.extendedResources?.['ephemeral-storage']">
          <el-tag type="warning">{{
            formatCapacity(detailData.extendedResources['ephemeral-storage'])
          }}</el-tag>
          <span class="ml-2 text-[var(--el-text-color-secondary)]">
            ({{ detailData.extendedResources['ephemeral-storage'] }})
          </span>
        </template>
        <template v-else>-</template>
      </el-descriptions-item>
      <el-descriptions-item label="rdma/hca">
        {{ detailData.extendedResources?.['rdma/hca'] || '-' }}
      </el-descriptions-item>
    </el-descriptions>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { useFlavorStore } from '@/stores/flavor'

type CapUnit = 'Ki' | 'Mi' | 'Gi' | 'Ti' | 'Pi'
const CAP_UNITS: CapUnit[] = ['Ki', 'Mi', 'Gi', 'Ti', 'Pi']

const route = useRoute()
const router = useRouter()
const store = useFlavorStore()

const detailData = computed(() => store.get(route.params.id as string))

function parseQuantityWithUnit(raw?: string): { val: number; unit: CapUnit } {
  if (!raw) return { val: 0, unit: 'Gi' }
  const s = String(raw).trim()
  const m = s.match(/^(\d+(?:\.\d+)?)(?:\s*)(Ki|Mi|Gi|Ti|Pi)$/i)
  if (!m) return { val: Number(s.replace(/[^\d.]/g, '')) || 0, unit: 'Gi' }
  const val = Number(m[1])
  const unitRaw = m[2]
  const unit = (unitRaw[0].toUpperCase() + unitRaw.slice(1).toLowerCase()) as CapUnit
  return { val, unit: CAP_UNITS.includes(unit) ? unit : 'Gi' }
}

function formatCapacity(raw?: string): string {
  const { val, unit } = parseQuantityWithUnit(raw)
  // Convert to a larger human-readable unit (base 1024)
  const order: CapUnit[] = ['Ki', 'Mi', 'Gi', 'Ti', 'Pi']
  let bytes = val
  let idx = order.indexOf(unit)
  // First convert to Ki
  while (idx > 0) {
    bytes *= 1024
    idx--
  }
  // Automatically pick the best-fit unit
  let i = 0
  let display = bytes
  while (display >= 1024 && i < order.length - 1) {
    display /= 1024
    i++
  }
  const num = display >= 10 ? Math.round(display) : Math.round(display * 10) / 10
  return `${num}${order[i]}`
}
</script>
