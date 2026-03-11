export {}

declare global {
  type SelectOption<T = string | number> = {
    label: string
    value: T
  }
}