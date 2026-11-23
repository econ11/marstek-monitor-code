import { expect, test } from 'vitest'
import type { PowerPoint, PowerSeries } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import { powerModel } from '../views/powerModel'
import { keyStep, nearest, powerChartSVG } from './powerChart'

const MIN = 60e3
const at = (h: number, m = 0) => new Date(2026, 10, 22, h, m).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const pt = (t: number, w: number | null): PowerPoint => ({ At: iso(t), W: w })
const series = (points: PowerPoint[], o: Partial<PowerSeries> = {}): PowerSeries => ({
  Start: iso(at(0)), End: iso(new Date(2026, 10, 23).getTime()), Step: MIN * 1e6, Clipped: false,
  Points: points, PeakW: null, PeakAt: null, Outages: [], ...o,
})

const m = powerModel(series([
  pt(at(9, 0), -300), pt(at(9, 1), -320),
  pt(at(9, 3), 200), // alone after a missing minute: a dot
  pt(at(9, 5), 150), pt(at(9, 6), 100),
], { PeakW: 812, PeakAt: iso(at(9, 1)), Outages: [{ Start: iso(at(8)), End: iso(at(10)), Ongoing: false }] }), at(14))

test('one area, green above zero and amber below, broken at the holes', () => {
  const { svg } = powerChartSVG(600, m)
  expect(svg).toContain('clip-path="url(#pw-above)" fill="var(--color-good)"')
  expect(svg).toContain('clip-path="url(#pw-below)" fill="var(--color-warn)"')
  const area = /class="pw-area pw-area--charge" d="([^"]+)"/.exec(svg)![1]
  // Two runs of two or more points, each closed on the zero line; the lone point is a dot.
  expect(area.match(/M/g)).toHaveLength(2)
  expect(area.match(/Z/g)).toHaveLength(2)
  expect(svg.match(/class="pw-dot"/g)).toHaveLength(1)
})

test('outages shaded, the rest of today dimmed, the peak marked, the axis 00:00–24:00', () => {
  const { svg } = powerChartSVG(600, m)
  expect(svg.match(/class="band"/g)).toHaveLength(1)
  expect(svg).toContain('class="pw-future"')
  expect(svg).toContain('>пік 812 Вт</text>')
  expect(svg).toContain('>00:00</text>')
  expect(svg).toContain('>24:00</text>')
})

test('the keyboard walks the points; the pointer picks the nearest', () => {
  expect(keyStep(m, 0, 'ArrowLeft')).toBe(0)
  expect(keyStep(m, 0, 'ArrowRight')).toBe(1)
  expect(keyStep(m, 1, 'End')).toBe(4)
  expect(keyStep(m, 4, 'Home')).toBe(0)
  expect(keyStep(m, 0, 'PageDown')).toBe(4)
  expect(keyStep(m, 0, 'a')).toBeNull()
  expect(nearest(m, at(9, 2) + 40e3)).toBe(2)
})

test('PageUp/PageDown move by time: an hour of minutes, a day of hours, whatever the holes', () => {
  const mins = Array.from({ length: 10 }, (_, i) => pt(at(9, i), -100)).concat(Array.from({ length: 6 }, (_, i) => pt(at(10, i), -100)))
  const d = powerModel(series(mins), at(14))
  expect(keyStep(d, 0, 'PageDown')).toBe(10) // 10:00, not the 60th point
  expect(keyStep(d, 15, 'PageUp')).toBe(5) // 10:05 − 1 h → nearest 09:05
  // Hours of three days with the middle day missing: a day from the first lands on the nearest hour.
  const hours: PowerPoint[] = []
  for (const day of [16, 18]) for (let h = 0; h < 24; h++) hours.push(pt(new Date(2026, 10, day, h).getTime(), 100))
  const w = powerModel(series(hours, { Start: iso(new Date(2026, 10, 16).getTime()), Step: 3600e3 * 1e6 }), at(14))
  expect(keyStep(w, 20, 'PageDown')).toBe(24) // 17.11 20:30 has no point: the nearest is 18.11 00:30
  expect(keyStep(w, 24, 'PageUp')).toBe(23)
})

test('an instant peak beyond the hourly scale is pinned to the bottom edge with an arrow', () => {
  const hours = [pt(new Date(2026, 10, 16, 19).getTime(), -380)]
  const w = powerModel(series(hours, { Start: iso(new Date(2026, 10, 16).getTime()), Step: 3600e3 * 1e6, PeakW: 1210, PeakAt: iso(new Date(2026, 10, 16, 19, 5).getTime()) }), at(14))
  const { svg } = powerChartSVG(600, w)
  expect(svg).toContain('class="pw-peak-arrow"')
  expect(/class="pw-peak" cx="[0-9.]+" cy="([0-9.]+)"/.exec(svg)![1]).toBe((w.height - 26).toFixed(1))
  expect(powerChartSVG(600, m).svg).not.toContain('pw-peak-arrow')
})
