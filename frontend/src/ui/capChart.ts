import { getLocale, INTL, t } from '../lib/i18n'
import type { CapMonth } from '../views/batteryModel'
import { escapeHTML as esc } from './tooltip'

// «Ємність по місяцях, кВт·год» (hi-fi 04-battery): one point per month on a
// line, the rated capacity as a dashed line with its label. The scale does
// not start at zero (a few per cent are invisible otherwise, hi-fi note 1).
// Approximate months (fewer than 3 discharges) are hollow points, and the
// line into or out of one is dashed (hi-fi note 2). Every point has a
// focusable hit circle (.hitpt, data-i) for its tooltip. When the month
// labels would overlap, only every k-th is drawn (with its value label),
// counted back from the last month; the year goes on the first label drawn
// and on each one where the year changes.

export type CapChartOpts = { height?: number; label?: string }

const PAD = { l: 44, r: 16, t: 24, b: 30 }
/** Grid steps, kWh: the smallest that keeps the scale at ≤ 5 lines. */
const STEPS = [0.1, 0.2, 0.5, 1, 2]
/** kWh kept free above and below the outermost value. */
const MARGIN = 0.08
/** Room at the sides for the first and last value labels. */
const INSET = 24
/** Rough width of one character of an 11 px axis label, and the gap kept
 *  between two labels: enough to decide how many labels fit. */
const CHAR_PX = 6.5, LABEL_GAP = 8
/** Characters a year adds to a label («␣2026»). */
const YEAR_CHARS = 5

const r1 = (v: number) => Math.round(v * 10) / 10
const fmt = (x: number, frac: number) => new Intl.NumberFormat(INTL[getLocale()], { minimumFractionDigits: frac, maximumFractionDigits: frac }).format(x)

function scale(values: number[]): { lo: number; hi: number; step: number } {
  const min = Math.min(...values) - MARGIN, max = Math.max(...values) + MARGIN
  for (const step of STEPS) {
    const lo = Math.floor(min / step) * step, hi = Math.ceil(max / step) * step
    if (Math.round((hi - lo) / step) <= 4) return { lo, hi, step }
  }
  const step = STEPS[STEPS.length - 1]
  return { lo: Math.floor(min / step) * step, hi: Math.ceil(max / step) * step, step }
}

/** thin returns each month's axis label, or null where it is left out so the
 *  labels do not overlap at slot px between months. */
function thin(months: CapMonth[], slot: number): (string | null)[] {
  const n = months.length
  const widest = Math.max(...months.map(m => m.label.length)) + YEAR_CHARS
  const every = Math.max(1, Math.ceil((widest * CHAR_PX + LABEL_GAP) / slot))
  let year: number | null = null
  return months.map((m, i) => {
    if ((n - 1 - i) % every) return null
    const y = m.at != null ? new Date(m.at).getFullYear() : null
    const text = y != null && y !== year ? `${m.label} ${y}` : m.label
    year = y
    return text
  })
}

/** capChartSVG draws the months at the given width (px); nominal is in kWh
 *  (0 = unknown: no rated line). */
export function capChartSVG(width: number, months: CapMonth[], nominal: number, opts: CapChartOpts = {}): string {
  const W = Math.max(width, PAD.l + PAD.r + 2 * INSET), H = opts.height ?? 250
  const label = opts.label ? ` aria-label="${esc(opts.label)}"` : ''
  // Nothing to draw (the tab shows its empty state instead): no scale to compute.
  if (!months.length) return `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="group"${label}></svg>`
  const { lo, hi, step } = scale([...months.map(m => m.value), ...(nominal > 0 ? [nominal] : [])])
  const n = months.length
  const x = (i: number) => r1(n > 1 ? PAD.l + INSET + i * (W - PAD.l - PAD.r - 2 * INSET) / (n - 1) : (PAD.l + W - PAD.r) / 2)
  const y = (v: number) => r1(PAD.t + (1 - (v - lo) / (hi - lo)) * (H - PAD.t - PAD.b))
  const shown = thin(months, n > 1 ? (W - PAD.l - PAD.r - 2 * INSET) / (n - 1) : Infinity)
  let s =`<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="group"${label}><g class="grid">`
  const ticks = Math.round((hi - lo) / step)
  for (let k = 0; k <= ticks; k++) s += `<line x1="${PAD.l}" x2="${W - PAD.r}" y1="${y(lo + k * step)}" y2="${y(lo + k * step)}"${k ? '' : ' class="base"'}/>`
  s += '</g><g class="axis">'
  for (let k = 0; k <= ticks; k++) s += `<text x="${PAD.l - 8}" y="${r1(y(lo + k * step) + 4)}" text-anchor="end">${esc(fmt(lo + k * step, 1))}</text>`
  shown.forEach((text, i) => { if (text != null) s += `<text x="${x(i)}" y="${H - 8}" text-anchor="middle"${i === n - 1 ? ' class="strong"' : ''}>${esc(text)}</text>` })
  s += '</g>'
  if (nominal > 0) {
    s += `<line class="nominal" x1="${PAD.l}" x2="${W - PAD.r}" y1="${y(nominal)}" y2="${y(nominal)}"/>` +
      `<text class="nominal-label" x="${W - PAD.r}" y="${r1(y(nominal) - 6)}" text-anchor="end">${esc(t('battery.chart.nominal', { capacity: fmt(nominal, 2) }))}</text>`
  }
  for (let i = 1; i < n; i++) {
    const dashed = months[i - 1].approx || months[i].approx
    s += `<path class="cap-line${dashed ? ' cap-line--approx' : ''}" d="M${x(i - 1)},${y(months[i - 1].value)}L${x(i)},${y(months[i].value)}"/>`
  }
  months.forEach((m, i) => {
    s += `<circle class="dot${m.approx ? ' approx' : ''}" cx="${x(i)}" cy="${y(m.value)}" r="4.5"/>` +
      (shown[i] != null ? `<text class="val" x="${x(i)}" y="${r1(y(m.value) - 12)}" text-anchor="middle">${esc(fmt(m.value, 2))}</text>` : '') +
      `<circle class="hitpt" data-i="${i}" cx="${x(i)}" cy="${y(m.value)}" r="18" tabindex="0" role="img" aria-label="${esc(m.a11y ?? m.label)}"/>`
  })
  return s + '</svg>'
}
