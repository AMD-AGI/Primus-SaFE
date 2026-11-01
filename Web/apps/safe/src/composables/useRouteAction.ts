import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'

/** One-shot `?action=` values a list page understands. */
export const ROUTE_ACTIONS = {
  CREATE: 'create',
} as const

/**
 * Consume a one-shot `?action=xxx` route query on mount.
 * Opens the create dialog and immediately strips the query to prevent re-triggering on refresh.
 *
 * @param handlers - map of action values to callbacks (e.g. `{ create: () => openDialog() }`)
 */
export function useRouteAction(handlers: Partial<Record<string, () => void>>) {
  const route = useRoute()
  const router = useRouter()

  onMounted(() => {
    const action = route.query.action as string | undefined
    if (!action) return

    const handler = handlers[action]
    if (handler) {
      handler()
    }

    const { action: _, ...rest } = route.query
    router.replace({ query: rest })
  })
}
