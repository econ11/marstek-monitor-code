import type { Bucket, Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import { clock, kwh, num } from '../lib/format'
import { t } from '../lib/i18n'
import { ymd } from '../ui/calendar'
import { escapeHTML as esc } from '../ui/tooltip'
import { bucketLabel, bucketTitle } from './outagesModel'
import type { RingProps } from './overviewModel'

// Everything the «Енергія» tab shows, derived from one energy.Report
// (Task 12; hi-fi 05-energy, copy §8). Pure: the view only turns this into DOM.
// The brief's types are extended with optional fields only (before, today).

export type EnergyBar = {
  key: string
  label: string
  /** kWh. */
  charge: number
  discharge: number
  /** Watched, but nothing was measured: hatched, never drawn as zero. */
  noData: boolean
  tip: string /*html*/
  /** Before monitoring began: not drawn at all. */
  before?: boolean
  /** Today's column: its axis label is emphasised. */
  today?: boolean
  /** Not measured yet (the minutes after midnight): drawn empty, read as «ще рахуємо», never as 0. */
  pending?: boolean
}

export type EnergyModel = {
  totals: { charged: string; discharged: string; dischargedMeta?: string; efficiency: string; efficiencyMeta?: string }
  incomplete?: string
  /** The rollup is still counting past days (Report.Backfilling). */
  backfilling?: string
  title: string
  bars: EnergyBar[]
  pvRing?: RingProps
  empty?: string
}

const NS_PER_MS = 1e6
const MIN = 60e3
/** The rollup covers only minutes that ended at least MaxStep (60 s) ago, so
 *  the last ≤ 2 minutes before now are not counted yet (core/monitor
 *  rollupEnergy): that tail is a lag, not a hole. */
const ROLLUP_LAG = 2 * MIN
/** Below this the totals read «0,0»: nothing was charged or discharged. */
const NOTHING_WH = 50
/** Less covered than this, the totals say they are incomplete. */
const COMPLETE = 0.99
const NONE = () => t('common.value.none')

const total = (wh: number) => t('energy.total.value', { kwh: kwh(wh) })
const line = (html: string, cls = '') => `<div class="tip__line${cls ? ' ' + cls : ''}">${html}</div>`
const dot = (tone: string) => `<span class="t-${tone}" aria-hidden="true">●</span> `

/** The period reaches now: core/history.PeriodFor cut its end at the moment
 *  of the report, which is not a midnight. Its last minutes are still being
 *  rolled up, and its last column is today (or this week). */
function isLive(r: Report): boolean {
  const d = new Date(r.Period.To)
  return d.getHours() + d.getMinutes() + d.getSeconds() + d.getMilliseconds() > 0
}

/** Nothing measured, but only for the rollup lag: the first minutes after midnight today. */
const lagOnly = (b: Bucket, last: boolean) => last && Date.parse(b.End) - Date.parse(b.Start) <= ROLLUP_LAG + MIN

/** Why a watched day has no energy: counting began after it (on an upgraded
 *  database the raw samples older than 30 days were gone), the backfill has
 *  not reached it yet, or a real gap. */
function noDataReason(b: Bucket, r: Report): string {
  const since = r.EnergySince ? Date.parse(r.EnergySince) : null
  const end = Date.parse(b.End)
  if (since != null && end <= since) return 'energy.tooltip.noData.old'
  if (r.Backfilling) return 'energy.tooltip.noData.pending'
  return 'energy.tooltip.noData'
}

function tipHTML(b: Bucket, r: Report, now: number): string {
  const weeks = r.Period.Weeks
  let html = `<div class="tip__title">${esc(bucketTitle(b, weeks, now))}</div>`
  if (b.NoData) {
    return html + line(esc(t(noDataReason(b, r))))
  }
  html += line(dot('good') + esc(t('energy.tooltip.charge', { kwh: kwh(b.ChargedWh) })))
  if (r.HasPV) html += line(esc(t('energy.tooltip.charge.split', { grid: kwh(Math.max(0, b.ChargedWh - b.PVChargeWh)), pv: kwh(b.PVChargeWh) })), 'tip__sub')
  html += line(dot('warn') + esc(t('energy.tooltip.discharge', { kwh: kwh(b.DischargedWh) })))
  if (b.OutageWh > 0) html += line(esc(t('energy.tooltip.discharge.outage', { kwh: kwh(b.OutageWh) })), 'tip__sub')
  if (b.PeakW != null && b.PeakAt) html += line(esc(t('energy.tooltip.peak', { watts: num(Math.round(b.PeakW)), time: clock(Date.parse(b.PeakAt)) })))
  return html
}

/** current: the live period's last column, today or this week. */
function barOf(b: Bucket, r: Report, now: number, current: boolean): EnergyBar {
  const bar: EnergyBar = {
    key: ymd(Date.parse(b.Start)),
    label: bucketLabel(b, r.Period.Weeks),
    charge: b.ChargedWh / 1000,
    discharge: b.DischargedWh / 1000,
    noData: b.NoData && !lagOnly(b, current),
    tip: '',
  }
  if (current) bar.today = true
  // Nothing covered and not «no data»: the bucket lies before monitoring began.
  if (b.Covered === 0 && !b.NoData) bar.before = true
  else if (b.NoData && !bar.noData) bar.pending = true
  else bar.tip = tipHTML(b, r, now)
  return bar
}

/** The share of the observed time that has data, leaving out the minutes the
 *  rollup has not reached yet. Coverage = covered / observed (core/energy), so
 *  observed = covered / Coverage. */
function coverage(r: Report, covered: number, live: boolean): number {
  if (covered <= 0 || r.Coverage <= 0) return 0
  if (!live) return r.Coverage
  const observed = covered / r.Coverage
  return Math.min(1, covered / Math.max(covered, observed - ROLLUP_LAG))
}

function pvRing(r: Report, buckets: Bucket[]): RingProps | undefined {
  if (!r.HasPV || r.ChargedWh <= 0) return undefined
  const pv = Math.min(r.ChargedWh, buckets.reduce((a, b) => a + b.PVChargeWh, 0))
  const grid = (r.ChargedWh - pv) / r.ChargedWh
  const gridPct = Math.round(grid * 100)
  return {
    key: 'split', title: t('energy.ring.split.label'), value: `${gridPct}%`,
    sub: t('energy.ring.split.sub', { pv: 100 - gridPct }), fill: grid, tone: 'info', stale: false,
  }
}

/** energyModel turns a report into what the tab shows at `now` (epoch ms). */
export function energyModel(r: Report, now: number): EnergyModel {
  const buckets = r.Buckets ?? []
  const covered = buckets.reduce((a, b) => a + b.Covered, 0) / NS_PER_MS
  const live = isLive(r)
  const bars = buckets.map((b, i) => barOf(b, r, now, live && i === buckets.length - 1))
  const m: EnergyModel = {
    totals: { charged: NONE(), discharged: NONE(), efficiency: NONE() },
    title: t(r.Period.Weeks ? 'energy.chart.title.weeks' : 'energy.chart.title'),
    bars,
  }
  const ring = pvRing(r, buckets)
  if (ring) m.pvRing = ring
  // Some columns are «no data»: the period was watched, it just has holes.
  const holes = bars.some(b => b.noData)
  if (covered > 0) {
    m.totals.charged = total(r.ChargedWh)
    m.totals.discharged = total(r.DischargedWh)
    if (r.OutageWh > 0) m.totals.dischargedMeta = t('energy.tooltip.discharge.outage', { kwh: kwh(r.OutageWh) })
    // Over 100%: the battery gave out charge it held before the period, so
    // the ratio is not an efficiency; «—» with the reason rather than a cap.
    if (r.Efficiency == null) m.totals.efficiencyMeta = t('energy.total.efficiency.tooSmall')
    else if (r.Efficiency > 100) m.totals.efficiencyMeta = t('energy.total.efficiency.over')
    else m.totals.efficiency = t('energy.total.efficiency.value', { percent: Math.round(r.Efficiency) })
    if (r.ChargedWh < NOTHING_WH && r.DischargedWh < NOTHING_WH) m.empty = t('energy.empty')
  } else if (bars.every(b => b.before)) {
    m.empty = t('energy.empty.noData')
  }
  // While past days are still being counted, their holes are not real: say that instead.
  if (r.Backfilling) {
    m.backfilling = t('energy.backfilling')
    delete m.empty
    return m
  }
  const c = coverage(r, covered, live)
  if ((covered > 0 || holes) && c < COMPLETE) m.incomplete = t('energy.incomplete', { percent: Math.round(c * 100) })
  return m
}
