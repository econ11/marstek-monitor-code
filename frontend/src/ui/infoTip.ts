import { t } from '../lib/i18n'
import { icon } from './icons'
import { escapeHTML as esc, hideTip, onTipEscape, showTip, tipAnchor } from './tooltip'

// The ⓘ next to a value or label (copy §15): a small button whose tooltip
// says what the value means. It opens on hover, on keyboard focus and on a
// click (Enter, Space); a click keeps it open after the pointer leaves, a
// second click, a click elsewhere or Esc closes it (Esc returns focus to the
// button). It is the app's one tooltip (tooltip.ts): edge-aware, one open at
// a time, and while open the button is described by it for the screen reader.

/** A copy key with its arguments, filled the same way as the row next to it. */
export type Info = { key: string; args?: Record<string, string | number> }

/** infoTipHTML is the ⓘ for the visible label `label`. at names the button
 *  for a redraw (keepInfoTip); it defaults to the key, so it only needs
 *  giving where one key has several buttons (a table). attrs: more
 *  attributes, as markup (Settings' data-focus). */
export function infoTipHTML(label: string, info: Info, at = info.key, attrs = ''): string {
  return `<button type="button" class="infotip" data-info="${esc(info.key)}" data-info-at="${esc(at)}" ` +
    `data-info-text="${esc(t(info.key, info.args))}" aria-label="${esc(t('info.button.a11y', { label }))}"${attrs}>${icon('info', 'icon--sm')}</button>`
}

/** The ⓘ the tooltip was opened on (null: none, or another tooltip took over). */
let open: HTMLElement | null = null
/** Opened by a click: it stays when the pointer leaves. */
let pinned = false
/** Opened by focus or a click: Esc gives the focus back (not after a hover). */
let owned = false
/** Focus put back by Esc or a redraw: it does not open the tooltip again. */
let quiet: Element | null = null
/** A redraw is replacing the buttons: their focusout and pointerout are not the user's. */
let redrawing = false

const infoAt = (target: EventTarget | null) =>
  (target && typeof (target as Element).closest === 'function' ? (target as Element).closest<HTMLElement>('.infotip') : null)

/** The ⓘ the tooltip shows now, if it is one. */
const shown = () => (open && tipAnchor() === open ? open : null)

function show(btn: HTMLElement, pin: boolean, own = pin) {
  open = btn
  pinned = pin
  owned = own
  showTip(btn, esc(btn.dataset.infoText ?? ''))
}

function close() {
  if (shown()) hideTip()
  open = null
  pinned = owned = false
}

/** initInfoTips wires every ⓘ of the window (after initTooltips). Call once. */
export function initInfoTips() {
  document.addEventListener('pointerover', e => {
    const btn = infoAt(e.target)
    if (btn && shown() !== btn) show(btn, false)
  })
  document.addEventListener('pointerout', e => {
    const btn = infoAt(e.target)
    const to = (e as PointerEvent).relatedTarget
    if (redrawing || !btn || shown() !== btn || pinned || (to instanceof Node && btn.contains(to))) return
    close()
  })
  document.addEventListener('focusin', e => {
    const btn = infoAt(e.target)
    if (btn && btn === quiet) { quiet = null; return }
    if (btn && shown() !== btn) show(btn, false, true)
    else if (btn) owned = true
  })
  document.addEventListener('focusout', e => {
    const btn = infoAt(e.target)
    if (!redrawing && btn && shown() === btn) close()
  })
  // A button turns Enter and Space into a click.
  document.addEventListener('click', e => {
    const btn = infoAt(e.target)
    if (!btn) return
    if (shown() === btn && pinned) close()
    else show(btn, true)
  })
  document.addEventListener('pointerdown', e => {
    if (pinned && shown() && !infoAt(e.target)) close()
  })
  // Esc closes every tooltip (tooltip.ts); an ⓘ also gets the focus back.
  onTipEscape(anchor => {
    if (anchor !== open) return
    const btn = open
    const give = owned
    close()
    if (give && btn.isConnected && document.activeElement !== btn) {
      quiet = btn
      btn.focus()
    }
  })
}

/** keepInfoTip runs a redraw that may replace the ⓘ buttons: an open one
 *  stays open on its successor (the button with the same data-info-at),
 *  with the new text, and keeps the focus; without a successor it closes. */
export function keepInfoTip(redraw: () => void) {
  const was = shown()
  if (!was) { redraw(); return }
  const focused = document.activeElement === was
  const at = was.dataset.infoAt ?? ''
  redrawing = true
  try {
    redraw()
  } finally {
    redrawing = false
  }
  const next = was.isConnected ? was : document.querySelector<HTMLElement>(`.infotip[data-info-at="${at}"]`)
  if (!next) { close(); return }
  if (focused && document.activeElement !== next) {
    quiet = next
    next.focus()
  }
  // The text may have changed (an argument): shown again, in place.
  show(next, pinned, owned)
}
