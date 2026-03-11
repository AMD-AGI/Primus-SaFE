import { ref, computed, type Ref } from 'vue'

const toJsDate = (x: any): Date | null =>
  x ? (x instanceof Date ? x : typeof x?.toDate === 'function' ? x.toDate() : null) : null

const pickPanelDate = (args: any[]): Date | null => {
  for (let i = args.length - 1; i >= 0; i--) {
    const d = toJsDate(args[i])
    if (d) return d
  }
  return null
}

const isSameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() &&
  a.getMonth() === b.getMonth() &&
  a.getDate() === b.getDate()

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate())
const addYears = (d: Date, y = 1) => {
  const x = new Date(d)
  x.setFullYear(x.getFullYear() + y)
  return x
}

export function useDatetimeLimit(model: Ref<string | Date | null>, years = 1) {
  const disabledDate = (d: Date) => {
    const now = new Date()
    const minDay = startOfDay(now)
    const maxDay = startOfDay(addYears(now, years))
    return d < minDay || d > maxDay
  }

  const disabledHours = (...args: any[]) => {
    const panel = pickPanelDate(args)
    const v = panel ?? (model.value ? new Date(model.value as any) : null)
    if (!v) return []
    const now = new Date()
    const max = addYears(now, years)

    const ret: number[] = []
    if (isSameDay(v, now)) for (let h = 0; h < now.getHours(); h++) ret.push(h) // Today: disable hours before now
    if (isSameDay(v, max)) for (let h = max.getHours() + 1; h <= 23; h++) ret.push(h) // Day one year from now: disable hours beyond max
    return Array.from(new Set(ret)).sort((a, b) => a - b)
  }

  const disabledMinutes = (hour: number, ...args: any[]) => {
    const panel = pickPanelDate(args)
    const v = panel ?? (model.value ? new Date(model.value as any) : null)
    if (!v) return []
    const now = new Date()
    const max = addYears(now, years)

    const ret: number[] = []
    if (isSameDay(v, now) && hour === now.getHours())
      for (let m = 0; m < now.getMinutes(); m++) ret.push(m) // Today current hour: disable minutes before now
    if (isSameDay(v, max) && hour === max.getHours())
      for (let m = max.getMinutes() + 1; m <= 59; m++) ret.push(m) // Max hour on the day one year from now: disable minutes beyond max
    return ret
  }

  const disabledSeconds = (hour: number, minute: number, ...args: any[]) => {
    const panel = pickPanelDate(args)
    const v = panel ?? (model.value ? new Date(model.value as any) : null)
    if (!v) return []
    const now = new Date()
    const max = addYears(now, years)

    const ret: number[] = []
    if (isSameDay(v, now) && hour === now.getHours() && minute === now.getMinutes())
      for (let s = 0; s < now.getSeconds(); s++) ret.push(s) // Today current hour and minute: disable seconds before now
    if (isSameDay(v, max) && hour === max.getHours() && minute === max.getMinutes())
      for (let s = max.getSeconds() + 1; s <= 59; s++) ret.push(s) // Max hour on the day one year from now: disable seconds beyond max
    return ret
  }

  const normalizeTodayToNow = () => {
    const v = model.value ? new Date(model.value as any) : null
    if (!v) return
    const now = new Date()
    if (isSameDay(v, now) && v.getHours() === 0 && v.getMinutes() === 0 && v.getSeconds() === 0) {
      model.value = now as any
    }
  }

  return {
    disabledDate,
    disabledHours,
    disabledMinutes,
    disabledSeconds,
    normalizeTodayToNow,
  }
}
