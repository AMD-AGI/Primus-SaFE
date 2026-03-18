import { ref, reactive, onMounted, shallowRef } from 'vue'

interface Pagination {
  pageNum: number
  pageSize: number
  total: number
}

type PageRes = {
  pageNum: number
  pageSize: number
  [key: string]: any
}

type FetchFunction<T, E extends any[]> = (
  params: PageRes,
  ...extra: E
) => Promise<{ data: T[]; total: number }>

export function usePaginatedTable<T, E extends any[]>(
  fetchFn: FetchFunction<T, E>,
  extraParams?: E,
  initialFilters: Record<string, any> = {},
  transformFilters?: (filters: Record<string, any>) => Record<string, any>
) {
  const tableData = shallowRef<T[]>([])
  const loading = ref(false)
  const filters = reactive({ ...initialFilters })

  const pagination = reactive<Pagination>({
    pageNum: 1,
    pageSize: 10,
    total: 0
  })

  const fetchData = async () => {
    loading.value = true
    try {
      const rawFilters = transformFilters ? transformFilters(filters) : filters
      const res = await fetchFn(
        {
          pageNum: pagination.pageNum,
          pageSize: pagination.pageSize,
          ...rawFilters
        },
        ...(extraParams || [])
      )
      tableData.value = res.data
      pagination.total = res.total
    } catch (e) {
      console.error('fetchData error:', e)
    } finally {
      loading.value = false
    }
  }

  const resetFilters = () => {
    for (const key in initialFilters) {
      const val = initialFilters[key]
      filters[key] = Array.isArray(val)
        ? []
        : typeof val === 'object' && val !== null
        ? { ...val }
        : val
    }
    pagination.pageNum = 1
    fetchData()
  }

  onMounted(fetchData)

  return {
    tableData,
    loading,
    pagination,
    filters,
    fetchData,
    resetFilters
  }
}
