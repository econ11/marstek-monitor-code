import { afterEach, beforeAll, expect, test, vi } from 'vitest'
import { setLocale } from '../lib/i18n'
import { FakeEl, fakeDocument, installFakeDom } from '../testing/fakeDom'
import { initInfoTips, infoTipHTML, keepInfoTip } from './infoTip'
import { hideTip, initTooltips, showTip, tipAnchor } from './tooltip'

// The ⓘ button as the tooltip engine sees it: closest('.infotip') is the
// button itself, its data-* attributes are read back from infoTipHTML().

class Btn extends FakeEl {
  constructor(html: string) {
    super()
    for (const m of Array.from(html.matchAll(/ (data-info(?:-[a-z]+)?|aria-label)="([^"]*)"/g))) {
      const v = m[2].replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&')
      this.setAttribute(m[1], v)
      if (m[1].startsWith('data-')) this.dataset[m[1].slice(5).replace(/-([a-z])/g, (_, c: string) => c.toUpperCase())] = v
    }
  }
  closest(sel: string): FakeEl | null { return sel === '.infotip' ? this : null }
  contains(...n: unknown[]) { return n[0] === this }
}
const button = (html: string) => new Btn(html) as Btn
/** The page: every ⓘ on it, found by its data-info-at. */
let page: Btn[] = []

let tip: FakeEl
beforeAll(() => {
  installFakeDom()
  vi.stubGlobal('window', { addEventListener() {}, setTimeout, clearTimeout })
  vi.stubGlobal('Element', FakeEl)
  // The one tooltip element, so its body can be read.
  const create = fakeDocument.createElement.bind(fakeDocument)
  fakeDocument.createElement = () => (tip = create())
  initTooltips()
  initInfoTips()
  fakeDocument.querySelector = ((sel: string) => {
    const at = /^\.infotip\[data-info-at="(.+)"\]$/.exec(sel)?.[1]
    return at ? page.find(b => b.isConnected && b.dataset.infoAt === at) ?? null : null
  }) as never
})

afterEach(() => {
  hideTip()
  fakeDocument.dispatch('pointerdown', { target: fakeDocument.body })
  fakeDocument.activeElement = null
  page = []
  setLocale('uk')
})

const body = () => tip.firstElementChild.innerHTML
const isOpen = () => tip.classList.contains('is-open')
const ev = (type: string, target: unknown, extra: Record<string, unknown> = {}) => fakeDocument.dispatch(type, { target, ...extra })

function mk(label = 'Покриття', key = 'info.outages.coverage', args?: Record<string, string | number>, at?: string) {
  const b = button(infoTipHTML(label, { key, args }, at))
  page.push(b)
  return b
}

test('the button is named after the label next to it', () => {
  expect(mk().getAttribute('aria-label')).toBe('Що означає «Покриття»')
  setLocale('en')
  expect(mk('Coverage').getAttribute('aria-label')).toBe('What “Coverage” means')
})

test('the arguments are filled in as on the visible row', () => {
  const b = mk('Потужність', 'info.overview.power', { max: '2 500' })
  ev('pointerover', b)
  expect(body()).toContain('«з 2 500 Вт макс.»')
  expect(body()).not.toContain('{max}')
})

test('hover opens it, leaving the button closes it', () => {
  const b = mk()
  ev('pointerover', b)
  expect(isOpen()).toBe(true)
  expect(tipAnchor()).toBe(b)
  expect(body()).toContain('Скільки відключень станція витримала')
  ev('pointerout', b, { relatedTarget: null })
  expect(isOpen()).toBe(false)
})

test('keyboard focus opens it and the screen reader gets the text; blur closes it', () => {
  const b = mk()
  ev('focusin', b)
  expect(isOpen()).toBe(true)
  expect(b.getAttribute('aria-describedby')).toBe('mm-tip')
  ev('focusout', b)
  expect(isOpen()).toBe(false)
  expect(b.getAttribute('aria-describedby')).toBe(null)
})

test('a click opens it and keeps it open when the pointer leaves; a second click closes it', () => {
  const b = mk()
  ev('click', b)
  expect(isOpen()).toBe(true)
  ev('pointerout', b, { relatedTarget: null })
  expect(isOpen()).toBe(true)
  ev('click', b)
  expect(isOpen()).toBe(false)
})

test('a click after hover keeps it open; a click elsewhere closes it', () => {
  const b = mk()
  ev('pointerover', b)
  ev('click', b)
  expect(isOpen()).toBe(true)
  ev('pointerdown', fakeDocument.body)
  expect(isOpen()).toBe(false)
})

test('Enter and Space open it (a button turns them into a click)', () => {
  const b = mk()
  // Focus first opens it; Enter then keeps it open instead of toggling it shut.
  ev('focusin', b)
  ev('click', b)
  expect(isOpen()).toBe(true)
})

test('Esc closes it and puts focus back on the button', () => {
  const b = mk()
  ev('pointerover', b)
  ev('click', b)
  fakeDocument.activeElement = null
  ev('keydown', fakeDocument.body, { key: 'Escape' })
  expect(isOpen()).toBe(false)
  expect(fakeDocument.activeElement).toBe(b)
  // The focus that returns does not open it again.
  ev('focusin', b)
  expect(isOpen()).toBe(false)
})

test('Esc after a hover-only open closes it and leaves the focus where it was', () => {
  const b = mk()
  ev('pointerover', b)
  ev('keydown', fakeDocument.body, { key: 'Escape' })
  expect(isOpen()).toBe(false)
  expect(fakeDocument.activeElement).toBe(null)
})

test('Esc after a keyboard-focus open puts the focus back on the button', () => {
  const b = mk()
  ev('focusin', b)
  fakeDocument.activeElement = null
  ev('keydown', fakeDocument.body, { key: 'Escape' })
  expect(fakeDocument.activeElement).toBe(b)
})

test('one open at a time', () => {
  const a = mk('Світло', 'info.outages.grid')
  const b = mk()
  ev('click', a)
  ev('pointerover', b)
  expect(tipAnchor()).toBe(b)
  expect(a.getAttribute('aria-describedby')).toBe(null)
  expect(body()).toContain('Скільки відключень')
})

test('another tooltip taking over closes the ⓘ', () => {
  const b = mk()
  ev('click', b)
  showTip(new FakeEl() as unknown as Element, 'chart')
  expect(tipAnchor()).not.toBe(b)
  // A later click on the ⓘ opens it again, not «closes» it.
  ev('click', b)
  expect(tipAnchor()).toBe(b)
})

test('a redraw keeps an open ⓘ open on the new button, and its focus', () => {
  const b = mk('Мінімальний заряд станції', 'info.overview.chart.floor', { floor: 10 }, 'floor')
  ev('focusin', b)
  fakeDocument.activeElement = b
  let next: Btn | undefined
  keepInfoTip(() => {
    b.isConnected = false
    ev('focusout', b)
    next = mk('Мінімальний заряд станції', 'info.overview.chart.floor', { floor: 25 }, 'floor')
  })
  expect(tipAnchor()).toBe(next)
  expect(fakeDocument.activeElement).toBe(next)
  expect(body()).toContain('(25%)')
})

test('a redraw without that ⓘ closes it', () => {
  const b = mk()
  ev('pointerover', b)
  keepInfoTip(() => { b.isConnected = false })
  expect(isOpen()).toBe(false)
})

test('a redraw with nothing open opens nothing', () => {
  mk()
  keepInfoTip(() => {})
  expect(isOpen()).toBe(false)
})
