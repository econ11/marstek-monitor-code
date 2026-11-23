import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import type { PowerSeries, Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import { setLocale } from '../lib/i18n'
import { host, installFakeDom, type FakeEl } from '../testing/fakeDom'
import { energyScreen } from './energy'
import type { PeriodMemory } from './shell'

// The Energy screen on a stand-in DOM (src/testing/fakeDom.ts): its ⓘ.
// What it draws is tested in energyModel.test.ts and powerModel.test.ts.

const now = new Date(2026, 10, 21, 15, 17).getTime()
const iso = (ms: number) => new Date(ms).toISOString()

beforeEach(() => {
  vi.useFakeTimers({ now })
  installFakeDom()
})
afterEach(() => {
  setLocale('uk')
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function report(coverage: number): Report {
  const from = new Date(now).setHours(0, 0, 0, 0)
  return {
    Period: { From: iso(from), To: iso(now), Weeks: false },
    ChargedWh: 4000, DischargedWh: 3500, OutageWh: 1000, Efficiency: 87.5, Coverage: coverage, HasPV: false, EnergySince: null, Backfilling: false,
    Buckets: [{ Start: iso(from), End: iso(now), ChargedWh: 4000, DischargedWh: 3500, OutageWh: 1000, PVChargeWh: 0, PeakW: null, PeakAt: null, Covered: (now - from) * 1e6, Span: (now - from) * 1e6, NoData: false }],
  } as Report
}

function series(): PowerSeries {
  const from = new Date(now).setHours(0, 0, 0, 0)
  return {
    Start: iso(from), End: iso(from + 24 * 3600e3), Step: 60e3 * 1e6, Clipped: false,
    Points: [{ At: iso(now - 120e3), W: -412 }, { At: iso(now - 60e3), W: -400 }],
    PeakW: 812, PeakAt: iso(now - 90e3), Outages: [],
  }
}

type MountOpts = { report?: () => Promise<Report>; memory?: PeriodMemory }
async function mount(coverage: number, power: () => Promise<PowerSeries> = async () => series(), o: MountOpts = {}) {
  const screen = energyScreen({ report: o.report ?? (async () => report(coverage)), power, memory: o.memory ?? { key: 'today' } })
  const root = host() as unknown as FakeEl
  screen.mount(root as unknown as HTMLElement)
  await vi.advanceTimersByTimeAsync(0)
  return { screen, root, slot: (name: string) => root.querySelector(`[data-slot="${name}"]`)! }
}

test('the totals have their ⓘ, named after their labels', async () => {
  const { root } = await mount(1)
  for (const key of ['info.energy.charged', 'info.energy.discharged', 'info.energy.efficiency']) expect(root.innerHTML).toContain(`data-info="${key}"`)
  expect(root.innerHTML).toContain('aria-label="Що означає «ККД циклу»"')
})

test('the «incomplete» note has its ⓘ', async () => {
  expect((await mount(0.82)).slot('incomplete').innerHTML).toContain('data-info="info.energy.incomplete"')
  expect((await mount(1)).slot('incomplete').innerHTML).toBe('')
})

test('the power chart: its peak caption, no error', async () => {
  const { slot } = await mount(1)
  expect(slot('power-peak').textContent).toBe('Пік розряду: 812 Вт о 15:15')
  expect(slot('power-error').hidden).toBe(true)
  expect(slot('power-empty').hidden).toBe(true)
})

test('a failed power chart shows an error with retry, kept across a language switch', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  let fail = true
  const { root, slot } = await mount(1, async () => { if (fail) throw new Error('db'); return series() })
  expect(slot('power-error').hidden).toBe(false)
  setLocale('en')
  await vi.advanceTimersByTimeAsync(0)
  expect(slot('power-error').hidden).toBe(false)
  expect(root.innerHTML).toContain('data-action="retry-power">Try again')
  fail = false
  await vi.advanceTimersByTimeAsync(60e3) // the minute refresh loads it again
  expect(slot('power-error').hidden).toBe(true)
  expect(slot('power-peak').textContent).toBe('Peak discharge: 812 W at 15:15')
  expect(warn).toHaveBeenCalledWith('energy power', expect.any(Error))
  warn.mockRestore()
})

test('while past days are counted, a past period already drawn is not read again every 5 s', async () => {
  const power = vi.fn(async () => series())
  const backfilling = async () => ({ ...report(1), Backfilling: true }) as Report
  await mount(1, power, { report: backfilling, memory: { key: 'custom', range: { from: '2026-11-18', to: '2026-11-19' } } })
  expect(power).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(5e3 * 4)
  expect(power).toHaveBeenCalledTimes(1)
  // Today's chart still follows: its last minutes keep coming.
  const today = vi.fn(async () => series())
  await mount(1, today, { report: backfilling })
  await vi.advanceTimersByTimeAsync(5e3 * 2)
  expect(today).toHaveBeenCalledTimes(3)
})

test('the first power load shows a loader, not an empty panel', async () => {
  const { slot } = await mount(1, () => new Promise<PowerSeries>(() => {}))
  expect(slot('power-loading').hidden).toBe(false)
  expect(slot('power-chart').hidden).toBe(true)
})

test('another station: the old chart goes; its failed load shows the error, not a stale chart', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  let fail = false
  const { screen, slot } = await mount(1, async () => { if (fail) throw new Error('db'); return series() })
  screen.update({ Station: { ID: 'A' } } as Snapshot)
  expect(slot('power-peak').hidden).toBe(false)
  fail = true
  screen.update({ Station: { ID: 'B' } } as Snapshot)
  expect(slot('power-loading').hidden).toBe(false)
  expect(slot('power-peak').hidden).toBe(true)
  await vi.advanceTimersByTimeAsync(0)
  expect(slot('power-error').hidden).toBe(false)
  expect(slot('power-loading').hidden).toBe(true)
  expect(slot('power-chart').hidden).toBe(true)
  expect(slot('power-peak').hidden).toBe(true)
  warn.mockRestore()
})

test('the first station update does not reset the chart a minute later', async () => {
  let calls = 0
  let release = () => {}
  const power = () => {
    calls++
    // The first load answers at once; the refresh stays pending.
    return calls === 1 ? Promise.resolve(series()) : new Promise<PowerSeries>(r => { release = () => r(series()) })
  }
  const { screen, slot } = await mount(1, power)
  screen.update({ Station: { ID: 'A' } } as Snapshot)
  await vi.advanceTimersByTimeAsync(60e3)
  expect(calls).toBe(2) // the minute refresh, not a reset
  expect(slot('power-loading').hidden).toBe(true)
  expect(slot('power-chart').hidden).toBe(false)
  expect(slot('power-peak').hidden).toBe(false)
  release()
})
