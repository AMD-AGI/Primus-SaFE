import { ref } from 'vue'
import { getWorkspace, getClusters } from '@/services/base'
import { getNodeFlavors, getSecrets } from '@/services/nodes'
import type { OptionsLoaderName, WizardFieldOption } from '../constants/guidedWorkflows'

type LoaderFn = () => Promise<WizardFieldOption[]>

const loaders: Record<OptionsLoaderName, LoaderFn> = {
  async workspaces() {
    const res = await getWorkspace()
    const items: any[] = res?.items ?? res ?? []
    return items.map((w) => ({
      label: w.workspaceName ?? w.name ?? w.workspaceId,
      value: w.workspaceName ?? w.workspaceId,
    }))
  },

  async clusters() {
    const res = await getClusters()
    const items = res?.items ?? []
    return items.map((c) => ({
      label: c.clusterId,
      value: c.clusterId,
    }))
  },

  async flavors() {
    const res = await getNodeFlavors()
    const items = res?.items ?? []
    return items.map((f: any) => ({
      label: f.name ?? f.flavorId,
      value: f.flavorId,
    }))
  },

  async secrets_ssh() {
    const res = await getSecrets({ type: 'ssh' })
    const items: any[] = res?.items ?? res ?? []
    return items.map((s: any) => ({
      label: s.name ?? s.secretId,
      value: s.secretId ?? s.name,
    }))
  },

  async secrets_image() {
    const res = await getSecrets({ type: 'image' })
    const items: any[] = res?.items ?? res ?? []
    return items.map((s: any) => ({
      label: s.name ?? s.secretId,
      value: s.secretId ?? s.name,
    }))
  },
}

const optionsCache = new Map<OptionsLoaderName, WizardFieldOption[]>()

export function useWizardOptions(loaderName?: OptionsLoaderName) {
  const options = ref<WizardFieldOption[]>([])
  const loading = ref(false)

  async function load() {
    if (!loaderName) return
    if (optionsCache.has(loaderName)) {
      options.value = optionsCache.get(loaderName)!
      return
    }
    loading.value = true
    try {
      const result = await loaders[loaderName]()
      options.value = result
      optionsCache.set(loaderName, result)
    } catch (e) {
      console.warn(`[useWizardOptions] Failed to load "${loaderName}":`, e)
      options.value = []
    } finally {
      loading.value = false
    }
  }

  if (loaderName) load()

  return { options, loading, reload: load }
}

export function clearWizardOptionsCache(name?: OptionsLoaderName) {
  if (name) {
    optionsCache.delete(name)
  } else {
    optionsCache.clear()
  }
}
