import { afterEach, expect, test } from 'vitest'
import copyDoc from '../../../docs/superpowers/specs/2026-11-21-marstek-monitor-copy.md?raw'
import { getLocale, onLocaleChange, setLocale, t } from '../lib/i18n'
import { en } from './en'
import { uk } from './uk'

afterEach(() => setLocale('uk'))

test('catalogues have the same keys', () => expect(Object.keys(en).sort()).toEqual(Object.keys(uk).sort()))

test('every en text keeps the placeholders of its uk text', () => {
  // {name} and the argument of {name, plural|select, …}; not the case texts.
  const names = (s: string) => Array.from(s.matchAll(/\{(\w+)(?:\}|, (?:plural|select),)/g), m => m[1]).sort()
  const off = Object.keys(uk).filter(k => names(uk[k]).join() !== names(en[k]).join())
  expect(off).toEqual([])
})

test('en plural', () => {
  setLocale('en')
  expect(t('overview.ring.offToday.sub', { count: 1 })).toBe('1 outage')
  expect(t('overview.ring.offToday.sub', { count: 5 })).toBe('5 outages')
  expect(t('duration.long.h', { h: 2500 })).toBe('2,500 hours')
})

test('uk is the default locale', () => {
  expect(getLocale()).toBe('uk')
  expect(t('nav.tab.overview')).toBe('Огляд')
})

test('setLocale switches the catalogue and tells the listeners', () => {
  let calls = 0
  const off = onLocaleChange(() => calls++)
  setLocale('en')
  expect(getLocale()).toBe('en')
  expect(t('nav.tab.overview')).toBe('Overview')
  expect(t('overview.ring.toEmpty.sub', { floor: 10, time: '22:47' })).toBe('to 10% · ≈ 22:47')
  setLocale('en') // no change, no call
  off()
  setLocale('uk')
  expect(calls).toBe(1)
  expect(t('overview.ring.toEmpty.sub', { floor: 10, time: '22:47' })).toBe('до 10% · ≈ о 22:47')
})

test('a key missing in the current language falls back to en, then uk', () => {
  uk['test.onlyUk'] = 'лише uk'
  try {
    setLocale('en')
    expect(t('test.onlyUk')).toBe('лише uk')
    expect(t('test.nowhere')).toBe('test.nowhere')
  } finally {
    delete uk['test.onlyUk']
  }
})

// §15 «Пояснення (ⓘ-підказки)»: every info.* key of the copy is in both
// catalogues, word for word.
test('the ⓘ texts are the copy §15 texts, in both languages', () => {
  const doc = copyDoc.replace(/\r/g, '')
  const s15 = doc.slice(doc.indexOf('## 15.'), doc.indexOf('## 16.'))
  const rows = Array.from(s15.matchAll(/^\| `(info\.[\w.]+)` \| (.*?) \| (.*?) \| .*\|$/gm), (m: RegExpMatchArray) => ({ key: m[1], uk: m[2], en: m[3] }))
  expect(rows.length).toBe(44)
  expect(rows.filter(r => uk[r.key] !== r.uk).map(r => r.key)).toEqual([])
  expect(rows.filter(r => en[r.key] !== r.en).map(r => r.key)).toEqual([])
  expect(uk['info.battery.cycles']).toMatch(/^Батарея розрахована на понад \{rated\} циклів/)
})

// macOS port, Task 2: a .mac key is a Mac variant of a key, in both languages.
test('every .mac key is in both catalogues, with its base key', () => {
  const mac = Object.keys({ ...uk, ...en }).filter(k => k.endsWith('.mac'))
  expect(mac.length).toBeGreaterThanOrEqual(12)
  const base = (k: string) => k.slice(0, -'.mac'.length)
  expect(mac.filter(k => !(k in uk) || !(k in en))).toEqual([])
  expect(mac.filter(k => !(base(k) in uk) || !(base(k) in en))).toEqual([])
})
