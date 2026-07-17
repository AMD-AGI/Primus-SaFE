import DOMPurify from 'dompurify'
import { marked } from 'marked'

/**
 * Render UNTRUSTED markdown (LLM output, backend reports, chat messages)
 * into sanitized HTML safe for v-html.
 */
export function renderMarkdown(raw: string | null | undefined): string {
  if (!raw) return ''
  const html = marked.parse(raw, { breaks: true }) as string
  return DOMPurify.sanitize(html)
}

/** Sanitize a raw HTML string (already-HTML, no markdown step). */
export function sanitizeHtml(raw: string | null | undefined): string {
  if (!raw) return ''
  return DOMPurify.sanitize(raw)
}

/**
 * Escape plain text, then convert newlines to <br>.
 * Safe replacement for the pattern `value.replace(/\n/g, '<br>')`.
 */
export function textToBr(raw: string | null | undefined): string {
  if (!raw) return ''
  const escaped = raw
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  return escaped.replace(/\n/g, '<br>')
}
