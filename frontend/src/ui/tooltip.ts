// Tooltip engine: one portalled .tip, edge-aware (shift + flip), never leaves
// the window. Port of the hi-fi reference (_src/app.js → place()); every
// tooltip in the app, charts included, goes through show().

const EDGE = 8 // px kept free between tooltip and the window edge
const GAP = 10 // px between anchor and tooltip
const OPEN_DELAY = 120
const WARM_MS = 300

export type AnchorRect = { left: number; top: number; bottom: number; width: number }

let tip: HTMLDivElement | null = null
let openTimer = 0
let lastClose = 0
let current: Element | null = null

const body = () => tip!.firstElementChild as HTMLElement
const arrow = () => tip!.lastElementChild as HTMLElement

function bounds() {
  const vw = document.documentElement.clientWidth, vh = document.documentElement.clientHeight
  return { left: EDGE, top: EDGE, right: vw - EDGE, bottom: vh - EDGE }
}

/** rect: anchor rect in viewport coordinates. Point anchors (crosshair) pass a 0-width rect. */
function place(rect: AnchorRect, prefer?: string) {
  const el = tip!
  const b = bounds()
  el.style.maxWidth = Math.min(280, b.right - b.left) + 'px'
  const w = el.offsetWidth, h = el.offsetHeight
  const cx = rect.left + rect.width / 2
  let side = prefer || 'top'
  let y = side === 'top' ? rect.top - GAP - h : rect.bottom + GAP
  if (side === 'top' && y < b.top) { side = 'bottom'; y = rect.bottom + GAP }
  if (side === 'bottom' && y + h > b.bottom) {
    if (rect.top - GAP - h >= b.top) { side = 'top'; y = rect.top - GAP - h }
    else y = Math.max(b.top, b.bottom - h) // no room either way: pin inside, cover the anchor
  }
  let x = Math.round(cx - w / 2)
  x = Math.min(Math.max(x, b.left), b.right - w)
  el.dataset.side = side
  el.style.left = x + 'px'
  el.style.top = Math.round(y) + 'px'
  const ax = Math.min(Math.max(cx - x - 5, 10), w - 20)
  arrow().style.left = ax + 'px'
}

/** show opens the tooltip with trusted HTML at the anchor (or at opts.rect). */
export function showTip(anchor: Element, html: string, opts: { rect?: AnchorRect; side?: string } = {}) {
  if (!tip) return
  clearTimeout(openTimer)
  body().innerHTML = html
  if (current && current !== anchor) current.removeAttribute('aria-describedby')
  current = anchor
  place(opts.rect || anchor.getBoundingClientRect(), opts.side)
  tip.classList.add('is-open')
  anchor.setAttribute('aria-describedby', 'mm-tip')
}

export function hideTip() {
  clearTimeout(openTimer)
  if (!tip || !tip.classList.contains('is-open')) return
  tip.classList.remove('is-open')
  if (current) current.removeAttribute('aria-describedby')
  current = null
  lastClose = Date.now()
}

let escaped: ((anchor: Element) => void) | null = null
/** onTipEscape: fn learns which anchor's tooltip Esc closed (the ⓘ takes the focus back). */
export function onTipEscape(fn: (anchor: Element) => void) { escaped = fn }

/** tipAnchor is the element the open tooltip belongs to, or null. */
export const tipAnchor = (): Element | null => current

export function escapeHTML(s: string): string {
  return s.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)
}

// data-tip holds plain text, or "#id" of a <template> with tooltip markup.
function contentFor(el: Element): string {
  const ref = el.getAttribute('data-tip') || ''
  if (ref.startsWith('#')) {
    const tpl = document.querySelector(ref)
    return tpl ? tpl.innerHTML : ''
  }
  return escapeHTML(ref)
}

function scheduleShow(el: HTMLElement) {
  clearTimeout(openTimer)
  const warm = Date.now() - lastClose < WARM_MS || tip!.classList.contains('is-open')
  openTimer = window.setTimeout(() => {
    if (el.isConnected) showTip(el, contentFor(el), { side: el.dataset.tipSide })
  }, warm ? 0 : OPEN_DELAY)
}

const tipTarget = (t: EventTarget | null) => (t instanceof Element ? t.closest<HTMLElement>('[data-tip]') : null)

/** initTooltips creates the tooltip element and wires [data-tip] anchors. Call once. */
export function initTooltips() {
  if (tip) return
  tip = document.createElement('div')
  tip.className = 'tip'
  tip.setAttribute('role', 'tooltip')
  tip.id = 'mm-tip'
  tip.innerHTML = '<div class="tip__body"></div><span class="tip__arrow"></span>'
  document.body.appendChild(tip)

  document.addEventListener('pointerover', e => {
    const el = tipTarget(e.target)
    if (el && el !== current) scheduleShow(el)
  })
  document.addEventListener('pointerout', e => {
    const el = tipTarget(e.target)
    if (el && !(e.relatedTarget instanceof Node && el.contains(e.relatedTarget))) hideTip()
  })
  document.addEventListener('focusin', e => { const el = tipTarget(e.target); if (el) scheduleShow(el) })
  document.addEventListener('focusout', e => { if (tipTarget(e.target)) hideTip() })
  document.addEventListener('keydown', e => {
    if (e.key !== 'Escape') return
    const anchor = current
    hideTip()
    if (anchor) escaped?.(anchor)
  })
  document.addEventListener('scroll', hideTip, true)
  window.addEventListener('resize', hideTip)
}
