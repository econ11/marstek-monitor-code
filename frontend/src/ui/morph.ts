// morphChildren brings the children of `from` (on screen) to those of `to`
// (fresh markup), keeping every node that can stay: a redraw then changes
// only what changed, so a switch or a segmented thumb animates from where it
// was and the rest of the screen is not rebuilt (no restart of transitions,
// the focus and an open tooltip stay on their nodes).
//
// Children are matched by key (data-row, data-key, id, data-focus), the rest
// by kind in order. Attributes and text follow the new markup, except the
// ones set from script: a thumb's style and data-placed, and the
// aria-describedby="mm-tip" of an anchor whose tooltip is open. A [data-slot]
// keeps what was mounted into it. Of the live properties only an input's
// value is synced (and not while it has the focus: that is the user's
// typing); the screens patched here have no checkbox, select or textarea.

type Parent = ParentNode & Node

/** Set from script on a thumb (syncSeg): not in the markup. */
const THUMB_KEEP = new Set(['style', 'data-placed'])

/** Whether an attribute missing from the new markup stays on a. */
function keeps(a: Element, name: string): boolean {
  if (name === 'aria-describedby' && a.getAttribute(name) === 'mm-tip') return true
  return THUMB_KEEP.has(name) && (a.getAttribute('class') ?? '').split(/\s+/).includes('seg__thumb')
}

const ELEMENT = 1

function keyOf(n: Node): string | null {
  if (n.nodeType !== ELEMENT) return null
  const el = n as Element
  return el.getAttribute('data-row') ?? el.getAttribute('data-key') ?? el.getAttribute('id') ?? el.getAttribute('data-focus')
}

const sameKind = (a: Node, b: Node) => a.nodeType === b.nodeType && a.nodeName === b.nodeName

function morphNode(from: Node, to: Node) {
  if (from.nodeType !== ELEMENT) {
    if (from.nodeValue !== to.nodeValue) from.nodeValue = to.nodeValue
    return
  }
  const a = from as Element, b = to as Element
  for (const { name } of Array.from(a.attributes)) {
    if (!b.hasAttribute(name) && !keeps(a, name)) a.removeAttribute(name)
  }
  for (const { name, value } of Array.from(b.attributes)) {
    if (a.getAttribute(name) !== value) a.setAttribute(name, value)
  }
  // A field shows the value it is drawn with (as a new one would), unless
  // the user is in it: a redraw never overwrites the typing.
  if (a.nodeName === 'INPUT' && a.ownerDocument?.activeElement !== a) {
    const v = b.getAttribute('value') ?? ''
    if ((a as HTMLInputElement).value !== v) (a as HTMLInputElement).value = v
  }
  morphChildren(a, b)
}

export function morphChildren(from: Parent, to: Parent) {
  if (from.nodeType === ELEMENT && (from as Element).hasAttribute('data-slot')) return
  const old = Array.from(from.childNodes)
  const byKey = new Map<string, Node>()
  for (const n of old) {
    const k = keyOf(n)
    if (k !== null && !byKey.has(k)) byKey.set(k, n)
  }
  const used = new Set<Node>()
  const next: Node[] = []
  for (const n of Array.from(to.childNodes)) {
    const k = keyOf(n)
    let match: Node | undefined
    if (k !== null) {
      const c = byKey.get(k)
      if (c && !used.has(c) && sameKind(c, n)) match = c
    } else {
      match = old.find(c => !used.has(c) && keyOf(c) === null && sameKind(c, n))
    }
    if (match) {
      used.add(match)
      morphNode(match, n)
      next.push(match)
    } else {
      next.push(n)
    }
  }
  for (const n of old) if (!used.has(n)) from.removeChild(n)
  next.forEach((n, i) => {
    const at = from.childNodes[i] ?? null
    if (at !== n) from.insertBefore(n, at)
  })
}
