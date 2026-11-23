import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import type { Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/history/models'
import { setLocale } from '../lib/i18n'
import { host, installFakeDom, type FakeEl } from '../testing/fakeDom'
import { outagesScreen } from './outages'

// The Outages screen on a stand-in DOM (src/testing/fakeDom.ts). What it
// draws is tested in outagesModel.test.ts.

const now = new Date(2026, 10, 21, 15, 17).getTime()

beforeEach(() => {
  vi.useFakeTimers({ now })
  installFakeDom()
  vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => {
  setLocale('uk')
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function mount(report: () => Promise<Report>) {
  const screen = outagesScreen({ report: vi.fn(report), memory: { key: '7d' } })
  const root = host()
  screen.mount(root)
  const slot = (name: string) => (root as unknown as FakeEl).querySelector(`[data-slot="${name}"]`)!
  return { screen, slot }
}

test('a fetch error survives a language switch', async () => {
  const { slot } = mount(() => Promise.reject(new Error('db')))
  await vi.advanceTimersByTimeAsync(0)
  expect(slot('error').hidden).toBe(false)
  expect(slot('loading').hidden).toBe(true)
  // The layout is built again in English: the error and its retry, not «Loading…».
  setLocale('en')
  expect(slot('error').hidden).toBe(false)
  expect(slot('loading').hidden).toBe(true)
  expect(slot('error').owner!.innerHTML).toContain('Try again')
})

test('«Сьогодні» draws the day by the hour, other periods the columns', async () => {
  const day = new Date(now).setHours(0, 0, 0, 0)
  const iso = (ms: number) => new Date(ms).toISOString()
  const rep = (from: number, weeks = false) => ({
    Period: { From: iso(from), To: iso(now), Weeks: weeks }, ObservedSince: iso(day - 7 * 864e5), Observed: 0, Off: 0, OffAtLeast: false,
    GridPct: 100, Count: 0, RanOut: 0, Coverage: null, CoverageApprox: false, Average: 0, Longest: 0, LongestAt: null, Uncertain: 0, Gaps: 0,
    Buckets: [{ Start: iso(from), End: iso(now), Off: 0, OffAtLeast: false, Count: 0, Ongoing: false, Longest: null, MinSOC: null,
      RanOut: false, Observed: 0, Gaps: 0, NoData: false, BeforeObs: false }],
    Rows: [], GapSpans: [],
  }) as unknown as Report
  const screen = outagesScreen({ report: async () => rep(day), memory: { key: 'today' } })
  const root = host()
  screen.mount(root)
  await vi.advanceTimersByTimeAsync(0)
  const slot = (name: string) => (root as unknown as FakeEl).querySelector(`[data-slot="${name}"]`)!
  expect(slot('bars-title').textContent).toBe('Сьогодні по годинах')
  expect(slot('bars-sub').textContent).toBe('00:00 – 24:00')
  expect(slot('bars').innerHTML).toContain('class="tl-now"')
  screen.unmount()

  const week = outagesScreen({ report: async () => rep(day - 6 * 864e5), memory: { key: '7d' } })
  const root2 = host()
  week.mount(root2)
  await vi.advanceTimersByTimeAsync(0)
  const bars = (root2 as unknown as FakeEl).querySelector('[data-slot="bars"]')!
  expect(bars.innerHTML).not.toContain('tl-now')
  expect((root2 as unknown as FakeEl).querySelector('[data-slot="bars-sub"]')!.textContent).toBe('години без світла')
})

test('the table cells and the legend draw their ⓘ, one name per row and cell', async () => {
  const iso = (ms: number) => new Date(ms).toISOString()
  const H = 3600e3, start = now - 5 * H, end = now - 3 * H
  const rep = {
    Period: { From: iso(now - 6 * 864e5), To: iso(now), Weeks: false }, ObservedSince: iso(now - 9 * 864e5), Observed: 0, Off: 0, OffAtLeast: true,
    GridPct: 99, Count: 1, RanOut: 0, Coverage: 100, CoverageApprox: false, Average: 0, Longest: 0, LongestAt: null, Uncertain: 0, Gaps: 0,
    Buckets: [], GapSpans: [],
    Rows: [{
      Outage: { ID: 4, StationID: 'st1', Start: iso(start), End: iso(end), SOCStart: 90, SOCEnd: 60, DiedAt: null, Partial: false },
      Bounds: { StartApprox: false, EndApprox: true, NotAfter: iso(end), Frozen: false, AsOf: iso(end), Known: H * 1e6, AtLeast: true },
    }],
  } as unknown as Report
  const { slot } = mount(async () => rep)
  await vi.advanceTimersByTimeAsync(0)
  const rows = slot('rows').innerHTML
  expect(rows).toContain('data-info="info.common.notAfter" data-info-at="4:end"')
  expect(rows).toContain('data-info="info.common.atLeast" data-info-at="4:duration"')
  expect(rows).toContain('data-info="info.outages.gapRow" data-info-at="4:note"')
  expect(rows).toContain('aria-label="Що означає «≥ 1 год»"')
  expect(slot('body').owner!.innerHTML).toContain('data-info="info.outages.noDataDay"')
})
