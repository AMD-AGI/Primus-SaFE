import type { Router } from 'vue-router'

/**
 * Build an absolute URL from a router location.
 * We prefer `router.resolve` to avoid relying on currentRoute.path.
 */
function buildAbsoluteUrl(
  router: Router,
  location: { origin: string },
  to: Parameters<Router['resolve']>[0],
) {
  const href = router.resolve(to).href // e.g. "/chatbot?x=1"
  return `${location.origin}${href}`
}

export function buildMessageShareUrl(
  router: Router,
  questionMessageId: number,
  answerMessageId: number,
): string {
  return buildAbsoluteUrl(router, window.location, {
    name: 'ChatbotFullPage',
    query: {
      share: '1',
      qid: String(questionMessageId),
      aid: String(answerMessageId),
    },
  })
}

export function buildConversationShareUrl(router: Router, conversationId: string): string {
  return buildAbsoluteUrl(router, window.location, {
    name: 'ChatbotFullPage',
    query: {
      share: 'conv',
      cid: conversationId,
    },
  })
}
