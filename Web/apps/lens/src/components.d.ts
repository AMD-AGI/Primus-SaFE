/* eslint-disable */
export {}

/* prettier-ignore */
declare module 'vue' {
  export interface GlobalComponents {
    BaseHeader: typeof import('./components/layout/BaseHeader.vue')['default']
    ElButton: typeof import('element-plus/es')['ElButton']
    ElConfigProvider: typeof import('element-plus/es')['ElConfigProvider']
    ElDatePicker: typeof import('element-plus/es')['ElDatePicker']
    ElIcon: typeof import('element-plus/es')['ElIcon']
    ElInput: typeof import('element-plus/es')['ElInput']
    ElMenu: typeof import('element-plus/es')['ElMenu']
    ElMenuItem: typeof import('element-plus/es')['ElMenuItem']
    ElMenuItemGroup: typeof import('element-plus/es')['ElMenuItemGroup']
    ElSubMenu: typeof import('element-plus/es')['ElSubMenu']
    ElSwitch: typeof import('element-plus/es')['ElSwitch']
    ElTag: typeof import('element-plus/es')['ElTag']
    // RouterLink: typeof import('vue-router')['RouterLink']
    // RouterView: typeof import('vue-router')['RouterView']
  }
}
