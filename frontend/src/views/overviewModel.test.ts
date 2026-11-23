import { expect, test, vi } from 'vitest'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import { chartNote, overviewFeed, overviewModel, type OverviewModel, type RightRow } from './overviewModel'
import { num } from '../lib/format'
import { setLocale } from '../lib/i18n'

const H = 3600e3, M = 60e3, NS = 1e6
const HOUR_NS = H * NS, MIN_NS = M * NS
const at = (h: number, m = 0, day = 21) => new Date(2026, 10, day, h, m, 0).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const now = at(15, 17)

const noForecast = { ToEmpty: null, ToFull: null, LastsFor: null, EmptyAt: null, FullAt: null, Warming: false }

function snap(o: Record<string, unknown> = {}, last: Record<string, unknown> | null = {}): Snapshot {
  const base = {
    Station: { ID: 'st1', Name: 'Дім', Model: 'Venus E 3.0', BLEMac: '', WifiMac: '', IP: '', Firmware: 151, CreatedAt: iso(at(9, 0, 17)) },
    Profile: { Model: 'Venus E 3.0', NominalWh: 5120, MaxChargeW: 2500, MaxDischargeW: 2500, MinSOC: 10, CycleLife: 6000, TempMinC: 0, TempMaxC: 45, Tested: true },
    Link: 'LIVE', Diagnosis: '',
    Last: last && { StationID: 'st1', At: iso(now - 3e3), Source: 'lan', SOC: 72, BatteryW: -420, OngridW: 0, OffgridW: 420, PVW: null, Grid: 2, Raw: null, ...last },
    Age: 3e3 * NS, Grid: 1, Mode: 3,
    F: { ...noForecast, ToEmpty: 7.5 * H * NS, EmptyAt: iso(at(22, 47)) },
    NoLightToday: (5 * H + 27 * M) * NS, OutagesToday: 2,
    Current: { ID: 2, StationID: 'st1', Start: iso(at(11, 50)), End: null, SOCStart: 100, SOCEnd: null, DiedAt: null, Partial: false },
    Previous: { ID: 1, StationID: 'st1', Start: iso(at(6, 10)), End: iso(at(8, 10)), SOCStart: 100, SOCEnd: 84, DiedAt: null, Partial: false },
    GapCard: null,
    Fills: { SOC: 0.72, Power: 0.168, Time: 62 / 90, NoLight: 0.357 },
    ObservedSince: iso(at(9, 0, 17)),
    LowThreshold: 20,
    NoLightAtLeast: false, GapsToday: 0, CurrentBounds: null, PreviousBounds: null,
  }
  return { ...base, ...o } as unknown as Snapshot
}
const withLink = (s: Snapshot, o: Record<string, unknown>) => ({ ...s, ...o }) as unknown as Snapshot

const liveDischarging = snap()
const liveCharging = snap({
  Grid: 0, Mode: 2, Current: null,
  F: { ...noForecast, ToFull: (H + 38 * M) * NS, FullAt: iso(at(16, 55)) },
  Fills: { SOC: 0.63, Power: 0.7, Time: 0.63, NoLight: 0.357 },
}, { SOC: 63, BatteryW: 1750, Grid: 1 })
const liveIdle = snap({
  Grid: 0, Mode: 1, Current: null, Previous: null, NoLightToday: 0, OutagesToday: 0,
  F: { ...noForecast, LastsFor: (10 * H + 14 * M) * NS },
  Fills: { SOC: 1, Power: 0, Time: 1, NoLight: 0 },
}, { SOC: 100, BatteryW: 0, Grid: 1 })
// Not fresh: the core clears mode, forecasts and the SOC / power / time fills.
const notFresh = { Grid: 2, Mode: 0, F: noForecast, Fills: { SOC: 0, Power: 0, Time: 0, NoLight: 0.355 } }
const stale = snap({ Link: 'STALE', Age: 2 * M * NS, ...notFresh }, { At: iso(at(15, 15)), SOC: 73, BatteryW: -415 })
// The open gap since the last measurement counts as a gap today.
const noLink = snap({ Link: 'NO_LINK', Diagnosis: 'lan_silent', Age: 20 * M * NS, GapsToday: 20 * MIN_NS, ...notFresh }, { At: iso(at(14, 57)), SOC: 73, BatteryW: -415 })
const likelyDead = snap({ Link: 'LIKELY_DEAD', Age: 30 * M * NS, GapsToday: 30 * MIN_NS, ...notFresh }, { At: iso(at(14, 47)), SOC: 10, BatteryW: -790 })
const connectingNoData = snap({ Link: 'CONNECTING', Grid: 0, Mode: 0, Current: null, Previous: null, NoLightToday: 0, OutagesToday: 0, F: noForecast, Fills: { SOC: 0, Power: 0, Time: 0, NoLight: 0 } }, null)

test('discharging row', () => { const m = overviewModel(liveDischarging, now); expect(m.rings.map(r => r.key)).toEqual(['soc','power','toEmpty','noLight']); expect(m.rings[2].fill).toBeCloseTo(0.689, 2) })
test('charging row', () => expect(overviewModel(liveCharging, now).rings[2].key).toBe('toFull'))
test('idle row', () => expect(overviewModel(liveIdle, now).rings[2].key).toBe('lastsFor'))
test('no-light ring has no percent', () => { const r = overviewModel(liveDischarging, now).rings[3]; expect(r.value).toBe('5:27'); expect(r.unit).toBe('год : хв'); expect(r.sub).not.toMatch(/%/) })
test('stale dims and blanks forecasts', () => { const m = overviewModel(stale, now); expect(m.rings.every(r => r.stale)).toBe(true); expect(m.rings[2].value).toBe('—') })
test('skeleton never shows zeros', () => expect(overviewModel(connectingNoData, now).variant).toBe('skeleton'))
test('outage rail', () => { expect(overviewModel(liveDischarging, now).outageRail).toBe('on'); expect(overviewModel(stale, now).outageRail).toBe('unknown') })

test('discharging rings read as in the mockup', () => {
  const [soc, power, eta, off] = overviewModel(liveDischarging, now).rings
  expect(soc).toMatchObject({ title: 'Заряд', value: '72%', sub: '3,69 з 5,12 кВт·год', tone: 'good', stale: false })
  expect(soc.fill).toBeCloseTo(0.72)
  expect(power).toMatchObject({ value: '420', unit: 'Вт · розряд', sub: 'з 2 500 Вт макс.', tone: 'warn', fill: 0.168 })
  expect(eta).toMatchObject({ title: 'До розряду', value: '7:30', unit: 'год : хв', sub: 'до 10% · ≈ о 22:47', tone: 'info' })
  expect(off).toMatchObject({ title: 'Без світла сьогодні', sub: '2 відключення', tone: 'warn', fill: 0.357 })
})
test('rings carry a full sentence for the screen reader', () => {
  const r = overviewModel(liveDischarging, now).rings
  expect(r[0].aria).toBe('Рівень заряду 72%, 3,69 з 5,12 кіловат-годин')
  expect(r[1].aria).toBe('Розряджається, 420 Вт з 2 500 максимальних')
  expect(r[2].aria).toBe('До нижньої межі 10% приблизно 7 годин 30 хвилин, близько 22:47')
  expect(r[3].aria).toBe('Сьогодні без світла 5 годин 27 хвилин. 2 відключення.')
  expect(overviewModel(stale, now).rings[0].aria).toBe('Рівень заряду 73%, 3,74 з 5,12 кіловат-годин (застаріле значення, станом на 15:15)')
})
test('charging and idle rings', () => {
  const c = overviewModel(liveCharging, now).rings
  expect(c[1]).toMatchObject({ value: '1 750', unit: 'Вт · заряд', sub: 'з 2 500 Вт макс.', tone: 'good' })
  expect(c[2]).toMatchObject({ title: 'До повного', value: '1:38', sub: 'до 100% · ≈ о 16:55' })
  const i = overviewModel(liveIdle, now).rings
  expect(i[1]).toMatchObject({ value: '0', unit: 'Вт · очікування', sub: 'ні заряду, ні розряду', tone: 'info' })
  expect(i[2]).toMatchObject({ title: 'Вистачить на', value: '10:14', sub: 'при сер. 450 Вт у відкл.', fill: 1 })
  expect(i[3]).toMatchObject({ value: '0:00', sub: 'без відключень' })
})
test('low charge turns the charge ring red', () => {
  expect(overviewModel(snap({}, { SOC: 17 }), now).rings[0].tone).toBe('bad')
  expect(overviewModel(snap({}, { SOC: 20 }), now).rings[0].tone).toBe('good')
})
test('ring tone uses the configured threshold', () => expect(overviewModel({ ...liveDischarging, LowThreshold: 30, Last: { ...liveDischarging.Last!, SOC: 25 } }, now).rings[0].tone).toBe('bad'))
test('a forecast still warming shows a dash and «рахуємо…», never a number', () => {
  const s = snap({ F: { ...noForecast, Warming: true } })
  expect(overviewModel(s, now).rings[2]).toMatchObject({ key: 'toEmpty', value: '—', sub: 'рахуємо…', fill: 0 })
})
test('idle without outage history says so', () => {
  const s = snap({ Grid: 0, Mode: 1, Current: null, F: noForecast }, { BatteryW: 0 })
  expect(overviewModel(s, now).rings[2]).toMatchObject({ key: 'lastsFor', value: '—', sub: 'ще немає даних', fill: 0 })
})
test('a forecast of a day or more is shown in whole hours; another day gets its weekday', () => {
  const s = snap({ F: { ...noForecast, ToEmpty: 38.4 * H * NS, EmptyAt: iso(at(5, 41, 23)) } })
  expect(overviewModel(s, now).rings[2]).toMatchObject({ value: '38', unit: 'год', sub: 'до 10% · ≈ пн 05:41' })
})
test('unknown values are a dash, not zero', () => {
  const [soc, power] = overviewModel(snap({}, { SOC: null, BatteryW: null }), now).rings
  expect(soc).toMatchObject({ value: '—', fill: 0 })
  expect(power).toMatchObject({ value: '—', fill: 0 })
})

test('stale keeps the last values, greyed, with a lower-bound outage time', () => {
  const m = overviewModel(stale, now)
  expect(m.variant).toBe('stale')
  expect(m.banner).toBe('Дані застарілі — останні о 15:15')
  expect(m.bannerLines).toEqual(['Станція не відповідає. Пробуємо знову…'])
  expect(m.rings.map(r => r.key)).toEqual(['soc', 'power', 'toEmpty', 'noLight'])
  expect(m.rings[0]).toMatchObject({ value: '73%' })
  expect(m.rings[0].fill).toBeCloseTo(0.73)
  expect(m.rings[1]).toMatchObject({ value: '415', unit: 'Вт · розряд' })
  expect(m.rings[1].fill).toBeCloseTo(415 / 2500)
  expect(m.rings[2].fill).toBe(0)
  expect(m.rings[3].value).toBe('≥ 5:27')
  expect(m.right[0]).toMatchObject({ label: 'Світло', value: 'невідомо · останнє: немає з 11:50 · ≥ 3 год 25 хв' })
  expect(m.right[0].hero).toBeFalsy()
  expect(m.source).toMatchObject({ value: 'LAN · 2 хв тому' })
})
test('connecting with old data is the stale view with a quiet banner', () => {
  const m = overviewModel(withLink(stale, { Link: 'CONNECTING' }), now)
  expect(m.variant).toBe('stale')
  expect(m.banner).toBe('Шукаємо станцію в локальній мережі…')
  expect(m.bannerKind).toBe('quiet')
})
test('no link: last charge with its time, no power, diagnosis and the router hint', () => {
  const m = overviewModel(noLink, now)
  expect(m.variant).toBe('nolink')
  expect(m.outageRail).toBe('unknown')
  expect(m.banner).toBe("Немає зв'язку зі станцією")
  expect(m.bannerLines?.[0]).toBe('Локальний API станції не відповідає з 14:57. Це відома проблема прошивки — зазвичай минає сама.')
  expect(m.bannerLines?.join(' ')).not.toMatch(/хмар/i)
  expect(m.bannerLines).toContain('Якщо немає світла, роутер теж міг вимкнутись.')
  expect(m.rings[0]).toMatchObject({ value: '73%', unit: 'о 14:57', stale: true })
  expect(m.rings[1]).toMatchObject({ value: '—', unit: 'Вт', fill: 0 })
  expect(m.rings[3]).toMatchObject({ sub: 'є пропуски в даних' })
  expect(m.source).toMatchObject({ label: 'Джерело даних', value: "немає зв'язку", meta: 'останні дані о 14:57', tone: 'bad' })
})
test('no link while the grid was on gives no router hint', () => {
  const m = overviewModel(withLink(noLink, { Current: null, Diagnosis: 'pc_offline' }), now)
  expect(m.bannerLines).toEqual(["Цей комп'ютер не в мережі — станція не досяжна. Перевірте Wi-Fi або кабель."])
})
test('without fresh data the time without light is a lower bound, also with no open outage', () => {
  const closed = { Current: null, NoLightToday: (H + 20 * M) * NS, OutagesToday: 1 }
  const staleRing = overviewModel(withLink(stale, closed), now).rings[3]
  expect(staleRing).toMatchObject({ value: '≥ 1:20', sub: '1 відключення' })
  expect(staleRing.aria).toBe('Сьогодні без світла щонайменше 1 година 20 хвилин. 1 відключення.')
  const lost = overviewModel(withLink(noLink, closed), now).rings[3]
  expect(lost).toMatchObject({ value: '≥ 1:20', sub: 'є пропуски в даних' })
  expect(overviewModel(liveDischarging, now).rings[3].aria).not.toMatch(/щонайменше/)
})
test('grid off with no outage record yet: the row agrees with the rail', () => {
  const m = overviewModel(snap({ Current: null }), now)
  expect(m.outageRail).toBe('on')
  expect(m.right[0]).toMatchObject({ value: 'немає', tone: 'warn', hero: true })
  const approx = snap({ Current: { ID: 2, StationID: 'st1', Start: iso(at(11, 50)), End: null, SOCStart: null, SOCEnd: null, DiedAt: null, Partial: true } })
  expect(overviewModel(approx, now).right[0].value).toBe('немає з ≈ 11:50 · 3 год 27 хв')
})
test('likely dead', () => {
  const m = overviewModel(likelyDead, now)
  expect(m.variant).toBe('likelyDead')
  expect(m.banner).toBe('Станція, ймовірно, розрядилась')
  expect(m.bannerLines).toEqual(["14:47 · заряд перед цим 10%. Світла немає з 11:50. Щойно з'явиться світло, станція ввімкнеться сама."])
  expect(m.rings[0]).toMatchObject({ value: '10%', unit: 'о 14:47', sub: 'ймовірно, розрядилась ≈ о 14:47' })
  expect(m.right[0]).toMatchObject({ value: 'немає з 11:50 · ≥ 2 год 57 хв', meta: 'на 14:47, далі невідомо' })
})

test('right column while the grid is off', () => {
  const m = overviewModel(liveDischarging, now)
  // Owner request (Task 21): no «Батарея» row; «Джерело даних» is in the app bar.
  expect(m.right.map(r => r.label)).toEqual(['Світло', 'Попереднє відключення', 'Помилки'])
  expect(m.right[0]).toMatchObject({ value: 'немає з 11:50 · 3 год 27 хв', tone: 'warn', hero: true })
  expect(m.right[1]).toMatchObject({ value: 'сьогодні 06:10–08:10 · 2 год', meta: '100% → 84%' })
  expect(m.source).toMatchObject({ label: 'Джерело даних', value: 'LAN · 3 с тому', tone: 'good', live: true })
  expect(m.titleState).toBe('Без світла 3 год 27 хв')
})
test('right column with the grid on', () => {
  expect(overviewModel(liveCharging, now).right[0]).toMatchObject({ value: 'є з 08:10 · 7 год 07 хв', tone: 'good' })
  const idle = overviewModel(liveIdle, now)
  expect(idle.right[0].value).toBe('є')
  expect(idle.right[1].value).toBe('ще не було')
  expect(idle.outageRail).toBe('off')
  expect(idle.titleState).toBeUndefined()
})
test('previous outage on another day, with the station running out', () => {
  const s = snap({ Previous: { ID: 1, StationID: 'st1', Start: iso(at(19, 30, 20)), End: iso(at(2, 40)), SOCStart: 100, SOCEnd: 10, DiedAt: iso(at(1, 20)), Partial: false } })
  expect(overviewModel(s, now).right[1]).toMatchObject({ value: 'вчора 19:30–02:40 · 7 год 10 хв', meta: 'станція, ймовірно, розрядилась о 01:20' })
  const older = snap({ Previous: { ID: 1, StationID: 'st1', Start: iso(at(7, 0, 18)), End: iso(at(9, 0, 18)), SOCStart: null, SOCEnd: null, DiedAt: null, Partial: false } })
  expect(overviewModel(older, now).right[1].value).toBe('18.11 07:00–09:00 · 2 год')
})

test('gap card', () => {
  const gap = { ID: 7, StationID: 'st1', Start: iso(at(1, 20)), End: iso(at(2, 40)), Reason: 'app_not_running' }
  const m = overviewModel(snap({ GapCard: gap, Grid: 0, Mode: 2, Current: null }), now)
  expect(m.gapCard).toEqual({ id: 7, title: 'Поки додаток не працював · 01:20–02:40', text: 'Що було зі світлом у цей час, невідомо.', info: { key: 'info.overview.gapCard' } })
  const still = overviewModel(snap({ GapCard: { ...gap, Start: iso(at(12, 0)), End: iso(at(13, 0)) } }), now)
  expect(still.gapCard?.text).toBe('Світла досі немає — без світла вже 3 год 27 хв.')
  // An outage that began after the gap ended says nothing about the gap.
  expect(overviewModel(snap({ GapCard: gap }), now).gapCard?.text).toBe('Що було зі світлом у цей час, невідомо.')
  const old = overviewModel(snap({ GapCard: { ...gap, Start: iso(at(22, 0, 20)) } }), now)
  expect(old.gapCard?.title).toBe('Поки додаток не працював · 20.11 22:00–02:40')
})
test('no station yet: skeleton, looking for the station', () => {
  const zero = { Station: { ID: '', Name: '', Model: '' }, Profile: {}, Link: '', Diagnosis: '', Last: null, Age: 0, Grid: 0, Mode: 0, F: noForecast, NoLightToday: 0, OutagesToday: 0, Current: null, Previous: null, GapCard: null, Fills: { SOC: 0, Power: 0, Time: 0, NoLight: 0 }, ObservedSince: null } as unknown as Snapshot
  const m = overviewModel(zero, now)
  expect(m.variant).toBe('skeleton')
  expect(m.rings).toEqual([])
  expect(m.outageRail).toBe('unknown')
})
test('never had data and the link is declared lost: dashes with the no-link banner', () => {
  const m = overviewModel(withLink(connectingNoData, { Link: 'NO_LINK', Diagnosis: 'lan_silent' }), now)
  expect(m.variant).toBe('nolink')
  expect(m.rings.map(r => r.value)).toEqual(['—', '—', '—', '—'])
  expect(m.rings[3]).toMatchObject({ sub: '—', fill: 0 })
  expect(m.banner).toBe("Немає зв'язку зі станцією")
})

// Plan 2, Task 2: time in a data gap is unknown, not «без світла».
// Shared helpers and fixtures (Tasks 4b, 11 and 15 reuse them).
const right = (m: OverviewModel, label: string): RightRow => {
  const row = m.right.find(r => r.label === label)
  if (!row) throw new Error(`no right row «${label}» in ${m.right.map(r => r.label).join(', ')}`)
  return row
}
const exact = (start: number, end: number | null) =>
  ({ StartApprox: false, EndApprox: false, NotAfter: null, Frozen: false, AsOf: iso(end ?? now), Known: ((end ?? now) - start) * NS, AtLeast: false })
// First run ever at 09:00: the night was never observed.
const morningBoot = snap({
  Grid: 0, Mode: 1, Current: null, Previous: null, NoLightToday: 0, OutagesToday: 0, GapsToday: 9 * HOUR_NS,
  ObservedSince: iso(at(9, 0)), F: noForecast, Fills: { SOC: 1, Power: 0, Time: 1, NoLight: 0 },
}, { SOC: 100, BatteryW: 0, Grid: 1 })
// Off at 22:00, PC off 23:00–09:00, grid back by the morning start.
const prevEndInGap = snap({
  Grid: 0, Mode: 2, Current: null, NoLightAtLeast: true, NoLightToday: 30e3 * NS, OutagesToday: 1, GapsToday: 9 * HOUR_NS,
  Previous: { ID: 1, StationID: 'st1', Start: iso(at(22, 0, 20)), End: iso(at(9, 0)), SOCStart: 90, SOCEnd: 60, DiedAt: null, Partial: false },
  PreviousBounds: { StartApprox: false, EndApprox: true, NotAfter: iso(at(9, 0)), Frozen: false, AsOf: iso(at(9, 0)), Known: HOUR_NS, AtLeast: true },
}, { BatteryW: 900, Grid: 1 })
// Morning start after the night gap, an outage still open in the DB.
const nightGap = { ID: 7, StationID: 'st1', Start: iso(at(23, 0, 20)), End: iso(at(9, 0)), Reason: 'app_not_running' }
const nightOutage = { ID: 2, StationID: 'st1', Start: iso(at(22, 0, 20)), End: null, SOCStart: 90, SOCEnd: null, DiedAt: null, Partial: false }
// Resyncing: the grid is not confirmed yet.
const bootGridUnknown = snap({ Link: 'RESYNCING', Grid: 2, GapCard: nightGap, Current: nightOutage })
const bootGridOff = snap({ GapCard: nightGap, Current: nightOutage })
// Found off after a gap: the start is approximate.
const partialOff = snap({
  Current: { ID: 2, StationID: 'st1', Start: iso(at(11, 50)), End: null, SOCStart: 100, SOCEnd: null, DiedAt: null, Partial: true },
  CurrentBounds: { ...exact(at(11, 50), null), StartApprox: true, AtLeast: true },
})

test('no-light ring is a lower bound with gaps today', () => { const r = overviewModel({ ...liveDischarging, NoLightToday: HOUR_NS, GapsToday: 9*HOUR_NS }, now).rings[3]; expect(r.value).toBe('≥ 1:00'); expect(r.sub).toBe('є пропуски в даних') })
test('morning boot never claims no outages', () => expect(overviewModel(morningBoot, now).rings[3].sub).toBe('є пропуски в даних'))
test('gaps under 15 min keep the count', () => expect(overviewModel({ ...liveIdle, GapsToday: 10*MIN_NS, OutagesToday: 0 }, now).rings[3].sub).toBe('без відключень'))
test('previous outage with end in gap', () => { const row = right(overviewModel(prevEndInGap, now), 'Попереднє відключення'); expect(row.value).toBe('вчора 22:00–не пізніше 09:00 · ≥ 1 год') })
test('gap card says still off only when grid is confirmed off', () => { expect(overviewModel(bootGridUnknown, now).gapCard?.text).not.toMatch(/досі немає/); expect(overviewModel(bootGridOff, now).gapCard?.text).toMatch(/досі немає/) })
test('grid row uses offApprox for a partial outage', () => expect(right(overviewModel(partialOff, now), 'Світло').value).toMatch(/^немає з ≈ /))

test('a lower bound from the core is shown even with small gaps', () => {
  expect(overviewModel({ ...liveDischarging, NoLightToday: HOUR_NS, NoLightAtLeast: true }, now).rings[3]).toMatchObject({ value: '≥ 1:00', sub: '2 відключення' })
})
test('previous outage with an approximate start', () => {
  const s = snap({ PreviousBounds: { ...exact(at(6, 10), at(8, 10)), StartApprox: true, AtLeast: true, Known: 2 * HOUR_NS } })
  expect(right(overviewModel(s, now), 'Попереднє відключення').value).toBe('сьогодні ≈ 06:10–08:10 · ≥ 2 год')
})
test('an exact previous outage reads as before', () => {
  const s = snap({ PreviousBounds: exact(at(6, 10), at(8, 10)) })
  expect(right(overviewModel(s, now), 'Попереднє відключення').value).toBe('сьогодні 06:10–08:10 · 2 год')
})

test('overview errors row', () => expect(right(overviewModel({ ...liveIdle, NewEvents: 3 }, now), 'Помилки').value).toBe('3 нові'))

test('the errors row opens the Errors tab; red only with new events', () => {
  expect(right(overviewModel({ ...liveIdle, NewEvents: 3 }, now), 'Помилки')).toMatchObject({ icon: 'alert', tone: 'bad', view: 'errors' })
  expect(right(overviewModel({ ...liveIdle, NewEvents: 0 }, now), 'Помилки')).toMatchObject({ value: 'нових немає', icon: 'check-circle', tone: 'good', view: 'errors' })
  expect(right(overviewModel({ ...liveIdle, NewEvents: 1 }, now), 'Помилки').value).toBe('1 нова')
  expect(right(overviewModel({ ...liveIdle, NewEvents: 5 }, now), 'Помилки').value).toBe('5 нових')
  // Waiting for the first data: no rows, only the source (in the app bar).
  const waiting = overviewModel(connectingNoData, now)
  expect(waiting.right).toEqual([])
  expect(waiting.source).toMatchObject({ label: 'Джерело даних', value: 'LAN · підключаємось…' })
})


// The app does not read the battery's own data (Bat.GetStatus, owner decision
// 2026-11-23): the charge ring's capacity is the profile's nominal, and an idle
// battery on the grid has no other explanation.
test('the charge ring uses the nominal capacity; idle on the grid is just idle', () => {
  expect(overviewModel(snap(), now).rings[0].sub).toBe('3,69 з 5,12 кВт·год')
  const unknown = snap({ Profile: { Model: 'X', NominalWh: 0, MaxChargeW: 0, MaxDischargeW: 0, MinSOC: 0, CycleLife: 0, TempMinC: 0, TempMaxC: 0, Tested: false } })
  expect(overviewModel(unknown, now).rings[0].sub).toBe('—')
  const idle = overviewModel(snap({ Grid: 0, Mode: 1, Current: null, F: noForecast }, { SOC: 80, BatteryW: 0, Grid: 1 }), now)
  expect(idle.rings[1].sub).toBe('ні заряду, ні розряду')
})

// Task 21 (plan 1 deferred).

test('older snapshot is ignored', () => {
  const feed = overviewFeed()
  const s5 = { ...liveDischarging, Seq: 5 } as Snapshot
  const s4 = { ...stale, Seq: 4 } as Snapshot
  expect(feed.update(s5)).toBe(true)
  // A bound call that raced the event brings an older one: it is dropped.
  expect(feed.update(s4)).toBe(false)
  expect(feed.current()?.snapshot.Seq).toBe(5)
  expect(feed.current()?.model.variant).toBe('live')
  expect(feed.update({ ...stale, Seq: 6 } as Snapshot)).toBe(true)
  expect(feed.current()?.snapshot.Seq).toBe(6)
})

test('empty live series shows the note', () => {
  const note = "Графік з'явиться, щойно надійдуть перші дані."
  expect(chartNote('live', 0)).toBe(note)
  expect(chartNote('skeleton', null)).toBe(note)
  expect(chartNote('live', null)).toBeUndefined() // not loaded yet
  expect(chartNote('live', 12)).toBeUndefined()
})

test('the feed computes the model once per snapshot and tick, for every listener', () => {
  const count = vi.fn(overviewModel)
  const feed = overviewFeed(() => now, count)
  const seen: string[] = []
  const off = feed.on((m, s) => seen.push(`a ${s.Seq} ${m.variant}`))
  feed.on(m => seen.push(`b ${m.variant}`))
  feed.update({ ...liveDischarging, Seq: 2 } as Snapshot)
  feed.update({ ...stale, Seq: 1 } as Snapshot) // older: no model
  feed.tick()
  off()
  feed.tick()
  expect(count).toHaveBeenCalledTimes(3)
  expect(seen).toEqual(['a 2 live', 'b live', 'a 2 live', 'b live', 'b live'])
  expect(feed.current()?.model.variant).toBe('live')
})

// ⓘ (copy §15): the key of every ring and row, its variant in the stated state,
// and the arguments as on the visible row.
const infoOf = (m: OverviewModel) => m.rings.map(r => r.info?.key)
test('every ring has its ⓘ, with the arguments of its caption', () => {
  const m = overviewModel(liveDischarging, now)
  expect(infoOf(m)).toEqual(['info.overview.soc', 'info.overview.power', 'info.overview.toEmpty', 'info.overview.offToday'])
  expect(m.rings[1].info?.args).toEqual({ max: num(2500) })
  expect(m.rings[1].sub).toContain(num(2500))
  expect(m.rings[2].info?.args).toEqual({ floor: '10' })
  expect(infoOf(overviewModel(liveCharging, now))[2]).toBe('info.overview.toFull')
  expect(infoOf(overviewModel(liveIdle, now))[2]).toBe('info.overview.lasts')
  // Idle: no limit on the caption, the model's own on the ⓘ.
  expect(overviewModel(liveIdle, now).rings[1].info?.args).toEqual({ max: num(2500) })
})
test('the forecast ⓘ follows «рахуємо…» and «ще немає даних»', () => {
  const warming = snap({ F: { ...noForecast, Warming: true } })
  expect(overviewModel(warming, now).rings[2].info?.key).toBe('info.overview.calculating')
  const idleWarming = { ...liveIdle, F: { ...noForecast, Warming: true } } as Snapshot
  expect(overviewModel(idleWarming, now).rings[2].info?.key).toBe('info.overview.calculating')
  const idleNoHistory = { ...liveIdle, F: noForecast } as Snapshot
  expect(overviewModel(idleNoHistory, now).rings[2].sub).toBe('ще немає даних')
  expect(overviewModel(idleNoHistory, now).rings[2].info?.key).toBe('info.overview.lasts.noData')
})
test('the no-light ⓘ: gaps in the data, else the plain one', () => {
  const gaps = overviewModel({ ...liveDischarging, NoLightToday: HOUR_NS, GapsToday: 9 * HOUR_NS }, now).rings[3]
  expect(gaps.info?.key).toBe('info.overview.offToday.gaps')
  const atLeast = overviewModel({ ...liveDischarging, NoLightAtLeast: true }, now).rings[3]
  expect(atLeast.value).toMatch(/^≥ /)
  expect(atLeast.info?.key).toBe('info.overview.offToday')
  expect(overviewModel(liveDischarging, now).rings[3].info?.key).toBe('info.overview.offToday')
})
test('the right column ⓘ: grid (unknown while not live), previous outage, errors', () => {
  const m = overviewModel(liveDischarging, now)
  expect(m.right.map(r => r.info?.key)).toEqual(['info.overview.grid', 'info.overview.prevOutage', 'info.errors.new'])
  expect(right(overviewModel(stale, now), 'Світло').info?.key).toBe('info.overview.grid.unknown')
  expect(right(overviewModel(bootGridUnknown, now), 'Світло').info?.key).toBe('info.overview.grid.unknown')
  // Likely dead keeps the last known «немає»: the plain one.
  expect(right(overviewModel(likelyDead, now), 'Світло').value).toMatch(/^немає/)
  expect(right(overviewModel(likelyDead, now), 'Світло').info?.key).toBe('info.overview.grid')
})
test('the source in the app bar, the chart legend and the gap card have their ⓘ', () => {
  const m = overviewModel(bootGridOff, now)
  expect(m.source.info?.key).toBe('info.overview.source')
  expect(overviewModel(connectingNoData, now).source.info?.key).toBe('info.overview.source')
  expect(m.legend).toEqual([
    { key: 'info.overview.chart.outage' }, { key: 'info.overview.chart.noData' },
    { key: 'info.overview.chart.floor', args: { floor: '10' } },
  ])
  expect(overviewModel({ ...liveDischarging, Profile: { ...liveDischarging.Profile, MinSOC: 25 } }, now).legend?.[2].args).toEqual({ floor: '25' })
  expect(m.gapCard?.info).toEqual({ key: 'info.overview.gapCard' })
})

// Fix round 1 (Ruling 47).
test('the no-light ring keeps its own ⓘ in every state, «.gaps» only for gaps', () => {
  expect(overviewModel({ ...liveDischarging, NoLightAtLeast: true }, now).rings[3].info?.key).toBe('info.overview.offToday')
  expect(overviewModel(stale, now).rings[3].value).toMatch(/^≥ /)
  expect(overviewModel(stale, now).rings[3].info?.key).toBe('info.overview.offToday')
  expect(overviewModel(noLink, now).rings[3].info?.key).toBe('info.overview.offToday.gaps')
})
test('a profile without a power limit gets the ⓘ without «з 0 Вт макс.»', () => {
  const noMax = { ...liveIdle, Profile: { ...liveIdle.Profile, MaxChargeW: 0, MaxDischargeW: 0 } } as Snapshot
  const r = overviewModel(noMax, now).rings[1]
  expect(r.info).toEqual({ key: 'info.overview.power.noMax' })
  expect(overviewModel({ ...liveDischarging, Profile: { ...liveDischarging.Profile, MaxDischargeW: 0 } } as Snapshot, now).rings[1].info).toEqual({ key: 'info.overview.power.noMax' })
})

// Owner request: an unconfirmed grid change (the core's 30 s check) is shown.
const pendingFields = (grid: number, pending: number) => ({ Grid: grid, GridPending: pending, GridPendingSince: iso(now - 10e3) })
test('grid row while «з\'являється… перевіряємо» (confirmed off, pending on)', () => {
  const r = right(overviewModel(snap(pendingFields(1, 0)), now), 'Світло')
  expect(r.value).toBe('з\'являється… перевіряємо')
  expect(r.tone).toBeUndefined()
  expect(r.hero).toBeFalsy()
  expect(r.info?.key).toBe('info.overview.grid')
})
test('grid row while «зникає… перевіряємо» (confirmed on, pending off)', () => {
  const r = right(overviewModel(withLink(liveCharging, pendingFields(0, 1)), now), 'Світло')
  expect(r.value).toBe('зникає… перевіряємо')
  expect(r.tone).toBeUndefined()
  expect(r.hero).toBeFalsy()
})
test('grid row pending from unknown says checking and keeps the unknown ⓘ', () => {
  const r = right(overviewModel(withLink(liveCharging, pendingFields(2, 0)), now), 'Світло')
  expect(r.value).toBe('перевіряємо…')
  expect(right(overviewModel(withLink(liveCharging, pendingFields(2, 1)), now), 'Світло').value).toBe('перевіряємо…')
  expect(r.info?.key).toBe('info.overview.grid.unknown')
})
test('grid row is normal when nothing is pending (2 = none, or the fields absent)', () => {
  const none = { Grid: 1, GridPending: 2, GridPendingSince: null }
  expect(right(overviewModel(withLink(liveDischarging, none), now), 'Світло')).toMatchObject({ value: 'немає з 11:50 · 3 год 27 хв', tone: 'warn', hero: true })
  expect(right(overviewModel(liveDischarging, now), 'Світло').value).toMatch(/^немає з/)
})
test('grid row while pending in English', async () => {
  const { setLocale } = await import('../lib/i18n')
  setLocale('en')
  try {
    expect(right(overviewModel(snap(pendingFields(1, 0)), now), 'Grid').value).toBe('coming back… checking')
    expect(right(overviewModel(withLink(liveCharging, pendingFields(0, 1)), now), 'Grid').value).toBe('going off… checking')
  } finally { setLocale('uk') }
})
test('grid row ignores GridPending without GridPendingSince (the zero value is «on»)', () => {
  const s = withLink(liveCharging, { Grid: 0, GridPending: 0, GridPendingSince: null })
  expect(right(overviewModel(s, now), 'Світло').value).toMatch(/^є/)
})

test('the charge caption shows energy with the capacity precision (100 % reads full)', async () => {
  const sub = (soc: number) => overviewModel(snap({}, { SOC: soc }), now).rings[0]
  expect(sub(100).sub).toBe('5,12 з 5,12 кВт·год')
  expect(sub(95).sub).toBe('4,86 з 5,12 кВт·год')
  expect(sub(100).aria).toContain('5,12 з 5,12')
  expect(sub(93.75).sub).toBe('4,80 з 5,12 кВт·год')
  const { setLocale } = await import('../lib/i18n')
  setLocale('en')
  try {
    expect(sub(100).sub).toBe('5.12 of 5.12 kWh')
    expect(sub(93.75).sub).toBe('4.80 of 5.12 kWh')
    expect(sub(100).aria).toContain('5.12 of 5.12 kilowatt-hours')
  } finally { setLocale('uk') }
})

// Owner 2026-11-23: a start that lay in a gap is shown as the window it lies
// in, «між {from} і {to}», not «≈ {to}» (core Bounds.StartFrom).
test('grid row: an outage found after a gap names the window', () => {
  const s = snap({
    Current: { ...partialOff.Current, Start: iso(at(11, 50)) },
    CurrentBounds: { ...exact(at(11, 50), null), StartApprox: true, AtLeast: true, StartFrom: iso(at(1, 10)) },
  })
  expect(right(overviewModel(s, now), 'Світло').value).toBe('немає, зникло між 01:10 і 11:50 · ≥ 3 год 27 хв')
  // The gap began yesterday: its start carries the day.
  const y = snap({ ...s, CurrentBounds: { ...s.CurrentBounds, StartFrom: iso(at(23, 40, 20)) } })
  expect(right(overviewModel(y, now), 'Світло').value).toBe('немає, зникло між вчора 23:40 і 11:50 · ≥ 3 год 27 хв')
  const d = snap({ ...s, CurrentBounds: { ...s.CurrentBounds, StartFrom: iso(at(23, 40, 19)) } })
  expect(right(overviewModel(d, now), 'Світло').value).toBe('немає, зникло між 19.11 23:40 і 11:50 · ≥ 3 год 27 хв')
  // Not current: the last known state says the same, up to the last data.
  const old = snap({ CurrentBounds: s.CurrentBounds, Link: 'LIKELY_DEAD', Age: 30 * M * NS }, { At: iso(at(14, 47)), SOC: 10, BatteryW: -790 })
  expect(right(overviewModel(old, now), 'Світло').value).toBe('немає, зникло між 01:10 і 11:50 · ≥ 2 год 57 хв')
  // Approximate without a window (first run): «≈» as before.
  expect(right(overviewModel(partialOff, now), 'Світло').value).toMatch(/^немає з ≈ 11:50/)
})
test('en: grid row and previous outage name the window', () => {
  setLocale('en')
  try {
    const s = snap({ CurrentBounds: { ...exact(at(11, 50), null), StartApprox: true, AtLeast: true, StartFrom: iso(at(23, 40, 20)) } })
    expect(right(overviewModel(s, now), 'Grid').value).toBe('off, went off between yesterday 23:40 and 11:50 · ≥ 3 h 27 min')
    const p = snap({ PreviousBounds: { ...exact(at(6, 10), at(8, 10)), StartApprox: true, AtLeast: true, Known: 2 * HOUR_NS, StartFrom: iso(at(1, 5)) } })
    expect(right(overviewModel(p, now), 'Previous outage').value).toBe('between 01:05 and 06:10 – 08:10 · ≥ 2 h')
  } finally { setLocale('uk') }
})
test('previous outage found after a gap names the window', () => {
  const s = snap({ PreviousBounds: { ...exact(at(6, 10), at(8, 10)), StartApprox: true, AtLeast: true, Known: 2 * HOUR_NS, StartFrom: iso(at(1, 5)) } })
  expect(right(overviewModel(s, now), 'Попереднє відключення').value).toBe('між 01:05 і 06:10 – 08:10 · ≥ 2 год')
  // And an end in a gap as well.
  const both = snap({ PreviousBounds: { ...s.PreviousBounds, EndApprox: true, NotAfter: iso(at(8, 10)) } })
  expect(right(overviewModel(both, now), 'Попереднє відключення').value).toBe('між 01:05 і 06:10 – не пізніше 08:10 · ≥ 2 год')
})
test('previous outage window: each day named once, relative to today (controller ruling)', () => {
  const b = (start: number, end: number, from: number) => ({ ...exact(start, end), StartApprox: true, AtLeast: true, Known: 2 * HOUR_NS, StartFrom: iso(from) })
  const prev = (start: number, end: number) => ({ ID: 1, StationID: 'st1', Start: iso(start), End: iso(end), SOCStart: null, SOCEnd: null, DiedAt: null, Partial: true })
  // From yesterday, the start and end today.
  const y = snap({ Previous: prev(at(6, 10), at(8, 10)), PreviousBounds: b(at(6, 10), at(8, 10), at(23, 40, 20)) })
  expect(right(overviewModel(y, now), 'Попереднє відключення').value).toBe('між вчора 23:40 і 06:10 – 08:10 · ≥ 2 год')
  // From two days ago, the start and end yesterday.
  const d = snap({ Previous: prev(at(6, 10, 20), at(8, 10, 20)), PreviousBounds: b(at(6, 10, 20), at(8, 10, 20), at(23, 40, 19)) })
  expect(right(overviewModel(d, now), 'Попереднє відключення').value).toBe('між 19.11 23:40 і вчора 06:10 – 08:10 · ≥ 2 год')
})
