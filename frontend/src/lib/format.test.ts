import { afterEach, expect, test } from 'vitest'
import { age, dateShort, dayTimes, dayShort, hm, hmLong, kwh, num, pct, spoken, weekdayShort } from './format'
import { setLocale, t } from './i18n'

afterEach(() => setLocale('uk'))

test('hm formats clock form', () => { expect(hm(5*3600e3+27*60e3)).toBe('5:27'); expect(hm(27*60e3)).toBe('0:27'); expect(hm(0)).toBe('0:00') })
test('kwh uses decimal comma', () => expect(kwh(3686.4)).toBe('3,7'))
test('plural uk', () => { expect(t('overview.ring.offToday.sub', {count: 2})).toBe('2 відключення'); expect(t('overview.ring.offToday.sub', {count: 5})).toBe('5 відключень') })

test('plural uk: one, many in the teens, one again at 21', () => {
  expect(t('overview.ring.offToday.sub', { count: 1 })).toBe('1 відключення')
  expect(t('overview.ring.offToday.sub', { count: 11 })).toBe('11 відключень')
  expect(t('overview.ring.offToday.sub', { count: 21 })).toBe('21 відключення')
})
test('select and plain arguments', () => {
  expect(t('gapCard.gridOff', { time: '01:20', approx: 'true' })).toBe('Світло зникло о 01:20 (приблизно).')
  expect(t('gapCard.gridOff', { time: '01:20', approx: 'false' })).toBe('Світло зникло о 01:20.')
  expect(t('overview.ring.toEmpty.sub', { floor: 10, time: '22:47' })).toBe('до 10% · ≈ о 22:47')
})
test('select with arguments inside a branch', () => {
  expect(t('overview.ring.power.a11y', { state: 'discharging', watts: '420', max: '2 500' })).toBe('Розряджається, 420 Вт з 2 500 максимальних')
  expect(t('overview.ring.power.a11y', { state: 'idle', watts: '0', max: '' })).toBe('Очікування, 0 Вт')
})
test('an unknown key comes back as the key', () => expect(t('no.such.key')).toBe('no.such.key'))

test('hmLong follows the short-duration rules', () => {
  expect(hmLong(7.5 * 3600e3)).toBe('7 год 30 хв')
  expect(hmLong(2 * 3600e3 + 5 * 60e3)).toBe('2 год 05 хв')
  expect(hmLong(45 * 60e3)).toBe('45 хв')
  expect(hmLong(20e3)).toBe('< 1 хв')
  expect(hmLong(2 * 3600e3)).toBe('2 год')
  expect(hmLong(27 * 3600e3 + 10 * 60e3)).toBe('1 д 3 год')
})
test('pct has no space', () => expect(pct(72)).toBe('72%'))
test('num groups thousands and keeps the profile capacity', () => {
  expect(num(2500)).toBe('2 500')
  expect(num(5.12, 2)).toBe('5,12')
})
test('age picks seconds, minutes, hours', () => {
  expect(age(3e3)).toBe('3 с тому')
  expect(age(2 * 60e3)).toBe('2 хв тому')
  expect(age(3 * 3600e3)).toBe('3 год тому')
})

test('en numbers and dates', () => {
  setLocale('en')
  expect(kwh(3686.4)).toBe('3.7')
  expect(num(2500)).toBe('2,500')
  expect(dateShort(Date.UTC(2026, 10, 21, 12))).toBe('21 Nov')
  expect(hmLong(2 * 3600e3 + 5 * 60e3)).toBe('2 h 05 min')
})
test('en weekdays, axis days, spoken durations, ages', () => {
  setLocale('en')
  const sat = new Date(2026, 10, 21, 12).getTime()
  expect(weekdayShort(sat)).toBe('Sat')
  expect(dayShort(sat)).toBe('Sat 21')
  expect(spoken(5 * 3600e3 + 27 * 60e3)).toBe('5 hours 27 minutes')
  expect(spoken(60e3)).toBe('1 minute')
  expect(age(3000)).toBe('3 s ago')
})
test('uk stays default', () => {
  setLocale('uk')
  expect(num(2500)).toBe('2 500')
  const sat = new Date(2026, 10, 21, 12).getTime()
  expect(dateShort(sat)).toBe('21.11')
  expect(dayShort(sat)).toBe('сб 21.11')
})

test('dayTimes names each day once, relative to today', () => {
  const at = (d: number, h: number, m: number) => new Date(2026, 10, d, h, m).getTime()
  const now = at(23, 15, 0)
  expect(dayTimes([at(23, 1, 5), at(23, 7, 10), at(23, 9, 0)], now)).toEqual(['01:05', '07:10', '09:00'])
  expect(dayTimes([at(22, 23, 40), at(23, 7, 10), at(23, 9, 0)], now)).toEqual(['вчора 23:40', '07:10', '09:00'])
  expect(dayTimes([at(21, 23, 40), at(22, 7, 10), at(22, 9, 0)], now)).toEqual(['21.11 23:40', 'вчора 07:10', '09:00'])
  expect(dayTimes([at(22, 1, 0), at(22, 7, 10), at(23, 9, 0)], now)).toEqual(['вчора 01:00', '07:10', 'сьогодні 09:00'])
  expect(dayTimes([at(19, 1, 0), at(19, 7, 10)], now)).toEqual(['19.11 01:00', '07:10'])
  setLocale('en')
  expect(dayTimes([at(22, 23, 40), at(23, 7, 10)], now)).toEqual(['yesterday 23:40', '07:10'])
})
