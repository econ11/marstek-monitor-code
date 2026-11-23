import { afterEach, expect, test } from 'vitest'
import type { PowerPoint, PowerSeries } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import { setLocale } from '../lib/i18n'
import { powerModel } from './powerModel'

afterEach(() => setLocale('uk'))

const MIN = 60e3, HOUR = 3600e3, NS = 1e6
const at = (day: number, h: number, m = 0, month = 10) => new Date(2026, month, day, h, m, 0).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const now = at(22, 14, 40)

const pt = (t: number, w: number | null): PowerPoint => ({ At: iso(t), W: w })

function day(points: PowerPoint[], o: Partial<PowerSeries> = {}, d = 22, month = 10): PowerSeries {
  return {
    Start: iso(at(d, 0, 0, month)), End: iso(at(d + 1, 0, 0, month)), Step: MIN * NS, Clipped: false,
    Points: points, PeakW: null, PeakAt: null, Outages: [], ...o,
  }
}

function week(points: PowerPoint[], o: Partial<PowerSeries> = {}): PowerSeries {
  return {
    Start: iso(at(16, 0)), End: iso(at(23, 0)), Step: HOUR * NS, Clipped: false,
    Points: points, PeakW: null, PeakAt: null, Outages: [], ...o,
  }
}

test('above zero is charge (green), below is discharge (amber), the deadband is idle', () => {
  const m = powerModel(day([pt(at(22, 14, 30), 300), pt(at(22, 14, 31), 12), pt(at(22, 14, 32), -412)]), now)
  expect(m.points.map(p => p.tone)).toEqual(['charge', 'idle', 'discharge'])
  expect(m.points[2].tip).toContain('14:32 · розряд 412 Вт')
  expect(m.points[0].tip).toContain('14:30 · заряд 300 Вт')
  expect(m.points[1].tip).toContain('14:31 · очікування')
  // The plot: charge above the zero line, discharge below it.
  expect(m.y(300)).toBeLessThan(m.y(0))
  expect(m.y(-412)).toBeGreaterThan(m.y(0))
})

test('a missing minute and an unknown value break the line; nothing is drawn as 0', () => {
  const m = powerModel(day([
    pt(at(22, 9, 0), -300), pt(at(22, 9, 1), -300),
    // 09:02 missing (the app was not running)
    pt(at(22, 9, 3), -200),
    pt(at(22, 9, 4), null),
    pt(at(22, 9, 5), 100), pt(at(22, 9, 6), 120),
  ]), now)
  expect(m.runs.map(r => r.map(p => new Date(p.t).getMinutes()))).toEqual([[0, 1], [3], [5, 6]])
  expect(m.runs.flat().some(p => p.w === 0)).toBe(false)
  // Only measured points are focusable.
  expect(m.points.map(p => new Date(p.t).getMinutes())).toEqual([0, 1, 3, 5, 6])
})

test('a measured 0 W is kept: only a missing value is a gap', () => {
  const m = powerModel(day([pt(at(22, 9, 0), 0), pt(at(22, 9, 1), 0)]), now)
  expect(m.runs).toEqual([[{ t: at(22, 9, 0), w: 0 }, { t: at(22, 9, 1), w: 0 }]])
})

test('a week is drawn by hourly averages: day ticks, hour-span tooltips, broken on a missing hour', () => {
  const m = powerModel(week([pt(at(21, 14), 300), pt(at(21, 15), -250.4), pt(at(21, 17), -100)]), now)
  expect(m.hourly).toBe(true)
  expect(m.runs.map(r => r.length)).toEqual([2, 1])
  expect(m.points[0].tip).toContain('21.11, 14:00–15:00 · заряд 300 Вт')
  expect(m.points[0].tip).toContain('середнє за годину')
  expect(m.points[1].tip).toContain('розряд 250 Вт')
  expect(m.ticks.map(k => k.label)).toEqual(['пн 16.11', 'вт 17.11', 'ср 18.11', 'чт 19.11', 'пт 20.11', 'сб 21.11', 'нд 22.11'])
  expect(m.title).toBe('Потужність батареї')
  expect(m.note).toBe('середнє за годину')
})

test('a day is drawn 00:00–24:00 by the clock, 25 hours on 25.10', () => {
  const m = powerModel(day([pt(at(25, 9, 0, 9), -300)], {}, 25, 9), at(26, 12, 0, 9))
  expect(m.t1 - m.t0).toBe(25 * HOUR)
  expect(m.ticks[0]).toEqual({ t: at(25, 0, 0, 9), label: '00:00' })
  expect(m.ticks[m.ticks.length - 1]).toEqual({ t: at(26, 0, 0, 9), label: '24:00' })
  expect(m.hourly).toBe(false)
  expect(m.note).toBeUndefined()
})

test('the discharge peak of the day: a mark on the chart and a caption', () => {
  const m = powerModel(day([pt(at(22, 19, 40), -700)], { PeakW: 812.4, PeakAt: iso(at(22, 19, 40) + 12e3) }), now)
  expect(m.peak).toEqual({ t: at(22, 19, 40) + 12e3, w: 812.4, label: 'пік 812 Вт', caption: 'Пік розряду: 812 Вт о 19:40', pinned: false })
  expect(m.y(-812.4)).toBeLessThanOrEqual(m.y(m.yMin))
  const w = powerModel(week([pt(at(21, 14), -300)], { PeakW: 2500, PeakAt: iso(at(21, 14, 5)) }), now)
  expect(w.peak?.caption).toBe('Пік розряду: 2 500 Вт, 21.11 о 14:05')
  expect(powerModel(day([pt(at(22, 9), 100)]), now).peak).toBeUndefined()
})

test('a point during an outage says «без світла»; there is no «на вихід станції» line', () => {
  const m = powerModel(day([pt(at(22, 7, 59), 300), pt(at(22, 8, 0), -412), pt(at(22, 9, 59), -400), pt(at(22, 10, 0), 200)],
    { Outages: [{ Start: iso(at(22, 8)), End: iso(at(22, 10)), Ongoing: false }] }), now)
  expect(m.points.map(p => p.tip.includes('без світла'))).toEqual([false, true, true, false])
  expect(m.points[1].aria).toBe('08:00 · розряд 412 Вт · без світла')
  expect(m.points.some(p => p.tip.includes('вихід'))).toBe(false)
})

test('hourly points sit at mid-hour; the tooltip names the whole hour', () => {
  const m = powerModel(week([pt(at(21, 14), 300), pt(at(21, 15), 200)], { Outages: [{ Start: iso(at(21, 15)), End: iso(at(21, 16)), Ongoing: false }] }), now)
  expect(m.points.map(p => p.t)).toEqual([at(21, 14, 30), at(21, 15, 30)])
  expect(m.runs[0].map(p => p.t)).toEqual([at(21, 14, 30), at(21, 15, 30)])
  expect(m.points[0].tip).toContain('21.11, 14:00–15:00')
  // An hour in the outage band (by its middle) says so.
  expect(m.points[1].tip).toContain('без світла')
})

test('hourly: the scale follows the hourly values, an instant peak beyond it is pinned to the edge', () => {
  const m = powerModel(week([pt(at(21, 14), 300), pt(at(21, 19), -380)], { PeakW: 1210, PeakAt: iso(at(21, 19, 5)) }), now)
  expect(m.yMin).toBe(-400)
  expect(m.peak?.pinned).toBe(true)
  expect(m.peak?.label).toBe('пік 1 210 Вт')
  const fits = powerModel(week([pt(at(21, 19), -380)], { PeakW: 390, PeakAt: iso(at(21, 19, 5)) }), now)
  expect(fits.peak?.pinned).toBe(false)
})

test('English: en-GB dates, «W», the same rules', () => {
  setLocale('en')
  const m = powerModel(day([pt(at(22, 14, 32), -2412)], { PeakW: 2500, PeakAt: iso(at(22, 14, 32)), Outages: [{ Start: iso(at(22, 14)), End: iso(at(22, 15)), Ongoing: false }] }), now)
  expect(m.points[0].tip).toContain('14:32 · discharging 2,412 W')
  expect(m.points[0].tip).toContain('grid off')
  expect(m.peak?.caption).toBe('Peak discharge: 2,500 W at 14:32')
  expect(m.title).toBe('Battery power')
  const w = powerModel(week([pt(at(21, 14), 300)]), now)
  expect(w.points[0].tip).toContain('21 Nov, 14:00–15:00 · charging 300 W')
})

test('outages are shaded bands; an empty period says so; a clipped one says it', () => {
  const m = powerModel(day([pt(at(22, 9), -300)], { Outages: [{ Start: iso(at(22, 8)), End: iso(at(22, 10)), Ongoing: false }] }), now)
  expect(m.bands).toEqual([{ start: at(22, 8), end: at(22, 10) }])
  expect(m.empty).toBeUndefined()
  expect(powerModel(day([]), now).empty).toBe('За цей період даних немає.')
  expect(powerModel(day([pt(at(22, 9), null)]), now).empty).toBe('За цей період даних немає.')
  const c = powerModel(week([pt(at(21, 14), 300)], { Clipped: true }), now)
  expect(c.note).toBe('середнє за годину · показано останній 31 день')
})

test('the y scale is round and always holds zero', () => {
  const m = powerModel(day([pt(at(22, 9), 730), pt(at(22, 9, 1), -1890)]), now)
  expect(m.yMax).toBe(800)
  expect(m.yMin).toBe(-2000)
  const idle = powerModel(day([pt(at(22, 9), 5)]), now)
  expect(idle.yMax).toBeGreaterThan(0)
  expect(idle.yMin).toBeLessThan(0)
})
