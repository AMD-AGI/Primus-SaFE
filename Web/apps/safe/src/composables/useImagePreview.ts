import { ref } from 'vue'

export const useImagePreview = () => {
  const imagePreviewVisible = ref(false)
  const imagePreviewUrl = ref('')

  const openImagePreview = (url: string) => {
    if (!url) return
    imagePreviewUrl.value = url
    imagePreviewVisible.value = true
  }

  const closeImagePreview = () => {
    imagePreviewVisible.value = false
    imagePreviewUrl.value = ''
  }

  const handleImageClick = (event: MouseEvent) => {
    const target = event.target
    if (!(target instanceof HTMLElement)) return
    const img = target.closest('img') as HTMLImageElement | null
    if (!img?.src) return
    openImagePreview(img.src)
  }

  return {
    imagePreviewVisible,
    imagePreviewUrl,
    openImagePreview,
    closeImagePreview,
    handleImageClick,
  }
}
