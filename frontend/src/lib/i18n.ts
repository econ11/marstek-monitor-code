import { en } from '../i18n/en'
import { uk } from '../i18n/uk'

// A small ICU MessageFormat: {name}, {name, plural, =0 {…} one {…} few {…}
// many {…} other {…}} with # for the number, and {name, select, a {…} other {…}}.

type Args = Record<string, string | number>
type Node =
  | string
  | { hash: true }
  | { arg: string; kind?: 'plural' | 'select'; cases?: Record<string, Node[]> }

export type Locale = 'uk' | 'en'

// Intl locale per UI language: en is British English (24 h, "21 Nov").
export const INTL: Record<Locale, string> = { uk: 'uk', en: 'en-GB' }
const catalogues: Record<Locale, Record<string, string>> = { uk, en }

let locale: Locale = 'uk'
let plurals = new Intl.PluralRules(INTL.uk)
let numbers = new Intl.NumberFormat(INTL.uk)
let cache = new Map<string, Node[]>()
const listeners = new Set<() => void>()

/** setLocale switches catalogue, plural rules and number format, then tells
 *  the onLocaleChange listeners (only when the language changes). */
export function setLocale(l: Locale): void {
  if (l === locale) return
  locale = l
  plurals = new Intl.PluralRules(INTL[l])
  numbers = new Intl.NumberFormat(INTL[l])
  cache = new Map()
  Array.from(listeners).forEach(fn => fn())
}

export function getLocale(): Locale {
  return locale
}

/** onLocaleChange calls fn after every language switch; returns the unsubscribe. */
export function onLocaleChange(fn: () => void): () => void {
  listeners.add(fn)
  return () => { listeners.delete(fn) }
}

class Parser {
  private i = 0
  constructor(private readonly src: string) {}

  // Reads nodes until the end, or until the "}" closing a plural / select case.
  message(nested: boolean, inPlural: boolean): Node[] {
    const out: Node[] = []
    let text = ''
    const flush = () => { if (text) { out.push(text); text = '' } }
    while (this.i < this.src.length) {
      const c = this.src[this.i]
      if (c === '}' && nested) break
      if (c === '{') { flush(); this.i++; out.push(this.argument(inPlural)); continue }
      if (c === '#' && inPlural) { flush(); this.i++; out.push({ hash: true }); continue }
      text += c
      this.i++
    }
    flush()
    return out
  }

  private until(stop: string): string {
    const from = this.i
    while (this.i < this.src.length && !stop.includes(this.src[this.i])) this.i++
    return this.src.slice(from, this.i).trim()
  }

  private space() {
    while (this.src[this.i] === ' ') this.i++
  }

  // Called just after "{".
  private argument(inPlural: boolean): Node {
    const arg = this.until(',}')
    if (this.src[this.i] !== ',') { this.i++; return { arg } }
    this.i++
    const kind = this.until(',') as 'plural' | 'select'
    this.i++
    const cases: Record<string, Node[]> = {}
    for (;;) {
      this.space()
      if (this.i >= this.src.length || this.src[this.i] === '}') { this.i++; break }
      const selector = this.until(' {')
      this.space()
      this.i++ // "{"
      cases[selector] = this.message(true, kind === 'plural' || inPlural)
      this.i++ // "}"
    }
    return { arg, kind, cases }
  }
}

function render(nodes: Node[], args: Args, hash: string): string {
  let out = ''
  for (const n of nodes) {
    if (typeof n === 'string') out += n
    else if ('hash' in n) out += hash
    else if (!n.kind) out += n.arg in args ? String(args[n.arg]) : `{${n.arg}}`
    else {
      const cases = n.cases ?? {}
      const v = args[n.arg]
      if (n.kind === 'plural') {
        const num = Number(v)
        const branch = cases[`=${num}`] ?? cases[plurals.select(num)] ?? cases.other ?? []
        out += render(branch, args, numbers.format(num))
      } else {
        out += render(cases[String(v)] ?? cases.other ?? [], args, hash)
      }
    }
  }
  return out
}

// t returns the text for a copy key in the current language. A key missing
// there takes the en, then the uk text; a key in neither comes back unchanged
// so a missing string is visible on screen instead of silently empty.
export function t(key: string, args: Record<string, string | number> = {}): string {
  const src = catalogues[locale][key] ?? en[key] ?? uk[key]
  if (src === undefined) return key
  let nodes = cache.get(key)
  if (!nodes) {
    nodes = new Parser(src).message(false, false)
    cache.set(key, nodes)
  }
  return render(nodes, args, '')
}
