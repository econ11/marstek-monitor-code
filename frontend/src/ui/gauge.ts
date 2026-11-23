import type { RingProps } from '../views/overviewModel'
import { infoTipHTML, keepInfoTip } from './infoTip'
import { escapeHTML as esc } from './tooltip'

// The ONE gauge. The markup repeats gauge() from the hi-fi build
// (_src/build.mjs): 120 px box, r = 50, starts at 12 o'clock. The stroke is
// --gauge-stroke (8.5 in the app, 10 in the hi-fi).

const TONED_UNIT: Record<string, true> = { good: true, warn: true }

// "72%" is drawn as the number with a small unit: 72<small>%</small>.
function splitValue(value: string): { num: string; small: string } {
  return value.endsWith('%') ? { num: value.slice(0, -1), small: '%' } : { num: value, small: '' }
}

function valueHTML(p: RingProps): string {
  const { num, small } = splitValue(p.value)
  // Long values step down one size to stay inside the inner circle.
  const long = (small && num.length >= 4) || num.length >= 6
  // Only the live power ring colours its unit line («Вт · розряд»).
  const utone = p.key === 'power' && !p.stale && TONED_UNIT[p.tone] ? ` t-${p.tone}` : ''
  return `<span class="gauge__num${long ? ' gauge__num--long' : ''}">${esc(num)}${small ? `<small>${small}</small>` : ''}</span>` +
    (p.unit ? `<span class="gauge__unit${utone}">${esc(p.unit)}</span>` : '')
}

const metaHTML = (p: RingProps) => (p.sub ? `<span class="gauge__primary">${esc(p.sub)}</span>` : '')
const className = (p: RingProps) => ['gauge', p.fill <= 0 ? 'gauge--empty' : '', p.stale ? 'gauge--stale' : ''].filter(Boolean).join(' ')
const dash = (p: RingProps) => `${Math.round(Math.min(Math.max(p.fill, 0), 1) * 1000) / 10} 100`
const aria = (p: RingProps) => p.aria || `${p.title}: ${p.value}${p.unit ? ' ' + p.unit : ''}. ${p.sub}`
/** The title and its ⓘ (copy §15), named for the ring, so a redraw keeps it open. */
function titleHTML(p: RingProps): string {
  if (!p.info) return `<span class="gauge__label">${esc(p.title)}</span>`
  // The ⓘ goes onto a new line only with the last word («сьогодні ⓘ»).
  const cut = p.title.lastIndexOf(' ') + 1
  return `<span class="gauge__label">${esc(p.title.slice(0, cut))}<span class="nowrap">${esc(p.title.slice(cut))}` +
    `${infoTipHTML(p.title, p.info, `ring:${p.key}`)}</span></span>`
}

export function gaugeHTML(p: RingProps): string {
  return `<figure class="${className(p)}" data-tone="${p.tone}" data-key="${p.key}">
  <figcaption class="gauge__title">${titleHTML(p)}</figcaption>
  <div class="gauge__dial" role="img" aria-label="${esc(aria(p))}">
    <svg viewBox="0 0 120 120" aria-hidden="true"><circle class="gauge__track" cx="60" cy="60" r="50"/><circle class="gauge__fill" cx="60" cy="60" r="50" pathLength="100" stroke-dasharray="${dash(p)}" transform="rotate(-90 60 60)"/></svg>
    <div class="gauge__value">${valueHTML(p)}</div>
  </div>
  <div class="gauge__meta">${metaHTML(p)}</div>
</figure>`
}

// The markup last written to each part: the browser's own serialisation
// differs from ours (<use/>), and an unchanged ⓘ must not be replaced.
const written = new WeakMap<Element, string>()
function setHTML(el: Element, html: string) {
  if (written.get(el) === html) return
  written.set(el, html)
  el.innerHTML = html
}

// Updates a gauge in place, so the fill animates from its previous value.
function patch(el: HTMLElement, p: RingProps) {
  el.className = className(p)
  el.dataset.tone = p.tone
  setHTML(el.querySelector('.gauge__title')!, titleHTML(p))
  el.querySelector('.gauge__dial')!.setAttribute('aria-label', aria(p))
  el.querySelector('.gauge__fill')!.setAttribute('stroke-dasharray', dash(p))
  setHTML(el.querySelector('.gauge__value')!, valueHTML(p))
  setHTML(el.querySelector('.gauge__meta')!, metaHTML(p))
}

/** renderGauges shows the rings in host, reusing the elements whose ring is the same. */
export function renderGauges(host: HTMLElement, rings: RingProps[]) {
  const old = Array.from(host.children) as HTMLElement[]
  const same = old.length === rings.length && rings.every((r, i) => old[i].dataset.key === r.key)
  keepInfoTip(() => {
    if (same) rings.forEach((r, i) => patch(old[i], r))
    else host.innerHTML = rings.map(gaugeHTML).join('')
  })
}

// Skeletons keep the gauge geometry: title line, 120 px ring, caption line.
const SKELETON_WIDTHS = [[56, 110], [80, 96], [76, 120], [104, 100]]

export const skeletonGaugesHTML = () =>
  SKELETON_WIDTHS.map(([title, sub]) =>
    `<div class="skel-gauge"><span class="skel skel-line" style="width:${title}px"></span><span class="skel-ring"></span><span class="skel skel-line" style="width:${sub}px"></span></div>`).join('')
