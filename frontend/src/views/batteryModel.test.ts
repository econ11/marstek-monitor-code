import { afterEach, expect, test } from 'vitest'
import type { Summary } from '../../bindings/github.com/econ11/marstek-monitor-code/core/health/models'
import { setLocale } from '../lib/i18n'
import { batteryModel, HEALTH_BAD_PCT } from './batteryModel'

afterEach(() => setLocale('uk'))

const month = (y: number, m: number) => new Date(y, m - 1, 1).toISOString()
const base: Summary = {
  HealthPct: null, ActualWh: null, Approx: false,
  Cycles: 0.4, CycleLife: 6000, RatedDoD: 0.9, NominalWh: 5120,
  MaxDelta: 34, Months: null,
}
const noEst: Summary = base
const withEst: Summary = {
  ...base, HealthPct: 97.07, ActualWh: 4970, Cycles: 142.3, MaxDelta: 88,
  Months: [
    { Month: month(2026, 10), MedianWh: 5010, Count: 2, Approx: true },
    { Month: month(2026, 11), MedianWh: 4970, Count: 8, Approx: false },
  ],
}

test('pending health', () => { const m = batteryModel(noEst); expect(m.rings[0].value).toBe('—'); expect(m.rings[0].sub).toBe('ще рахуємо'); expect(m.empty!.progress).toBe('Найбільший розряд поки 34 п.п. з потрібних 50.') })
test('cycles are an estimate', () => { const r = batteryModel(withEst).rings[1]; expect(r.value).toBe('≈ 142'); expect(r.sub).toBe('з понад 6000 при 90% DoD · оцінка') })
test('two rings, health and cycles, and no note about the temperature', () => { const m = batteryModel(withEst); expect(m.rings.map(r => r.key)).toEqual(['health', 'cycles']); expect(m).not.toHaveProperty('hiddenNote') })

test('pending health is never 0 and has no fill', () => {
  const m = batteryModel(noEst)
  expect(m.rings[0]).toMatchObject({ key: 'health', title: "Здоров'я", fill: 0, tone: 'muted' })
  expect(m.empty).toMatchObject({ title: 'Ще замало даних' })
  expect(m.empty!.body).toMatch(/^Оцінка ємності/)
  expect(m.chart.months).toEqual([])
})
test('no progress line before any discharge', () => expect(batteryModel({ ...noEst, MaxDelta: 0 }).empty!.progress).toBeUndefined())

test('health from the estimate', () => {
  const m = batteryModel(withEst)
  expect(m.empty).toBeUndefined()
  expect(m.rings[0]).toMatchObject({ value: '97%', sub: '4,97 з 5,12 кВт·год', tone: 'good' })
  expect(m.rings[0].fill).toBeCloseTo(0.9707, 4)
})
test('a preliminary health says so', () => {
  const r = batteryModel({ ...withEst, Approx: true, HealthPct: 108 }).rings[0]
  expect(r).toMatchObject({ value: '≈ 108%', sub: 'попередня оцінка', fill: 1 })
})
test(`health is red below ${HEALTH_BAD_PCT}%`, () => {
  expect(HEALTH_BAD_PCT).toBe(70)
  expect(batteryModel({ ...withEst, HealthPct: 69.4 }).rings[0].tone).toBe('bad')
  expect(batteryModel({ ...withEst, HealthPct: 70 }).rings[0].tone).toBe('good')
})

test('cycles fill the rated life', () => {
  const r = batteryModel(withEst).rings[1]
  expect(r).toMatchObject({ key: 'cycles', title: 'Ресурс циклів', tone: 'info' })
  expect(r.fill).toBeCloseTo(142.3 / 6000, 6)
})
test('unknown cycles read «—», never 0', () => {
  const r = batteryModel({ ...withEst, Cycles: null, CycleLife: 0, RatedDoD: 0 }).rings[1]
  expect(r).toMatchObject({ value: '—', fill: 0, sub: '' })
})

test('capacity by month', () => {
  const c = batteryModel(withEst).chart
  expect(c.nominal).toBe(5.12)
  expect(c.months.map(m => [m.value, m.approx])).toEqual([[5.01, true], [4.97, false]])
  expect(c.months[1].label).toBe('лист')
  expect(c.months[1].tip).toContain('листопад 2026: 4,97 кВт·год · розрядів у розрахунку: 8')
  expect(c.months[0].tip).toContain('Поки що оцінка спирається на один-два розряди')
  expect(c.months[1].tip).not.toContain('Поки що')
})

test('en', () => {
  setLocale('en')
  const m = batteryModel(withEst)
  expect(m.rings[1]).toMatchObject({ value: '≈ 142', sub: 'of 6000+ at 90% DoD · estimate' })
  expect(m.rings[0].sub).toBe('4.97 of 5.12 kWh')
  expect(m.chart.months[1].tip).toContain('November 2026: 4.97 kWh · based on 8 discharges')
  expect(batteryModel(noEst).empty!.progress).toBe('Largest discharge so far: 34 points of the 50 needed.')
  expect(batteryModel(noEst).rings[0].sub).toBe('still estimating')
})

test('cycles: one decimal below 10, whole from 10 (Ruling: fix round 1)', () => {
  const ring = (c: number) => batteryModel({ ...withEst, Cycles: c }).rings[1].value
  expect(batteryModel(noEst).rings[1].value).toBe('≈ 0,4')
  expect(ring(0.04)).toBe('≈ 0')
  expect(ring(9.96)).toBe('≈ 9,9')
  expect(ring(10.7)).toBe('≈ 10')
  expect(ring(1500.2)).toBe('≈ 1 500') // uk groups with a no-break space
  setLocale('en')
  expect(ring(3.45)).toBe('≈ 3.4')
})

// ⓘ (copy §15).
test('the health ⓘ: pending, preliminary, else the plain one', () => {
  expect(batteryModel(noEst).rings[0].info).toEqual({ key: 'info.battery.health.pending' })
  expect(batteryModel({ ...withEst, Approx: true }).rings[0].info).toEqual({ key: 'info.battery.health.prelim' })
  expect(batteryModel(withEst).rings[0].info).toEqual({ key: 'info.battery.health' })
})
test('the cycles ⓘ takes the rated life, the depth and one cycle from the summary', () => {
  expect(batteryModel(withEst).rings[1].info).toEqual({ key: 'info.battery.cycles', args: { rated: '6000', dod: 90, cycleKWh: '4,6' } })
  setLocale('en')
  expect(batteryModel(withEst).rings[1].info?.args).toEqual({ rated: '6000', dod: 90, cycleKWh: '4.6' })
  // Without a rated life the ring says nothing of it, and neither does the ⓘ.
  expect(batteryModel({ ...withEst, CycleLife: 0 }).rings[1].info).toBeUndefined()
})
