import { describe, it, expect } from 'vitest'
import {
  fillMissingKeys,
  toSnakeCase,
  toCamelCase,
  parseRangeToMs,
  getStepByRangeKey,
  getStepByTimestampDiff,
  byte2Gi,
  formatBytes,
  fmtVal,
  convertKeyValueMapToList,
  convertListToKeyValueMap,
  fmtTs,
  toKVList,
  CAP_UNITS,
  parseQuantityWithUnit,
  encodeToBase64String,
  decodeFromBase64String,
  formatNodeInfo,
  calculateDefaultTime,
} from '../index'

// ---------------------------------------------------------------------------
// fillMissingKeys
// ---------------------------------------------------------------------------
describe('fillMissingKeys', () => {
  it('fills missing fields with template defaults', () => {
    const obj = { name: 'alice' } as any
    fillMissingKeys(obj, { name: '', age: 0 })
    expect(obj).toEqual({ name: 'alice', age: 0 })
  })

  it('does not overwrite existing fields', () => {
    const obj = { a: 1, b: 2 }
    fillMissingKeys(obj, { a: 99, b: 99, c: 3 } as any)
    expect(obj).toEqual({ a: 1, b: 2, c: 3 })
  })

  it('returns the same object reference', () => {
    const obj = {}
    const result = fillMissingKeys(obj, { x: 1 } as any)
    expect(result).toBe(obj)
  })
})

// ---------------------------------------------------------------------------
// toSnakeCase / toCamelCase
// ---------------------------------------------------------------------------
describe('toSnakeCase', () => {
  it('converts camelCase keys to snake_case', () => {
    expect(toSnakeCase({ fooBar: 1, bazQux: 2 })).toEqual({ foo_bar: 1, baz_qux: 2 })
  })

  it('handles nested objects', () => {
    expect(toSnakeCase({ outerKey: { innerKey: 'v' } })).toEqual({
      outer_key: { inner_key: 'v' },
    })
  })

  it('handles arrays', () => {
    expect(toSnakeCase([{ fooBar: 1 }, { bazQux: 2 }])).toEqual([
      { foo_bar: 1 },
      { baz_qux: 2 },
    ])
  })

  it('returns primitives unchanged', () => {
    expect(toSnakeCase('hello')).toBe('hello')
    expect(toSnakeCase(42)).toBe(42)
    expect(toSnakeCase(null)).toBe(null)
  })
})

describe('toCamelCase', () => {
  it('converts snake_case keys to camelCase', () => {
    expect(toCamelCase({ foo_bar: 1, baz_qux: 2 })).toEqual({ fooBar: 1, bazQux: 2 })
  })

  it('handles nested objects', () => {
    expect(toCamelCase({ outer_key: { inner_key: 'v' } })).toEqual({
      outerKey: { innerKey: 'v' },
    })
  })

  it('handles arrays', () => {
    expect(toCamelCase([{ foo_bar: 1 }])).toEqual([{ fooBar: 1 }])
  })

  it('returns primitives unchanged', () => {
    expect(toCamelCase(null)).toBe(null)
    expect(toCamelCase(123)).toBe(123)
  })
})

// ---------------------------------------------------------------------------
// parseRangeToMs
// ---------------------------------------------------------------------------
describe('parseRangeToMs', () => {
  it('parses hours', () => {
    expect(parseRangeToMs('1h')).toBe(3_600_000)
    expect(parseRangeToMs('3h')).toBe(10_800_000)
  })

  it('parses days', () => {
    expect(parseRangeToMs('1d')).toBe(86_400_000)
    expect(parseRangeToMs('7d')).toBe(604_800_000)
  })

  it('defaults to 1 hour for unknown units', () => {
    expect(parseRangeToMs('5x')).toBe(3_600_000)
  })
})

// ---------------------------------------------------------------------------
// getStepByRangeKey / getStepByTimestampDiff
// ---------------------------------------------------------------------------
describe('getStepByRangeKey', () => {
  it('returns mapped step for known keys', () => {
    expect(getStepByRangeKey('1h')).toBe(60)
    expect(getStepByRangeKey('1d')).toBe(1800)
    expect(getStepByRangeKey('30d')).toBe(21600)
  })

  it('defaults to 3600 for unknown keys', () => {
    expect(getStepByRangeKey('99y')).toBe(3600)
  })
})

describe('getStepByTimestampDiff', () => {
  it('returns 30s for ≤1h', () => {
    expect(getStepByTimestampDiff(0, 3600)).toBe(30)
  })

  it('returns 60s for ≤3h', () => {
    expect(getStepByTimestampDiff(0, 3 * 3600)).toBe(60)
  })

  it('returns 600s for >24h', () => {
    expect(getStepByTimestampDiff(0, 48 * 3600)).toBe(600)
  })
})

// ---------------------------------------------------------------------------
// byte2Gi / formatBytes / fmtVal
// ---------------------------------------------------------------------------
describe('byte2Gi', () => {
  it('converts bytes to Gi string', () => {
    const oneGi = 1024 ** 3
    expect(byte2Gi(oneGi)).toBe('1 Gi')
  })

  it('supports fractionDigits', () => {
    const val = 1.5 * 1024 ** 3
    expect(byte2Gi(val, 1)).toBe('1.5 Gi')
  })

  it('supports withUnit=false', () => {
    expect(byte2Gi(1024 ** 3, 0, false)).toBe('1')
  })

  it('returns 0 for NaN or negative', () => {
    expect(byte2Gi(NaN)).toBe('0 Gi')
    expect(byte2Gi(-1)).toBe('0 Gi')
  })
})

describe('formatBytes', () => {
  it('formats bytes with appropriate unit', () => {
    expect(formatBytes(0)).toBe('0.00 B')
    expect(formatBytes(999)).toBe('999.00 B')
    expect(formatBytes(1000)).toBe('1.00 KB')
    expect(formatBytes(1_500_000)).toBe('1.50 MB')
  })

  it('returns 0 B for NaN or negative', () => {
    expect(formatBytes(NaN)).toBe('0 B')
    expect(formatBytes(-5)).toBe('0 B')
  })
})

describe('fmtVal', () => {
  it('returns "-" for null/undefined', () => {
    expect(fmtVal(null, 'any')).toBe('-')
    expect(fmtVal(undefined, 'cpu')).toBe('-')
    expect(fmtVal('-', 'cpu')).toBe('-')
  })

  it('formats memory-related keys as Gi', () => {
    expect(fmtVal(1024 ** 3, 'memory')).toBe('1 Gi')
  })

  it('stringifies other values', () => {
    expect(fmtVal(42, 'cpu')).toBe('42')
  })
})

// ---------------------------------------------------------------------------
// convertKeyValueMapToList / convertListToKeyValueMap
// ---------------------------------------------------------------------------
describe('convertKeyValueMapToList', () => {
  it('converts record to list', () => {
    expect(convertKeyValueMapToList({ a: '1', b: '2' })).toEqual([
      { key: 'a', value: '1' },
      { key: 'b', value: '2' },
    ])
  })

  it('returns default pair for empty/null input', () => {
    expect(convertKeyValueMapToList(undefined)).toEqual([{ key: '', value: '' }])
    expect(convertKeyValueMapToList({})).toEqual([{ key: '', value: '' }])
  })
})

describe('convertListToKeyValueMap', () => {
  it('converts list to record, filtering empty keys', () => {
    expect(
      convertListToKeyValueMap([
        { key: 'a', value: '1' },
        { key: '', value: 'skip' },
        { key: 'b', value: '2' },
      ]),
    ).toEqual({ a: '1', b: '2' })
  })

  it('returns empty object for empty list', () => {
    expect(convertListToKeyValueMap([])).toEqual({})
  })
})

// ---------------------------------------------------------------------------
// fmtTs
// ---------------------------------------------------------------------------
describe('fmtTs', () => {
  it('returns empty string for falsy input', () => {
    expect(fmtTs('')).toBe('')
  })

  it('strips fractional seconds and formats', () => {
    const result = fmtTs('2024-01-15 12:30:45.123')
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
  })

  it('handles ISO format with T separator', () => {
    const result = fmtTs('2024-01-15T12:30:45')
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
  })
})

// ---------------------------------------------------------------------------
// toKVList
// ---------------------------------------------------------------------------
describe('toKVList', () => {
  it('converts an object to KV list', () => {
    expect(toKVList({ foo: 'bar', baz: 42 })).toEqual([
      { key: 'foo', value: 'bar' },
      { key: 'baz', value: '42' },
    ])
  })

  it('converts an array of KV-like items', () => {
    expect(toKVList([{ key: 'a', value: 'b' }])).toEqual([{ key: 'a', value: 'b' }])
  })

  it('returns default pair for null/undefined/string', () => {
    expect(toKVList(null)).toEqual([{ key: '', value: '' }])
    expect(toKVList(undefined)).toEqual([{ key: '', value: '' }])
    expect(toKVList('hello')).toEqual([{ key: '', value: '' }])
  })

  it('returns default pair for empty object/array', () => {
    expect(toKVList({})).toEqual([{ key: '', value: '' }])
    expect(toKVList([])).toEqual([{ key: '', value: '' }])
  })
})

// ---------------------------------------------------------------------------
// parseQuantityWithUnit
// ---------------------------------------------------------------------------
describe('parseQuantityWithUnit', () => {
  it('parses quantity with unit', () => {
    expect(parseQuantityWithUnit('100Gi')).toEqual({ val: '100', unit: 'Gi' })
    expect(parseQuantityWithUnit('50Mi')).toEqual({ val: '50', unit: 'Mi' })
  })

  it('parses quantity with space before unit', () => {
    expect(parseQuantityWithUnit('200 Ti')).toEqual({ val: '200', unit: 'Ti' })
  })

  it('defaults unit to Gi when missing', () => {
    expect(parseQuantityWithUnit('100')).toEqual({ val: '100', unit: 'Gi' })
  })

  it('handles decimal values', () => {
    expect(parseQuantityWithUnit('1.5Gi')).toEqual({ val: '1.5', unit: 'Gi' })
  })

  it('returns empty val for empty/undefined input', () => {
    expect(parseQuantityWithUnit(undefined)).toEqual({ val: '', unit: 'Gi' })
    expect(parseQuantityWithUnit('')).toEqual({ val: '', unit: 'Gi' })
  })

  it('extracts numeric part from invalid format', () => {
    const result = parseQuantityWithUnit('abc123xyz')
    expect(result.val).toBe('123')
    expect(result.unit).toBe('Gi')
  })
})

describe('CAP_UNITS', () => {
  it('contains the expected units in order', () => {
    expect(CAP_UNITS).toEqual(['Pi', 'Ti', 'Gi', 'Mi', 'Ki'])
  })
})

// ---------------------------------------------------------------------------
// encodeToBase64String / decodeFromBase64String
// ---------------------------------------------------------------------------
describe('encodeToBase64String', () => {
  it('encodes a simple string', () => {
    expect(encodeToBase64String('hello')).toBe(btoa('hello'))
  })

  it('encodes Unicode characters', () => {
    const encoded = encodeToBase64String('你好世界')
    expect(encoded).toBeTruthy()
    expect(decodeFromBase64String(encoded)).toBe('你好世界')
  })

  it('returns empty string for empty input', () => {
    expect(encodeToBase64String('')).toBe('')
  })
})

describe('decodeFromBase64String', () => {
  it('decodes a valid Base64 string', () => {
    expect(decodeFromBase64String(btoa('hello'))).toBe('hello')
  })

  it('returns original for non-Base64 input', () => {
    expect(decodeFromBase64String('not-base64!!!')).toBe('not-base64!!!')
  })

  it('returns empty string for empty input', () => {
    expect(decodeFromBase64String('')).toBe('')
  })

  it('handles URL-safe Base64 (- and _)', () => {
    const standard = encodeToBase64String('test~data')
    const urlSafe = standard.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
    expect(decodeFromBase64String(urlSafe)).toBe('test~data')
  })
})

// ---------------------------------------------------------------------------
// formatNodeInfo
// ---------------------------------------------------------------------------
describe('formatNodeInfo', () => {
  it('converts object to label-value pairs with formatted labels', () => {
    const result = formatNodeInfo({ gpu_count: 8, os_version: 'Linux' })
    expect(result).toEqual([
      { label: 'GPU Count', value: '8' },
      { label: 'OS Version', value: 'Linux' },
    ])
  })

  it('trims string values', () => {
    const result = formatNodeInfo({ name: '  alice  ' })
    expect(result[0].value).toBe('alice')
  })
})

// ---------------------------------------------------------------------------
// calculateDefaultTime
// ---------------------------------------------------------------------------
describe('calculateDefaultTime', () => {
  it('returns null when no start time', () => {
    expect(calculateDefaultTime(undefined, undefined, undefined)).toBeNull()
  })

  it('returns [start, end] when both are provided', () => {
    const result = calculateDefaultTime('2024-01-01 00:00:00', '2024-01-02 00:00:00')
    expect(result).not.toBeNull()
    expect(result!.length).toBe(2)
    expect(result![0]).toBeInstanceOf(Date)
    expect(result![1]).toBeInstanceOf(Date)
    expect(result![1].getTime()).toBeGreaterThan(result![0].getTime())
  })

  it('falls back to creationTime when startTime is missing', () => {
    const result = calculateDefaultTime(undefined, undefined, '2024-06-01 12:00:00')
    expect(result).not.toBeNull()
    expect(result![0]).toBeInstanceOf(Date)
  })

  it('returns [start, start] when end < start', () => {
    const result = calculateDefaultTime('2024-06-01 12:00:00', '2024-01-01 00:00:00')
    expect(result![0].getTime()).toBe(result![1].getTime())
  })
})
