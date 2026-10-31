import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { describe, expect, it } from 'vitest'
import { NO_ACCESS_ERROR_CODE, isNoAccess, statusOf } from './httpStatus'

// An error as axios raises it for a response with this status and body.
const httpError = (status: number, data?: unknown) =>
  new AxiosError('failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    statusText: '',
    data,
    headers: {},
    config: {} as InternalAxiosRequestConfig,
  })

describe('NO_ACCESS_ERROR_CODE', () => {
  // The apiserver's contract; the tests elsewhere write the code out on purpose.
  it('is the code the apiserver sends for "no access yet"', () => {
    expect(NO_ACCESS_ERROR_CODE).toBe('Primus.00051')
  })
})

describe('isNoAccess', () => {
  it.each([
    [
      '403 with the no-access code',
      httpError(403, { errorCode: 'Primus.00051', errorMessage: 'The user has no access yet' }),
    ],
    ['403 with the code and no message', httpError(403, { errorCode: 'Primus.00051' })],
  ])('is true for a %s', (_label, error) => {
    expect(isNoAccess(error)).toBe(true)
  })

  it.each([
    [
      '403 with another code',
      httpError(403, { errorCode: 'Primus.00003', errorMessage: 'denied' }),
    ],
    ['403 with an empty object body', httpError(403, {})],
    ['403 with no body', httpError(403)],
    ['403 with a null body', httpError(403, null)],
    ['403 with a string body', httpError(403, 'Primus.00051')],
    ['403 with an HTML body', httpError(403, '<html>Forbidden</html>')],
    ['403 with an array body', httpError(403, [{ errorCode: 'Primus.00051' }])],
    ['403 with a numeric code', httpError(403, { errorCode: 51 })],
    ['403 with the code under another key', httpError(403, { code: 'Primus.00051' })],
    ['403 with the code nested', httpError(403, { error: { errorCode: 'Primus.00051' } })],
    ['401 with the code', httpError(401, { errorCode: 'Primus.00051' })],
    ['500 with the code', httpError(500, { errorCode: 'Primus.00051' })],
    ['a network error (no response)', new AxiosError('Network Error', 'ERR_NETWORK')],
    ['a plain Error', new Error('Primus.00051')],
    ['a string', 'Primus.00051'],
    [
      'a plain object that looks like one',
      { response: { status: 403, data: { errorCode: 'Primus.00051' } } },
    ],
    ['null', null],
    ['undefined', undefined],
  ])('is false for %s', (_label, error) => {
    expect(isNoAccess(error)).toBe(false)
  })
})

describe('statusOf', () => {
  it('reads the status of an axios error with a response', () => {
    expect(statusOf(httpError(403))).toBe(403)
  })

  it.each([
    ['a network error', new AxiosError('Network Error', 'ERR_NETWORK')],
    ['a string', 'Request failed.'],
    ['undefined', undefined],
  ])('is undefined for %s', (_label, error) => {
    expect(statusOf(error)).toBeUndefined()
  })
})
