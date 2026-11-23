import { getLocale, INTL, t } from '../lib/i18n'
import { weekdayShort } from '../lib/format'
import { icon } from './icons'
import { escapeHTML as esc } from './tooltip'

// The "custom period" calendar popover (hi-fi 02-outages.html): one month,
// weeks from Monday, future days disabled; the first click starts a range,
// the second ends it (in either order), «Показати» applies it.

/** DayRange is a span of whole local days, 'YYYY-MM-DD', both ends included. */
export type DayRange = { from: string; to: string }

const pad = (n: number) => String(n).padStart(2, '0')

/** ymd is the local day of at: 'YYYY-MM-DD'. */
export function ymd(at: number): string {
  const d = new Date(at)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** dayStart is local midnight of a 'YYYY-MM-DD' day. */
export function dayStart(day: string): number {
  const [y, m, d] = day.split('-').map(Number)
  return new Date(y, m - 1, d).getTime()
}

/** calendarDays lists every day of the month containing month (epoch ms):
 *  whether it is after now's day, and whether it lies in sel. */
export function calendarDays(month: number, now: number, sel?: DayRange): { day: string; future: boolean; inRange: boolean }[] {
  const m = new Date(month)
  const today = ymd(now)
  const count = new Date(m.getFullYear(), m.getMonth() + 1, 0).getDate()
  return Array.from({ length: count }, (_, i) => {
    const day = ymd(new Date(m.getFullYear(), m.getMonth(), i + 1).getTime())
    return { day, future: day > today, inRange: !!sel && day >= sel.from && day <= sel.to }
  })
}

/** Pick is the selection being made: open until its second day is picked. */
export type Pick = DayRange & { open: boolean }

/** pickDay is the selection after a click on day: the first click starts a
 *  range, the second closes it, ordered whichever day came first; a click
 *  after that starts a new one. */
export function pickDay(sel: Pick | undefined, day: string): Pick {
  if (!sel || !sel.open) return { from: day, to: day, open: true }
  return day < sel.from ? { from: day, to: sel.from, open: false } : { from: sel.from, to: day, open: false }
}

export type CalendarOpts = {
  now: number
  sel?: DayRange
  onApply(r: DayRange): void
  /** Called after the popover closes, by apply or not. */
  onClose(): void
}

// A Monday, for the weekday heads.
const MONDAY = new Date(2026, 0, 5).getTime()
const DAY = 24 * 3600e3

/** openCalendar shows the popover below anchor, inside container (a
 *  positioned element). Esc, «Скасувати» or a click outside close it
 *  without a change; focus goes back to anchor. */
export function openCalendar(anchor: HTMLElement, container: HTMLElement, opts: CalendarOpts): { close(o?: { restoreFocus?: boolean }): void } {
  const pop = document.createElement('div')
  pop.className = 'popover cal'
  pop.setAttribute('role', 'dialog')
  pop.setAttribute('aria-label', t('common.period.custom'))
  const a = anchor.getBoundingClientRect(), c = container.getBoundingClientRect()
  pop.style.left = `${a.left - c.left}px`
  pop.style.top = `${a.bottom - c.top + 6}px`
  container.appendChild(pop)

  let sel: Pick | undefined = opts.sel && { ...opts.sel, open: false }
  const base = new Date(opts.sel ? dayStart(opts.sel.to) : opts.now)
  let month = new Date(base.getFullYear(), base.getMonth(), 1).getTime()
  const thisMonth = new Date(new Date(opts.now).getFullYear(), new Date(opts.now).getMonth(), 1).getTime()
  const dateLong = new Intl.DateTimeFormat(INTL[getLocale()], { dateStyle: 'long' })
  const monthName = new Intl.DateTimeFormat(INTL[getLocale()], { month: 'long' })

  function render(focusDay?: string) {
    const m = new Date(month)
    const title = `${monthName.format(m)} ${m.getFullYear()}`
    const lead = (m.getDay() + 6) % 7
    const dows = Array.from({ length: 7 }, (_, i) => `<span class="dow" aria-hidden="true">${esc(weekdayShort(MONDAY + i * DAY))}</span>`).join('')
    const days = calendarDays(month, opts.now, sel).map(d => {
      const cls = [
        sel && d.day === sel.from ? 'a' : '', sel && d.day === sel.to ? 'b' : '',
        d.inRange && sel && d.day !== sel.from && d.day !== sel.to ? 'in' : '', d.future ? 'future' : '',
      ].filter(Boolean).join(' ')
      return `<button type="button" class="${cls}" data-day="${d.day}"${d.future ? ' disabled' : ''} aria-pressed="${d.inRange}" aria-label="${esc(dateLong.format(dayStart(d.day)))}">${Number(d.day.slice(8))}</button>`
    }).join('')
    pop.innerHTML = `<div class="cal__head">` +
      `<button type="button" class="iconbtn cal__prev" data-step="-1" aria-label="${esc(t('common.calendar.prev.a11y'))}">${icon('chevron-right')}</button>` +
      `<span aria-live="polite">${esc(title)}</span>` +
      `<button type="button" class="iconbtn" data-step="1" aria-label="${esc(t('common.calendar.next.a11y'))}"${month >= thisMonth ? ' disabled' : ''}>${icon('chevron-right')}</button></div>` +
      `<div class="cal__grid">${dows}${'<span></span>'.repeat(lead)}${days}</div>` +
      `<div class="cal__foot"><button type="button" class="btn btn--ghost" data-act="cancel">${esc(t('common.action.cancel'))}</button>` +
      `<button type="button" class="btn btn--primary" data-act="apply"${sel ? '' : ' disabled'}>${esc(t('common.period.custom.apply'))}</button></div>`
    const target = (focusDay && pop.querySelector<HTMLElement>(`[data-day="${focusDay}"]`)) || null
    target?.focus()
  }

  let closed = false
  /** close removes the popover; focus goes back to the anchor unless it has
   *  moved on (Tab out of the popover) or the anchor is going away (unmount). */
  function close(o: { restoreFocus?: boolean } = {}) {
    if (closed) return
    closed = true
    pop.remove()
    document.removeEventListener('keydown', onKey, true)
    document.removeEventListener('pointerdown', onOutside, true)
    if (o.restoreFocus !== false) anchor.focus()
    opts.onClose()
  }
  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    e.preventDefault()
    e.stopPropagation()
    close()
  }
  function onOutside(e: Event) {
    const el = e.target as Node
    if (!pop.contains(el) && !anchor.contains(el)) close()
  }

  pop.addEventListener('click', e => {
    const el = (e.target as Element).closest<HTMLButtonElement>('button')
    if (!el || el.disabled) return
    if (el.dataset.day) {
      sel = pickDay(sel, el.dataset.day)
      render(el.dataset.day)
    } else if (el.dataset.step) {
      const m = new Date(month)
      month = new Date(m.getFullYear(), m.getMonth() + Number(el.dataset.step), 1).getTime()
      render()
      // The same arrow keeps the focus; at the current month, the other one.
      const same = pop.querySelector<HTMLElement>(`[data-step="${el.dataset.step}"]:not([disabled])`)
      ;(same ?? pop.querySelector<HTMLElement>('[data-step="-1"]'))?.focus()
    } else if (el.dataset.act === 'apply' && sel) {
      const r = { from: sel.from, to: sel.to }
      close()
      opts.onApply(r)
    } else if (el.dataset.act === 'cancel') {
      close()
    }
  })
  // Tab out of the popover (onto a header tab, say) closes it and leaves the
  // focus there: Enter on that tab then unmounts the period with nothing to
  // pull the focus back to a removed button. A redraw inside the popover
  // drops the focused day (no relatedTarget): that is not leaving.
  pop.addEventListener('focusout', e => {
    const to = (e as FocusEvent).relatedTarget as Node | null
    if (to && !pop.contains(to) && !anchor.contains(to)) close({ restoreFocus: false })
  })
  document.addEventListener('keydown', onKey, true)
  document.addEventListener('pointerdown', onOutside, true)

  // Focus starts on the selected end, or on today.
  render(sel?.to ?? ymd(opts.now))
  return { close }
}
