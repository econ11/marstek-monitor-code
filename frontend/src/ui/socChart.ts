import { clock, dateShort, dayShort, daysBefore, hmLong, midnight } from '../lib/format'
import { t } from '../lib/i18n'
import { escapeHTML as esc, hideTip, showTip } from './tooltip'

// Charge chart (Overview). One renderer for every connection state; port of
// the hi-fi reference (_src/soc-chart.js). Times are epoch milliseconds.

export type ChartPoint = { t: number; soc: number | null }
/** label is a copy key: overview.chart.band.outage[.ongoing] or gap.label.* */
export type ChartBand = { start: number; end: number; kind: 'outage' | 'gap'; label: string }
export type ChartRange = '24h' | '7d' | '30d'
export type ChartOpts = {
  range: ChartRange
  now: number
  /** Last fresh measurement; defaults to now. */
  lastData?: number
  /** What is drawn between lastData and now. */
  tail?: null | 'stale' | 'nolink' | 'dead'
  skeleton?: boolean
  /** The axis left of it is empty, with no frame: not a gap. */
  observedSince?: number
  /** Station minimum charge (the effective MinSOC), %; 0 = unknown, no line. */
  floor?: number
  height?: number
}

const HOUR = 3600e3, DAY = 24 * HOUR
const SPAN: Record<ChartRange, number> = { '24h': DAY, '7d': 7 * DAY, '30d': 30 * DAY }
// The line never joins two measurements further apart than this.
const MAX_JOIN: Record<ChartRange, number> = { '24h': 10 * 60e3, '7d': 10 * 60e3, '30d': 3 * HOUR }
const P = { l: 40, r: 8, t: 26, b: 26 }
const CHAR_W = 6.1 // average glyph width of an 11 px label
const LABEL_GAP = 5 // free space kept around every label, px each side
const ID = 'soc'

type Gap = ChartBand & { open?: boolean }
type Geometry = {
  W: number; H: number; t0: number; t1: number; last: number
  pts: ChartPoint[]; outages: ChartBand[]; gaps: Gap[]
  tail: ChartOpts['tail']; lineColor: string
  x: (t: number) => number; y: (v: number) => number
}

const capital = (s: string) => s.charAt(0).toLocaleUpperCase('uk') + s.slice(1)

function ticks(range: ChartRange, t0: number, t1: number): { t: number; label: string; strong: boolean }[] {
  const out: { t: number; label: string; strong: boolean }[] = []
  if (range === '24h') {
    for (let t = midnight(t0); t <= t1; t += 3 * HOUR) {
      if (t < t0) continue
      const day = new Date(t).getHours() === 0
      out.push({ t, label: day ? dayShort(t) : clock(t), strong: day })
    }
    return out
  }
  const every = range === '7d' ? 1 : 4
  const today = new Date(midnight(t1))
  for (let back = 0; ; back += every) {
    const t = new Date(today.getFullYear(), today.getMonth(), today.getDate() - back).getTime()
    if (t < t0) break
    out.unshift({ t, label: dayShort(t), strong: false })
  }
  return out
}

/** socChartSVG builds the chart markup for a plot W px wide. */
export function socChartSVG(W: number, points: ChartPoint[], bands: ChartBand[], opts: ChartOpts): { svg: string; geom: Geometry } {
  const { range, now } = opts
  const H = opts.height || 236
  const last = opts.lastData ?? now, tail = opts.tail || null
  const t1 = now, t0 = t1 - SPAN[range]
  const since = opts.observedSince ?? -Infinity
  const floor = opts.floor ?? 10
  const x = (t: number) => P.l + ((t - t0) / (t1 - t0)) * (W - P.l - P.r)
  const y = (v: number) => P.t + (1 - v / 100) * (H - P.t - P.b)
  const lineColor = tail ? 'var(--color-text-subtle)' : 'var(--color-good)'
  const geom: Geometry = { W, H, t0, t1, last, pts: [], outages: [], gaps: [], tail, lineColor, x, y }

  // Text never overlaps text: every label claims its box, and a label whose
  // box touches a claimed one is not drawn. Lower-priority labels wait in
  // `later` until the important ones are placed.
  const boxes: { x0: number; x1: number; y0: number; y1: number }[] = []
  const later: (() => unknown)[] = []
  let labels = ''
  const put = (cls: string, text: string, left: number, baseline: number): boolean => {
    const box = { x0: left - LABEL_GAP, x1: left + text.length * CHAR_W + LABEL_GAP, y0: baseline - 11, y1: baseline + 3 }
    if (boxes.some(b => box.x0 < b.x1 && box.x1 > b.x0 && box.y0 < b.y1 && box.y1 > b.y0)) return false
    boxes.push(box)
    labels += `<text class="${cls}" x="${left.toFixed(1)}" y="${baseline.toFixed(1)}">${esc(text)}</text>`
    return true
  }

  const rangeName = t(`common.chart.range.${range}`)
  const outageCount = bands.filter(b => b.kind === 'outage').length
  let s = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img" aria-label="${esc(t('overview.chart.a11y', { range: rangeName, count: outageCount }))}">`
  s += `<defs><linearGradient id="socfill-${ID}" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stop-color="var(--color-good)" stop-opacity=".13"/><stop offset="1" stop-color="var(--color-good)" stop-opacity="0"/></linearGradient><clipPath id="plot-${ID}"><rect x="${P.l}" y="0" width="${W - P.l - P.r}" height="${H}"/></clipPath></defs>`
  s += '<g class="grid">'
  for (const v of [0, 25, 50, 75, 100]) s += `<line x1="${P.l}" x2="${W - P.r}" y1="${y(v)}" y2="${y(v)}" class="${v === 0 ? 'base' : ''}"/>`
  s += '</g><g class="axis">'
  for (const v of [0, 25, 50, 75, 100]) s += `<text x="${P.l - 8}" y="${y(v) + 4}" text-anchor="end">${v}</text>`
  for (const k of ticks(range, t0, t1)) {
    s += `<text x="${x(k.t).toFixed(1)}" y="${H - 6}" text-anchor="middle" class="${k.strong ? 'strong' : ''}">${k.label}</text>`
  }
  s += '</g>'
  if (opts.skeleton) {
    s += `<rect x="${P.l}" y="${P.t}" width="${W - P.l - P.r}" height="${H - P.t - P.b}" rx="6" class="skel-fill"/></svg>`
    return { svg: s, geom }
  }

  if (since > t0) {
    const date = daysBefore(since, now) === 0 ? clock(since) : dateShort(since)
    // The note sits in the empty part of the axis unless that is too narrow for it.
    const label = t('chart.observedSince', { date }), tw = label.length * CHAR_W
    const left = x(since) - P.l > tw + 16 ? x(since) - 8 - tw : x(since) + 8
    later.push(() => put('gap-label', label, left, y(50) + 4))
    s += `<line x1="${x(since)}" x2="${x(since)}" y1="${P.t}" y2="${H - P.b}" stroke="var(--color-border-strong)"/>`
  }
  s += `<g clip-path="url(#plot-${ID})">`

  // Outage bands (no grid). Labels never collide: full text → duration only → none.
  const lo = Math.max(t0, since)
  for (const o of bands) {
    if (o.kind !== 'outage' || o.start >= last) continue
    // Without fresh data a band stops at the last measurement.
    const a = Math.max(o.start, lo), b = Math.min(o.end, tail ? last : t1)
    if (b <= a) continue
    geom.outages.push({ ...o, start: a, end: b })
    const bx = x(a), bw = x(b) - bx
    s += `<rect class="band" x="${bx.toFixed(1)}" y="${P.t}" width="${bw.toFixed(1)}" height="${H - P.t - P.b}"/>`
    const duration = hmLong(b - a)
    // Without fresh data an outage that was still on is only a lower bound.
    const frozen = tail && o.label.endsWith('.ongoing')
    const texts = frozen
      ? [t('overview.chart.band.atLeast', { duration }), `≥ ${duration}`]
      : [t(o.label, { duration }), duration]
    if (bw < 30 && range !== '24h') continue
    later.push(() => texts.some(txt => {
      const tw = txt.length * CHAR_W
      return put('band-label', txt, Math.min(Math.max(bx + bw / 2 - tw / 2, P.l), W - P.r - tw), P.t - 9)
    }))
  }

  // Gaps: no data, with the reason when it is known and fits.
  const gaps: Gap[] = []
  for (const g of bands) {
    if (g.kind !== 'gap') continue
    const a = Math.max(g.start, lo), b = Math.min(g.end, t1)
    if (b > a) gaps.push({ ...g, start: a, end: b, open: b >= now - 5e3 })
  }
  if ((tail === 'nolink' || tail === 'dead') && !gaps.some(g => g.open) && last < now) {
    gaps.push({ start: Math.max(last, lo), end: now, kind: 'gap', label: tail === 'dead' ? 'gap.label.likelyOut' : 'gap.label', open: true })
  }
  geom.gaps = gaps
  for (const g of gaps) {
    const gx = x(g.start), gw = Math.max(2, x(g.end) - gx)
    s += `<rect class="gap${g.open ? ' gap--open' : ''}" x="${gx.toFixed(1)}" y="${P.t + 1}" width="${gw.toFixed(1)}" height="${H - P.t - P.b - 2}" rx="3"/>`
    const label = t(g.label), tw = label.length * CHAR_W
    if (gw > tw + 8) later.push(() => put('gap-label', label, gx + gw / 2 - tw / 2, y(50) + 4))
  }

  // The line: broken at every gap and never interpolated, then thinned to
  // about one point per pixel. A point without a value marks a break.
  const inGap = (a: number, b: number) => gaps.some(g => g.start < b && g.end > a)
  const pts: ChartPoint[] = []
  let prevX = -Infinity
  let before: ChartPoint | null = null // the measurement just before p
  let skipped: ChartPoint | null = null // the newest measurement thinned out
  for (const p of points) {
    if (p.t < lo || p.t > last + 1) continue
    const px = x(p.t)
    const breaks = p.soc == null || before == null || before.soc == null || p.t - before.t > MAX_JOIN[range] || inGap(before.t, p.t)
    before = p
    if (breaks) {
      // Close the run with its real last measurement, then start a new one.
      if (skipped) pts.push(skipped)
      if (pts.length && pts[pts.length - 1].soc != null) pts.push({ t: p.t, soc: null })
      if (p.soc != null) { pts.push(p); prevX = px }
      skipped = null
      continue
    }
    if (px - prevX < 0.75) { skipped = p; continue }
    pts.push(p)
    prevX = px
    skipped = null
  }
  // The newest measurement is always drawn: it is the value on the rings.
  if (skipped) pts.push(skipped)
  geom.pts = pts
  let line = '', area = '', dots = '', seg: ChartPoint[] = []
  const flush = () => {
    // A measurement with no neighbour to join is a dot, not nothing.
    if (seg.length === 1) dots += `<circle class="soc-dot" cx="${x(seg[0].t).toFixed(1)}" cy="${y(seg[0].soc!).toFixed(1)}" r="2" fill="${lineColor}"/>`
    if (seg.length > 1) {
      const d = seg.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.soc!).toFixed(1)}`).join('')
      line += d
      area += d + `L${x(seg[seg.length - 1].t).toFixed(1)},${y(0)}L${x(seg[0].t).toFixed(1)},${y(0)}Z`
    }
    seg = []
  }
  for (const p of pts) {
    if (p.soc == null) flush()
    else seg.push(p)
  }
  flush()

  if (!tail && area) s += `<path d="${area}" fill="url(#socfill-${ID})"/>`
  if (floor > 0) s += `<line class="floor" x1="${P.l}" x2="${W - P.r}" y1="${y(floor)}" y2="${y(floor)}"/>`
  if (line) s += `<path d="${line}" fill="none" stroke="${lineColor}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`
  s += dots
  const lp = [...pts].reverse().find(p => p.soc != null)
  if (lp) {
    s += `<circle cx="${x(lp.t).toFixed(1)}" cy="${y(lp.soc!).toFixed(1)}" r="3.5" fill="${tail ? 'var(--color-surface-1)' : lineColor}" stroke="${tail ? lineColor : 'var(--color-surface-1)'}" stroke-width="2"/>`
  }
  s += '</g>'
  // Labels, most important first; one that would touch an earlier one is left out.
  if (floor > 0) {
    const floorText = `${floor}%`
    put('floor-label', floorText, W - P.r - floorText.length * CHAR_W, y(floor) - 5)
  }
  if (lp && tail) {
    // The last known value, left of its point (right of it when there is no room).
    const text = `${Math.round(lp.soc!)}% ${t('date.at', { time: clock(lp.t) })}`, tw = text.length * CHAR_W
    const left = x(lp.t) - 6 - tw >= P.l ? x(lp.t) - 6 - tw : x(lp.t) + 6
    put('gap-label', text, left, Math.max(y(lp.soc!) - 10, 11))
  }
  for (const draw of later) draw()
  s += labels
  s += `<g class="hover" style="display:none"><line class="crosshair" y1="${P.t}" y2="${H - P.b}"/><circle r="4" fill="${lineColor}" stroke="var(--color-surface-1)" stroke-width="2"/></g>`
  s += `<rect class="hit" x="${P.l}" y="${P.t}" width="${W - P.l - P.r}" height="${H - P.t - P.b}" fill="transparent"/></svg>`
  return { svg: s, geom }
}

const when = (at: number) => `${dayShort(at)} · ${clock(at)}`

/** tooltipAt is the tooltip for the time under the pointer: its HTML and the point to mark, if any. */
export function tooltipAt(g: Geometry, at: number): { html: string; point: ChartPoint | null; t: number } {
  const title = (text: string, meta: string) => `<div class="tip__title">${esc(text)}<small>${esc(meta)}</small></div>`
  const gap = g.gaps.find(k => at >= k.start && at <= k.end)
  if (gap) {
    const key = gap.label === 'gap.label.appOff' ? 'gap.tooltip.appOff.v1' : 'gap.tooltip'
    return { html: title(capital(t(gap.label)), when(at)) + `<div>${esc(t(key, { start: clock(gap.start), end: clock(gap.end) }))}</div>`, point: null, t: at }
  }
  if (at > g.last && g.tail) {
    // The banner's words, the time once: yesterday's measurement is named by
    // the day word beside it, an older one by its date in the title.
    const ago = daysBefore(g.last, g.t1), time = clock(g.last)
    const text = ago <= 1 ? t('stale.banner.today', { time }) : t('stale.banner.otherDay', { date: dateShort(g.last), time })
    return { html: title(text, ago === 1 ? t('date.yesterday', { time: '' }).trim() : ''), point: null, t: at }
  }
  let best: ChartPoint | null = null
  for (const p of g.pts) {
    if (p.soc != null && (!best || Math.abs(p.t - at) < Math.abs(best.t - at))) best = p
  }
  if (!best) return { html: '', point: null, t: at }
  const o = g.outages.find(k => best!.t >= k.start && best!.t <= k.end)
  const state = o
    ? t('overview.chart.tooltip.band', { start: clock(o.start), end: clock(o.end), duration: hmLong(o.end - o.start) })
    : capital(t('state.grid.on'))
  return { html: title(`${Math.round(best.soc!)}%`, when(best.t)) + `<div class="${o ? 't-warn' : 't-good'}">${esc(state)}</div>`, point: best, t: best.t }
}

/** Where the pointer is over each chart host (clientX), while it is there:
 *  a redraw opens the tooltip again at the same place. */
const pointer = new WeakMap<HTMLElement, number>()

/** clearChartPointer forgets the pointer over host (the window was hidden):
 *  the next redraw opens no tooltip. */
export function clearChartPointer(host: HTMLElement) {
  pointer.delete(host)
}

/** renderSocChart draws the chart into host and wires the crosshair tooltip.
 *  A redraw while the pointer is over the chart keeps its tooltip open. */
export function renderSocChart(host: HTMLElement, points: ChartPoint[], bands: ChartBand[], opts: ChartOpts) {
  const W = host.clientWidth
  if (W <= P.l + P.r) return
  const { svg: markup, geom: g } = socChartSVG(W, points, bands, opts)
  hideTip()
  host.innerHTML = markup
  const svg = host.firstElementChild as SVGSVGElement
  const hover = svg.querySelector<SVGGElement>('.hover'), hit = svg.querySelector<SVGRectElement>('.hit')
  if (!hover || !hit) return
  const cross = hover.firstElementChild as SVGLineElement, dot = hover.lastElementChild as SVGCircleElement
  const move = (clientX: number) => {
    const r = svg.getBoundingClientRect()
    const at = g.t0 + ((clientX - r.left - P.l) / (g.W - P.l - P.r)) * (g.t1 - g.t0)
    const tip = tooltipAt(g, at)
    if (!tip.html) return
    const px = g.x(Math.min(tip.t, g.t1)), ay = tip.point ? g.y(tip.point.soc!) : g.y(50)
    hover.style.display = ''
    dot.style.display = tip.point ? '' : 'none'
    dot.setAttribute('cx', String(px))
    dot.setAttribute('cy', String(ay))
    cross.setAttribute('x1', String(px))
    cross.setAttribute('x2', String(px))
    showTip(svg, tip.html, { rect: { left: r.left + px, width: 0, top: r.top + ay - 6, bottom: r.top + ay + 6 } })
  }
  hit.addEventListener('pointermove', e => { pointer.set(host, e.clientX); move(e.clientX) })
  hit.addEventListener('pointerleave', () => { pointer.delete(host); hover.style.display = 'none'; hideTip() })
  const x = pointer.get(host)
  if (x != null) move(x)
}
