/**
 * Composable: page-level guided tour (driver.js)
 *
 * Usage:
 *   const { startTour, getDriver } = usePageTour((tourId) => {
 *     switch (tourId) {
 *       case 'create': return createSteps
 *       case 'connect': return connectSteps
 *       default: return defaultSteps
 *     }
 *   })
 *
 * When navigated to with `?tour=<id>`, the matching tour auto-starts on mount.
 * The composable exposes `getDriver()` so page code can call `driver.moveNext()`
 * inside custom `onNextClick` handlers (e.g. to open a dialog first).
 */
import { onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { driver, type DriveStep, type Driver } from 'driver.js'
import 'driver.js/dist/driver.css'

/** Wait for a CSS-selector to appear in the DOM (returns null on timeout). */
export function waitForEl(selector: string, timeout = 5000): Promise<Element | null> {
  return new Promise((resolve) => {
    const hit = document.querySelector(selector)
    if (hit) return resolve(hit)
    const obs = new MutationObserver(() => {
      const el = document.querySelector(selector)
      if (el) {
        obs.disconnect()
        resolve(el)
      }
    })
    obs.observe(document.body, { childList: true, subtree: true })
    setTimeout(() => {
      obs.disconnect()
      resolve(null)
    }, timeout)
  })
}

/**
 * @param getSteps - receives the tour ID from `?tour=<id>` and returns
 *                   the matching DriveStep[]. May also perform side-effects
 *                   such as switching a tab before returning.
 */
export function usePageTour(getSteps: (tourId: string) => DriveStep[]) {
  const route = useRoute()
  const router = useRouter()
  let d: Driver | null = null

  async function startTour(tourId: string) {
    const steps = getSteps(tourId)
    if (!steps.length) return

    // Allow side-effects inside getSteps (e.g. tab switch) to render
    await nextTick()
    await new Promise((r) => setTimeout(r, 120))

    d = driver({
      steps,
      allowClose: true,
      showProgress: true,
      overlayColor: 'rgba(0,0,0,.55)',
      stagePadding: 6,
      nextBtnText: 'Next',
      prevBtnText: 'Previous',
      doneBtnText: 'Done',
      onDestroyed: () => {
        // Strip ?tour from the URL so refreshing won't re-trigger
        if (route.query.tour) {
          const q = { ...route.query }
          delete q.tour
          router.replace({ query: q })
        }
      },
    })
    d.drive(0)
  }

  /** Access the driver instance (e.g. to call moveNext() in async hooks). */
  function getDriver() {
    return d
  }

  onMounted(async () => {
    const tourId = route.query.tour as string
    if (tourId) {
      await nextTick()
      // Small delay to let DOM settle (tables, buttons, etc.)
      setTimeout(() => startTour(tourId), 350)
    }
  })

  return { startTour, getDriver }
}
