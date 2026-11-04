import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getSafeRedirect } from '@/utils'
import {
  leaveConsoleFor,
  OUTSIDE_CONSOLE_PREFIXES,
  outsideConsoleReturn,
  REPEAT_RETURN_WINDOW_MS,
} from './returnPath'

const SELF = 'https://console.example.com'

describe('outsideConsoleReturn', () => {
  it('publishes exactly the agreed prefixes', () => {
    expect([...OUTSIDE_CONSOLE_PREFIXES]).toEqual(['/dispatron', '/autopilot', '/binding'])
  })

  it.each([
    ['/dispatron', `${SELF}/dispatron`],
    ['/dispatron/', `${SELF}/dispatron/`],
    ['/dispatron/runs/42?tab=log#tail', `${SELF}/dispatron/runs/42?tab=log#tail`],
    ['/autopilot/', `${SELF}/autopilot/`],
    ['/autopilot/prs?state=open&next=%2Fx', `${SELF}/autopilot/prs?state=open&next=%2Fx`],
    ['/binding/sign-in', `${SELF}/binding/sign-in`],
  ])('returns %s as a full-page target', (value, want) => {
    expect(outsideConsoleReturn(value, SELF)).toBe(want)
  })

  it('reads the first value of a repeated query parameter', () => {
    expect(outsideConsoleReturn(['/autopilot/', '/workloads'], SELF)).toBe(`${SELF}/autopilot/`)
    expect(outsideConsoleReturn(['//evil.example.net', '/autopilot/'], SELF)).toBeNull()
  })

  it.each([
    // console routes stay with the router
    '/',
    '/workloads',
    '/login?redirect=/dispatron',
    // not a path at all
    '',
    'dispatron',
    'https://console.example.com/dispatron',
    'http://evil.example.net/dispatron',
    'https://evil.example.net/dispatron',
    'javascript:alert(1)',
    'data:text/html,x',
    // protocol-relative and what the URL parser folds into it
    '//evil.example.net/dispatron',
    '///evil.example.net/dispatron',
    '/\\evil.example.net/dispatron',
    '/\\/evil.example.net',
    '\\\\evil.example.net/dispatron',
    '/\t/evil.example.net/dispatron',
    '/\n/evil.example.net/dispatron',
    '/ /evil.example.net',
    '/dispatron\\..\\..\\login',
    // prefix look-alikes and escapes from the prefix
    '/dispatronx',
    '/dispatron.evil',
    '/autopilot-admin/',
    '/Dispatron/',
    '/dispatron/../login',
    '/dispatron/./runs',
    '/dispatron/%2e%2e/login',
    '/dispatron/a/%2e%2e',
    '/dispatron/a/%2E%2E/',
    '/autopilot/a/.%2e/',
    '/autopilot/x%2Ey',
    '/dispatron/%2E%2E/%2E%2E/x',
    '/dispatron/%2f%2fevil.example.net',
    '/dispatron%2F..%2Flogin',
    '/dispatron/%5cevil.example.net',
    '/%2F%2Fevil.example.net',
    '/%5C%5Cevil.example.net',
    '/binding@evil.example.net',
  ])('refuses %j', (value) => {
    expect(outsideConsoleReturn(value, SELF)).toBeNull()
  })

  it.each([undefined, null, 42, {}, []])('refuses a non-string %j', (value) => {
    expect(outsideConsoleReturn(value, SELF)).toBeNull()
  })

  it('never returns another origin, whatever the input', () => {
    const inputs = ['/dispatron', '/autopilot/x?y=1', '/binding/', '/\\x', '//x', '/dispatron/..']
    for (const v of inputs) {
      const got = outsideConsoleReturn(v, SELF)
      if (got !== null) expect(new URL(got).origin).toBe(SELF)
    }
  })
})

describe('getSafeRedirect', () => {
  it.each(['/', '/workloads', '/workloads?x=1#y', '/dispatron/runs'])('keeps %s', (v) => {
    expect(getSafeRedirect(v)).toBe(v)
  })

  it.each([
    '',
    'workloads',
    'https://evil.example.net',
    'http://evil.example.net',
    '//evil.example.net',
    '/\\evil.example.net',
    '/\\/evil.example.net',
    '/\t/evil.example.net',
    '/\n/evil.example.net',
    '/\u0000x',
    undefined,
    null,
    42,
  ])('sends %j to /', (v) => {
    expect(getSafeRedirect(v)).toBe('/')
  })
})

describe('leaveConsoleFor', () => {
  const A = `${SELF}/dispatron/runs`
  const B = `${SELF}/autopilot/`
  let store: Map<string, string>
  let replace: ReturnType<typeof vi.fn>

  beforeEach(() => {
    store = new Map()
    replace = vi.fn()
    vi.stubGlobal('window', { location: { replace } })
    vi.stubGlobal('sessionStorage', {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
    })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('returns the first time', () => {
    expect(leaveConsoleFor(A, 1_000)).toBe(true)
    expect(replace).toHaveBeenCalledWith(A)
  })

  it('does not return to the same page again within the window', () => {
    leaveConsoleFor(A, 1_000)
    replace.mockClear()
    expect(leaveConsoleFor(A, 1_000 + REPEAT_RETURN_WINDOW_MS - 1)).toBe(false)
    expect(replace).not.toHaveBeenCalled()
  })

  it('returns again once the window has passed', () => {
    leaveConsoleFor(A, 1_000)
    expect(leaveConsoleFor(A, 1_000 + REPEAT_RETURN_WINDOW_MS)).toBe(true)
    expect(replace).toHaveBeenCalledTimes(2)
  })

  it('a refused repeat does not extend the window', () => {
    leaveConsoleFor(A, 1_000)
    expect(leaveConsoleFor(A, 2_000)).toBe(false)
    expect(leaveConsoleFor(A, 1_000 + REPEAT_RETURN_WINDOW_MS)).toBe(true)
  })

  it('another page is not a repeat', () => {
    leaveConsoleFor(A, 1_000)
    expect(leaveConsoleFor(B, 1_001)).toBe(true)
    expect(replace).toHaveBeenLastCalledWith(B)
  })

  it.each(['not json', '{"href":"x"}', 'null', `{"href":"${A}","at":"1000"}`])(
    'a saved record %j does not block',
    (saved) => {
      store.set('console.outsideReturn', saved)
      expect(leaveConsoleFor(A, 1_000)).toBe(true)
    },
  )

  it('a record from the future (clock set back) does not block', () => {
    leaveConsoleFor(A, 50_000)
    expect(leaveConsoleFor(A, 40_000)).toBe(true)
  })

  it('still returns when sessionStorage throws', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
    })
    expect(leaveConsoleFor(A, 1_000)).toBe(true)
    expect(replace).toHaveBeenCalledWith(A)
  })
})
