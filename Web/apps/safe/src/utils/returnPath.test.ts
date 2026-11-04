import { describe, expect, it } from 'vitest'
import { getSafeRedirect } from '@/utils'
import { OUTSIDE_CONSOLE_PREFIXES, outsideConsoleReturn } from './returnPath'

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
