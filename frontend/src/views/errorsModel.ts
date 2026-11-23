import type { Entry } from '../../bindings/github.com/econ11/marstek-monitor-code/core/journal/models'
import { clock, dateShort, dayShort, daysBefore, hmLong, midnight } from '../lib/format'
import { t } from '../lib/i18n'

// Everything the «Помилки» tab shows, derived from the journal entries
// (Task 10, core/journal; copy §6, §6.1; hi-fi 03-errors). Pure: the view only
// turns this into DOM, and escapes every text, args included.

export type ErrorFilter = 'all' | 'error' | 'warning' | 'connection'
export type ErrorRow = { id: string; when: string; whenSub: string; level: 'info' | 'warning' | 'error'; levelLabel: string; icon: string; what: string; whatSub?: string; todo: string; isNew: boolean }
export type JournalCounts = { All: number; Errors: number; Warnings: number; Connection: number; New?: number }
export type PageInfo = { counts: JournalCounts; more: boolean }
export type ErrorsModel = { strip: string; filters: { key: ErrorFilter; label: string; count: number; disabled: boolean; tip?: string }[]; hint: string; rows: ErrorRow[]; empty?: string; more: boolean; showing: string }

const LEVELS = new Set(['info', 'warning', 'error'])
const FILTER_LABEL: Record<ErrorFilter, string> = {
  all: 'errors.filter.all', error: 'errors.filter.errors', warning: 'errors.filter.warnings', connection: 'errors.filter.connection',
}
/** The «зв'язок» filter: the app's connection events and its own absences. */
const CONNECTION_GROUPS = new Set(['connection', 'app'])
/** The icon is about the subject (hi-fi 03-errors note 1); a station event is «!». */
const ICON: Record<string, string> = {
  'conn.event.apiSilent': 'wifi-off', 'conn.event.lanLost.noFallback': 'wifi-off', 'conn.event.lanRestored': 'wifi',
  'conn.event.ipChanged': 'network', 'journal.linkDropped': 'wifi-off', 'journal.appOff': 'clock', 'journal.pcSleep': 'moon',
  'journal.appUpdate': 'download',
}
const GROUP_ICON: Record<string, string> = { connection: 'wifi', station: 'alert', app: 'clock' }

const levelOf = (e: Entry): ErrorRow['level'] => (LEVELS.has(e.Level) ? e.Level as ErrorRow['level'] : 'info')
const matches = (e: Entry, f: ErrorFilter) =>
  f === 'all' || (f === 'connection' ? CONNECTION_GROUPS.has(e.Group) : levelOf(e) === f)
/** New, as the tab's badge counts it (core/journal NewSince): a warning or an error after seenAt. */
const isNew = (e: Entry, seenAt: number) => levelOf(e) !== 'info' && Date.parse(e.At) > seenAt

/** «12:00:10», or undefined when the instant can't be read. */
function clockSec(iso: string | undefined): string | undefined {
  const ms = iso ? Date.parse(iso) : NaN
  return Number.isNaN(ms) ? undefined : `${clock(ms)}:${String(new Date(ms).getSeconds()).padStart(2, '0')}`
}

/** «14:05» on the day of ref, else «20.11 14:05». */
function timeText(at: number, ref: number): string {
  return midnight(at) === midnight(ref) ? clock(at) : t('date.shortTime', { date: dateShort(at), time: clock(at) })
}

/** The args as the copy texts name them; an arg that can't be read is left
 *  out, so its {placeholder} shows instead of a wrong value. */
function argsOf(e: Entry, at: number): Record<string, string> {
  const a = e.Args ?? {}
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(a)) if (v != null) out[k] = v
  const time = (iso: string | undefined, ref: number) => {
    const ms = iso ? Date.parse(iso) : NaN
    return Number.isNaN(ms) ? undefined : timeText(ms, ref)
  }
  const put = (k: string, v: string | undefined) => { if (v === undefined) delete out[k]; else out[k] = v }
  put('time', time(a.at, at))
  put('start', time(a.start, at))
  // The end against the start: «з 23:00 до 21.11 07:30».
  const start = a.start ? Date.parse(a.start) : NaN
  put('end', time(a.end, Number.isNaN(start) ? at : start))
  const dms = a.duration_ms != null && a.duration_ms !== '' ? Number(a.duration_ms) : NaN
  put('duration', Number.isFinite(dms) ? hmLong(dms) : undefined)
  // A short link drop: the times to the second, the duration in seconds under a minute.
  put('from', clockSec(a.from))
  put('to', clockSec(a.to))
  const secs = a.seconds != null && a.seconds !== '' ? Number(a.seconds) : NaN
  if (Number.isFinite(secs)) put('duration', secs < 60 ? t('duration.short.s', { s: Math.max(0, Math.floor(secs)) }) : hmLong(secs * 1000))
  return out
}

/** «сьогодні», «вчора», else «пт 20.11». */
function dayWord(at: number, now: number): string {
  const ago = daysBefore(at, now)
  return ago === 0 ? t('date.today', { time: '' }).trim() : ago === 1 ? t('date.yesterday', { time: '' }).trim() : dayShort(at)
}

function rowOf(e: Entry, seenAt: number, now: number): ErrorRow {
  const at = Date.parse(e.At)
  const level = levelOf(e)
  const args = argsOf(e, at)
  return {
    id: e.ID,
    when: clock(at),
    whenSub: dayWord(at, now),
    level,
    levelLabel: t(`errors.level.${level}`),
    icon: ICON[e.Key] ?? GROUP_ICON[e.Group] ?? 'info',
    what: t(e.Key, args),
    todo: e.TodoKey ? t(e.TodoKey, args) : '',
    isNew: isNew(e, seenAt),
  }
}

/** errorsModel: the tab at `now` for the entries (newest first) under a
 *  filter. seenAt is when the user last opened the tab before this visit
 *  (0 = never); observedSince when the app began recording (null = not yet). */
export function errorsModel(entries: Entry[], filter: ErrorFilter, seenAt: number, observedSince: number | null, now: number, page?: PageInfo): ErrorsModel {
  // In v1 the station sends no error log over LAN (spec §9 item 3): said once
  // across the tab, and again on the «помилки» filter it switches off.
  const strip = t('errors.empty.noLogSource', { source: t('source.lan') })
  // The counts are the whole period's when the server sent them; else what is loaded.
  const COUNT_KEY: Record<ErrorFilter, Exclude<keyof JournalCounts, 'New'>> = { all: 'All', error: 'Errors', warning: 'Warnings', connection: 'Connection' }
  const totalOf = (key: ErrorFilter) => page ? page.counts[COUNT_KEY[key]] : entries.filter(e => matches(e, key)).length
  const filters = (Object.keys(FILTER_LABEL) as ErrorFilter[]).map(key => {
    const off = key === 'error'
    return { key, label: t(FILTER_LABEL[key]), count: totalOf(key), disabled: off, ...(off ? { tip: strip } : {}) }
  })
  const rows = entries.filter(e => matches(e, filter)).map(e => rowOf(e, seenAt, now))
  // New: the core counted it over the whole period; else what is loaded.
  const news = t('errors.hint.new', { count: page?.counts.New ?? entries.filter(e => isNew(e, seenAt)).length })
  const m: ErrorsModel = {
    strip,
    filters,
    hint: observedSince == null ? news : t('errors.hint', { new: news, date: dayShort(observedSince) }),
    rows,
    more: page?.more ?? false,
    showing: '',
  }
  if (m.more) m.showing = t('errors.showing', { shown: rows.length, total: totalOf(filter) })
  if (rows.length === 0 && !m.more) m.empty = t(filter === 'all' ? 'errors.empty.all' : 'errors.empty.filtered')
  return m
}

/** The At of the newest entry (as sent, RFC 3339), what MarkJournalSeen
 *  marks seen up to: a warning logged after the list was read stays new.
 *  null for no entries. */
export function seenUpTo(entries: Entry[]): string | null {
  let best: string | null = null
  let bestMs = -Infinity
  for (const e of entries) {
    const ms = Date.parse(e.At)
    if (ms > bestMs) {
      best = e.At
      bestMs = ms
    }
  }
  return best
}

/** The journal order (core/journal): newest first; the same instant by ID,
 *  descending: the number when the code is the same, else the code. */
export function compareEntries(a: Entry, b: Entry): number {
  const d = Date.parse(b.At) - Date.parse(a.At)
  if (d !== 0) return d
  const split = (id: string): [string, number] => {
    const i = id.lastIndexOf(':')
    return i < 0 ? [id, 0] : [id.slice(0, i), Number(id.slice(i + 1)) || 0]
  }
  const [ac, an] = split(a.ID)
  const [bc, bn] = split(b.ID)
  if (ac !== bc) return ac < bc ? 1 : -1
  if (an !== bn) return bn - an
  return a.ID === b.ID ? 0 : a.ID < b.ID ? 1 : -1
}
