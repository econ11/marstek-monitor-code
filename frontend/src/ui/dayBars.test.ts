import { expect, test } from 'vitest'
import { dayBarsSVG, type BarItem } from './dayBars'

const items: BarItem[] = [
  { key: '2026-11-19', label: 'чт 19.11', hours: 0, kind: 'zero', died: false, tip: '' },
  { key: '2026-11-20', label: 'пт 20.11', hours: 0, kind: 'noData', died: false, tip: '' },
  { key: '2026-11-21', label: 'сб 21.11', hours: 5.4, kind: 'value', died: true, tip: '' },
]
test('no-data day is hatched, zero day is a tick', () => { const svg = dayBarsSVG(600, items); expect(svg).toContain('url(#nodata-hatch)'); expect(svg).toMatch(/class="bar zero"/) })

test('bars SVG: values, «≥», the died dot, selection and the observed-since line', () => {
  const svg = dayBarsSVG(600, [
    { key: 'a', label: 'пн 16.11', hours: 0, kind: 'before', died: false, tip: '' },
    ...items,
    { key: 'b', label: 'нд 22.11', hours: 1.5, kind: 'value', died: false, tip: '', atLeast: true },
  ], { selected: '2026-11-21', observedSince: { index: 1, label: 'спостереження з чт 19.11' } })
  expect(svg).toContain('5:24')
  expect(svg).toContain('≥ 1:30')
  expect(svg).toMatch(/<circle class="died"/)
  expect(svg).toMatch(/class="bar-g is-selected" data-key="2026-11-21"/)
  expect(svg).toContain('спостереження з чт 19.11')
  expect(svg).not.toContain('data-key="a"') // days before monitoring are not drawn
  expect(svg).toContain('немає даних')
  expect(svg).toContain('0 відключень')
  // Only days with outages are buttons.
  expect(svg.match(/role="button"/g)).toHaveLength(2)
})

test('the selected column is announced as pressed', () => {
  const svg = dayBarsSVG(600, items, { selected: '2026-11-21' })
  expect(svg).toContain('data-key="2026-11-21" data-i="2" tabindex="0" role="button" aria-pressed="true"')
  expect(dayBarsSVG(600, items)).toContain('aria-pressed="false"')
})
