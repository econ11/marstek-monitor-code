import { clock } from '../lib/format'
import { t } from '../lib/i18n'
import type { TimelineModel } from '../views/outagesModel'
import { HATCH } from './dayBars'
import { escapeHTML as esc } from './tooltip'

// «Сьогодні по годинах» (hi-fi 02-outages, renderTimeline): the local day
// from midnight on one strip, light on in green, outages in amber with their
// times above, gaps hatched as «немає даних», the rest of the day dimmed
// after the «зараз» mark. Hours are placed by the clock, so a DST day of 23
// or 25 hours keeps its real length; a grid line is labelled with the time it
// falls on (spring forward: «03:00» does not exist, the line is at 04:00 and
// says «04:00»). Each span is a <g class="bar-g" data-key
// data-i>, focusable for its tooltip, as the day columns are.

const H = 196
const PAD = { l: 34, r: 4 }
const STRIP = { y: 80, h: 32 }
const CHAR_W = 6.1 // average glyph width of an 11 px label
const LABEL_GAP = 6
/** A span is never narrower than this to point at (a one-minute outage). */
const MIN_HIT = 8

const r1 = (v: number) => v.toFixed(1)

/** dayTimelineSVG draws the day at the given width (px). */
export function dayTimelineSVG(width: number, m: TimelineModel): string {
  const W = Math.max(width, PAD.l + PAD.r + 24)
  const x = (at: number) => PAD.l + ((at - m.start) / (m.end - m.start)) * (W - PAD.l - PAD.r)
  let s = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="group" aria-label="${esc(t('outages.timeline.a11y'))}">${HATCH}<g class="grid">`
  const d = new Date(m.start)
  const ticks: { at: number; label: string }[] = []
  for (let h = 0; h < 24; h += 3) {
    const at = new Date(d.getFullYear(), d.getMonth(), d.getDate(), h).getTime()
    ticks.push({ at, label: clock(at) })
  }
  ticks.push({ at: m.end, label: '24:00' })
  for (const k of ticks) s += `<line x1="${r1(x(k.at))}" x2="${r1(x(k.at))}" y1="60" y2="132"/>`
  s += '</g><g class="axis">'
  for (const k of ticks) s += `<text x="${r1(x(k.at))}" y="152" text-anchor="middle">${k.label}</text>`
  s += '</g>'

  // Light on from when monitoring began; the hours still to come are dimmed.
  const from = x(m.observedFrom), nowX = x(Math.min(m.now, m.end))
  s += `<rect class="tl-base" x="${r1(from)}" y="${STRIP.y}" width="${r1(W - PAD.r - from)}" height="${STRIP.h}" rx="6" fill="var(--color-good-soft)"/>`
  s += `<rect class="tl-future" x="${r1(nowX)}" y="${STRIP.y}" width="${r1(W - PAD.r - nowX)}" height="${STRIP.h}" fill="var(--color-bg-page)" opacity=".7"/>`

  // Labels never touch: one that would is left out.
  const boxes: { x0: number; x1: number }[] = []
  let labels = ''
  m.items.forEach((it, i) => {
    const a = x(it.start), b = x(it.end), w = Math.max(b - a, 1)
    const hitW = Math.max(w, MIN_HIT), hitX = a + w / 2 - hitW / 2
    s += `<g class="bar-g tl-span" data-key="${esc(it.key)}" data-i="${i}" tabindex="0" role="img" aria-label="${esc(it.aria)}">`
    s += `<rect class="bar-hit" x="${r1(hitX)}" y="${STRIP.y}" width="${r1(hitW)}" height="${STRIP.h}"/>`
    s += it.kind === 'gap'
      ? `<rect class="bar nodata" fill="url(#nodata-hatch)" x="${r1(a)}" y="${STRIP.y}" width="${r1(w)}" height="${STRIP.h}"/>`
      : `<rect class="bar" x="${r1(a)}" y="${STRIP.y}" width="${r1(w)}" height="${STRIP.h}" rx="3"/>`
    s += '</g>'
    if (!it.label) return
    const tw = it.label.length * CHAR_W
    const left = Math.min(Math.max((a + b) / 2 - tw / 2, PAD.l), W - PAD.r - tw)
    const box = { x0: left - LABEL_GAP, x1: left + tw + LABEL_GAP }
    if (boxes.some(o => box.x0 < o.x1 && box.x1 > o.x0)) return
    boxes.push(box)
    labels += `<text class="band-label" x="${r1(left)}" y="70">${esc(it.label)}</text>`
  })
  s += labels
  if (m.now < m.end) {
    s += `<line class="tl-now" x1="${r1(nowX)}" x2="${r1(nowX)}" y1="74" y2="118" stroke="var(--color-text-primary)" stroke-width="1.5"/>` +
      `<text class="band-label" x="${r1(nowX)}" y="132" text-anchor="middle" style="fill:var(--color-text-primary)">${esc(t('outages.timeline.now'))}</text>`
  }
  return s + '</svg>'
}
