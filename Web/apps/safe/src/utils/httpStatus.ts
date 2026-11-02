import axios from 'axios'

/** The HTTP status of a failed request, or undefined when the failure is not an axios error with a response. */
export const statusOf = (e: unknown): number | undefined =>
  axios.isAxiosError(e) ? e.response?.status : undefined

/**
 * The error code the apiserver puts in the body of the 403 that means "this user has no access
 * yet" (a user who holds only the first-login role). Other 403s carry other codes, or none.
 */
export const NO_ACCESS_ERROR_CODE = 'Primus.00051'

/**
 * Whether a failed request was refused with "no access yet": an axios error whose response is a
 * 403 carrying NO_ACCESS_ERROR_CODE in `errorCode`. A request sent quietly (skipErrorHandler)
 * reaches the caller as this raw axios error. Any other failure, including a 403 without the
 * code or with a body of another shape, is false.
 */
export const isNoAccess = (e: unknown): boolean => {
  if (!axios.isAxiosError(e) || e.response?.status !== 403) return false
  const body: unknown = e.response.data
  return (
    typeof body === 'object' &&
    body !== null &&
    (body as { errorCode?: unknown }).errorCode === NO_ACCESS_ERROR_CODE
  )
}
