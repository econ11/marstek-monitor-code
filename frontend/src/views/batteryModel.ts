import type { Month, Summary } from '../../bindings/github.com/econ11/marstek-monitor-code/core/health/models'
import { num } from '../lib/format'
import { getLocale, INTL, t } from '../lib/i18n'
import { escapeHTML as esc } from '../ui/tooltip'
import type { RingProps } from './overviewModel'

// Everything the «Батарея» tab shows, derived from one health.Summary
// (Task 14; hi-fi 04-battery, copy §7, spec §7.5). Pure: the view only turns
// this into DOM. The brief's types are extended with optional fields only.
// The tab has two rings, health and cycles: the app does not read the
// battery's own data (Bat.GetStatus restarts some stations; owner decision
// 2026-11-23, docs/dev-notes.md).

/** at: the month's first day (epoch ms), for the year on the axis. value:
 *  our estimate, in kWh. */
export type CapMonth = { label: string; value: number; approx: boolean; tip: string /*html*/; a11y?: string; at?: number }

export type BatteryModel = {
  rings: RingProps[] /* 'health', 'cycles' */
  chart: { months: CapMonth[]; nominal: number }
  empty?: { title: string; body: string; progress?: string }
}

/** Below this the health ring turns red (info.battery.health; owner may change it). */
export const HEALTH_BAD_PCT = 70
/** The discharge depth a capacity estimate needs (core/health MinDeltaSOC). */
const MIN_DELTA_SOC = 50

const NONE = () => t('common.value.none')
const estimate = (v: string) => `${t('common.estimate.prefix')} ${v}`
const fixed = (x: number, frac: number) => new Intl.NumberFormat(INTL[getLocale()], { minimumFractionDigits: frac, maximumFractionDigits: frac }).format(x)
const fixed2 = (x: number) => fixed(x, 2)
const clamp01 = (x: number) => Math.min(Math.max(x, 0), 1)
const kwh2 = (wh: number) => Math.round(wh / 10) / 100

/** The cycles shown, on the ring and on the overview row alike: one decimal
 *  below 10 («≈ 0,3» on a new station), whole cycles from 10; rounded down,
 *  so a cycle is never shown before it is complete. */
export const shownCycles = (c: number) => (c < 10 ? Math.floor(c * 10) / 10 : Math.floor(c))

function healthRing(s: Summary): RingProps {
  const title = t('battery.ring.health.label')
  if (s.HealthPct == null) {
    return { key: 'health', title, value: NONE(), sub: t('battery.ring.health.noData.sub'), fill: 0, tone: 'muted', stale: false, info: { key: 'info.battery.health.pending' } }
  }
  const pct = t('battery.ring.health.value', { percent: Math.round(s.HealthPct) })
  const sub = s.Approx
    ? t('battery.ring.health.sub.prelim')
    : s.ActualWh != null ? t('battery.ring.health.sub', { actual: fixed2(s.ActualWh / 1000), nominal: fixed2(s.NominalWh / 1000) }) : ''
  return {
    key: 'health', title, value: s.Approx ? estimate(pct) : pct, sub,
    fill: clamp01(s.HealthPct / 100), tone: s.HealthPct < HEALTH_BAD_PCT ? 'bad' : 'good', stale: false,
    info: { key: s.Approx ? 'info.battery.health.prelim' : 'info.battery.health' },
  }
}

function cyclesRing(s: Summary): RingProps {
  const title = t('battery.ring.cycles.label')
  const rated = s.CycleLife > 0 && s.RatedDoD > 0
  // The station does not report cycles: we count them, so they carry «≈».
  const args = { rated: String(s.CycleLife), dod: Math.round(s.RatedDoD * 100) }
  const sub = rated ? t('battery.ring.cycles.sub.estimated', args) : ''
  // The ⓘ with the ring's own figures; one cycle = RatedDoD × NominalWh (Ruling 19).
  const info = rated ? { key: 'info.battery.cycles', args: { ...args, cycleKWh: fixed((s.RatedDoD * s.NominalWh) / 1000, 1) } } : undefined
  if (s.Cycles == null) return { key: 'cycles', title, value: NONE(), sub, fill: 0, tone: 'info', stale: false, info }
  return {
    key: 'cycles', title, value: estimate(t('battery.ring.cycles.value', { cycles: num(shownCycles(s.Cycles), 1) })), sub,
    fill: s.CycleLife > 0 ? clamp01(s.Cycles / s.CycleLife) : 0, tone: 'info', stale: false, info,
  }
}

/** One chart month of our estimate. */
function monthOf(m: Month): CapMonth {
  const d = new Date(Date.parse(m.Month))
  const locale = INTL[getLocale()]
  // «лист.» → «лист» (hi-fi 04-battery axis).
  const label = new Intl.DateTimeFormat(locale, { month: 'short' }).format(d).replace(/\.$/, '')
  const name = `${new Intl.DateTimeFormat(locale, { month: 'long' }).format(d)} ${d.getFullYear()}`
  const value = kwh2(m.MedianWh)
  const capacity = fixed2(value)
  let tip = `<div class="tip__line">${esc(t('battery.chart.tooltip', { month: name, capacity, count: m.Count }))}</div>`
  if (m.Approx) tip += `<div class="tip__foot">${esc(t('info.battery.health.prelim'))}</div>`
  const a11y = t(m.Approx ? 'battery.chart.point.approx.a11y' : 'battery.chart.point.a11y', { month: name, capacity })
  return { label, value, approx: m.Approx, tip, a11y, at: new Date(d.getFullYear(), d.getMonth(), 1).getTime() }
}

/** batteryModel turns a health summary into what the tab shows. */
export function batteryModel(s: Summary): BatteryModel {
  const months = (s.Months ?? []).map(monthOf)
  const m: BatteryModel = {
    rings: [healthRing(s), cyclesRing(s)],
    chart: { months, nominal: kwh2(s.NominalWh) },
  }
  if (!months.length) {
    m.empty = { title: t('battery.empty.title'), body: t('battery.empty.body') }
    const pp = Math.round(s.MaxDelta)
    if (pp > 0 && pp < MIN_DELTA_SOC) m.empty.progress = t('battery.empty.progress', { pp })
  }
  return m
}
