import { expect, test } from 'vitest'
import { morphChildren } from './morph'

// A small node tree with the calls morph makes (no DOM library here), to see
// which nodes a redraw keeps and which it replaces.
class N {
  parentNode: N | null = null
  childNodes: N[] = []
  attrs = new Map<string, string>()
  value = ''
  constructor(public nodeType: number, public nodeName: string, public nodeValue: string | null = null) {}
  get attributes() { return Array.from(this.attrs, ([name, value]) => ({ name, value })) }
  getAttribute(n: string) { return this.attrs.get(n) ?? null }
  setAttribute(n: string, v: string) { this.attrs.set(n, v); if (n === 'value' && !this.value) this.value = v }
  removeAttribute(n: string) { this.attrs.delete(n) }
  hasAttribute(n: string) { return this.attrs.has(n) }
  removeChild(c: N) { this.childNodes.splice(this.childNodes.indexOf(c), 1); c.parentNode = null; return c }
  insertBefore(c: N, ref: N | null) {
    c.parentNode?.removeChild(c)
    const i = ref ? this.childNodes.indexOf(ref) : this.childNodes.length
    this.childNodes.splice(i, 0, c)
    c.parentNode = this
    return c
  }
  appendChild(c: N) { return this.insertBefore(c, null) }
}

const h = (tag: string, attrs: Record<string, string> = {}, ...kids: (N | string)[]) => {
  const el = new N(1, tag.toUpperCase())
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v)
  for (const k of kids) el.appendChild(typeof k === 'string' ? new N(3, '#text', k) : k)
  return el
}

const toggleRow = (id: string, on: boolean) =>
  h('div', { class: 'opt', 'data-row': id }, h('div', { class: 'opt__t' }, `Label ${id}`), h('button', { class: 'switch', 'aria-checked': String(on), 'data-focus': id }))
const segRow = (id: string, value: string) =>
  h('div', { class: 'opt', 'data-row': id },
    h('div', { class: 'seg' }, h('span', { class: 'seg__thumb' }),
      ...['a', 'b'].map(v => h('button', { class: 'seg__item', 'aria-pressed': String(v === value), 'data-focus': `${id}:${v}` }, v))))
const body = (...rows: N[]) => h('section', {}, h('h2', { id: 'settings-h' }, 'General'), h('div', { class: 'panel' }, ...rows))
const morph = (from: N, to: N) => morphChildren(from as unknown as ParentNode & Node, to as unknown as ParentNode & Node)
const panel = (b: N) => b.childNodes[1]

test('rows unchanged: no node is replaced', () => {
  const live = body(toggleRow('a', true), segRow('lang', 'a'))
  const before = [live.childNodes[0], panel(live), ...panel(live).childNodes, panel(live).childNodes[1].childNodes[0].childNodes[0]]
  morph(live, body(toggleRow('a', true), segRow('lang', 'a')))
  const after = [live.childNodes[0], panel(live), ...panel(live).childNodes, panel(live).childNodes[1].childNodes[0].childNodes[0]]
  after.forEach((n, i) => expect(n).toBe(before[i]))
})

test('a switched toggle keeps its node and only its attribute changes', () => {
  const live = body(toggleRow('a', false), toggleRow('b', true))
  const [a, b] = panel(live).childNodes
  const sw = a.childNodes[1]
  morph(live, body(toggleRow('a', true), toggleRow('b', true)))
  expect(panel(live).childNodes[0]).toBe(a)
  expect(panel(live).childNodes[1]).toBe(b)
  expect(a.childNodes[1]).toBe(sw)
  expect(sw.getAttribute('aria-checked')).toBe('true')
})

test('a segmented change keeps the thumb and its position', () => {
  const live = body(segRow('lang', 'a'))
  const thumb = panel(live).childNodes[0].childNodes[0].childNodes[0]
  thumb.setAttribute('style', 'width: 40px; transform: translateX(3px)')
  thumb.setAttribute('data-placed', '1')
  morph(live, body(segRow('lang', 'b')))
  const seg = panel(live).childNodes[0].childNodes[0]
  expect(seg.childNodes[0]).toBe(thumb)
  // The thumb slides from where it was: its style and mark stay.
  expect(thumb.getAttribute('style')).toContain('40px')
  expect(thumb.getAttribute('data-placed')).toBe('1')
  expect(seg.childNodes[1].getAttribute('aria-pressed')).toBe('false')
  expect(seg.childNodes[2].getAttribute('aria-pressed')).toBe('true')
})

test('a row shown or hidden leaves the other rows in place', () => {
  const live = body(toggleRow('a', true), toggleRow('c', true))
  const [a, c] = panel(live).childNodes
  morph(live, body(toggleRow('a', true), toggleRow('b', false), toggleRow('c', true)))
  expect(panel(live).childNodes.map(n => n.getAttribute('data-row'))).toEqual(['a', 'b', 'c'])
  expect(panel(live).childNodes[0]).toBe(a)
  expect(panel(live).childNodes[2]).toBe(c)
  morph(live, body(toggleRow('c', true)))
  expect(panel(live).childNodes).toEqual([c])
})

test('a row whose control changed kind is replaced alone', () => {
  const live = body(toggleRow('a', true), toggleRow('b', true))
  const [a, b] = panel(live).childNodes
  const next = segRow('b', 'a')
  morph(live, body(toggleRow('a', true), next))
  expect(panel(live).childNodes[0]).toBe(a)
  // Same row, new markup inside: the row node stays, its switch goes.
  expect(panel(live).childNodes[1]).toBe(b)
  expect(b.childNodes.map(n => n.getAttribute('class'))).toEqual(['seg'])
})

test('text, input values and removed attributes follow the new markup', () => {
  const live = h('div', {}, h('input', { 'data-focus': 'name', value: 'Old', 'aria-invalid': 'true' }), h('p', {}, 'old error'))
  const input = live.childNodes[0]
  input.value = 'typed'
  morph(live, h('div', {}, h('input', { 'data-focus': 'name', value: 'New' }), h('p', {}, '')))
  expect(live.childNodes[0]).toBe(input)
  expect(input.value).toBe('New')
  expect(input.hasAttribute('aria-invalid')).toBe(false)
  expect(live.childNodes[1].childNodes[0].nodeValue).toBe('')
})

test('a slot keeps what was mounted into it', () => {
  const live = h('div', {}, h('div', { 'data-slot': 'period' }, h('div', { class: 'period' })))
  const mounted = live.childNodes[0].childNodes[0]
  morph(live, h('div', {}, h('div', { 'data-slot': 'period' })))
  expect(live.childNodes[0].childNodes).toEqual([mounted])
})

test('a focused field keeps what the user typed', () => {
  const doc = { activeElement: null as N | null }
  const live = h('div', {}, h('input', { 'data-focus': 'name', value: 'Old' }))
  const input = live.childNodes[0]
  ;(input as unknown as { ownerDocument: unknown }).ownerDocument = doc
  doc.activeElement = input
  input.value = 'typed, not valid'
  morph(live, h('div', {}, h('input', { 'data-focus': 'name', value: 'Old', 'aria-invalid': 'true' })))
  expect(input.value).toBe('typed, not valid')
  expect(input.getAttribute('aria-invalid')).toBe('true')
  doc.activeElement = null
  morph(live, h('div', {}, h('input', { 'data-focus': 'name', value: 'Old' })))
  expect(input.value).toBe('Old')
})

test('style and data-placed are kept on a thumb only', () => {
  const live = h('div', {}, h('span', { class: 'seg__thumb', style: 'width: 4px', 'data-placed': '1' }), h('div', { 'data-key': 'x', style: 'color: red', 'data-placed': '1' }))
  morph(live, h('div', {}, h('span', { class: 'seg__thumb' }), h('div', { 'data-key': 'x' })))
  expect(live.childNodes[0].getAttribute('style')).toBe('width: 4px')
  expect(live.childNodes[0].getAttribute('data-placed')).toBe('1')
  expect(live.childNodes[1].hasAttribute('style')).toBe(false)
  expect(live.childNodes[1].hasAttribute('data-placed')).toBe(false)
})

test('an open tooltip keeps describing its anchor', () => {
  const live = h('div', {}, h('button', { 'data-focus': 'a:info', 'aria-describedby': 'mm-tip' }), h('button', { 'data-focus': 'b', 'aria-describedby': 'old' }))
  morph(live, h('div', {}, h('button', { 'data-focus': 'a:info' }), h('button', { 'data-focus': 'b' })))
  expect(live.childNodes[0].getAttribute('aria-describedby')).toBe('mm-tip')
  expect(live.childNodes[1].hasAttribute('aria-describedby')).toBe(false)
})
