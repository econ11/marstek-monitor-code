import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import type { SeriesResponse } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import { host, installFakeDom, type FakeEl } from '../testing/fakeDom'
import { setLocale } from '../lib/i18n'
import { clearChartPointer } from '../ui/socChart'
import { hideTip, initTooltips, showTip, tipAnchor } from '../ui/tooltip'
import { overviewScreen } from './overview'
import { overviewFeed, overviewModel } from './overviewModel'
import { frameHTML, mountShell } from './shell'

// The Overview screen on a stand-in DOM (src/testing/fakeDom.ts): its timers,
// visibility and the chart note. What it draws is tested in overviewModel.test.ts.

const NS = 1e6, M = 60e3
const at = (h: number, m = 0) => new Date(2026, 10, 21, h, m, 0).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const now = at(15, 17)
const noForecast = { ToEmpty: null, ToFull: null, LastsFor: null, EmptyAt: null, FullAt: null, Warming: false }

const live = {
  Seq: 1,
  Station: { ID: 'st1', Name: 'Дім', Model: 'Venus E 3.0', BLEMac: '', WifiMac: '', IP: '', Firmware: 151, CreatedAt: iso(at(9)) },
  Profile: { Model: 'Venus E 3.0', NominalWh: 5120, MaxChargeW: 2500, MaxDischargeW: 2500, MinSOC: 10, CycleLife: 6000, TempMinC: 0, TempMaxC: 45, Tested: true },
  Link: 'LIVE', Diagnosis: '',
  Last: { StationID: 'st1', At: iso(now - 3e3), Source: 'lan', SOC: 72, BatteryW: 0, OngridW: 0, OffgridW: 0, PVW: null, Grid: 1, Raw: null },
  Age: 3e3 * NS, Grid: 0, Mode: 1, F: noForecast, NoLightToday: 0, OutagesToday: 0,
  Current: null, Previous: null, GapCard: null, Fills: { SOC: 0.72, Power: 0, Time: 0, NoLight: 0 },
  ObservedSince: iso(at(9)), LowThreshold: 20, NoLightAtLeast: false, GapsToday: 0, CurrentBounds: null, PreviousBounds: null,
} as unknown as Snapshot

vi.mock('../ui/socChart', async orig => ({ ...(await orig<typeof import('../ui/socChart')>()), clearChartPointer: vi.fn() }))

let doc: ReturnType<typeof installFakeDom>
beforeEach(() => {
  vi.useFakeTimers({ now })
  doc = installFakeDom()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

// The feed and its one-second tick, as main.ts runs them.
function app(model = overviewModel) {
  const feed = overviewFeed(Date.now, model)
  const tick = setInterval(feed.tick, 1000)
  return { feed, stop: () => clearInterval(tick) }
}

function mount(points: SeriesResponse['Points'] = [{ At: iso(now - M), SOC: 72 }], { feed } = app()) {
  const series = vi.fn(async () => ({ Points: points, Bands: [] }) as SeriesResponse)
  const screen = overviewScreen({ series, dismissGapCard: async () => {}, memory: { range: '24h', dismissed: new Set() }, feed })
  const root = host()
  screen.mount(root)
  feed.update(live)
  screen.update(live)
  const slot = (name: string) => (root as unknown as FakeEl).querySelector(`[data-slot="${name}"]`)!
  return { screen, series, slot, feed }
}

test('no chart refresh while hidden', async () => {
  const { series } = mount()
  await vi.advanceTimersByTimeAsync(0)
  expect(series).toHaveBeenCalledTimes(1)
  doc.visibilityState = 'hidden'
  doc.dispatch('visibilitychange')
  await vi.advanceTimersByTimeAsync(5 * M)
  expect(series).toHaveBeenCalledTimes(1)
  // Shown again: the chart, older than a minute by now, is fetched at once.
  doc.visibilityState = 'visible'
  doc.dispatch('visibilitychange')
  await vi.advanceTimersByTimeAsync(0)
  expect(series).toHaveBeenCalledTimes(2)
  // Shown again within the minute: nothing to catch up.
  doc.visibilityState = 'hidden'
  doc.dispatch('visibilitychange')
  doc.visibilityState = 'visible'
  doc.dispatch('visibilitychange')
  await vi.advanceTimersByTimeAsync(0)
  expect(series).toHaveBeenCalledTimes(2)
})

test('unmount clears intervals', async () => {
  const a = app()
  const before = vi.getTimerCount() // the app's tick
  // Every subscription to the feed is counted, and every unsubscribe.
  let listening = 0
  const on = a.feed.on
  a.feed.on = fn => { listening++; const off = on(fn); return () => { listening--; off() } }
  const { screen, slot } = mount(undefined, a)
  await vi.advanceTimersByTimeAsync(0)
  expect(listening).toBe(1)
  screen.unmount()
  expect(vi.getTimerCount()).toBe(before)
  expect(doc.count('visibilitychange')).toBe(0)
  // Unmounted, it no longer listens to the feed: a new model leaves it as it was.
  expect(listening).toBe(0)
  const drawn = slot('status').innerHTML
  a.feed.update({ ...live, Seq: 2, NewEvents: 3 } as Snapshot)
  expect(a.feed.current()?.model.right[2].value).toBe('3 нові')
  expect(slot('status').innerHTML).toBe(drawn)
  a.stop()
  expect(vi.getTimerCount()).toBe(0)
})

test('one tick computes the overview model once', () => {
  const count = vi.fn(overviewModel)
  const a = app(count)
  const titles: string[] = []
  mountShell(host(), { setTitle: t => titles.push(t), feed: a.feed })
  const { slot } = mount(undefined, a)
  // The snapshot: one model, drawn by the shell and the tab.
  expect(count).toHaveBeenCalledTimes(1)
  expect(titles).toEqual(['Marstek Monitor'])
  expect(slot('status').innerHTML).toContain('Світло')
  vi.advanceTimersByTime(1000)
  expect(count).toHaveBeenCalledTimes(2)
  a.stop()
})

test('empty live series shows the note', async () => {
  const { slot } = mount([])
  await vi.advanceTimersByTimeAsync(0)
  expect(slot('chart-empty').hidden).toBe(false)
  expect(slot('chart-empty').textContent).toBe("Графік з'явиться, щойно надійдуть перші дані.")
  const other = mount()
  await vi.advanceTimersByTimeAsync(0)
  expect(other.slot('chart-empty').hidden).toBe(true)
})

// Owner request (Task 21): «Джерело даних» is a compact status in the app bar.
test('the data source is in the app bar, not in the right column', () => {
  const a = app()
  const bar = host() as unknown as FakeEl
  mountShell(bar as unknown as HTMLElement, { setTitle: () => {}, feed: a.feed })
  const { slot } = mount(undefined, a)
  const source = bar.querySelector('[data-slot="source"]')!
  // The dot is green and pulsing while data arrives; the row's label stays its accessible name.
  expect(source.innerHTML).toContain('<span class="sr-only">Джерело даних: </span>')
  expect(source.innerHTML).toMatch(/<span class="src live" data-tone="good" title="Джерело даних: LAN · 3 с тому">/)
  expect(source.innerHTML).toContain('<span class="src__text">LAN · 3 с тому</span>')
  expect(slot('status').innerHTML).not.toContain('Джерело даних')
  expect(slot('status').innerHTML).not.toContain('Батарея')
  // No link: a red dot, the last data in the tooltip.
  a.feed.update({ ...live, Seq: 2, Link: 'NO_LINK', Last: { ...live.Last!, At: iso(at(14, 57)) } } as Snapshot)
  expect(source.innerHTML).toMatch(/<span class="src" data-tone="bad" title="Джерело даних: немає зв&#39;язку · останні дані о 14:57">/)
  a.stop()
})

// Owner request: «Підтримати автора» is a heart button right before the gear.
test('the app bar has the support heart before the gear and it opens the donate page', async () => {
  const html = frameHTML()
  const heart = /<button type="button" class="iconbtn" data-act="donate">/
  expect(html).toMatch(heart)
  expect(html.search(heart)).toBeLessThan(html.indexOf('href="#settings"'))
  expect(html).toContain('<use href="#i-heart"')
  const a = app()
  const bar = host() as unknown as FakeEl
  const openURL = vi.fn(() => Promise.resolve())
  mountShell(bar as unknown as HTMLElement, { setTitle: () => {}, feed: a.feed, openURL })
  const btn = bar.querySelector('[data-act="donate"]')!
  expect(btn.getAttribute('aria-label')).toBe('Підтримати автора')
  expect(btn.getAttribute('data-tip')).toBe('Підтримати автора')
  btn.dispatch('click')
  expect(openURL).toHaveBeenCalledWith('https://donatello.to/econ11')
  setLocale('en')
  expect(btn.getAttribute('data-tip')).toBe('Support the author')
  expect(btn.getAttribute('aria-label')).toBe('Support the author')
  setLocale('uk')
  a.stop()
})

test('a hidden window forgets the pointer over the chart', () => {
  const { slot } = mount()
  doc.visibilityState = 'hidden'
  doc.dispatch('visibilitychange')
  // The next redraw then opens no tooltip for a pointer that is gone.
  expect(clearChartPointer).toHaveBeenCalledWith(slot('chart'))
})

// ⓘ (copy §15): which key goes where is overviewModel.test.ts; here, that the tab draws them.
test('the right column, the legend and the app bar draw their ⓘ', () => {
  const a = app()
  const bar = host() as unknown as FakeEl
  mountShell(bar as unknown as HTMLElement, { setTitle: () => {}, feed: a.feed })
  const { slot } = mount(undefined, a)
  const status = slot('status').innerHTML
  expect(status).toContain('data-info="info.overview.grid"')
  expect(status).toContain('aria-label="Що означає «Світло»"')
  // The errors row is a link: its ⓘ is a button beside the link, not inside it.
  expect(status).toMatch(/<a class="status__link" href="#errors" data-view="errors" aria-label="Помилки: нових немає">Помилки<\/a><button type="button" class="infotip" data-info="info.errors.new"/)
  expect(slot('legend').innerHTML).toContain('data-info="info.overview.chart.floor"')
  expect(slot('legend').innerHTML).toContain('(10%)')
  a.feed.update({ ...live, Seq: 2, Profile: { ...live.Profile, MinSOC: 25 } } as Snapshot)
  expect(slot('legend').innerHTML).toContain('(25%)')
  // The age moves every second; the ⓘ beside it is drawn apart, so it stays.
  expect(bar.querySelector('[data-slot="source-info"]')!.innerHTML).toContain('data-info="info.overview.source"')
  expect(bar.querySelector('[data-slot="source"]')!.innerHTML).not.toContain('infotip')
  a.stop()
})

test('with «нові» on the Errors tab, the tab link itself carries the explanation (focus opens it, it describes the link)', () => {
  const a = app()
  const bar = host() as unknown as FakeEl
  mountShell(bar as unknown as HTMLElement, { setTitle: () => {}, feed: a.feed })
  a.feed.update({ ...live, NewEvents: 2 } as Snapshot)
  const badge = bar.querySelector('[data-slot="errors-badge"]')!
  const tab = badge.closest('.tab')!
  expect(tab.getAttribute('data-tip')).toMatch(/^Попередження й помилки, що з'явились/)
  // Not on the badge: it is aria-hidden, a description there reaches no one.
  expect(badge.hasAttribute('data-tip')).toBe(false)
  expect(badge.hasAttribute('title')).toBe(false)
  // Nothing new: no tooltip on the tab.
  a.feed.update({ ...live, Seq: 2, NewEvents: 0 } as Snapshot)
  expect(tab.hasAttribute('data-tip')).toBe(false)
  a.stop()
})

// Fix round 2: the tab's tooltip closes when the count clears, and follows the language.
test('an open «нові» tooltip on the Errors tab closes when the count drops to 0', () => {
  vi.stubGlobal('window', { addEventListener() {}, setTimeout, clearTimeout })
  vi.stubGlobal('Element', (doc.body as unknown as object).constructor)
  initTooltips()
  const a = app()
  const bar = host() as unknown as FakeEl
  mountShell(bar as unknown as HTMLElement, { setTitle: () => {}, feed: a.feed })
  a.feed.update({ ...live, NewEvents: 2 } as Snapshot)
  const tab = bar.querySelector('[data-slot="errors-badge"]')!.closest('.tab')! as unknown as Element
  showTip(tab, 'нові')
  expect(tipAnchor()).toBe(tab)
  // Opening the tab marks the journal seen: the count clears under the pointer.
  a.feed.update({ ...live, Seq: 2, NewEvents: 0 } as Snapshot)
  expect(tipAnchor()).toBe(null)
  // Another anchor's tooltip is left alone.
  const other = bar.querySelector('.other') as unknown as Element
  a.feed.update({ ...live, Seq: 3, NewEvents: 1 } as Snapshot)
  showTip(other, 'x')
  a.feed.update({ ...live, Seq: 4, NewEvents: 0 } as Snapshot)
  expect(tipAnchor()).toBe(other)
  hideTip()
  a.stop()
})

test('the «нові» tooltip text follows a language switch, without waiting for the feed', () => {
  // A feed that draws once and never again: the shell's own language switch must do it.
  let draw: (m: ReturnType<typeof overviewModel>, s: Snapshot) => void = () => {}
  const feed = { on: (fn: typeof draw) => { draw = fn; return () => {} } } as unknown as Parameters<typeof mountShell>[1]['feed']
  const bar = host() as unknown as FakeEl
  mountShell(bar as unknown as HTMLElement, { setTitle: () => {}, feed })
  const s = { ...live, NewEvents: 2 } as Snapshot
  draw(overviewModel(s, now), s)
  const tab = bar.querySelector('[data-slot="errors-badge"]')!.closest('.tab')!
  try {
    setLocale('en')
    expect(tab.getAttribute('data-tip')).toBe('Warnings and errors since you last opened this tab. Opening it clears the count.')
    expect(tab.getAttribute('aria-label')).toBe('Errors, 2 new events')
  } finally {
    setLocale('uk')
  }
})
