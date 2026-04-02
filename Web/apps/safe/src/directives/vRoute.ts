import type { Directive } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import router from '@/router'

const ROUTE_KEY = Symbol('v-route')

interface RouteEl extends HTMLElement {
  [ROUTE_KEY]?: RouteLocationRaw
}

function syncHref(el: RouteEl, to: RouteLocationRaw) {
  el[ROUTE_KEY] = to
  const anchor = el.tagName === 'A' ? el : el.querySelector('a')
  if (anchor) anchor.setAttribute('href', router.resolve(to).href)
}

/**
 * v-route directive: add SPA-aware navigation to any el-link / anchor.
 *
 * - Normal click  → router.push (SPA navigation)
 * - Ctrl / Cmd click → browser opens href in new tab
 * - Right-click "Open in new tab" → works via real href
 *
 * Usage:  <el-link v-route="{ path: '/foo', query: { id } }">text</el-link>
 */
const vRoute: Directive<RouteEl, RouteLocationRaw | undefined> = {
  mounted(el, { value }) {
    if (!value) return
    syncHref(el, value)
    el.addEventListener('click', (e: MouseEvent) => {
      const to = el[ROUTE_KEY]
      if (!to || e.ctrlKey || e.metaKey) return
      e.preventDefault()
      router.push(to)
    })
  },
  updated(el, { value }) {
    if (value) syncHref(el, value)
  },
}

export default vRoute
