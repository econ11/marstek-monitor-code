import { expect, test } from 'vitest'
import type { RingProps } from '../views/overviewModel'
import { gaugeHTML } from './gauge'

const ring = (o: Partial<RingProps> = {}): RingProps =>
  ({ key: 'coverage', title: 'Покриття', value: '80%', sub: '1 розрядилась', fill: 0.8, tone: 'good', stale: false, ...o })

test('the ⓘ sits next to the title, named after it, outside the title text', () => {
  const html = gaugeHTML(ring({ info: { key: 'info.outages.coverage' } }))
  expect(html).toMatch(/<figcaption class="gauge__title"><span class="gauge__label"><span class="nowrap">Покриття<button type="button" class="infotip" data-info="info.outages.coverage" data-info-at="ring:coverage"/)
  // A long title wraps before its last word, never between the word and the ⓘ.
  const long = gaugeHTML(ring({ key: 'noLight', title: 'Без світла сьогодні', info: { key: 'info.overview.offToday' } }))
  expect(long).toContain('<span class="gauge__label">Без світла <span class="nowrap">сьогодні<button')
  expect(html).toContain('aria-label="Що означає «Покриття»"')
  expect(gaugeHTML(ring())).not.toContain('infotip')
})

test('the ⓘ text has the ring arguments', () => {
  const html = gaugeHTML(ring({ key: 'power', title: 'Потужність', info: { key: 'info.overview.power', args: { max: '2 500' } } }))
  expect(html).toContain('«з 2 500 Вт макс.»')
})
