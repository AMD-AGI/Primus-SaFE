import { useRoute, useRouter } from 'vue-router'
import type { LocationQuery } from 'vue-router'
import { useUserStore } from '@/stores/user'

export type WorkloadScope = 'All' | 'My Workloads'

interface WorkloadListSearchParams {
  onlyMyself: string
  userId: string
  [key: string]: unknown
}

interface WorkloadListPagination {
  page: number
  pageSize: number
  [key: string]: unknown
}

interface WorkloadListQueryConfig {
  searchParams: WorkloadListSearchParams
  pagination: WorkloadListPagination
  /** Scope shown when the URL carries no explicit scope. Defaults to "My Workloads". */
  defaultScope?: WorkloadScope
  /** Serialize page-specific filter fields into flat query values. Empty values are omitted. */
  serializeFilters: () => Record<string, string | undefined>
  /** Hydrate page-specific filter fields from the URL query. */
  parseFilters: (query: LocationQuery) => void
}

/**
 * Centralizes the URL <-> state handling for workload list pages so that the
 * URL query is the single source of truth.
 *
 * It fixes three recurring problems:
 * - `userId` is always derived from the scope toggle and never stored in the
 *   URL, removing the coupled/redundant param that could drift out of sync.
 * - `writeQuery` builds a fresh query instead of spreading the previous one, so
 *   cleared filters never linger across navigations.
 * - the scope default is configurable and applied consistently on read.
 */
export function useWorkloadListQuery(config: WorkloadListQueryConfig) {
  const route = useRoute()
  const router = useRouter()
  const userStore = useUserStore()
  const { searchParams, pagination } = config
  const defaultScope: WorkloadScope = config.defaultScope ?? 'My Workloads'

  // userId is always derived from the scope toggle; it is never stored in the URL.
  const syncUserId = () => {
    searchParams.userId = searchParams.onlyMyself === 'All' ? '' : userStore.userId
  }

  // The URL query is the single source of truth: hydrate every param from it.
  const readQuery = () => {
    const q = route.query
    searchParams.onlyMyself = (q.onlyMyself as string) || defaultScope
    config.parseFilters(q)
    pagination.page = Number(q.page || 1)
    pagination.pageSize = Number(q.pageSize || pagination.pageSize)
    syncUserId()
  }

  // Write a fresh, minimal query (no stale carryover, no redundant userId).
  const writeQuery = () => {
    const query: Record<string, string> = {}
    for (const [key, value] of Object.entries(config.serializeFilters())) {
      if (value !== undefined && value !== null && value !== '') {
        query[key] = value
      }
    }
    // Only persist the scope toggle when it differs from the page default.
    if (searchParams.onlyMyself !== defaultScope) {
      query.onlyMyself = searchParams.onlyMyself
    }
    query.page = String(pagination.page)
    query.pageSize = String(pagination.pageSize)
    router.replace({ query })
  }

  return { readQuery, writeQuery, syncUserId, defaultScope }
}
