import { computed, ref } from 'vue'
import type { Router } from 'vue-router'
import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import { copyText } from '@/utils'
import { isNoAccess } from '@/utils/httpStatus'

/**
 * The logic of the "no access yet" page, kept apart from the template so it can be tested.
 *
 * The page shows what the signed-in user already holds in the user store (the record read at
 * start-up) and the contact line of GET /envs. It makes no request of its own for the user's data
 * and never reads other users: an account in this state may read nothing but its own record.
 * The only requests it can cause are the ones named below (envs when start-up did not get them,
 * clusters for "Check again", logout for "Sign out").
 */
export function useNoAccess(router: Pick<Router, 'replace'>) {
  const userStore = useUserStore()
  const clusterStore = useClusterStore()

  const name = computed(() => userStore.profile?.name ?? '')
  const email = computed(() => userStore.profile?.email ?? '')
  const id = computed(() => userStore.profile?.id ?? '')
  const rolesText = computed(() => (userStore.profile?.roles ?? []).join(', '))

  // Plain text only: the page interpolates it, never renders it as HTML.
  const contact = computed(() => {
    const value = userStore.envs?.accessContact
    return typeof value === 'string' ? value.trim() : ''
  })

  const checking = ref(false)
  const signingOut = ref(false)
  // True after "Check again" was answered with 403: the account still has no access.
  const stillNoAccess = ref(false)

  /**
   * /envs is read once at start-up and kept in the user store. Ask again only when that read
   * failed or has not happened; the contact line is optional, so a failure here is not an error.
   */
  async function loadContact() {
    if (userStore.envs) return
    try {
      await userStore.fetchEnvs(true)
    } catch {
      // No contact line.
    }
  }

  async function copyId() {
    if (id.value) await copyText(id.value)
  }

  /**
   * Ask the clusters route again. The store is overwritten by a success and cleared by a failure,
   * so nothing needs resetting first (a reset would also drop the saved cluster of a user who
   * does have access and merely opened this page from a bookmark).
   * - success: go home; the store is fetched now, so the guard does not ask a second time.
   * - refused with "no access yet": stay, and say so.
   * - anything else (server down, session over, a 403 that is not "no access yet"): go home too;
   *   the guard asks again and treats it as it always did (/error, or the sign-in page).
   */
  async function checkAgain() {
    if (checking.value) return
    checking.value = true
    stillNoAccess.value = false
    try {
      await clusterStore.fetchClusters(true)
    } catch (e) {
      if (isNoAccess(e)) {
        stillNoAccess.value = true
        return
      }
    } finally {
      checking.value = false
    }
    await router.replace('/')
  }

  /** The same path as the error page's "Switch Account": the user store's logout, then /login. */
  async function signOut() {
    if (signingOut.value) return
    signingOut.value = true
    try {
      await userStore.logout()
    } catch {
      // The store clears the session even when the request fails.
    } finally {
      signingOut.value = false
    }
    await router.replace('/login')
  }

  return {
    name,
    email,
    id,
    rolesText,
    contact,
    checking,
    signingOut,
    stillNoAccess,
    loadContact,
    copyId,
    checkAgain,
    signOut,
  }
}
