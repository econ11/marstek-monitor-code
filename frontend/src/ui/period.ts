import { getLocale, onLocaleChange, t } from '../lib/i18n'
import { dateShort } from '../lib/format'
import { dayStart, openCalendar, ymd, type DayRange } from './calendar'
import { icon } from './icons'
import { escapeHTML as esc } from './tooltip'

// The period switcher of Outages and Energy (hi-fi 02-outages.html):
// Сьогодні · 7 днів · 30 днів · a calendar button for a custom range.

export type { DayRange } from './calendar'
export type PeriodKey = 'today' | '7d' | '30d' | 'custom'

const KEYS: PeriodKey[] = ['today', '7d', '30d', 'custom']
const LABEL: Record<Exclude<PeriodKey, 'custom'>, string> = { today: 'common.period.today', '7d': 'common.period.7d', '30d': 'common.period.30d' }
const SPAN: Record<Exclude<PeriodKey, 'custom'>, number> = { today: 1, '7d': 7, '30d': 30 }

/** periodRange: today, or today and the 6 (29) days before it — counted in
 *  local calendar days, so a 23 or 25 hour day does not shift the start. */
export function periodRange(key: Exclude<PeriodKey, 'custom'>, now: number): DayRange {
  const d = new Date(now)
  const from = new Date(d.getFullYear(), d.getMonth(), d.getDate() - (SPAN[key] - 1)).getTime()
  return { from: ymd(from), to: ymd(now) }
}

/** rangeLabel: «15.11 – 21.11»; en «15 – 21 Nov», «28 Oct – 3 Nov» (copy §9.3 date.range). */
export function rangeLabel(r: DayRange): string {
  const from = dayStart(r.from), to = dayStart(r.to)
  if (r.from === r.to) return dateShort(from)
  const sameMonth = r.from.slice(0, 7) === r.to.slice(0, 7)
  const start = getLocale() === 'en' && sameMonth ? String(new Date(from).getDate()) : dateShort(from)
  return t('date.range', { start, end: dateShort(to) })
}

const nextFrame = (fn: () => void) => {
  if (typeof requestAnimationFrame === 'function') requestAnimationFrame(fn)
  else setTimeout(fn, 16)
}

/** syncSeg moves the segmented control's thumb under its pressed item. The
 *  first time (a new seg) it is put there without the slide from the left
 *  edge: seg--init turns the transition off until the next frame. */
export function syncSeg(seg: HTMLElement) {
  const on = seg.querySelector<HTMLElement>('[aria-pressed="true"]')
  const thumb = seg.querySelector<HTMLElement>('.seg__thumb')!
  if (!on) { thumb.style.width = '0'; return }
  const width = on.offsetWidth
  // Not laid out yet (width 0): the next sync is still the first.
  const first = !thumb.dataset.placed && width > 0
  if (first) seg.classList.add('seg--init')
  thumb.style.width = width + 'px'
  thumb.style.transform = `translateX(${on.offsetLeft}px)`
  if (!first) return
  thumb.dataset.placed = '1'
  // The place is computed now, with the transition off.
  void thumb.offsetWidth
  nextFrame(() => seg.classList.remove('seg--init'))
}

export type PeriodOpts = {
  initial: PeriodKey
  /** The range of an initial 'custom' (without one, 'custom' starts as 7 days). */
  range?: DayRange
  now: () => number
  /** Called on every change the user makes, and by set() for a fixed period. */
  onChange(r: DayRange, key: PeriodKey): void
}

/** mountPeriod draws the switcher into host. set() selects a period as a
 *  click would ('custom' opens the calendar); unmount() removes it. */
export function mountPeriod(host: HTMLElement, opts: PeriodOpts): { set(key: PeriodKey): void; unmount(): void } {
  let key: PeriodKey = opts.initial === 'custom' && !opts.range ? '7d' : opts.initial
  let custom: DayRange | undefined = opts.range
  let cal: { close(o?: { restoreFocus?: boolean }): void } | null = null
  const root = document.createElement('div')
  root.className = 'period'
  host.appendChild(root)

  function render(carry = true) {
    const items = KEYS.map(k => {
      if (k !== 'custom') {
        return `<button type="button" class="seg__item" aria-pressed="${k === key}" data-value="${k}">${esc(t(LABEL[k]))}</button>`
      }
      const label = key === 'custom' && custom ? rangeLabel(custom) : t('common.period.custom')
      return `<button type="button" class="seg__item" aria-pressed="${key === 'custom'}" data-value="custom" aria-haspopup="dialog" aria-expanded="${cal !== null}" ` +
        `aria-label="${esc(`${label}. ${t('common.period.custom.a11y')}`)}">${icon('calendar')}<span>${esc(label)}</span></button>`
    }).join('')
    // The new thumb starts where the old one was, so a change slides from
    // there (the first one is put in place, see syncSeg).
    const old = root.querySelector<HTMLElement>('.seg__thumb')
    const was = carry && old?.dataset.placed ? { width: old.style.width, transform: old.style.transform } : null
    root.innerHTML = `<div class="seg" role="group" aria-label="${esc(t('common.period.a11y'))}"><span class="seg__thumb"></span>${items}</div>`
    if (was) {
      const thumb = root.querySelector<HTMLElement>('.seg__thumb')!
      Object.assign(thumb.style, was)
      thumb.dataset.placed = '1'
    }
    sync()
  }
  const seg = () => root.querySelector<HTMLElement>('.seg')!
  const sync = () => syncSeg(seg())

  function openCal() {
    const btn = root.querySelector<HTMLElement>('[data-value="custom"]')!
    btn.setAttribute('aria-expanded', 'true')
    cal = openCalendar(btn, root, {
      now: opts.now(),
      sel: custom,
      onApply: r => {
        key = 'custom'
        custom = r
        render()
        root.querySelector<HTMLElement>('[data-value="custom"]')?.focus()
        opts.onChange(r, 'custom')
      },
      onClose: () => {
        cal = null
        root.querySelector<HTMLElement>('[data-value="custom"]')?.setAttribute('aria-expanded', 'false')
      },
    })
  }

  function set(k: PeriodKey) {
    if (k === 'custom') {
      if (!cal) openCal()
      return
    }
    cal?.close()
    key = k
    render()
    opts.onChange(periodRange(k, opts.now()), k)
  }

  root.addEventListener('click', e => {
    const item = (e.target as Element).closest<HTMLElement>('.seg__item')
    if (!item) return
    const k = item.dataset.value as PeriodKey
    if (k === 'custom') {
      if (cal) cal.close()
      else openCal()
    } else if (k !== key) {
      set(k)
    }
  })

  render()
  const resize = new ResizeObserver(sync)
  resize.observe(root)
  void document.fonts?.ready.then(() => { if (root.isConnected) sync() })
  const off = onLocaleChange(() => { cal?.close(); render(false) })

  return {
    set,
    unmount() {
      // The button goes with the switcher: the focus stays where it went.
      cal?.close({ restoreFocus: false })
      off()
      resize.disconnect()
      root.remove()
    },
  }
}
