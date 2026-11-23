import { num } from '../lib/format'
import { t } from '../lib/i18n'
import { POWER_PAD, tone, type PowerModel } from '../views/powerModel'
import { escapeHTML as esc, hideTip, showTip } from './tooltip'

// «Потужність батареї» (Energy tab, Task 21c): one area around the zero
// line, green above (charge) and amber below (discharge), outages shaded,
// holes left empty (the line is broken, never drawn at 0), the discharge peak
// marked. The axis is the model's (00:00–24:00 by the clock, or the days).
// The chart is one tab stop: ← → move between the points, PageUp/PageDown by
// an hour (a day for hourly averages), Home/End to the ends; the tooltip
// follows, and its text is announced through a live region.

const P = { l: 48, r: 10 }
const HOUR = 3600e3, DAY = 24 * HOUR
const CHAR_W = 6.1 // average glyph width of an 11 px label
const r1 = (v: number) => v.toFixed(1)
const COLOR = { charge: 'var(--color-good)', discharge: 'var(--color-warn)', idle: 'var(--color-text-subtle)' }

type Geometry = { W: number; x: (at: number) => number }

/** powerChartSVG draws the model at the given width (px). */
export function powerChartSVG(width: number, m: PowerModel): { svg: string; geom: Geometry } {
  const W = Math.max(width, P.l + P.r + 24), H = m.height
  const x = (at: number) => P.l + ((at - m.t0) / (m.t1 - m.t0)) * (W - P.l - P.r)
  const y = m.y, top = POWER_PAD.t, bottom = H - POWER_PAD.b, zero = y(0)
  let s = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" aria-hidden="true">`
  // The colours stop a little short of zero: a line lying on it (idle) stays
  // neutral rather than half green, half amber.
  const edge = 1.5
  const clip = (id: string, y0: number, y1: number) => `<clipPath id="${id}"><rect x="${P.l}" y="${r1(y0)}" width="${W - P.l - P.r}" height="${r1(Math.max(0, y1 - y0))}"/></clipPath>`
  s += `<defs>${clip('pw-above', 0, zero)}${clip('pw-below', zero, H)}${clip('pw-above-line', 0, zero - edge)}${clip('pw-below-line', zero + edge, H)}</defs>`

  // Grid: the ends, the halves and zero; the values signed (− discharge).
  s += '<g class="grid">'
  const levels = [m.yMax, m.yMax / 2, 0, m.yMin / 2, m.yMin]
  for (const v of levels) s += `<line x1="${P.l}" x2="${W - P.r}" y1="${r1(y(v))}" y2="${r1(y(v))}" class="${v === 0 ? 'base' : ''}"/>`
  for (const k of m.ticks) s += `<line x1="${r1(x(k.t))}" x2="${r1(x(k.t))}" y1="${top}" y2="${bottom}" class="pw-vline"/>`
  s += '</g><g class="axis">'
  for (const v of levels) s += `<text x="${P.l - 8}" y="${r1(y(v) + 4)}" text-anchor="end">${esc(num(v))}</text>`
  s += `<text x="${P.l - 8}" y="${top - 12}" text-anchor="end">${esc(t('unit.w'))}</text>`
  m.ticks.forEach((k, i) => {
    // The end labels stay inside the plot.
    const anchor = i === 0 && x(k.t) - P.l < 16 ? 'start' : x(k.t) > W - P.r - 16 ? 'end' : 'middle'
    s += `<text x="${r1(x(k.t))}" y="${H - 6}" text-anchor="${anchor}">${esc(k.label)}</text>`
  })
  s += '</g>'

  // Outages (no grid), then what is still to come today, dimmed.
  for (const b of m.bands) {
    const a = Math.max(b.start, m.t0), e = Math.min(b.end, m.t1)
    if (e > a) s += `<rect class="band" x="${r1(x(a))}" y="${top}" width="${r1(Math.max(1, x(e) - x(a)))}" height="${bottom - top}"/>`
  }
  if (m.now < m.t1) {
    const nx = x(Math.max(m.now, m.t0))
    s += `<rect class="pw-future" x="${r1(nx)}" y="${top}" width="${r1(W - P.r - nx)}" height="${bottom - top}"/>`
  }

  // The area: every run closes on the zero line, so a hole stays empty.
  let area = '', line = '', dots = ''
  for (const run of m.runs) {
    if (run.length === 1) {
      const p = run[0]
      dots += `<circle class="pw-dot" cx="${r1(x(p.t))}" cy="${r1(y(p.w))}" r="2" fill="${COLOR[tone(p.w)]}"/>`
      continue
    }
    const d = run.map((p, i) => `${i ? 'L' : 'M'}${r1(x(p.t))},${r1(y(p.w))}`).join('')
    line += d
    area += d + `L${r1(x(run[run.length - 1].t))},${r1(zero)}L${r1(x(run[0].t))},${r1(zero)}Z`
  }
  if (area) {
    s += `<path class="pw-area pw-area--charge" d="${area}" clip-path="url(#pw-above)" fill="var(--color-good)"/>`
    s += `<path class="pw-area pw-area--discharge" d="${area}" clip-path="url(#pw-below)" fill="var(--color-warn)"/>`
    s += `<path class="pw-line" d="${line}" stroke="var(--color-text-subtle)"/>`
    s += `<path class="pw-line" d="${line}" clip-path="url(#pw-above-line)" stroke="var(--color-good)"/>`
    s += `<path class="pw-line" d="${line}" clip-path="url(#pw-below-line)" stroke="var(--color-warn)"/>`
  }
  s += dots

  // The peak: a ring on the discharge side, its label beside it inside the
  // plot; an instant peak beyond the hourly scale sits on the bottom edge,
  // a small arrow saying it goes further.
  if (m.peak && m.peak.t >= m.t0 && m.peak.t <= m.t1) {
    const px = x(m.peak.t), py = m.peak.pinned ? bottom : y(-m.peak.w)
    if (m.peak.pinned) s += `<path class="pw-peak-arrow" d="M${r1(px - 4)},${r1(bottom + 5)}L${r1(px + 4)},${r1(bottom + 5)}L${r1(px)},${r1(bottom + 10)}Z" fill="var(--color-warn)"/>`
    s += `<circle class="pw-peak" cx="${r1(px)}" cy="${r1(py)}" r="3.5" fill="var(--color-surface-1)" stroke="var(--color-warn)" stroke-width="2"/>`
    const tw = m.peak.label.length * CHAR_W
    const left = px + 8 + tw <= W - P.r ? px + 8 : px - 8 - tw
    s += `<text class="pw-peak-label" x="${r1(Math.max(P.l, left))}" y="${r1(Math.min(py + 4, bottom - 4))}">${esc(m.peak.label)}</text>`
  }
  s += `<g class="hover" style="display:none"><line class="crosshair" y1="${top}" y2="${bottom}"/><circle r="4" stroke="var(--color-surface-1)" stroke-width="2"/></g>`
  s += '</svg>'
  return { svg: s, geom: { W, x } }
}

/** nearest is the index of the point closest in time to at (-1 without points). */
export function nearest(m: PowerModel, at: number): number {
  let best = -1
  m.points.forEach((p, i) => { if (best < 0 || Math.abs(p.t - at) < Math.abs(m.points[best].t - at)) best = i })
  return best
}

/** keyStep is where a key moves the selection from index i, or null for another key. */
export function keyStep(m: PowerModel, i: number, key: string): number | null {
  const last = m.points.length - 1
  if (last < 0) return null
  // Pages are time, not points: an hour of minutes, a day of hourly averages.
  const page = (dir: number) => {
    const j = nearest(m, m.points[i].t + dir * (m.hourly ? DAY : HOUR))
    // Across a long hole the nearest point may be this one: still move.
    return j === i ? Math.min(last, Math.max(0, i + dir)) : j
  }
  switch (key) {
    case 'ArrowLeft': return Math.max(0, i - 1)
    case 'ArrowRight': return Math.min(last, i + 1)
    case 'PageUp': return page(-1)
    case 'PageDown': return page(1)
    case 'Home': return 0
    case 'End': return last
    default: return null
  }
}

export type PowerChart = {
  draw(m: PowerModel | null): void
  /** The time of the point the keyboard is on, if any. */
  selected(): number | null
  /** Put the keyboard selection on the point nearest at (after a rebuild). */
  select(at: number | null): void
}

/** powerChart wires a host (tabindex=0) and a live region to the chart. */
export function powerChart(host: HTMLElement, live: HTMLElement): PowerChart {
  let m: PowerModel | null = null
  let geom: Geometry | null = null
  /** The keyboard selection, by time: a redraw keeps it on the same minute. */
  let sel: number | null = null
  /** The pointer over the chart (clientX): a redraw reopens its tooltip. */
  let pointerX: number | null = null
  const svg = () => host.querySelector<SVGSVGElement>('svg')

  function show(i: number, announce: boolean) {
    const el = svg()
    if (!m || !geom || !el || i < 0) return
    const p = m.points[i]
    const hover = el.querySelector<SVGGElement>('.hover')!
    const cross = hover.firstElementChild as SVGLineElement, dot = hover.lastElementChild as SVGCircleElement
    const px = geom.x(p.t), py = m.y(p.w)
    hover.style.display = ''
    cross.setAttribute('x1', String(px))
    cross.setAttribute('x2', String(px))
    dot.setAttribute('cx', String(px))
    dot.setAttribute('cy', String(py))
    dot.setAttribute('fill', COLOR[p.tone])
    const r = el.getBoundingClientRect()
    showTip(host, p.tip, { rect: { left: r.left + px, width: 0, top: r.top + py - 6, bottom: r.top + py + 6 } })
    if (announce) live.textContent = p.aria
  }
  function clear() {
    const hover = svg()?.querySelector<SVGGElement>('.hover')
    if (hover) hover.style.display = 'none'
    hideTip()
  }
  const focused = () => document.activeElement === host
  const atX = (clientX: number) => {
    const r = svg()!.getBoundingClientRect()
    return m!.t0 + ((clientX - r.left - P.l) / (geom!.W - P.l - P.r)) * (m!.t1 - m!.t0)
  }

  host.addEventListener('pointermove', e => {
    if (!m || !geom || !svg()) return
    pointerX = e.clientX
    show(nearest(m, atX(e.clientX)), false)
  })
  host.addEventListener('pointerleave', () => {
    pointerX = null
    if (focused() && sel != null && m) show(nearest(m, sel), false)
    else clear()
  })
  host.addEventListener('focus', () => {
    if (!m || !m.points.length) return
    // First visit: the peak of discharge if there is one, else the newest point.
    if (sel == null) sel = m.peak ? m.points[nearest(m, m.peak.t)].t : m.points[m.points.length - 1].t
    show(nearest(m, sel), true)
  })
  host.addEventListener('blur', () => { if (pointerX == null) clear() })
  host.addEventListener('keydown', e => {
    // Esc closes the tooltip (ui/tooltip); the crosshair goes with it.
    if (e.key === 'Escape') { clear(); return }
    if (!m || !m.points.length) return
    const i = sel == null ? m.points.length - 1 : nearest(m, sel)
    const j = keyStep(m, i, e.key)
    if (j == null) return
    e.preventDefault()
    sel = m.points[j].t
    show(j, true)
  })

  return {
    draw(next) {
      m = next
      hideTip()
      if (!next || !next.points.length) {
        host.innerHTML = ''
        geom = null
        return
      }
      const out = powerChartSVG(host.clientWidth || 600, next)
      host.innerHTML = out.svg
      geom = out.geom
      host.setAttribute('aria-label', next.a11y)
      // The redraw replaces the marks: the tooltip comes back where it was.
      if (pointerX != null) show(nearest(next, atX(pointerX)), false)
      else if (focused() && sel != null) show(nearest(next, sel), false)
    },
    selected: () => sel,
    select(at) {
      sel = at
      if (at != null && m && focused()) show(nearest(m, at), true)
    },
  }
}
