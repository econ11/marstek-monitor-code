import { expect, test } from 'vitest'
import type { CapMonth } from '../views/batteryModel'
import { capChartSVG } from './capChart'

const m = (label: string, value: number, approx: boolean): CapMonth => ({ label, value, approx, tip: '', a11y: `${label}: ${value}` })
const months = [m('черв', 5.1, true), m('лип', 5.06, true), m('серп', 5.03, false), m('вер', 5.01, false)]

test('approximate months are hollow, and the line into or out of them is dashed', () => {
  const svg = capChartSVG(600, months, 5.12)
  expect(svg.match(/class="dot approx"/g)).toHaveLength(2)
  expect(svg.match(/class="dot"/g)).toHaveLength(2)
  // черв→лип and лип→серп touch an approximate month; серп→вер does not.
  expect(svg.match(/class="cap-line cap-line--approx"/g)).toHaveLength(2)
  expect(svg.match(/class="cap-line"/g)).toHaveLength(1)
})

test('the rated line, the values and a focusable point per month', () => {
  const svg = capChartSVG(600, months, 5.12, { label: 'Ємність по місяцях' })
  expect(svg).toContain('aria-label="Ємність по місяцях"')
  expect(svg).toContain('class="nominal"')
  expect(svg).toContain('номінал 5,12')
  expect(svg).toContain('>5,03<')
  expect(svg.match(/class="hitpt" data-i="\d" [^>]*tabindex="0"/g)).toHaveLength(4)
  expect(svg).toContain('aria-label="вер: 5.01"')
  expect(svg).toMatch(/class="strong">вер</)
})

test('the scale does not start at zero', () => {
  const svg = capChartSVG(600, months, 5.12)
  expect(svg).not.toContain('>0,0<')
  expect(svg).toContain('>5,0<')
})

test('one month is centred, with no line', () => {
  const svg = capChartSVG(600, [m('лист', 4.97, true)], 5.12)
  expect(svg).not.toContain('cap-line')
  expect(svg).toMatch(/<circle class="dot approx" cx="314"/)
})

test('no months and no rated capacity: an empty chart, never NaN', () => {
  expect(capChartSVG(600, [], 0)).not.toContain('NaN')
  expect(capChartSVG(600, [], 5.12)).not.toContain('NaN')
  expect(capChartSVG(600, [m('лист', 4.97, false)], 0)).not.toContain('NaN')
})

// 24 months from Dec 2025: 2 years on the axis.
const SHORT = ['січ', 'лют', 'бер', 'квіт', 'трав', 'черв', 'лип', 'серп', 'вер', 'жовт', 'лист', 'груд']
const long = Array.from({ length: 24 }, (_, i) => {
  const at = new Date(2025, 11 + i, 1).getTime()
  return { ...m(SHORT[new Date(at).getMonth()], 5 - i * 0.005, false), at }
})
const axisLabels = (svg: string) => Array.from(svg.matchAll(/<text x="[\d.]+" y="\d+" text-anchor="middle"(?: class="strong")?>([^<]+)<\/text>/g), x => x[1])

test('month labels thin out when they would overlap, keeping the last', () => {
  const narrow = axisLabels(capChartSVG(420, long, 5.12))
  expect(narrow.length).toBeLessThan(24)
  expect(narrow[narrow.length - 1]).toBe('лист')
  expect(capChartSVG(420, long, 5.12).match(/class="val"/g)!.length).toBe(narrow.length)
  expect(axisLabels(capChartSVG(4000, long, 5.12))).toHaveLength(24)
})

test('the year goes on the first label and wherever the year changes', () => {
  const wide = axisLabels(capChartSVG(4000, long, 5.12))
  expect(wide[0]).toBe('груд 2025')
  expect(wide[1]).toBe('січ 2026')
  expect(wide[2]).toBe('лют')
  expect(wide[13]).toBe('січ 2027')
  const narrow = axisLabels(capChartSVG(420, long, 5.12))
  expect(narrow[0]).toMatch(/ 202[56]$/)
  expect(narrow.filter(l => / 2027$/.test(l))).toHaveLength(1)
})
