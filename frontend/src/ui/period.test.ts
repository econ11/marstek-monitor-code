import { expect, test, vi } from 'vitest'
import { calendarDays, pickDay } from './calendar'
import { periodRange, rangeLabel, syncSeg } from './period'
import { FakeEl, installFakeDom } from '../testing/fakeDom'
import { setLocale } from '../lib/i18n'

const nov2026 = Date.parse('2026-11-01T00:00')

test('periodRange', () => {
  const now = Date.parse('2026-11-21T14:00')
  expect(periodRange('today', now)).toEqual({ from: '2026-11-21', to: '2026-11-21' })
  expect(periodRange('7d', now)).toEqual({ from: '2026-11-15', to: '2026-11-21' })
  expect(periodRange('30d', now).from).toBe('2026-10-23')
  expect(periodRange('30d', now).to).toBe('2026-11-21')
})

test('periodRange counts days from local midnight, not 24 h steps', () => {
  // The tests run in Europe/Kyiv (vite.config.ts test.env), whatever the
  // machine's zone: a UTC runner would pass this test without a DST day.
  expect(new Date(2026, 2, 28).getTimezoneOffset() - new Date(2026, 2, 30).getTimezoneOffset()).toBe(60)
  // 29.03.2026 has 23 hours in Europe/Kyiv (the Windows zone here): six
  // 24 h steps back from 04.04 00:30 land on 28.03 23:30, a day too early.
  expect(periodRange('7d', Date.parse('2026-04-04T00:30'))).toEqual({ from: '2026-03-29', to: '2026-04-04' })
  // 25.10.2026 has 25 hours.
  expect(periodRange('30d', Date.parse('2026-11-23T00:10')).from).toBe('2026-10-25')
})

test('calendar disables future days', () => {
  const days = calendarDays(nov2026, Date.parse('2026-11-21T14:00'))
  expect(days).toHaveLength(30)
  expect(days[0].day).toBe('2026-11-01')
  expect(days.find(d => d.day === '2026-11-22')!.future).toBe(true)
  expect(days.find(d => d.day === '2026-11-21')!.future).toBe(false)
})

test('calendar marks the selected range', () => {
  const days = calendarDays(nov2026, Date.parse('2026-11-21T14:00'), { from: '2026-11-15', to: '2026-11-20' })
  expect(days.filter(d => d.inRange).map(d => d.day)).toEqual(
    ['2026-11-15', '2026-11-16', '2026-11-17', '2026-11-18', '2026-11-19', '2026-11-20'])
})

test('custom range is ordered', () => {
  // Click 20.11, then 15.11: the range runs from the earlier day.
  const first = pickDay(undefined, '2026-11-20')
  expect(first).toEqual({ from: '2026-11-20', to: '2026-11-20', open: true })
  expect(pickDay(first, '2026-11-15')).toEqual({ from: '2026-11-15', to: '2026-11-20', open: false })
  // A third click starts a new range.
  expect(pickDay({ from: '2026-11-15', to: '2026-11-20', open: false }, '2026-11-03')).toEqual({ from: '2026-11-03', to: '2026-11-03', open: true })
})

// Task 20c (Task 7 review): the custom range button reads as copy §9.3
// date.range: «15.11 – 21.11» / «15 – 21 Nov».
test('rangeLabel follows copy §9.3', () => {
  expect(rangeLabel({ from: '2026-11-15', to: '2026-11-21' })).toBe('15.11 – 21.11')
  expect(rangeLabel({ from: '2026-11-21', to: '2026-11-21' })).toBe('21.11')
  setLocale('en')
  try {
    expect(rangeLabel({ from: '2026-11-15', to: '2026-11-21' })).toBe('15 – 21 Nov')
    expect(rangeLabel({ from: '2026-10-28', to: '2026-11-03' })).toBe('28 Oct – 3 Nov')
    expect(rangeLabel({ from: '2026-11-21', to: '2026-11-21' })).toBe('21 Nov')
  } finally {
    setLocale('uk')
  }
})

test('syncSeg: the first paint places the thumb without a slide', () => {
  installFakeDom()
  const frames: (() => void)[] = []
  vi.stubGlobal('requestAnimationFrame', (fn: () => void) => { frames.push(fn); return frames.length })
  const seg = new FakeEl()
  const thumb = seg.querySelector('.seg__thumb')!
  const on = seg.querySelector('[aria-pressed="true"]')!
  on.offsetWidth = 40
  on.offsetLeft = 3
  syncSeg(seg as unknown as HTMLElement)
  // Placed with the transition off; it comes back in the next frame.
  expect(seg.classList.contains('seg--init')).toBe(true)
  expect(thumb.style.width).toBe('40px')
  expect(thumb.style.transform).toBe('translateX(3px)')
  expect(thumb.dataset.placed).toBe('1')
  frames.splice(0).forEach(fn => fn())
  expect(seg.classList.contains('seg--init')).toBe(false)
  // Later moves slide.
  on.offsetLeft = 50
  syncSeg(seg as unknown as HTMLElement)
  expect(seg.classList.contains('seg--init')).toBe(false)
  expect(thumb.style.transform).toBe('translateX(50px)')
  vi.unstubAllGlobals()
})

test('syncSeg: a seg not laid out yet is placed on a later sync, still without a slide', () => {
  installFakeDom()
  vi.stubGlobal('requestAnimationFrame', () => 0)
  const seg = new FakeEl()
  const thumb = seg.querySelector('.seg__thumb')!
  const on = seg.querySelector('[aria-pressed="true"]')!
  syncSeg(seg as unknown as HTMLElement)
  expect(thumb.dataset.placed).toBeUndefined()
  expect(seg.classList.contains('seg--init')).toBe(false)
  on.offsetWidth = 40
  syncSeg(seg as unknown as HTMLElement)
  expect(seg.classList.contains('seg--init')).toBe(true)
  expect(thumb.dataset.placed).toBe('1')
  vi.unstubAllGlobals()
})
