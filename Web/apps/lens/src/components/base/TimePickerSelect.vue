<template>
   <el-select v-model="modelValueProxy" placeholder="Select" style="width: 240px" size="large">
    <el-option
        v-for="item in optionsList"
        :key="item.value"
        :label="item.label"
        :value="item.value"
    />
  </el-select>
</template>

<script setup lang="ts">
import {ref, defineProps, computed, defineEmits} from 'vue'
import {TIME_RANGE_OPTIONS} from '@/constants'

const props = defineProps<{
    options?: {label: string, value: string}[],
    modelValue: string,
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const optionsList = computed(() => props.options || TIME_RANGE_OPTIONS )

const modelValueProxy = computed({
  get: () => props.modelValue,
  set: (val: string) => emit('update:modelValue', val),
})

</script>

<style>

</style>