import { afterEach, expect, test } from 'vitest'
import type { Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/history/models'
import { setLocale } from '../lib/i18n'
import { hasLiveRow, outagesModel, rangeAround } from './outagesModel'

afterEach(() => setLocale('uk'))

const H = 3600e3, M = 60e3, NS = 1e6
const at = (day: number, h: number, m = 0, month = 10) => new Date(2026, month, day, h, m, 0).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const now = at(21, 14, 30)

type Row = { id: number; start: number; end?: number; socStart?: number; socEnd?: number; diedAt?: number; partial?: boolean; bounds?: Record<string, unknown> }

function row(r: Row) {
  const asOf = r.end ?? now
  return {
    Outage: {
      ID: r.id, StationID: 'st1', Start: iso(r.start), End: r.end ? iso(r.end) : null,
      SOCStart: r.socStart ?? null, SOCEnd: r.socEnd ?? null, DiedAt: r.diedAt ? iso(r.diedAt) : null, Partial: r.partial ?? false,
    },
    Bounds: { StartApprox: false, EndApprox: false, NotAfter: null, Frozen: false, AsOf: iso(asOf), Known: (asOf - r.start) * NS, AtLeast: false, StartFrom: null, ...r.bounds },
  }
}

// days 15.11..21.11 (today is cut at now); the bucket of each day with its outages.
function days(from: number, to: number, o: Record<number, Record<string, unknown>> = {}) {
  const out: Record<string, unknown>[] = []
  for (let d = from; d <= to; d++) {
    const s = at(d, 0), e = d === 21 ? now : at(d + 1, 0)
    out.push({
      Start: iso(s), End: iso(e), Off: 0, OffAtLeast: false, Count: 0, Ongoing: false, Longest: null, MinSOC: null,
      RanOut: false, Observed: (e - s) * NS, Gaps: 0, NoData: false, BeforeObs: false, ...o[d],
    })
  }
  return out
}

function report(o: Record<string, unknown> = {}): Report {
  return {
    Period: { From: iso(at(15, 0)), To: iso(now), Weeks: false },
    ObservedSince: iso(at(1, 9)),
    Observed: (now - at(15, 0)) * NS, Off: 0, OffAtLeast: false, GridPct: 100,
    Count: 0, RanOut: 0, Coverage: null, CoverageApprox: false, Average: 0, Longest: 0, LongestAt: null,
    Uncertain: 0, Gaps: 0, Buckets: days(15, 21), Rows: [], GapSpans: [],
    ...o,
  } as unknown as Report
}

// Review Focus 1: off at 22:00, PC off at 23:00, grid back at 01:00, PC on at 09:00.
const gapNight = row({
  id: 7, start: at(20, 22), end: at(21, 9), socStart: 100, socEnd: 80,
  bounds: { EndApprox: true, NotAfter: iso(at(21, 9)), Known: H * NS, AtLeast: true },
})
const reportWithGapNight = report({
  Count: 1, Off: H * NS, OffAtLeast: true, GridPct: 99, Uncertain: 1, Average: H * NS, Longest: H * NS, LongestAt: iso(at(20, 22)),
  Gaps: 10 * H * NS, CoverageApprox: true,
  Rows: [gapNight],
  GapSpans: [{ ID: 1, StationID: 'st1', Start: iso(at(20, 23)), End: iso(at(21, 9)), Reason: 'app_not_running' }],
  Buckets: days(15, 21, {
    20: { Off: H * NS, OffAtLeast: true, Count: 1, Gaps: H * NS },
    21: { Off: 0, OffAtLeast: true, Count: 1, Gaps: 9 * H * NS },
  }),
})

const reportFrozen = report({
  Count: 1, Rows: [row({ id: 9, start: at(21, 11, 50), socStart: 100, bounds: { Frozen: true, AsOf: iso(at(21, 14, 5)), Known: (2 * H + 15 * M) * NS, AtLeast: true } })],
})

const reportDied = report({
  Count: 1, RanOut: 1, Coverage: 0,
  Rows: [row({ id: 3, start: at(20, 19, 30), end: at(21, 2, 40), socStart: 100, socEnd: 10, diedAt: at(21, 0, 55) })],
})

const report7d = report({
  Count: 3, Coverage: 100,
  Rows: [
    row({ id: 12, start: at(20, 8), end: at(20, 9), socStart: 90, socEnd: 70 }),
    row({ id: 11, start: at(19, 18), end: at(19, 20), socStart: 100, socEnd: 60 }),
    row({ id: 10, start: at(19, 6), end: at(19, 7), socStart: 100, socEnd: 85 }),
  ],
  Buckets: days(15, 21, { 19: { Off: 3 * H * NS, Count: 2 }, 20: { Off: H * NS, Count: 1 } }),
})

// Monitoring started on 20.11, and nothing since.
const reportNoneEver = report({ ObservedSince: iso(at(20, 10)), Observed: (now - at(20, 10)) * NS })

const reportBeforeHistory = report({
  Period: { From: iso(at(1, 0)), To: iso(at(8, 0)), Weeks: false },
  ObservedSince: iso(at(21, 8)), Observed: 0, GridPct: null,
  Buckets: days(1, 7, Object.fromEntries([1, 2, 3, 4, 5, 6, 7].map(d => [d, { BeforeObs: true, Observed: 0 }]))),
})

const report45d = report({ Period: { From: iso(at(8, 0, 9)), To: iso(now), Weeks: true } })

test('rows show bounds', () => { const m = outagesModel(reportWithGapNight, now, {}); expect(m.rows[0]).toMatchObject({ end: 'не пізніше 09:00', duration: '≥ 1 год', note: 'є пропуск у даних' }) })
test('frozen ongoing row', () => expect(outagesModel(reportFrozen, now, {}).rows[0]).toMatchObject({ end: "зв'язок втрачено", asOf: 'на 14:05' }))
test('likely out note says probably', () => expect(outagesModel(reportDied, now, {}).rows[0].note).toBe('станція, ймовірно, розрядилась ≈ о 00:55'))
test('footnote with uncertain rows', () => expect(outagesModel(reportWithGapNight, now, {}).footnote).toMatch(/^1 відключення з неточною тривалістю/))
test('click filter', () => { const m = outagesModel(report7d, now, { filterDay: '2026-11-19' }); expect(m.rows.every(r => r.start.includes('19.11'))).toBe(true); expect(m.filter!.label).toMatch(/^Показано відключення за /) })
test('empty states', () => { expect(outagesModel(reportNoneEver, now, {}).empty!.title).toBe('Відключень ще не записано'); expect(outagesModel(reportBeforeHistory, now, {}).empty!.action).toBe('fromStart') })

test('weeks title over 30 days', () => expect(outagesModel(report45d, now, {}).bars.title).toBe('Без світла по тижнях'))

// Beyond the brief's list.

test('the filter keeps only that day, and is dropped for an unknown day', () => {
  expect(outagesModel(report7d, now, { filterDay: '2026-11-19' }).rows.map(r => r.id)).toEqual([11, 10])
  const m = outagesModel(report7d, now, { filterDay: '2026-10-01' })
  expect(m.filter).toBeUndefined()
  expect(m.rows).toHaveLength(3)
})

test('the highlighted row', () => {
  const m = outagesModel(report7d, now, { highlight: 11 })
  expect(m.rows.map(r => r.highlighted)).toEqual([false, true, false])
})

test('a start right after the PC was off says so, with «≈»', () => {
  const r = report({
    Count: 1,
    Rows: [row({ id: 5, start: at(19, 21, 41), end: at(19, 23, 5), socEnd: 71, partial: true, bounds: { StartApprox: true, AtLeast: true } })],
    GapSpans: [{ ID: 2, StationID: 'st1', Start: iso(at(19, 18)), End: iso(at(19, 21, 40)), Reason: 'pc_sleep' }],
  })
  expect(outagesModel(r, now, {}).rows[0]).toMatchObject({ start: '19.11 ≈ 21:41', end: '23:05', soc: '— → 71%', note: 'почалось, поки додаток не працював' })
  // The same start after a no-link gap is only «є пропуск у даних».
  const noLink = { ...r, GapSpans: [{ ...r.GapSpans![0], Reason: 'no_link' }] } as unknown as Report
  expect(outagesModel(noLink, now, {}).rows[0].note).toBe('є пропуск у даних')
  // Final review M1: an update gap (the app was restarting) counts as off too.
  const upd = { ...r, GapSpans: [{ ...r.GapSpans![0], Reason: 'app_update' }] } as unknown as Report
  expect(outagesModel(upd, now, {}).rows[0].note).toBe('почалось, поки додаток не працював')
})

test('today, yesterday and the end on another day', () => {
  const m = outagesModel(reportWithGapNight, now, {})
  expect(m.rows[0]).toMatchObject({ start: 'вчора 22:00', endDay: 'сьогодні', soc: '100% → 80%' })
  const same = outagesModel(report7d, now, {}).rows[0]
  expect(same).toMatchObject({ start: 'вчора 08:00', end: '09:00' })
  expect(same.endDay).toBeUndefined()
})

test('an ongoing live row', () => {
  const r = report({ Count: 1, Rows: [row({ id: 4, start: at(21, 13), socStart: 95 })] })
  expect(outagesModel(r, now, {}).rows[0]).toMatchObject({ end: 'триває', endKind: 'ongoing', duration: '1 год 30 хв', soc: '95% → —', note: '' })
})

test('grid ring: «≥» from OffAtLeast; without outages, the time observed', () => {
  const g = outagesModel(reportWithGapNight, now, {}).rings[0]
  expect(g).toMatchObject({ key: 'grid', title: 'Світло', value: '99%', sub: '≥ 1 год без світла' })
  const none = outagesModel(report(), now, {}).rings[0]
  expect(none).toMatchObject({ value: '100%', sub: 'за 6 д 14 год спостережень' })
})

test('coverage ring: «—» without closed outages, «≈» when an end was in a gap', () => {
  expect(outagesModel(report(), now, {}).rings[1]).toMatchObject({ key: 'coverage', value: '—', sub: 'ще не було відключень', fill: 0 })
  expect(outagesModel(reportDied, now, {}).rings[1]).toMatchObject({ value: '0%', sub: '1 раз, ймовірно, розрядилась' })
  const approx = report({ Count: 2, Coverage: 100, CoverageApprox: true, Rows: [row({ id: 1, start: at(20, 1), end: at(20, 2) })] })
  expect(outagesModel(approx, now, {}).rings[1]).toMatchObject({ value: '≈ 100%', sub: 'усі протягнула' })
})

test('stats and hint', () => {
  const m = outagesModel(reportWithGapNight, now, {})
  expect(m.stats).toMatchObject({ count: '1', average: '1 год', longest: '1 год', longestMeta: 'пт 20.11, 22:00' })
  expect(m.hint).toBe('Дані за 15.11 – 21.11 · 1 пропуск у даних')
  expect(outagesModel(report(), now, {}).hint).toBe('Дані за 15.11 – 21.11')
  expect(outagesModel(report(), now, {}).stats).toMatchObject({ count: '0', average: '—', longest: '—', longestMeta: '' })
})

test('bars: kinds, labels and tooltips', () => {
  const r = report({
    ObservedSince: iso(at(17, 9)),
    GapSpans: [{ ID: 3, StationID: 'st1', Start: iso(at(19, 23)), End: iso(at(21, 0, 30)), Reason: 'pc_sleep' }],
    Buckets: days(15, 21, {
      15: { BeforeObs: true, Observed: 0 }, 16: { BeforeObs: true, Observed: 0 },
      18: { Off: 2.25 * H * NS, Count: 2, MinSOC: 41, Longest: row({ id: 2, start: at(18, 6), end: at(18, 7, 30) }) },
      20: { NoData: true, Observed: 0, Gaps: 24 * H * NS },
      21: { Off: 2 * H * NS, OffAtLeast: true, Count: 1, Ongoing: true, RanOut: true, MinSOC: 9, Gaps: 30 * M * NS },
    }),
  })
  const b = outagesModel(r, now, {}).bars
  expect(b.title).toBe('Без світла по днях')
  expect(b.items.map(i => i.kind)).toEqual(['before', 'before', 'zero', 'value', 'zero', 'noData', 'value'])
  expect(b.items.map(i => i.label)).toEqual(['нд 15.11', 'пн 16.11', 'вт 17.11', 'ср 18.11', 'чт 19.11', 'пт 20.11', 'сб 21.11'])
  expect(b.items[6]).toMatchObject({ key: '2026-11-21', hours: 2, died: true })
  const day18 = b.items[3].tip
  expect(day18).toContain('Ср, 18 листопада')
  expect(day18).toContain('Відключень: 2')
  expect(day18).toContain('Без світла: 2 год 15 хв (9% доби)')
  expect(day18).toContain('Найдовше: 1 год 30 хв (06:00–07:30)')
  expect(day18).toContain('Мін. заряд: 41%')
  expect(day18).toContain('Клацніть, щоб показати ці відключення в таблиці')
  const today = b.items[6].tip
  expect(today).toContain('(сьогодні)')
  expect(today).toContain('Відключень: 1 (одне триває)')
  expect(today).toContain('Без світла: ≥ 2 год')
  expect(today).toContain('Мін. заряд: 9% — станція, ймовірно, розрядилась')
  expect(today).toContain('Є пропуски в даних: 30 хв')
  expect(today).not.toContain('Найдовше')
  expect(b.items[4].tip).toContain('Відключень не було')
  expect(b.items[4].tip).not.toContain('Клацніть')
  expect(b.items[5].tip).toContain('немає даних · ПК спав')
  expect(b.observedSince).toEqual({ index: 2, label: 'спостереження з вт 17.11' })
})

test('week bars', () => {
  const weeks = [[9, 16], [16, 23]].map(([s, e]) => ({
    Start: iso(at(s, 0)), End: iso(e === 23 ? now : at(e, 0)), Off: 10 * H * NS, OffAtLeast: false, Count: 3, Ongoing: false,
    Longest: null, MinSOC: null, RanOut: false, Observed: 0, Gaps: 0, NoData: false, BeforeObs: false,
  }))
  const r = report({ Period: { From: iso(at(9, 0)), To: iso(now), Weeks: true }, Buckets: weeks })
  const b = outagesModel(r, now, { filterDay: '2026-11-16' })
  expect(b.bars.items.map(i => i.label)).toEqual(['9–15.11', '16–21.11'])
  expect(b.bars.items[0].tip).toContain('Тиждень 09.11 – 15.11')
  expect(b.bars.items[0].tip).toContain('Без світла: 10 год (6% тижня)')
  expect(b.filter).toMatchObject({ day: '2026-11-16', label: 'Показано відключення за 16–21.11 · Показати всі' })
})

test('empty: period with and without gaps', () => {
  expect(outagesModel(report(), now, {}).empty).toEqual({ body: 'За цей період відключень не було.' })
  expect(outagesModel(report({ Gaps: 3 * H * NS }), now, {}).empty!.body).toBe('Записаних відключень немає, але за 3 год даних немає — відключення могли бути.')
  expect(outagesModel(reportNoneEver, now, {}).empty!.body).toMatch(/^Додаток рахує відключення/)
  expect(outagesModel(reportBeforeHistory, now, {}).empty!.body).toBe('Цей період — до початку спостережень (21.11).')
  expect(outagesModel(report({ ObservedSince: null, GridPct: null }), now, {}).empty!.title).toBe('Відключень ще не записано')
  expect(outagesModel(report7d, now, {}).empty).toBeUndefined()
})

test('en', () => {
  setLocale('en')
  const m = outagesModel(reportWithGapNight, now, {})
  expect(m.rows[0]).toMatchObject({ start: 'yesterday 22:00', end: 'by 09:00', duration: '≥ 1 h', note: 'gap in data' })
  expect(m.bars.title).toBe('Grid off by day')
  expect(m.footnote).toMatch(/^1 outage has an uncertain duration/)
  expect(outagesModel(reportBeforeHistory, now, {}).empty!.body).toBe('This period is before monitoring started (21 Nov).')
})

test('the start in two lines, «≈» muted', () => {
  const r = report({ Count: 1, Rows: [row({ id: 5, start: at(19, 21, 41), end: at(19, 23, 5), bounds: { StartApprox: true, AtLeast: true } })] })
  expect(outagesModel(r, now, {}).rows[0]).toMatchObject({ startTime: '≈ 21:41', startDay: '19.11', startTone: 'muted' })
  expect(outagesModel(reportFrozen, now, {}).rows[0]).toMatchObject({ startTime: '11:50', startDay: 'сьогодні' })
})

// Fix round 1.

test('stats show «—», not 0, for a period with no observation', () => {
  expect(outagesModel(reportBeforeHistory, now, {}).stats).toMatchObject({ count: '—', countMeta: '' })
  expect(outagesModel(report({ ObservedSince: null, Observed: 0, GridPct: null }), now, {}).stats).toMatchObject({ count: '—', countMeta: '' })
  expect(outagesModel(report(), now, {}).stats.count).toBe('0')
})

test('the gap hint counts only gaps of a minute or more', () => {
  const r = report({ GapSpans: [
    { ID: 1, StationID: 'st1', Start: iso(at(19, 10)), End: iso(at(19, 10) + 20e3), Reason: 'no_link' },
    { ID: 2, StationID: 'st1', Start: iso(at(19, 12)), End: iso(at(19, 12, 5)), Reason: 'no_link' },
  ] })
  expect(outagesModel(r, now, {}).hint).toBe('Дані за 15.11 – 21.11 · 1 пропуск у даних')
})

test('coverage with only an ongoing outage says none has ended', () => {
  const r = report({ Count: 1, Rows: [row({ id: 4, start: at(21, 13), socStart: 95 })] })
  expect(outagesModel(r, now, {}).rings[1]).toMatchObject({ value: '—', sub: 'жодне ще не закінчилось' })
})

test('«≈ X на день» is per observed day', () => {
  // 7 outages over 3.5 observed days of a 7-day period: 2 a day, not 1.
  const r = report({ Count: 7, Observed: 3.5 * 24 * H * NS, Rows: [gapNight] })
  expect(outagesModel(r, now, {}).stats.countMeta).toBe('≈ 2 на день')
})

test('Light ring is below 100% whenever there was an outage', () => {
  const r = report({ Count: 1, Off: 0, OffAtLeast: true, GridPct: 100, Rows: [gapNight] })
  expect(outagesModel(r, now, {}).rings[0].value).toBe('99%')
})

test('coverage aria says the percent once, with «≈» when approximate', () => {
  const approx = report({ Count: 2, Coverage: 75, CoverageApprox: true, RanOut: 1, Rows: [gapNight] })
  const aria = outagesModel(approx, now, {}).rings[1].aria!
  expect(aria).toBe('Покриття. Станція протягнула ≈ 75% відключень до кінця. Ймовірно, розрядилась раніше: 1.')
  expect(aria.match(/75/g)).toHaveLength(1)
})

test('an ongoing row ends with the live charge only while live', () => {
  const r = report({ Count: 1, Rows: [row({ id: 4, start: at(21, 13), socStart: 95 })] })
  expect(outagesModel(r, now, { liveSoc: 62 }).rows[0]).toMatchObject({ soc: '95% → 62%', socLive: true, end: 'триває' })
  expect(outagesModel(r, now, {}).rows[0].soc).toBe('95% → —')
  // A frozen row never takes a live charge.
  expect(outagesModel(reportFrozen, now, { liveSoc: 62 }).rows[0].soc).toBe('100% → —')
})

test('rangeAround: 7 days holding the outage, never past today', () => {
  expect(rangeAround(at(1, 22), now)).toEqual({ from: '2026-11-01', to: '2026-11-07' })
  expect(rangeAround(at(19, 6), now)).toEqual({ from: '2026-11-15', to: '2026-11-21' })
  expect(rangeAround(at(21, 6), now)).toEqual({ from: '2026-11-15', to: '2026-11-21' })
})

test('hasLiveRow: only an ongoing, not frozen row takes the live charge', () => {
  expect(hasLiveRow(report({ Count: 1, Rows: [row({ id: 4, start: at(21, 13) })] }))).toBe(true)
  expect(hasLiveRow(reportFrozen)).toBe(false)
  expect(hasLiveRow(report7d)).toBe(false)
  expect(hasLiveRow(report())).toBe(false)
})

// Task 21 (Task 9 review): «Сьогодні» is a 24-hour timeline, not one column.
const today = (o: Record<string, unknown> = {}, n = now) => report({
  Period: { From: iso(new Date(n).setHours(0, 0, 0, 0)), To: iso(n), Weeks: false },
  Buckets: [{ Start: iso(new Date(n).setHours(0, 0, 0, 0)), End: iso(n), Off: 0, OffAtLeast: false, Count: 0, Ongoing: false, Longest: null,
    MinSOC: null, RanOut: false, Observed: 0, Gaps: 0, NoData: false, BeforeObs: false }],
  ...o,
})

test('today: a timeline of the local day with outages, the ongoing one up to now, and gaps', () => {
  const r = today({
    Count: 3,
    Rows: [
      row({ id: 2, start: at(21, 11, 50), socStart: 100 }),
      row({ id: 1, start: at(21, 6, 10), end: at(21, 8, 10), socStart: 100, socEnd: 84 }),
      // Began last night: drawn from midnight, its duration is the whole outage.
      row({ id: 0, start: at(20, 23), end: at(21, 1), socStart: 100, socEnd: 90 }),
    ],
    GapSpans: [{ ID: 4, StationID: 'st1', Start: iso(at(21, 3, 10)), End: iso(at(21, 4, 20)), Reason: 'pc_sleep' }],
  })
  const m = outagesModel(r, now, {})
  expect(m.bars.title).toBe('Сьогодні по годинах')
  expect(m.bars.sub).toBe('00:00 – 24:00')
  const tl = m.timeline!
  expect(tl).toMatchObject({ start: at(21, 0), end: at(22, 0), now, observedFrom: at(21, 0) })
  const [a, gap, b, c] = tl.items
  expect(a).toMatchObject({ kind: 'outage', start: at(21, 0), end: at(21, 1), label: '23:00–01:00 · 2 год' })
  expect(b).toMatchObject({ kind: 'outage', start: at(21, 6, 10), end: at(21, 8, 10), label: '06:10–08:10 · 2 год' })
  expect(c).toMatchObject({ kind: 'outage', start: at(21, 11, 50), end: now, label: '11:50–зараз · 2 год 40 хв' })
  expect(b.tip).toContain('Без світла 06:10–08:10 · 2 год')
  expect(b.aria).toBe('Без світла 06:10–08:10 · 2 год')
  expect(gap).toMatchObject({ kind: 'gap', start: at(21, 3, 10), end: at(21, 4, 20) })
  expect(gap.tip).toContain('немає даних · ПК спав')
  expect(gap.tip).toContain('Немає даних з 03:10 до 04:20.')
  // Other periods keep the columns.
  expect(outagesModel(report7d, now, {}).timeline).toBeUndefined()
  expect(outagesModel(report7d, now, {}).bars.sub).toBe('години без світла')
})

test('today: a frozen outage stops where the data stops, «≥»', () => {
  const r = today({ Count: 1, Rows: reportFrozen.Rows })
  const [o] = outagesModel(r, now, {}).timeline!.items
  expect(o).toMatchObject({ start: at(21, 11, 50), end: at(21, 14, 5), label: '11:50–14:05 · ≥ 2 год 15 хв' })
  expect(o.tip).toContain('тривалість заморожена до відновлення')
})

test('today: the timeline of a DST day is 25 hours from local midnight', () => {
  const n = at(25, 15, 0, 9) // 25.10.2026, clocks go back at 04:00
  const tl = outagesModel(today({}, n), n, {}).timeline!
  expect(tl.start).toBe(at(25, 0, 0, 9))
  expect(tl.end - tl.start).toBe(25 * H)
  expect(tl.end).toBe(at(26, 0, 0, 9))
})

test('today: the timeline items, outages and gaps, are in time order (the Tab order)', () => {
  const r = today({
    Count: 2,
    Rows: [row({ id: 2, start: at(21, 11, 50) }), row({ id: 1, start: at(21, 6, 10), end: at(21, 8, 10) })],
    GapSpans: [{ ID: 4, StationID: 'st1', Start: iso(at(21, 3, 10)), End: iso(at(21, 4, 20)), Reason: 'pc_sleep' },
      { ID: 5, StationID: 'st1', Start: iso(at(21, 9)), End: iso(at(21, 10)), Reason: 'no_link' }],
  })
  expect(outagesModel(r, now, {}).timeline!.items.map(i => i.key)).toEqual(['g4', 'o1', 'g5', 'o2'])
})

test('today: the timeline of a spring-forward day is 23 hours', () => {
  const n = at(29, 15, 0, 2) // 29.03.2026, clocks go forward at 03:00
  const tl = outagesModel(today({}, n), n, {}).timeline!
  expect(tl.start).toBe(at(29, 0, 0, 2))
  expect(tl.end - tl.start).toBe(23 * H)
})

// ⓘ (copy §15) on the rings and on the marks of a row.
test('the rings have their ⓘ', () => {
  expect(outagesModel(reportWithGapNight, now, {}).rings.map(r => r.info?.key)).toEqual(['info.outages.grid', 'info.outages.coverage'])
})
test('a row explains «≥», «не пізніше» and its gap note', () => {
  const r = outagesModel(reportWithGapNight, now, {}).rows[0]
  expect(r.info).toEqual({ end: { key: 'info.common.notAfter' }, duration: { key: 'info.common.atLeast' }, note: { key: 'info.outages.gapRow' } })
})
test('a row explains «≈», a start while the app was off, a lost link and a station that ran out', () => {
  const r = report({
    Count: 1,
    Rows: [row({ id: 5, start: at(19, 21, 41), end: at(19, 23, 5), partial: true, bounds: { StartApprox: true } })],
    GapSpans: [{ ID: 2, StationID: 'st1', Start: iso(at(19, 18)), End: iso(at(19, 21, 40)), Reason: 'pc_sleep' }],
  })
  expect(outagesModel(r, now, {}).rows[0].info).toEqual({ start: { key: 'info.common.approx' }, note: { key: 'info.outages.startedWhileOff' } })
  // Frozen: the «зв'язок втрачено» pill (its note says the same, so one ⓘ) and the «≥» duration.
  expect(outagesModel(reportFrozen, now, {}).rows[0].info).toEqual({ end: { key: 'info.outages.linkLost' }, duration: { key: 'info.common.atLeast' } })
  expect(outagesModel(reportDied, now, {}).rows[0].info).toEqual({ note: { key: 'info.outages.stationOut' } })
  // An exact, ended outage: nothing to explain.
  expect(outagesModel(report7d, now, {}).rows[0].info).toEqual({})
})

test('a hatched column says in its tooltip what a day without data means (fix round 1)', () => {
  const r = report({ Buckets: days(15, 21, { 18: { NoData: true, Observed: 0 } }) })
  const tip = outagesModel(r, now, {}).bars.items[3].tip
  expect(tip).toContain('<div class="tip__foot">За заштрихований день даних немає зовсім')
  expect(outagesModel(r, now, {}).bars.items[2].tip).not.toContain('За заштрихований день')
})

// Owner 2026-11-23: a start that lay in a gap is the window it lies in,
// «між {from} і {to}», in the table, the column tooltip and the timeline.
test('a start in a gap names the window in the table', () => {
  const r = report({
    Count: 1,
    Rows: [row({ id: 5, start: at(19, 7, 10), end: at(19, 9), partial: true, bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(19, 1, 5)) } })],
    GapSpans: [{ ID: 2, StationID: 'st1', Start: iso(at(19, 1, 5)), End: iso(at(19, 7, 9)), Reason: 'app_not_running' }],
  })
  const m = outagesModel(r, now, {}).rows[0]
  expect(m).toMatchObject({ start: 'між 19.11 01:05 і 07:10', startTime: 'між 19.11 01:05 і 07:10', startDay: '', startTone: 'muted', end: '09:00' })
  expect(m.info.start).toEqual({ key: 'info.common.approx' })
  // The gap began the day before: its start carries the day.
  const y = { ...r, Rows: [row({ id: 5, start: at(19, 7, 10), end: at(19, 9), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(18, 23, 40)) } })] } as unknown as Report
  expect(outagesModel(y, now, {}).rows[0].startTime).toBe('між 18.11 23:40 і 19.11 07:10')
  setLocale('en')
  expect(outagesModel(y, now, {}).rows[0].startTime).toBe('between 18 Nov 23:40 and 19 Nov 07:10')
})
test('the longest outage of a column names the window too', () => {
  const r = report({
    Buckets: days(15, 21, {
      18: { Off: 2.25 * H * NS, Count: 2, Longest: row({ id: 2, start: at(18, 6), end: at(18, 7, 30), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(18, 2)) } }) },
    }),
  })
  expect(outagesModel(r, now, {}).bars.items[3].tip).toContain('Найдовше: ≥ 1 год 30 хв (між 18.11 02:00 і 06:00 – 07:30)')
})
test('today: a timeline span whose start lay in a gap names the window', () => {
  const r = today({
    Count: 1,
    Rows: [row({ id: 1, start: at(21, 6, 10), end: at(21, 8, 10), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(21, 1, 5)) } })],
  })
  const [o] = outagesModel(r, now, {}).timeline!.items
  expect(o).toMatchObject({ start: at(21, 6, 10), label: 'між 01:05 і 06:10 – 08:10 · ≥ 2 год', aria: 'Без світла між 01:05 і 06:10 – 08:10 · ≥ 2 год' })
  expect(o.tip).toContain('Без світла між 01:05 і 06:10 – 08:10 · ≥ 2 год')
})
test('a window into today names only the earlier day (controller ruling)', () => {
  const r = report({ Count: 1, Rows: [row({ id: 5, start: at(21, 7, 10), end: at(21, 9), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(20, 23, 40)) } })] })
  expect(outagesModel(r, now, {}).rows[0]).toMatchObject({ startTime: 'між вчора 23:40 і 07:10', startDay: '', end: '09:00' })
  const same = report({ Count: 1, Rows: [row({ id: 5, start: at(21, 7, 10), end: at(21, 9), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(21, 1, 5)) } })] })
  expect(outagesModel(same, now, {}).rows[0]).toMatchObject({ startTime: 'між 01:05 і 07:10', startDay: '' })
})
test('today: a span first seen yesterday that ended today names the end day (review)', () => {
  const r = today({
    Count: 1,
    Rows: [row({ id: 1, start: at(20, 23, 0), end: at(21, 8, 10), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(20, 20, 0)) } })],
  })
  const [o] = outagesModel(r, now, {}).timeline!.items
  expect(o.label).toMatch(/^між вчора 20:00 і 23:00 – сьогодні 08:10 · /)
  expect(o.aria).toMatch(/^Без світла між вчора 20:00 і 23:00 – сьогодні 08:10 · /)
  // Ongoing: the end is «зараз», no day.
  const live = today({ Count: 1, Rows: [row({ id: 1, start: at(20, 23, 0), bounds: { StartApprox: true, AtLeast: true, StartFrom: iso(at(20, 20, 0)) } })] })
  expect(outagesModel(live, now, {}).timeline!.items[0].label).toMatch(/^між вчора 20:00 і 23:00 – зараз · /)
})
