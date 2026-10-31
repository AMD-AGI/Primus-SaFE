import axios from 'axios'

/** The HTTP status of a failed request, or undefined when the failure is not an axios error with a response. */
export const statusOf = (e: unknown): number | undefined =>
  axios.isAxiosError(e) ? e.response?.status : undefined
