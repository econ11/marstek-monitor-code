import { afterEach, expect, test, vi } from 'vitest'
import { host, installFakeDom, type FakeEl } from '../testing/fakeDom'
import { clearChartPointer, renderSocChart, socChartSVG, tooltipAt, type ChartBand, type ChartPoint } from './socChart'
import { hideTip, showTip } from './tooltip'

vi.mock('./tooltip', async orig => ({ ...(await orig<typeof import('./tooltip')>()), showTip: vi.fn(), hideTip: vi.fn() }))
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks() })

const M = 60e3, H = 3600e3
const at = (h: number, m = 0, day = 21) => new Date(2026, 10, day, h, m, 0).getTime()
const now = at(15, 17)

// One measurement a minute from 09:00, 100% falling by 0.1 a minute.
function points(from: number, to: number, skip?: [number, number]): ChartPoint[] {
  const out: ChartPoint[] = []
  for (let t = from; t <= to; t += M) {
    if (skip && t > skip[0] && t < skip[1]) continue
    out.push({ t, soc: 100 - ((t - from) / M) * 0.1 })
  }
  return out
}
const subpaths = (svg: string) => (svg.match(/<path d="([^"]*)" fill="none"/)?.[1].match(/M/g) ?? []).length

test('the station minimum is a labelled line at the floor, 10% by default', () => {
  const { svg, geom } = socChartSVG(800, points(at(9), now), [], { range: '24h', now })
  expect(svg).toContain('class="floor"')
  expect(svg).toContain('class="floor-label"')
  expect(svg).toContain('>10%<')
  expect(svg).toContain(`y1="${geom.y(10)}"`)
})

test('the floor line follows a changed station minimum', () => {
  const { svg, geom } = socChartSVG(800, points(at(9), now), [], { range: '24h', now, floor: 25 })
  expect(svg).toContain('>25%<')
  expect(svg).not.toContain('>10%<')
  expect(svg).toContain(`y1="${geom.y(25)}"`)
})

test('the notification threshold is not on the chart; an unknown floor draws no line', () => {
  const { svg } = socChartSVG(800, points(at(9), now), [], { range: '24h', now })
  expect(svg).not.toContain('threshold')
  expect(socChartSVG(800, points(at(9), now), [], { range: '24h', now, floor: 0 }).svg).not.toContain('class="floor')
})

test('an outage is a band labelled with its duration; an ongoing one says so', () => {
  const bands: ChartBand[] = [
    { start: at(5), end: at(7, 5), kind: 'outage', label: 'overview.chart.band.outage' },
    { start: at(14), end: now, kind: 'outage', label: 'overview.chart.band.outage.ongoing' },
  ]
  const { svg } = socChartSVG(800, points(at(9), now), bands, { range: '24h', now })
  expect(svg.match(/class="band"/g)).toHaveLength(2)
  expect(svg).toContain('>без світла 2 год 05 хв<')
  expect(svg).toContain('>без світла 1 год 17 хв, триває<')
})

test('labels of bands that would collide fall back to the duration, then to nothing', () => {
  const bands: ChartBand[] = [
    { start: at(10), end: at(10, 40), kind: 'outage', label: 'overview.chart.band.outage' },
    { start: at(12, 30), end: at(13, 10), kind: 'outage', label: 'overview.chart.band.outage' },
    { start: at(13, 15), end: at(13, 30), kind: 'outage', label: 'overview.chart.band.outage' },
  ]
  const { svg } = socChartSVG(800, points(at(9), now), bands, { range: '24h', now })
  expect(svg).toContain('>без світла 40 хв<')
  expect(svg.match(/class="band-label"/g)).toHaveLength(2)
  expect(svg).toContain('>40 хв<')
  expect(svg).not.toContain('>15 хв<')
})

test('labels never overlap: the last value wins over a band label at the same spot', () => {
  // As seen in the app: a one-minute outage and a 100% tail label, both at the right edge.
  const last = at(15, 5)
  const bands: ChartBand[] = [{ start: at(14, 58), end: at(14, 59), kind: 'outage', label: 'overview.chart.band.outage' }]
  const flat = points(at(14), last).map(p => ({ ...p, soc: 100 }))
  const { svg } = socChartSVG(800, flat, bands, { range: '24h', now, lastData: last, tail: 'nolink', observedSince: at(14) })
  expect(svg).toContain('>100% о 15:05<')
  expect(svg).not.toContain('class="band-label"')
  const boxes = Array.from(svg.matchAll(/<text class="(?:band-label|gap-label|floor-label)" x="([\d.]+)" y="([\d.]+)">([^<]*)</g))
    .map(m => ({ x0: +m[1], x1: +m[1] + m[3].length * 6.1, y: +m[2] }))
  expect(boxes.length).toBeGreaterThanOrEqual(3)
  for (const a of boxes) for (const b of boxes) {
    if (a !== b && Math.abs(a.y - b.y) < 14) expect(a.x1 <= b.x0 || b.x1 <= a.x0).toBe(true)
  }
})

test('the line is broken across a gap, which is framed and named', () => {
  const gap: ChartBand = { start: at(11), end: at(13), kind: 'gap', label: 'gap.label.pcSleep' }
  const whole = socChartSVG(800, points(at(9), now), [], { range: '24h', now })
  const broken = socChartSVG(800, points(at(9), now, [gap.start, gap.end]), [gap], { range: '24h', now })
  expect(subpaths(whole.svg)).toBe(1)
  expect(subpaths(broken.svg)).toBe(2)
  expect(broken.svg).toContain('class="gap"')
  expect(broken.svg).toContain('>ПК спав<')
})

test('measurements far apart are not joined even without a recorded gap', () => {
  const { svg } = socChartSVG(800, points(at(9), now, [at(11), at(11, 30)]), [], { range: '24h', now })
  expect(subpaths(svg)).toBe(2)
})

test('stale: grey line that stops at the last measurement, outage only «at least»', () => {
  const last = at(15, 15)
  const bands: ChartBand[] = [{ start: at(11, 50), end: now, kind: 'outage', label: 'overview.chart.band.outage.ongoing' }]
  const { svg } = socChartSVG(800, points(at(9), last), bands, { range: '24h', now, lastData: last, tail: 'stale' })
  expect(svg).toContain('stroke="var(--color-text-subtle)"')
  expect(svg).not.toContain('url(#socfill-soc)')
  expect(svg).toContain('>без світла ≥ 3 год 25 хв<')
  expect(svg).not.toContain('gap--open')
  expect(svg).toContain('о 15:15<')
})

test('no link: an open frame runs from the last measurement to now', () => {
  const last = at(14, 57)
  const none = socChartSVG(800, points(at(9), last), [], { range: '24h', now, lastData: last, tail: 'nolink' })
  expect(none.svg.match(/gap--open/g)).toHaveLength(1)
  expect(none.geom.gaps[0]).toMatchObject({ start: last, end: now, label: 'gap.label' })
  // The gap the core already recorded is not drawn twice.
  const recorded: ChartBand = { start: last, end: now, kind: 'gap', label: 'gap.label.noLink' }
  const once = socChartSVG(800, points(at(9), last), [recorded], { range: '24h', now, lastData: last, tail: 'nolink' })
  expect(once.svg.match(/<rect class="gap/g)).toHaveLength(1)
  // The reason is written inside the frame once the frame is wide enough for it.
  const dead = socChartSVG(800, points(at(9), at(10)), [], { range: '24h', now, lastData: at(10), tail: 'dead' })
  expect(dead.svg).toContain('>ймовірно, розрядилась<')
})

test('before the first measurement the axis is empty and says since when', () => {
  const { svg } = socChartSVG(800, points(at(9), now), [], { range: '24h', now, observedSince: at(9) })
  expect(svg).toContain('>спостереження з 09:00<')
  const week = socChartSVG(800, points(at(9), now), [], { range: '7d', now, observedSince: at(9, 0, 17) })
  expect(week.svg).toContain('>спостереження з 17.11<')
})

test('axis: clock every three hours with the day at midnight; days on the longer ranges', () => {
  const day = socChartSVG(800, [], [], { range: '24h', now }).svg
  for (const label of ['18:00', '21:00', 'сб 21.11', '03:00', '15:00']) expect(day).toContain(`>${label}<`)
  const week = socChartSVG(800, [], [], { range: '7d', now }).svg
  for (const label of ['нд 15.11', 'пн 16.11', 'сб 21.11']) expect(week).toContain(`>${label}<`)
  const month = socChartSVG(800, [], [], { range: '30d', now }).svg
  for (const label of ['сб 21.11', 'вт 17.11', 'пт 13.11']) expect(month).toContain(`>${label}<`)
  expect(month).not.toContain('>пт 20.11<')
})

test('skeleton has axes and no data marks', () => {
  const { svg } = socChartSVG(800, points(at(9), now), [], { range: '24h', now, skeleton: true })
  expect(svg).toContain('skel-fill')
  expect(svg).not.toContain('class="floor"')
})

test('a day of 5-second samples is thinned to about a point per pixel', () => {
  const dense: ChartPoint[] = []
  for (let t = now - 24 * H; t <= now; t += 5e3) dense.push({ t, soc: 50 })
  const { geom } = socChartSVG(800, dense, [], { range: '24h', now })
  expect(geom.pts.length).toBeLessThan(1100)
  expect(geom.pts.length).toBeGreaterThan(700)
})

test('thinning does not break the line: a week of one-minute points stays one line', () => {
  const week = points(now - 6 * 24 * H, now)
  const { svg, geom } = socChartSVG(800, week, [], { range: '7d', now })
  expect(subpaths(svg)).toBe(1)
  expect(geom.pts.length).toBeLessThan(1100)
  expect(geom.pts[geom.pts.length - 1].t).toBe(week[week.length - 1].t)
})

test('tooltip: a point, a point inside an outage, a gap, the stale tail', () => {
  const bands: ChartBand[] = [
    { start: at(10), end: at(12), kind: 'outage', label: 'overview.chart.band.outage' },
    { start: at(13), end: at(14), kind: 'gap', label: 'gap.label.appOff' },
  ]
  const last = at(15, 15)
  const { geom } = socChartSVG(800, points(at(9), last, [at(13), at(14)]), bands, { range: '24h', now, lastData: last, tail: 'stale' })
  const on = tooltipAt(geom, at(9, 30))
  expect(on.point?.t).toBeCloseTo(at(9, 30), -5)
  expect(on.html).toContain('97%')
  expect(on.html).toContain('Світло є')
  expect(tooltipAt(geom, at(11)).html).toContain('Без світла 10:00–12:00 · 2 год')
  const gap = tooltipAt(geom, at(13, 30))
  expect(gap.point).toBeNull()
  expect(gap.html).toContain('Додаток не працював')
  expect(gap.html).toContain('Додаток не працював з 13:00 до 14:00. Що було зі світлом, невідомо.')
  expect(tooltipAt(geom, at(15, 16, 21) + 30e3).html).toContain('Дані застарілі — останні о 15:15')
})

test('a restart hole under 5 min breaks the line where the backend marks it', () => {
  // I-4: the PC rebooted 12:00–12:03. Under 5 min is no gap (no band, no
  // card), but the backend puts a point without a value before the first
  // measurement after the hole, and the line must not be drawn across it.
  const hole: [number, number] = [at(12), at(12, 4)]
  const pts = points(at(9), now, hole)
  const resumed = pts.findIndex(p => p.t === at(12, 4))
  const marked = [...pts.slice(0, resumed), { t: at(12, 4), soc: null }, ...pts.slice(resumed)]
  const { svg, geom } = socChartSVG(800, marked, [], { range: '24h', now })
  expect(subpaths(svg)).toBe(2)
  expect(svg).not.toContain('class="gap')
  // Each run ends and starts on a real measurement: nothing is interpolated.
  const i = geom.pts.findIndex(p => p.soc == null)
  expect(geom.pts[i - 1].t).toBe(at(12))
  expect(geom.pts[i + 1].t).toBe(at(12, 4))
  // A hole of the same length while the app was running is only a delay (flows §2.2): joined.
  expect(subpaths(socChartSVG(800, pts, [], { range: '24h', now }).svg)).toBe(1)
})

// Task 21 (plan 1 deferred): the overview chart polish.

test('stale tail tooltip names the day', () => {
  // The last measurement was yesterday at 23:40; it is 00:10 now.
  const now = at(0, 10, 22), last = at(23, 40, 21)
  const { geom } = socChartSVG(800, points(at(20, 0, 21), last), [], { range: '24h', now, lastData: last, tail: 'stale' })
  const html = tooltipAt(geom, last + 10 * M).html
  // The time once: the title has it, the small part names the day.
  expect(html).toContain('Дані застарілі — останні о 23:40<small>вчора</small>')
  expect(html.match(/23:40/g)).toHaveLength(1)
  // Older: the date is in the title, nothing more.
  const older = socChartSVG(800, points(at(20, 0, 19), at(23, 40, 19)), [], { range: '7d', now, lastData: at(23, 40, 19), tail: 'stale' })
  expect(tooltipAt(older.geom, now - M).html).toContain('Дані застарілі — останні 19.11 о 23:40<small></small>')
})

test('single point segment is drawn', () => {
  const pt: ChartPoint = { t: at(12), soc: 60 }
  expect(socChartSVG(600, [pt], [], { range: '24h', now }).svg).toMatch(/<circle class="soc-dot"/)
  // A lone measurement between two breaks is a dot too, not nothing.
  const lone = [...points(at(9), at(10)), { t: at(11), soc: 80 }, ...points(at(12), now)]
  expect(socChartSVG(800, lone, [], { range: '24h', now }).svg.match(/class="soc-dot"/g)).toHaveLength(1)
})

test('minute redraw keeps the open tooltip', () => {
  installFakeDom()
  const el = host(800)
  const hit = () => (el.firstElementChild as unknown as FakeEl).querySelector('.hit')!
  renderSocChart(el, points(at(9), now), [], { range: '24h', now })
  hit().dispatch('pointermove', { clientX: 600 })
  expect(showTip).toHaveBeenCalledTimes(1)
  // The refresh a minute later replaces the markup: the tooltip opens again
  // where the pointer is, after the redraw closed it.
  renderSocChart(el, points(at(9), now + M), [], { range: '24h', now: now + M })
  expect(showTip).toHaveBeenCalledTimes(2)
  const lastHide = Math.max(...vi.mocked(hideTip).mock.invocationCallOrder)
  expect(vi.mocked(showTip).mock.invocationCallOrder[1]).toBeGreaterThan(lastHide)
  // Once the pointer has left, a redraw leaves it closed.
  hit().dispatch('pointerleave')
  renderSocChart(el, points(at(9), now + 2 * M), [], { range: '24h', now: now + 2 * M })
  expect(showTip).toHaveBeenCalledTimes(2)
})

test('a forgotten pointer opens no tooltip on the next redraw', () => {
  installFakeDom()
  const el = host(800)
  renderSocChart(el, points(at(9), now), [], { range: '24h', now })
  ;(el.firstElementChild as unknown as FakeEl).querySelector('.hit')!.dispatch('pointermove', { clientX: 600 })
  clearChartPointer(el)
  renderSocChart(el, points(at(9), now + M), [], { range: '24h', now: now + M })
  expect(showTip).toHaveBeenCalledTimes(1)
})
