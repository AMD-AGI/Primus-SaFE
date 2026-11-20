import type { LowUtilizationMode, LowUtilizationPolicy } from '@/services'

export const LOW_UTIL_MAX_RULES = 5

// Low GPU utilization: durations are edited in minutes
export type LowUtilizationRuleFormEntry = {
  threshold: number | undefined
  window: number | undefined
}
export type LowUtilizationFormEntry = {
  mode: LowUtilizationMode
  // Loaded as a legacy single threshold and window, saved back that way while it has one rule
  legacy: boolean
  rules: LowUtilizationRuleFormEntry[]
  gracePeriod: number | undefined
  notifyBefore: number | undefined
  // Unset uses the shortest rule window
  cooldown: number | undefined
  dryRun: boolean
}

// Convert minutes (number) to Go duration string: 15 → "0h15m", 90 → "1h30m"
export const minutesToGoDuration = (minutes: number): string => {
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return `${h}h${m}m`
}

// Convert Go duration string to minutes: "0h15m0s" → 15, "1h30m0s" → 90
export const goDurationToMinutes = (dur: string): number | undefined => {
  if (!dur) return undefined
  const hMatch = dur.match(/(\d+)h/)
  const mMatch = dur.match(/(\d+)m/)
  const h = hMatch ? parseInt(hMatch[1], 10) : 0
  const m = mMatch ? parseInt(mMatch[1], 10) : 0
  return h * 60 + m || undefined
}

export const newLowUtilizationEntry = (): LowUtilizationFormEntry => ({
  mode: 'Average',
  legacy: false,
  rules: [{ threshold: 5, window: 60 }],
  gracePeriod: 30,
  notifyBefore: 30,
  cooldown: undefined,
  dryRun: true,
})

export const parseLowUtilization = (
  raw: Record<string, LowUtilizationPolicy>,
): Record<string, LowUtilizationFormEntry> => {
  const parsed: Record<string, LowUtilizationFormEntry> = {}
  for (const [scope, policy] of Object.entries(raw)) {
    const legacy = !policy.rules?.length
    // A threshold of 0 is omitted from the stored legacy policy
    const rules = legacy
      ? [{ threshold: policy.threshold ?? 0, window: policy.window ?? '' }]
      : policy.rules!
    parsed[scope] = {
      mode: policy.mode,
      legacy,
      rules: rules.map((rule) => ({
        threshold: rule.threshold ?? 0,
        window: goDurationToMinutes(rule.window),
      })),
      gracePeriod: goDurationToMinutes(policy.gracePeriod ?? ''),
      notifyBefore: goDurationToMinutes(policy.notifyBefore ?? ''),
      cooldown: policy.cooldown ? (goDurationToMinutes(policy.cooldown) ?? 0) : undefined,
      dryRun: !!policy.dryRun,
    }
  }
  return parsed
}

export const buildLowUtilization = (
  entries: Record<string, LowUtilizationFormEntry>,
  scopes: string[],
): Record<string, LowUtilizationPolicy> => {
  const policies: Record<string, LowUtilizationPolicy> = {}
  scopes.forEach((scope) => {
    const entry = entries[scope]
    const rules = (entry?.rules ?? [])
      .filter((rule) => !!rule.window)
      .map((rule) => ({
        threshold: rule.threshold ?? 0,
        window: minutesToGoDuration(rule.window!),
      }))
    if (!entry || !rules.length) return
    policies[scope] = {
      mode: entry.mode,
      ...(entry.legacy && rules.length === 1 ? rules[0] : { rules }),
      ...(entry.gracePeriod ? { gracePeriod: minutesToGoDuration(entry.gracePeriod) } : {}),
      ...(entry.notifyBefore ? { notifyBefore: minutesToGoDuration(entry.notifyBefore) } : {}),
      ...(typeof entry.cooldown === 'number'
        ? { cooldown: minutesToGoDuration(entry.cooldown) }
        : {}),
      dryRun: entry.dryRun,
    }
  })
  return policies
}
