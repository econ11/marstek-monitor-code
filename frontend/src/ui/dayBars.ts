import { hm, hmLong } from '../lib/format'
import { t } from '../lib/i18n'
import { escapeHTML as esc } from './tooltip'

// «Без світла по днях» (hi-fi 02-outages, 09-states §3.2): one column per day
// (or week). A day wholly in a gap is hatched to full height with «немає
// даних», never drawn as zero; a day with no outages is a thin tick on the
// baseline; days before monitoring began are not drawn at all.

export type BarItem = {
  key: string
  label: string
  hours: number
  kind: 'value' | 'zero' | 'noData' | 'before'
  died: boolean
  tip: string /*html*/
  /** The hours are a lower bound: the value reads «≥ 1:20». */
  atLeast?: boolean
  /** Today's column: its axis label is emphasised. */
  today?: boolean
}

export type DayBarsOpts = {
  height?: number
  /** Key of the clicked column. */
  selected?: string
  /** The first observed column and its «спостереження з …» label. */
  observedSince?: { index: number; label: string }
  /** Accessible name of the chart. */
  label?: string
}

const HOUR = 3600e3
const PAD = { l: 40, r: 4, t: 22, b: 30 }
/** Grid steps, hours: the smallest that keeps the scale at ≤ 6 lines (2 h for days). */
const STEPS = [2, 4, 6, 12, 24, 48]
/** Up to this many columns every column gets its label and value. */
const DENSE = 10

/** The «немає даних» hatch (also the day timeline's). */
export const HATCH = '<defs><pattern id="nodata-hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">' +
  '<line x1="0" y1="0" x2="0" y2="6" stroke="rgb(138 148 161 / 0.22)" stroke-width="1.5"/></pattern></defs>'

const r1 = (v: number) => Math.round(v * 10) / 10

function scale(items: BarItem[]): { max: number; step: number } {
  const top = Math.max(0, ...items.filter(i => i.kind === 'value').map(i => i.hours))
  const step = STEPS.find(s => Math.ceil(top / s) <= 6) ?? STEPS[STEPS.length - 1]
  return { max: Math.max(step, Math.ceil(top / step) * step), step }
}

function ariaOf(i: BarItem): string {
  if (i.kind === 'noData') return `${i.label}: ${t('outages.chart.day.noData')}`
  if (i.kind === 'zero') return `${i.label}: ${t('outages.chart.day.zero')}`
  const d = hmLong(i.hours * HOUR)
  return t('outages.chart.bar.a11y', { day: i.label, duration: i.atLeast ? t('duration.atLeast.short', { duration: d }) : d })
}

/** dayBarsSVG draws the columns at the given width (px). Each drawn column is
 *  a <g class="bar-g" data-key data-i>; those with outages are buttons. */
export function dayBarsSVG(width: number, items: BarItem[], opts: DayBarsOpts = {}): string {
  const W = Math.max(width, PAD.l + PAD.r + items.length), H = opts.height ?? 250
  const { max, step } = scale(items)
  const y = (v: number) => PAD.t + (1 - v / max) * (H - PAD.t - PAD.b)
  const y0 = y(0)
  const slot = (W - PAD.l - PAD.r) / Math.max(1, items.length)
  const bw = Math.min(44, slot * 0.56)
  const dense = items.length <= DENSE
  const label = opts.label ? ` aria-label="${esc(opts.label)}"` : ''
  let s = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="group"${label}>${HATCH}<g class="grid">`
  for (let v = 0; v <= max; v += step) s += `<line x1="${PAD.l}" x2="${W - PAD.r}" y1="${r1(y(v))}" y2="${r1(y(v))}"${v ? '' : ' class="base"'}/>`
  s += '</g><g class="axis">'
  for (let v = 0; v <= max; v += step) {
    s += `<text x="${PAD.l - 8}" y="${r1(y(v) + 4)}" text-anchor="end">${v ? esc(t('outages.chart.axis.tick', { hours: v })) : '0'}</text>`
  }
  items.forEach((it, i) => {
    // Many columns: every fifth label, counted back from the last (today).
    if (dense || i % 5 === (items.length - 1) % 5) {
      s += `<text x="${r1(PAD.l + slot * i + slot / 2)}" y="${H - 10}" text-anchor="middle"${it.today ? ' class="strong"' : ''}>${esc(it.label)}</text>`
    }
  })
  s += '</g>'
  const obs = opts.observedSince
  if (obs && obs.index > 0) {
    const x = r1(PAD.l + slot * obs.index)
    s += `<g class="observed"><rect x="${PAD.l}" y="${PAD.t}" width="${r1(x - PAD.l)}" height="${r1(y0 - PAD.t)}"/>` +
      `<line x1="${x}" x2="${x}" y1="${PAD.t}" y2="${r1(y0)}"/><text class="gap-label" x="${r1(x - 8)}" y="${r1(y(max / 2) + 4)}" text-anchor="end">${esc(obs.label)}</text></g>`
  }
  items.forEach((it, i) => {
    if (it.kind === 'before') return
    const cx = PAD.l + slot * i + slot / 2, x = r1(cx - bw / 2)
    const button = it.kind === 'value' ? ` tabindex="0" role="button" aria-pressed="${opts.selected === it.key}"` : ''
    s += `<g class="bar-g${opts.selected === it.key ? ' is-selected' : ''}" data-key="${esc(it.key)}" data-i="${i}"${button} aria-label="${esc(ariaOf(it))}">`
    s += `<rect class="bar-hit" x="${r1(PAD.l + slot * i)}" y="${PAD.t}" width="${r1(slot)}" height="${r1(y0 - PAD.t)}"/>`
    if (it.kind === 'noData') {
      const mid = r1(PAD.t + (y0 - PAD.t) / 2)
      s += `<rect class="bar nodata" fill="url(#nodata-hatch)" x="${x}" y="${PAD.t}" width="${r1(bw)}" height="${r1(y0 - PAD.t)}" rx="4"/>`
      if (bw >= 14) s += `<text class="val" x="${r1(cx)}" y="${mid}" text-anchor="middle" dominant-baseline="middle" transform="rotate(-90 ${r1(cx)} ${mid})">${esc(t('outages.chart.day.noData'))}</text>`
    } else if (it.kind === 'zero') {
      s += `<rect class="bar zero" x="${x}" y="${r1(y0 - 2)}" width="${r1(bw)}" height="2" rx="1"/>`
      if (dense) s += `<text class="val" x="${r1(cx)}" y="${r1(y0 - 8)}" text-anchor="middle">${esc(t('outages.chart.day.zero'))}</text>`
    } else {
      const top = y(it.hours), h = y0 - top
      if (h > 0) {
        const rr = Math.min(4, h / 2, bw / 2)
        s += `<path class="bar" d="M${x},${r1(y0)}V${r1(top + rr)}q0,-${r1(rr)} ${r1(rr)},-${r1(rr)}h${r1(bw - 2 * rr)}q${r1(rr)},0 ${r1(rr)},${r1(rr)}V${r1(y0)}Z"/>`
      }
      if (dense) {
        const v = hm(it.hours * HOUR)
        s += `<text class="val" x="${r1(cx)}" y="${r1(top - 6)}" text-anchor="middle">${esc(it.atLeast ? t('duration.atLeast.short', { duration: v }) : v)}</text>`
      }
    }
    if (it.died) {
      const top = it.kind === 'value' ? y(it.hours) : PAD.t
      s += `<circle class="died" cx="${r1(dense ? cx + 22 : cx)}" cy="${r1(dense ? top - 10 : top - 6)}" r="3.5"/>`
    }
    s += '</g>'
  })
  return s + '</svg>'
}
