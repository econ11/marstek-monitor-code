import { getLocale, INTL, t } from './i18n'

// Numbers, durations and dates in the current language (copy.md §9).
// Durations are in milliseconds, instants are epoch milliseconds, all in the
// Windows time zone.

const MIN = 60e3, HOUR = 3600e3, DAY = 24 * HOUR
const WEEKDAYS_UK = ['нд', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб']
const intl = () => INTL[getLocale()]

const pad = (n: number) => String(n).padStart(2, '0')
const wholeMinutes = (ms: number) => Math.max(0, Math.floor(ms / MIN))

/** num: uk "2 500" and "3,7", en "2,500" and "3.7"; up to maxFrac decimals. */
export function num(x: number, maxFrac = 0): string {
  return new Intl.NumberFormat(intl(), { maximumFractionDigits: maxFrac }).format(x)
}

/** numFixed: exactly `frac` decimals, never trimmed: 4.8 → "4,80" (en "4.80"). */
export function numFixed(x: number, frac: number): string {
  return new Intl.NumberFormat(intl(), { minimumFractionDigits: frac, maximumFractionDigits: frac }).format(x)
}

/** hm is the clock form for ring centres: 19620000 → "5:27", 1620000 → "0:27". */
export function hm(ms: number): string {
  const m = wholeMinutes(ms)
  return t('overview.ring.eta.value.hm', { h: Math.floor(m / 60), mm: pad(m % 60) })
}

/** hmLong is the short text form: "7 год 30 хв", "45 хв", "2 год", "1 д 3 год". */
export function hmLong(ms: number): string {
  const m = wholeMinutes(ms)
  if (m < 1) return t('duration.short.lt1m')
  if (m < 60) return t('duration.short.m', { m })
  const h = Math.floor(m / 60)
  if (h >= 24) return t('duration.short.dh', { d: Math.floor(h / 24), h: h % 24 })
  if (m % 60 === 0) return t('duration.short.h', { h })
  return t('duration.short.hm', { h, mm: pad(m % 60) })
}

/** spoken is the long form for screen readers: "5 годин 27 хвилин". */
export function spoken(ms: number): string {
  const m = wholeMinutes(ms)
  const h = Math.floor(m / 60)
  const minutes = t('duration.long.m', { m: m % 60 })
  if (h === 0) return minutes
  if (h >= 24) {
    return t('duration.long.hm', { hours: t('duration.long.d', { d: Math.floor(h / 24) }), minutes: t('duration.long.h', { h: h % 24 }) })
  }
  const hours = t('duration.long.h', { h })
  return m % 60 === 0 ? hours : t('duration.long.hm', { hours, minutes })
}

/** kwh formats watt-hours as kilowatt-hours with one decimal: 3686.4 → "3,7" (en "3.7"). */
export function kwh(wh: number): string {
  return new Intl.NumberFormat(intl(), { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(wh / 1000)
}

/** pct: 72 → "72%" (no space). */
export function pct(x: number): string {
  return `${Math.round(x)}%`
}

/** age: "3 с тому", "2 хв тому", "3 год тому". */
export function age(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return t('age.seconds', { seconds: s })
  if (s < 3600) return t('age.minutes', { minutes: Math.floor(s / 60) })
  return t('age.hours', { hours: Math.floor(s / 3600) })
}

/** clock: "14:05". */
export function clock(at: number): string {
  const d = new Date(at)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** dateShort: "21.11", en "21 Nov". */
export function dateShort(at: number): string {
  const d = new Date(at)
  if (getLocale() === 'en') return new Intl.DateTimeFormat(intl(), { day: 'numeric', month: 'short' }).format(d)
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}`
}

/** weekdayShort: "пт", en "Fri". */
export function weekdayShort(at: number): string {
  const d = new Date(at)
  if (getLocale() === 'en') return new Intl.DateTimeFormat(intl(), { weekday: 'short' }).format(d)
  return WEEKDAYS_UK[d.getDay()]
}

/** dayShort is the axis label: "сб 21.11", en "Sat 21" (copy.md §9.3). */
export function dayShort(at: number): string {
  if (getLocale() === 'en') return `${weekdayShort(at)} ${new Date(at).getDate()}`
  return `${weekdayShort(at)} ${dateShort(at)}`
}

/** midnight is the start of the local day containing at. */
export function midnight(at: number): number {
  const d = new Date(at)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

/** daysBefore: 0 when at is on the same local day as now, 1 for yesterday… */
export function daysBefore(at: number, now: number): number {
  return Math.round((midnight(now) - midnight(at)) / DAY)
}

/** dayTimes: the times of one outage window ({from}, {to}[, {end}]), in order,
 *  each day named once, relative to now (controller ruling 2026-11-23): a day
 *  other than today gets «вчора» / «19.11» the first time it is mentioned;
 *  today stays a plain time, except after another day past the window's own
 *  two times («між вчора 01:00 і 07:10 – сьогодні 09:00»). */
export function dayTimes(times: number[], now: number): string[] {
  return times.map((at, i) => {
    const ago = daysBefore(at, now)
    const same = i > 0 && midnight(times[i - 1]) === midnight(at)
    if (same || (ago === 0 && i < 2)) return clock(at)
    if (ago === 0) return t('date.today', { time: clock(at) })
    if (ago === 1) return t('date.yesterday', { time: clock(at) })
    return t('date.shortTime', { date: dateShort(at), time: clock(at) })
  })
}
