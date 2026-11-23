import { vi } from 'vitest'

// A stand-in for the DOM calls the tab screens make, so their tests run in
// node without a DOM library (none is installed; router.test.ts does the
// same on a smaller scale). It does not parse markup: an element keeps the
// HTML it is given, and querySelector hands out one stand-in per selector
// until the HTML is replaced. A [data-slot] lookup finds the slot only when
// the markup has it, and starts `hidden` when its tag carries the attribute.

type Listener = (e: unknown) => void

class Listeners {
  map = new Map<string, Set<Listener>>()
  addEventListener(type: string, fn: Listener) {
    if (!this.map.has(type)) this.map.set(type, new Set())
    this.map.get(type)!.add(fn)
  }
  removeEventListener(type: string, fn: Listener) { this.map.get(type)?.delete(fn) }
  /** dispatch calls the listeners of type with ev (a plain object). */
  dispatch(type: string, ev: Record<string, unknown> = {}) {
    for (const fn of Array.from(this.map.get(type) ?? [])) fn({ preventDefault() {}, ...ev })
  }
  count(type: string) { return this.map.get(type)?.size ?? 0 }
}

const SLOT = /^\[data-slot="([^"]+)"\]$/

export class FakeEl extends Listeners {
  hidden = false
  textContent = ''
  className = ''
  dataset: Record<string, string> = {}
  style: Record<string, string> = {}
  attrs = new Map<string, string>()
  clientWidth = 0
  offsetWidth = 0
  offsetLeft = 0
  isConnected = true
  kids: FakeEl[] = []
  /** The element whose querySelector handed this one out. */
  owner: FakeEl | null = null
  private html = ''
  private found = new Map<string, FakeEl>()
  private first: FakeEl | null = null
  classList = (() => {
    const set = new Set<string>()
    return {
      set,
      add: (...c: string[]) => c.forEach(x => set.add(x)),
      remove: (...c: string[]) => c.forEach(x => set.delete(x)),
      toggle: (c: string, on?: boolean) => { const v = on ?? !set.has(c); if (v) set.add(c); else set.delete(c); return v },
      contains: (c: string) => set.has(c),
    }
  })()

  get innerHTML() { return this.html }
  set innerHTML(v: string) { this.html = v; this.found.clear(); this.first = null }
  get children() { return [] as FakeEl[] }
  get firstElementChild() { return (this.first ??= new FakeEl()) }
  get lastElementChild() { return this.firstElementChild }

  querySelector(sel: string): FakeEl | null {
    const slot = SLOT.exec(sel)
    if (slot && !this.html.includes(`data-slot="${slot[1]}"`)) return null
    let el = this.found.get(sel)
    if (!el) {
      el = new FakeEl()
      el.owner = this
      if (slot) {
        const tag = new RegExp(`<[^>]*data-slot="${slot[1]}"[^>]*>`).exec(this.html)?.[0] ?? ''
        el.hidden = /\shidden(?=[\s>])/.test(tag)
      }
      this.found.set(sel, el)
    }
    return el
  }
  querySelectorAll(): FakeEl[] { return [] }
  getAttribute(n: string) { return this.attrs.get(n) ?? null }
  setAttribute(n: string, v: string) { this.attrs.set(n, String(v)) }
  removeAttribute(n: string) { this.attrs.delete(n) }
  hasAttribute(n: string) { return this.attrs.has(n) }
  /** An element handed out by querySelector has every ancestor asked for
   *  (one stand-in per selector); a free-standing one has none. */
  closest(sel: string) { return this.owner?.querySelector(`closest ${sel}`) ?? null }
  contains() { return false }
  focus() { fakeDocument.activeElement = this }
  remove() { this.isConnected = false }
  appendChild(c: FakeEl) { this.kids.push(c); return c }
  insertAdjacentHTML(_where: string, html: string) { this.html += html }
  replaceChildren() { this.kids = []; this.html = '' }
  scrollIntoView() {}
  getBoundingClientRect() { return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 } }
}

class FakeDocument extends Listeners {
  visibilityState: 'visible' | 'hidden' = 'visible'
  activeElement: FakeEl | null = null
  body = new FakeEl()
  documentElement = new FakeEl()
  createElement() { return new FakeEl() }
  querySelector() { return null }
}

export let fakeDocument = new FakeDocument()

/** installFakeDom puts a fresh document, window, ResizeObserver and Node in
 *  the globals; vi.unstubAllGlobals() takes them away. */
export function installFakeDom() {
  fakeDocument = new FakeDocument()
  vi.stubGlobal('document', fakeDocument)
  vi.stubGlobal('window', globalThis)
  vi.stubGlobal('Node', FakeEl)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  return fakeDocument
}

/** host is an element to mount a screen into. */
export const host = (width = 0) => { const el = new FakeEl(); el.clientWidth = width; return el as unknown as HTMLElement }
