import { expect, test } from 'vitest'
import type { TimelineModel } from '../views/outagesModel'
import { dayTimelineSVG } from './dayTimeline'

const at = (h: number, m = 0, day = 21, month = 10) => new Date(2026, month, day, h, m).getTime()
const now = at(15, 17)
const W = 838 // a plot of 800 px between the 34 px axis and the 4 px edge (hi-fi)

function tl(o: Partial<TimelineModel> = {}): TimelineModel {
  return {
    start: at(0), end: at(0, 0, 22), now, observedFrom: at(0),
    items: [
      { key: 'o1', kind: 'outage', start: at(6, 10), end: at(8, 10), label: '06:10–08:10 · 2 год', tip: '<b>t</b>', aria: 'Без світла 06:10–08:10 · 2 год' },
      { key: 'g4', kind: 'gap', start: at(3, 10), end: at(4, 20), tip: 'g', aria: 'немає даних · ПК спав' },
      { key: 'o2', kind: 'outage', start: at(11, 50), end: now, label: '11:50–зараз · 3 год 27 хв', tip: 'o', aria: 'a' },
    ],
    ...o,
  }
}
const xOf = (svg: string, label: string) => Number(new RegExp(`<text x="([0-9.]+)"[^>]*>${label}<`).exec(svg)?.[1])

test('hours from local midnight every three hours, 00:00 to 24:00', () => {
  const svg = dayTimelineSVG(W, tl())
  for (const h of ['00:00', '03:00', '06:00', '09:00', '12:00', '15:00', '18:00', '21:00', '24:00']) expect(svg).toContain(`>${h}<`)
  expect(xOf(svg, '00:00')).toBe(34)
  expect(xOf(svg, '12:00')).toBe(434)
  expect(xOf(svg, '24:00')).toBe(834)
})

test('a DST day is 25 hours long: the hours after the change sit one hour further right', () => {
  const day: TimelineModel = { start: at(0, 0, 25, 9), end: at(0, 0, 26, 9), now: at(15, 0, 25, 9), observedFrom: at(0, 0, 25, 9), items: [] }
  const svg = dayTimelineSVG(W, day)
  expect(xOf(svg, '06:00')).toBeCloseTo(34 + (7 / 25) * 800, 1)
  expect(xOf(svg, '24:00')).toBe(834)
})

test('outages are focusable spans with their label; gaps are hatched; now is marked', () => {
  const svg = dayTimelineSVG(W, tl())
  expect(svg).toMatch(/<g class="bar-g tl-span" data-key="o1" data-i="0" tabindex="0" role="img" aria-label="Без світла 06:10–08:10 · 2 год">/)
  expect(svg.match(/<rect class="bar"/g)).toHaveLength(2)
  expect(svg).toContain('>06:10–08:10 · 2 год<')
  expect(svg).toMatch(/<g class="bar-g tl-span" data-key="g4" data-i="1" tabindex="0" role="img"[^>]*><rect class="bar-hit"[^>]*\/><rect class="bar nodata" fill="url\(#nodata-hatch\)"/)
  expect(svg).toContain('class="tl-now"')
  expect(svg).toContain('>зараз<')
  // The rest of the day is still to come: dimmed.
  expect(svg).toContain('class="tl-future"')
})

test('the day before monitoring began is not drawn as light', () => {
  const svg = dayTimelineSVG(W, tl({ observedFrom: at(9) }))
  expect(svg).toMatch(/<rect class="tl-base" x="334\.0"/)
})

test('labels that would collide: the later one is left out', () => {
  const close = tl({ items: [
    { key: 'o1', kind: 'outage', start: at(6), end: at(6, 30), label: '06:00–06:30 · 30 хв', tip: '', aria: '' },
    { key: 'o2', kind: 'outage', start: at(7), end: at(7, 20), label: '07:00–07:20 · 20 хв', tip: '', aria: '' },
  ] })
  const svg = dayTimelineSVG(W, close)
  expect(svg).toContain('>06:00–06:30 · 30 хв<')
  expect(svg).not.toContain('>07:00–07:20 · 20 хв<')
})

test('spring forward: a 23-hour day; the 03:00 line is at 04:00 and says so', () => {
  // 29.03.2026, Europe/Kyiv: clocks go from 03:00 to 04:00.
  const day: TimelineModel = { start: at(0, 0, 29, 2), end: at(0, 0, 30, 2), now: at(15, 0, 29, 2), observedFrom: at(0, 0, 29, 2), items: [] }
  const svg = dayTimelineSVG(W, day)
  const labels = Array.from(svg.matchAll(/text-anchor="middle">([0-9]{2}:[0-9]{2})</g), m => m[1])
  expect(labels).toEqual(['00:00', '04:00', '06:00', '09:00', '12:00', '15:00', '18:00', '21:00', '24:00'])
  expect(xOf(svg, '04:00')).toBeCloseTo(34 + (3 / 23) * 800, 1)
  expect(xOf(svg, '06:00')).toBeCloseTo(34 + (5 / 23) * 800, 1)
})

test('timeline spans are tooltip-only: no pointer cursor', () => {
  expect(dayTimelineSVG(W, tl())).toContain('class="bar-g tl-span"')
})
