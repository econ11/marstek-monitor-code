import type { PowerSeries } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import { clock, dateShort, dayShort, num } from '../lib/format'
import { t } from '../lib/i18n'
import { escapeHTML as esc } from '../ui/tooltip'

// The «Потужність батареї» chart of the Energy tab (Task 21c), derived from
// one energy.PowerSeries. Pure: the chart (ui/powerChart) only draws it.
// Above zero is charge (green), below is discharge (amber); a missing minute
// (hour) or an unknown value breaks the line and is never drawn as 0.

export type PowerTone = 'charge' | 'discharge' | 'idle'
/** One measured minute (hour): a focusable point of the chart. */
export type PowerPointView = { t: number; w: number; tone: PowerTone; tip: string /*html*/; aria: string }

export type PowerModel = {
  title: string
  /** «середнє за годину» (and «показано останній 31 день» when clipped). */
  note?: string
  /** Hourly averages (a period of several days), not minutes. */
  hourly: boolean
  /** Bucket length, ms. */
  step: number
  /** The axis: 00:00–24:00 of the day (by the clock), or the period's days. */
  t0: number
  t1: number
  now: number
  ticks: { t: number; label: string }[]
  /** Continuous stretches of the line: a break between two runs is a hole. */
  runs: { t: number; w: number }[][]
  points: PowerPointView[]
  /** pinned: beyond the scale (an instant peak over hourly averages): drawn on the edge. */
  peak?: { t: number; w: number; label: string; caption: string; pinned: boolean }
  /** Outages, shaded. */
  bands: { start: number; end: number }[]
  /** The y scale, W: yMin ≤ 0 ≤ yMax, round numbers. */
  yMax: number
  yMin: number
  /** The plot's y (px) of a power value. */
  y: (w: number) => number
  height: number
  a11y: string
  empty?: string
}

const NS_PER_MS = 1e6
const MIN = 60e3, HOUR = 3600e3
/** ±20 W is idle (core/detect.BatteryDeadbandW). */
const DEADBAND_W = 20
/** The smallest scale on either side, W. */
const MIN_SCALE = 100
/** Chart height and its plot margins, px. */
export const POWER_H = 236
export const POWER_PAD = { t: 26, b: 26 }

export const tone = (w: number): PowerTone => (w > DEADBAND_W ? 'charge' : w < -DEADBAND_W ? 'discharge' : 'idle')
const watts = (w: number) => num(Math.round(Math.abs(w)))

/** nice rounds up to 1, 1.5, 2, 2.5, 3, 4, 5, 6 or 8 × 10ⁿ: grid lines at round watts. */
function nice(v: number): number {
  if (v <= 0) return 0
  const p = Math.pow(10, Math.floor(Math.log10(v)))
  for (const k of [1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10]) if (k * p >= v - 1e-9) return k * p
  return 10 * p
}

function ticksOf(t0: number, t1: number, hourly: boolean): { t: number; label: string }[] {
  const out: { t: number; label: string }[] = []
  const d = new Date(t0)
  if (!hourly) {
    // Placed by the clock: on a 23 or 25 hour day the hours keep their real place.
    for (let h = 0; h < 24; h += 3) {
      const at = new Date(d.getFullYear(), d.getMonth(), d.getDate(), h).getTime()
      out.push({ t: at, label: clock(at) })
    }
    out.push({ t: t1, label: '24:00' })
    return out
  }
  const days: number[] = []
  for (let i = 0; ; i++) {
    const at = new Date(d.getFullYear(), d.getMonth(), d.getDate() + i).getTime()
    if (at >= t1) break
    days.push(at)
  }
  const every = days.length <= 10 ? 1 : days.length <= 16 ? 2 : 4
  // Counted back from the last day, so the newest day is always labelled.
  for (let i = days.length - 1; i >= 0; i -= every) out.unshift({ t: days[i], label: dayShort(days[i]) })
  return out
}

function when(at: number, hourly: boolean): string {
  if (!hourly) return clock(at)
  return t('energy.power.tooltip.hour', { day: dateShort(at), start: clock(at), end: clock(at + HOUR) })
}

/** start: the minute (hour) the value is for; t: where it is drawn — an
 *  hour at its middle, so the area lines up with outage bands and the peak. */
function pointOf(start: number, at: number, w: number, outage: boolean, hourly: boolean): PowerPointView {
  const k = tone(w)
  const time = when(start, hourly)
  const head = k === 'idle' ? t('energy.power.tooltip.idle', { time }) : t(`energy.power.tooltip.${k}`, { time, watts: watts(w) })
  const lines: string[] = []
  if (hourly) lines.push(t('energy.power.hourly'))
  if (outage) lines.push(t('overview.chart.legend.outage'))
  const tip = `<div class="tip__title">${esc(head)}</div>` + lines.map(l => `<div class="tip__line">${esc(l)}</div>`).join('')
  return { t: at, w, tone: k, tip, aria: [head, ...lines].join(' · ') }
}

/** powerModel turns a series into what the chart shows at `now` (epoch ms). */
export function powerModel(s: PowerSeries, now: number): PowerModel {
  const t0 = Date.parse(s.Start), t1 = Date.parse(s.End)
  const step = s.Step / NS_PER_MS || MIN
  const hourly = step >= HOUR
  const bands = (s.Outages ?? []).map(o => ({ start: Date.parse(o.Start), end: Date.parse(o.End) }))
  const runs: { t: number; w: number }[][] = []
  const points: PowerPointView[] = []
  let run: { t: number; w: number }[] = []
  let prev = -Infinity
  for (const p of s.Points ?? []) {
    const start = Date.parse(p.At), at = hourly ? start + step / 2 : start
    // A missing bucket between two points, or an unknown value, is a hole.
    if (p.W == null || at - prev > step) {
      if (run.length) runs.push(run)
      run = []
    }
    prev = at
    if (p.W == null) continue
    run.push({ t: at, w: p.W })
    points.push(pointOf(start, at, p.W, bands.some(b => at >= b.start && at < b.end), hourly))
  }
  if (run.length) runs.push(run)

  let peak: PowerModel['peak']
  if (s.PeakW != null && s.PeakAt) {
    const at = Date.parse(s.PeakAt), w = s.PeakW
    const caption = hourly
      ? t('energy.power.peak.range', { watts: watts(w), date: dateShort(at), time: clock(at) })
      : t('energy.tooltip.peak', { watts: watts(w), time: clock(at) })
    peak = { t: at, w, label: t('energy.power.peak.mark', { watts: watts(w) }), caption, pinned: false }
  }

  const ws = points.map(p => p.w)
  let yMax = nice(Math.max(MIN_SCALE, ...ws))
  // A day's scale holds its peak; hourly averages keep their own scale (an
  // instant peak would flatten them) and a peak beyond it is pinned.
  let yMin = -nice(Math.max(MIN_SCALE, ...ws.map(w => -w), peak && !hourly ? peak.w : 0))
  // The quieter side keeps a quarter of the other, so zero never sits on the edge.
  yMax = Math.max(yMax, nice(-yMin / 4))
  yMin = Math.min(yMin, -nice(yMax / 4))
  if (peak) peak.pinned = -peak.w < yMin
  const height = POWER_H
  const y = (w: number) => POWER_PAD.t + ((yMax - w) / (yMax - yMin)) * (height - POWER_PAD.t - POWER_PAD.b)

  const notes: string[] = []
  if (hourly) notes.push(t('energy.power.hourly'))
  if (s.Clipped) notes.push(t('energy.power.clipped'))
  const m: PowerModel = {
    title: t('energy.power.title'),
    hourly, step, t0, t1, now,
    ticks: ticksOf(t0, t1, hourly),
    runs, points, bands,
    yMax, yMin, y, height,
    a11y: t(hourly ? 'energy.power.a11y.hours' : 'energy.power.a11y.minutes'),
  }
  if (notes.length) m.note = notes.join(' · ')
  if (peak) m.peak = peak
  if (!points.length) m.empty = t('energy.empty.noData')
  return m
}
