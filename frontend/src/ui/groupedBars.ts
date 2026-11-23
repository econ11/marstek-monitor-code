import { kwh, num } from '../lib/format'
import { t } from '../lib/i18n'
import type { EnergyBar } from '../views/energyModel'
import { escapeHTML as esc } from './tooltip'

// «кВт·год по днях» (hi-fi 05-energy): a pair of columns per day (or week),
// charge (green) then discharge (amber), 2 px apart, the pair at most 40 px
// wide. The whole day's slot is the hover and focus target. A day with no
// data is hatched to full height, never drawn as zero; days before
// monitoring began are not drawn at all.

export type GroupedBarsOpts = {
  height?: number
  /** Accessible name of the chart. */
  label?: string
}

const PAD = { l: 40, r: 4, t: 16, b: 30 }
/** Grid steps, kWh: the smallest that keeps the scale at ≤ 6 lines. */
const STEPS = [0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000]
/** Up to this many columns every column gets its label. */
const DENSE = 10
/** One column of the pair; two of them and the gap make ≤ 40 px. */
const BAR_MAX = 19, BAR_GAP = 2

// energy-dis: discharge is amber with faint diagonal stripes, so the pair is
// not told apart by colour alone (with the fixed order: charge left).
const HATCH = '<defs><pattern id="energy-hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">' +
  '<line x1="0" y1="0" x2="0" y2="6" stroke="rgb(138 148 161 / 0.22)" stroke-width="1.5"/></pattern>' +
  '<pattern id="energy-dis" width="5" height="5" patternUnits="userSpaceOnUse" patternTransform="rotate(-45)">' +
  '<rect width="5" height="5" fill="var(--color-warn)"/><line x1="0" y1="0" x2="0" y2="5" stroke="rgb(14 17 21 / 0.28)" stroke-width="1.5"/></pattern></defs>'

const r1 = (v: number) => Math.round(v * 10) / 10

function scale(items: EnergyBar[]): { max: number; step: number } {
  const top = Math.max(0, ...items.filter(i => !i.noData && !i.before).map(i => Math.max(i.charge, i.discharge)))
  const step = STEPS.find(s => Math.ceil(top / s) <= 6) ?? STEPS[STEPS.length - 1]
  return { max: Math.max(step * 2, Math.ceil(top / step) * step), step }
}

function ariaOf(i: EnergyBar): string {
  if (i.noData) return `${i.label}: ${t('outages.chart.day.noData')}`
  if (i.pending) return `${i.label}: ${t('energy.chart.day.pending')}`
  return t('energy.chart.bar.a11y', { day: i.label, charge: kwh(i.charge * 1000), discharge: kwh(i.discharge * 1000) })
}

/** A column with a rounded top from the baseline y0 up to top. */
function bar(cls: string, x: number, w: number, top: number, y0: number): string {
  const h = y0 - top
  if (h < 1) return ''
  const rr = Math.min(3, h / 2, w / 2)
  return `<path class="bar ${cls}" d="M${r1(x)},${r1(y0)}V${r1(top + rr)}q0,-${r1(rr)} ${r1(rr)},-${r1(rr)}h${r1(w - 2 * rr)}q${r1(rr)},0 ${r1(rr)},${r1(rr)}V${r1(y0)}Z"/>`
}

/** groupedBarsSVG draws the pairs at the given width (px). Each drawn day is
 *  a <g class="bar-g" data-key data-i>; those with a tooltip are focusable. */
export function groupedBarsSVG(width: number, items: EnergyBar[], opts: GroupedBarsOpts = {}): string {
  const W = Math.max(width, PAD.l + PAD.r + items.length), H = opts.height ?? 300
  const { max, step } = scale(items)
  const y = (v: number) => PAD.t + (1 - v / max) * (H - PAD.t - PAD.b)
  const y0 = y(0)
  const slot = (W - PAD.l - PAD.r) / Math.max(1, items.length)
  const bw = Math.max(1, Math.min(BAR_MAX, (slot * 0.62 - BAR_GAP) / 2))
  const dense = items.length <= DENSE
  const label = opts.label ? ` aria-label="${esc(opts.label)}"` : ''
  let s = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="group"${label}>${HATCH}<g class="grid">`
  const ticks = Math.round(max / step)
  for (let k = 0; k <= ticks; k++) s += `<line x1="${PAD.l}" x2="${W - PAD.r}" y1="${r1(y(k * step))}" y2="${r1(y(k * step))}"${k ? '' : ' class="base"'}/>`
  s += '</g><g class="axis">'
  for (let k = 0; k <= ticks; k++) s += `<text x="${PAD.l - 8}" y="${r1(y(k * step) + 4)}" text-anchor="end">${esc(num(k * step, 1))}</text>`
  items.forEach((it, i) => {
    // Many columns: every fifth label, counted back from the last (today).
    if (dense || i % 5 === (items.length - 1) % 5) {
      s += `<text x="${r1(PAD.l + slot * i + slot / 2)}" y="${H - 10}" text-anchor="middle"${it.today ? ' class="strong"' : ''}>${esc(it.label)}</text>`
    }
  })
  s += '</g>'
  items.forEach((it, i) => {
    if (it.before) return
    const cx = PAD.l + slot * i + slot / 2
    const focus = it.tip ? ' tabindex="0"' : ''
    s += `<g class="bar-g" data-key="${esc(it.key)}" data-i="${i}"${focus} role="img" aria-label="${esc(ariaOf(it))}">`
    s += `<rect class="bar-hit" x="${r1(PAD.l + slot * i)}" y="${PAD.t}" width="${r1(slot)}" height="${r1(y0 - PAD.t)}"/>`
    if (it.noData) {
      s += `<rect class="bar nodata" fill="url(#energy-hatch)" x="${r1(cx - bw - BAR_GAP / 2)}" y="${PAD.t}" width="${r1(2 * bw + BAR_GAP)}" height="${r1(y0 - PAD.t)}" rx="4"/>`
    } else {
      s += bar('c', cx - bw - BAR_GAP / 2, bw, y(it.charge), y0) + bar('d', cx + BAR_GAP / 2, bw, y(it.discharge), y0)
    }
    s += '</g>'
  })
  return s + '</svg>'
}
