import { afterEach, expect, test } from 'vitest'
import type { Bucket, Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import { setLocale } from '../lib/i18n'
import { energyModel } from './energyModel'

afterEach(() => setLocale('uk'))

const M = 60e3, NS = 1e6
const at = (day: number, h: number, m = 0, month = 10) => new Date(2026, month, day, h, m, 0).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const now = at(21, 14, 30)

// Days from..to of November; today (21) ends at now, as core/history.PeriodFor cuts it.
function days(from: number, to: number, o: Record<number, Partial<Bucket>> = {}, end = now): Bucket[] {
  const out: Bucket[] = []
  for (let d = from; d <= to; d++) {
    const s = at(d, 0), e = Math.min(at(d + 1, 0), end)
    out.push({
      Start: iso(s), End: iso(e), ChargedWh: 0, DischargedWh: 0, OutageWh: 0, PVChargeWh: 0,
      PeakW: null, PeakAt: null, Covered: (e - s) * NS, Span: (e - s) * NS, NoData: false, ...o[d],
    })
  }
  return out
}

function report(buckets: Bucket[], o: Partial<Report> = {}): Report {
  const sum = (f: (b: Bucket) => number) => buckets.reduce((a, b) => a + f(b), 0)
  return {
    Period: { From: buckets[0].Start, To: buckets[buckets.length - 1].End, Weeks: false },
    ChargedWh: sum(b => b.ChargedWh), DischargedWh: sum(b => b.DischargedWh), OutageWh: sum(b => b.OutageWh),
    Efficiency: null, Coverage: 1, HasPV: false, Buckets: buckets, EnergySince: null, Backfilling: false, ...o,
  }
}

// Seven days 15.11..21.11: 29.2 kWh in, 25.6 kWh out, 3.1 kWh of it in outages (none on the 15th).
const rep7d = report(days(15, 21, {
  15: { ChargedWh: 4000, DischargedWh: 3500 },
  16: { ChargedWh: 4200, DischargedWh: 3600, OutageWh: 1000, PeakW: 812.4, PeakAt: iso(at(16, 19, 40)) },
  17: { ChargedWh: 4100, DischargedWh: 3700 },
  18: { ChargedWh: 4300, DischargedWh: 3800, OutageWh: 2100 },
  19: { ChargedWh: 4200, DischargedWh: 3700 },
  20: { ChargedWh: 4400, DischargedWh: 3800 },
  21: { ChargedWh: 4000, DischargedWh: 3500 },
}), { Efficiency: 25600 / 29200 * 100 })

const repSmall = report(days(21, 21, { 21: { ChargedWh: 400, DischargedWh: 300 } }), { Efficiency: null })

test('totals in kWh with comma', () => expect(energyModel(rep7d, now).totals.charged).toBe('29,2 кВт·год'))
test('efficiency hidden on small volumes', () => expect(energyModel(repSmall, now).totals.efficiency).toBe('—'))
test('no PV ring for Venus E', () => expect(energyModel(rep7d, now).pvRing).toBeUndefined())
test('incomplete note', () => expect(energyModel({ ...rep7d, Coverage: 0.82 }, now).incomplete).toBe('Дані неповні: покрито 82% часу'))
test('tooltip lines only when non-zero', () => expect(energyModel(rep7d, now).bars[0].tip).not.toMatch(/під час відключень/))

test('totals: discharged with its outage share, efficiency, the reason it is hidden', () => {
  const m = energyModel(rep7d, now)
  expect(m.totals.discharged).toBe('25,6 кВт·год')
  expect(m.totals.dischargedMeta).toBe('під час відключень 3,1')
  expect(m.totals.efficiency).toBe('88%')
  expect(m.totals.efficiencyMeta).toBeUndefined()
  expect(m.incomplete).toBeUndefined()
  expect(m.empty).toBeUndefined()
  const s = energyModel(repSmall, now)
  expect(s.totals.dischargedMeta).toBeUndefined()
  expect(s.totals.efficiencyMeta).toBe('замало даних — потрібно ≥ 0,5 кВт·год')
})

test('en: a decimal point, kWh', () => {
  setLocale('en')
  const m = energyModel(rep7d, now)
  expect(m.totals.charged).toBe('29.2 kWh')
  expect(m.totals.dischargedMeta).toBe('during outages 3.1')
  expect(energyModel(repSmall, now).totals.efficiencyMeta).toBe('not enough data — needs ≥ 0.5 kWh')
  expect(energyModel({ ...rep7d, Coverage: 0.82 }, now).incomplete).toBe('Data incomplete: 82% of the time covered')
})

test('bars: one per day in kWh, today marked, the title by days', () => {
  const m = energyModel(rep7d, now)
  expect(m.title).toBe('кВт·год по днях')
  expect(m.bars).toHaveLength(7)
  expect(m.bars[1]).toMatchObject({ key: '2026-11-16', label: 'пн 16.11', charge: 4.2, discharge: 3.6, noData: false })
  expect(m.bars[6].today).toBe(true)
  expect(m.bars[0].today).toBeFalsy()
})

test('tooltip: charge, discharge with its outage part, the peak', () => {
  const tip = energyModel(rep7d, now).bars[1].tip
  expect(tip).toContain('Пн, 16 листопада')
  expect(tip).toContain('Заряд: 4,2 кВт·год')
  expect(tip).toContain('Розряд: 3,6 кВт·год')
  expect(tip).toContain('під час відключень 1,0')
  expect(tip).toContain('Пік розряду: 812 Вт о 19:40')
  expect(tip).not.toContain('з мережі')
  expect(energyModel(rep7d, now).bars[0].tip).not.toContain('Пік розряду')
})

test('PV: the grid / solar ring and the split in the tooltip', () => {
  const r = report(days(20, 21, { 20: { ChargedWh: 4000, PVChargeWh: 1200 }, 21: { ChargedWh: 1000 } }), { HasPV: true })
  const m = energyModel(r, now)
  expect(m.pvRing).toMatchObject({ key: 'split', title: 'Мережа / сонце', value: '76%', sub: 'з мережі · 24% сонце', tone: 'info' })
  expect(m.pvRing!.fill).toBeCloseTo(0.76)
  expect(m.bars[0].tip).toContain('з мережі 2,8 · сонце 1,2')
})

test('weeks: the title and week labels', () => {
  const b: Bucket[] = [at(2, 0), at(9, 0), at(16, 0)].map((s, i, a) => {
    const e = i < a.length - 1 ? a[i + 1] : now
    return { Start: iso(s), End: iso(e), ChargedWh: 9000, DischargedWh: 8000, OutageWh: 0, PVChargeWh: 0, PeakW: null, PeakAt: null, Covered: (e - s) * NS, Span: (e - s) * NS, NoData: false }
  })
  const m = energyModel({ ...report(b), Period: { From: iso(at(2, 0)), To: iso(now), Weeks: true } }, now)
  expect(m.title).toBe('кВт·год по тижнях')
  expect(m.bars[0].label).toBe('2–08.11')
  expect(m.bars[0].tip).toContain('Тиждень 02.11 – 08.11')
})

test('no data: hatched, never a zero; the tooltip says why', () => {
  const r = report(days(15, 21, { 17: { Covered: 0, NoData: true }, 21: { ChargedWh: 1000 } }), { Coverage: 0.86 })
  const m = energyModel(r, now)
  expect(m.bars[2].noData).toBe(true)
  expect(m.bars[2].tip).toContain('Немає даних: комп')
  expect(m.bars[2].tip).not.toContain('Заряд')
})

test('no data before energy counting began: the tooltip explains the missing readings', () => {
  // Counting began on 11.11 (the first rolled-up minute): 10.11 lies before it.
  const old = report(days(10, 12, { 10: { Covered: 0, NoData: true }, 11: { ChargedWh: 800 } }), { Coverage: 0.66, EnergySince: iso(at(11, 0)) })
  expect(energyModel(old, at(21, 14, 30, 11)).bars[0].tip).toContain('детальні виміри зберігаються 30 днів')
})

test('an old gap after counting began is a gap, however old', () => {
  // Counting since 1.10; seen on 21.12, the gap of 10.11 is 41 days back but after it.
  const r = report(days(10, 12, { 10: { Covered: 0, NoData: true }, 11: { ChargedWh: 800 } }), { Coverage: 0.66, EnergySince: iso(at(1, 0, 0, 9)) })
  const tip = energyModel(r, at(21, 14, 30, 11)).bars[0].tip
  expect(tip).toContain('Немає даних: комп')
  expect(tip).not.toContain('30 днів')
})

test('backfilling: a note instead of «incomplete», no-data days are «still counting»', () => {
  const r = report(days(15, 21, { 15: { Covered: 0, NoData: true }, 21: { ChargedWh: 1000 } }), { Coverage: 0.3, Backfilling: true, EnergySince: iso(at(14, 0)) })
  const m = energyModel(r, now)
  expect(m.backfilling).toBe('Дораховуємо енергію за минулі дні…')
  expect(m.incomplete).toBeUndefined()
  expect(m.bars[0].tip).toContain('Ще рахуємо')
  expect(energyModel(report(days(15, 21)), now).backfilling).toBeUndefined()
})

test('efficiency over 100%: a dash with the reason, not a number', () => {
  const m = energyModel({ ...rep7d, Efficiency: 112 }, now)
  expect(m.totals.efficiency).toBe('—')
  expect(m.totals.efficiencyMeta).toBe('батарея віддала більше, ніж отримала: частина — заряд, що був до початку періоду')
})

test('days before monitoring are not drawn', () => {
  const r = report(days(15, 21, { 15: { Covered: 0 }, 16: { Covered: 0 }, 21: { ChargedWh: 1000 } }), { Coverage: 1 })
  const m = energyModel(r, now)
  expect(m.bars[0]).toMatchObject({ noData: false, before: true, tip: '' })
  expect(m.bars[2].before).toBeFalsy()
})

test('just after midnight today is not a scary «no data»', () => {
  const t = at(21, 0, 1, 10) + 30e3 // 00:01:30
  const r = report(days(21, 21, { 21: { Covered: 0, NoData: true } }, t), { Coverage: 0 })
  const m = energyModel(r, t)
  expect(m.bars[0].noData).toBe(false)
  expect(m.bars[0].pending).toBe(true)
  expect(m.bars[0].tip).toBe('')
  expect(m.incomplete).toBeUndefined()
  expect(m.empty).toBeUndefined()
  expect(m.totals.charged).toBe('—')
})

test('the last minutes not yet rolled up do not make the data «incomplete»', () => {
  // Today at 01:00: the rollup lags ≤ 2 min, so 58 of 60 minutes are covered.
  const t = at(21, 1, 0)
  const b = days(21, 21, { 21: { ChargedWh: 300, Covered: 58 * M * NS } }, t)
  expect(energyModel(report(b, { Coverage: 58 / 60 }), t).incomplete).toBeUndefined()
  // A real hole still shows.
  const holed = days(21, 21, { 21: { ChargedWh: 300, Covered: 40 * M * NS } }, t)
  expect(energyModel(report(holed, { Coverage: 40 / 60 }), t).incomplete).toBe('Дані неповні: покрито 69% часу')
})

test('empty: observed, but nothing charged or discharged', () => {
  const m = energyModel(report(days(15, 21)), now)
  expect(m.empty).toBe('За цей період станція не заряджалась і не віддавала енергію.')
  expect(m.totals.charged).toBe('0,0 кВт·год')
})

test('nothing observed at all: dashes, not zeros, and a no-data note', () => {
  const r = report(days(15, 21, { 15: { Covered: 0, NoData: true }, 16: { Covered: 0, NoData: true }, 17: { Covered: 0, NoData: true }, 18: { Covered: 0, NoData: true }, 19: { Covered: 0, NoData: true }, 20: { Covered: 0, NoData: true }, 21: { Covered: 0, NoData: true } }), { Coverage: 0 })
  const m = energyModel(r, now)
  expect(m.totals.charged).toBe('—')
  expect(m.totals.discharged).toBe('—')
  expect(m.totals.efficiency).toBe('—')
  expect(m.totals.efficiencyMeta).toBeUndefined()
  expect(m.empty).toBeUndefined()
  expect(m.incomplete).toBe('Дані неповні: покрито 0% часу')
})

test('never monitored: no columns, the no-data text', () => {
  const r = report(days(15, 21, { 15: { Covered: 0 }, 16: { Covered: 0 }, 17: { Covered: 0 }, 18: { Covered: 0 }, 19: { Covered: 0 }, 20: { Covered: 0 }, 21: { Covered: 0 } }), { Coverage: 0 })
  const m = energyModel(r, now)
  expect(m.bars.every(b => b.before)).toBe(true)
  expect(m.empty).toBe('За цей період даних немає.')
  expect(m.incomplete).toBeUndefined()
})
