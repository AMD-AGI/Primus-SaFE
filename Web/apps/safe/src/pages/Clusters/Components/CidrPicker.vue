<template>
  <div class="cidr-picker">
    <!-- First octet: only 10 / 172 / 192 -->
    <el-select v-model="first" class="w-20" :teleported="false">
      <el-option v-for="n in FIRSTS" :key="n" :label="n" :value="n" />
    </el-select>

    <span class="sep">.</span>

    <!-- Second octet: range limited by first; 192 forces 168 and disables -->
    <template v-if="first === 192">
      <el-input class="w-20" :model-value="168" disabled />
    </template>
    <template v-else>
      <el-tooltip :content="secondTip" trigger="click" placement="top" effect="light">
        <el-input-number
          v-model="second"
          class="w-24"
          :min="secondMin"
          :max="secondMax"
          :step="1"
          controls-position="right"
        />
      </el-tooltip>
    </template>

    <span class="sep">.</span>

    <!-- Third octet -->
    <!-- <template v-if="first === 192">
      <el-input class="w-20" :model-value="168" />
    </template> -->
    <!-- <template> -->
    <el-input-number
      v-model="state.third"
      class="w-24"
      :min="0"
      :max="255"
      :step="1"
      controls-position="right"
    />
    <!-- </template> -->

    <span class="sep">.</span>

    <!-- Fourth octet -->
    <el-input-number
      v-model="state.fourth"
      :min="0"
      :max="255"
      :step="1"
      controls-position="right"
      class="w-20"
    />

    <span class="sep">/</span>

    <!-- Prefix: selectable range based on first octet -->
    <el-select v-model="prefix" class="w-18" :teleported="false">
      <el-option v-for="p in prefixOptions" :key="p" :label="p" :value="p" />
    </el-select>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, watchEffect } from 'vue'

/** Supported private network ranges */
const FIRSTS = [10, 172, 192] as const

const props = defineProps<{ modelValue?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

/** Parse incoming "a.b.c.d/p" (fallback to 10.0.0.0/16 if invalid) */
const state = reactive({
  first: 10,
  second: 0,
  third: 0,
  fourth: 0,
  prefix: 16,
})

function parse(v?: string) {
  const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)\/(\d+)$/.exec(v ?? '')
  if (!m) return
  const [a, b, c, d, p] = m.slice(1).map((n) => Number(n))
  if (!FIRSTS.includes(a as any)) return
  state.first = a as (typeof FIRSTS)[number]
  state.second = b
  state.third = c
  state.fourth = d
  state.prefix = p
}

// Initialize: parse from incoming value
parse(props.modelValue)

/** Range rules for second octet */
const secondMin = computed(() => (state.first === 172 ? 16 : 0))
const secondMax = computed(() => (state.first === 172 ? 31 : 255))
const secondTip = computed(() => {
  if (state.first === 10) return 'range: 0–255'
  if (state.first === 172) return 'range: 16–31'
  return '168'
})

/** Selectable rules for prefix */
const prefixOptions = computed(() => {
  // 10.* -> 12..24; others (172.* / 192.168.*) -> 16..24
  // const start = state.first === 10 ? 12 : 16
  const start = 12
  return Array.from({ length: 24 - start + 1 }, (_, i) => start + i)
})

/** Force second=168 for 192; limit 16..31 for 172; 0..255 for 10 */
watchEffect(() => {
  if (state.first === 192) state.second = 168
  else {
    if (state.second < secondMin.value) state.second = secondMin.value
    if (state.second > secondMax.value) state.second = secondMax.value
  }
  // Clamp prefix to valid range
  const pmin = prefixOptions.value[0]
  const pmax = prefixOptions.value[prefixOptions.value.length - 1]
  if (state.prefix < pmin) state.prefix = pmin
  if (state.prefix > pmax) state.prefix = pmax

  // Third octet default 0; editable
  state.third = Math.min(255, Math.max(0, state.third))
  // Third octet default 0; not editable
  // state.fourth = 0
  // emit
  const cidr = `${state.first}.${state.second}.${state.third}.${state.fourth}/${state.prefix}`
  emit('update:modelValue', cidr)
})

const first = computed({
  get: () => state.first,
  set: (v: number) => (state.first = v as any),
})
const second = computed({
  get: () => state.second,
  set: (v: number) => (state.second = Number(v ?? 0)),
})
// const third = computed({
//   get: () => state.third,
//   set: (v: number) => (state.third = Number(v ?? 0)),
// })
const prefix = computed({
  get: () => state.prefix,
  set: (v: number) => (state.prefix = v),
})
</script>

<style scoped>
.cidr-picker {
  display: inline-flex;
  align-items: center;
}
.sep {
  margin: 0 6px;
  opacity: 0.8;
}
.w-18 {
  width: 72px;
} /* Narrow width shorthand only */
.w-20 {
  width: 80px;
}
.w-24 {
  width: 96px;
}
</style>
