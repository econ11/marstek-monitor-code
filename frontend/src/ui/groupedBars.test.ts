import { expect, test } from 'vitest'
import type { EnergyBar } from '../views/energyModel'
import { groupedBarsSVG } from './groupedBars'

const items: EnergyBar[] = [
  { key: '2026-11-18', label: 'ср 18.11', charge: 0, discharge: 0, noData: false, tip: '', before: true },
  { key: '2026-11-19', label: 'чт 19.11', charge: 0, discharge: 0, noData: true, tip: '<b>x</b>' },
  { key: '2026-11-20', label: 'пт 20.11', charge: 4.2, discharge: 3.6, noData: false, tip: '<b>y</b>' },
  { key: '2026-11-21', label: 'сб 21.11', charge: 1.1, discharge: 0, noData: false, tip: '<b>z</b>', today: true },
]

test('a pair per day, ≤ 40 px wide; no data hatched; before monitoring not drawn', () => {
  const svg = groupedBarsSVG(1200, items)
  expect(svg).not.toContain('data-key="2026-11-18"')
  expect(svg).toMatch(/<g class="bar-g" data-key="2026-11-19"[^>]*>.*url\(#energy-hatch\)/)
  expect(svg.match(/class="bar c"/g)).toHaveLength(2)
  // A zero discharge draws nothing, not a 0 px column.
  expect(svg.match(/class="bar d"/g)).toHaveLength(1)
  const hatch = svg.match(/class="bar nodata"[^>]*width="([\d.]+)"/)!
  expect(Number(hatch[1])).toBeLessThanOrEqual(40)
  expect(svg).toContain('aria-label="пт 20.11: заряд 4,2, розряд 3,6 кВт·год"')
  expect(svg).toContain('aria-label="чт 19.11: немає даних"')
  expect(svg).toMatch(/class="strong">сб 21.11/)
})

test('a column not measured yet reads «ще рахуємо», never zero', () => {
  const svg = groupedBarsSVG(600, [{ key: 'a', label: 'сб 21.11', charge: 0, discharge: 0, noData: false, tip: '', pending: true }])
  expect(svg).toContain('aria-label="сб 21.11: ще рахуємо"')
  expect(svg).not.toContain('0,0')
})

test('the scale fits the higher column, with comma decimals', () => {
  const svg = groupedBarsSVG(600, [{ key: 'a', label: 'a', charge: 1.3, discharge: 0.4, noData: false, tip: 't' }])
  expect(svg).toContain('>0,5</text>')
  expect(svg).toContain('>1,5</text>')
  expect(svg).not.toContain('>2</text>')
})
