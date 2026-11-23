import type { Bucket, Report, View } from '../../bindings/github.com/econ11/marstek-monitor-code/core/history/models'
import type { Gap } from '../../bindings/github.com/econ11/marstek-monitor-code/core/store/models'
import { clock, dateShort, dayTimes, dayShort, daysBefore, hmLong, midnight, num, weekdayShort } from '../lib/format'
import { getLocale, INTL, t } from '../lib/i18n'
import { ymd } from '../ui/calendar'
import type { BarItem } from '../ui/dayBars'
import type { Info } from '../ui/infoTip'
import { rangeLabel, type DayRange } from '../ui/period'
import { escapeHTML as esc } from '../ui/tooltip'
import type { RingProps } from './overviewModel'

// Everything the «Відключення» tab shows, derived from one history.Report
// (Task 8; spec §5.2, copy §5, §13.5). Pure: the view only turns this into DOM.
// The brief's types are extended with optional fields only (endDay, endKind…).

export type { BarItem } from '../ui/dayBars'

export type OutageRow = {
  id: number
  /** «вчора 22:00», «19.11 ≈ 21:41». */
  start: string
  /** The same in two lines, as the table shows it: «≈ 21:41» over «19.11». */
  startTime: string
  startDay: string
  startTone?: 'muted'
  end: string
  endTone?: 'muted'
  /** The end's day when it is not the start's: «сьогодні», «вчора», «19.11». */
  endDay?: string
  /** An ongoing row shows a pill instead of the end. */
  endKind?: 'ongoing' | 'linkLost'
  duration: string
  asOf?: string
  soc: string
  /** The end of soc is the live charge of an ongoing outage. */
  socLive?: boolean
  note: string
  noteTone?: 'bad' | 'muted'
  highlighted: boolean
  /** The ⓘ of the cells (copy §15): «≈», «не пізніше» or the lost link, «≥», the note. */
  info: { start?: Info; end?: Info; duration?: Info; note?: Info }
}

/** filterDay: the key of a clicked column; highlight: an outage id;
 *  liveSoc: the charge now, only while the link is LIVE. */
export type ModelOpts = { filterDay?: string; highlight?: number; liveSoc?: number }

export type OutagesModel = {
  empty?: { title?: string; body: string; action?: 'fromStart' }
  rings: RingProps[]
  stats: { count: string; countMeta: string; average: string; longest: string; longestMeta: string }
  footnote?: string
  hint: string
  bars: {
    title: string
    /** The line beside the title: the unit, or the timeline's span. */
    sub: string
    items: BarItem[]
    /** Monitoring began inside the period: the first observed column, and its label. */
    observedSince?: { index: number; label: string }
  }
  filter?: { day: string; label: string }
  rows: OutageRow[]
  /** «Сьогодні»: the day as a 24-hour timeline instead of one column. */
  timeline?: TimelineModel
}

/** One span on the timeline: an outage (clipped to the day; an ongoing one
 *  runs to now, a frozen one to the last data) or a gap in the data. */
export type TimelineItem = {
  key: string
  kind: 'outage' | 'gap'
  start: number
  end: number
  /** The text above an outage: «06:10–08:10 · 2 год». */
  label?: string
  tip: string /*html*/
  aria: string
}

/** The local day from midnight to the next one (23 or 25 h on a DST day);
 *  observedFrom: monitoring began then, if later than midnight. */
export type TimelineModel = { start: number; end: number; now: number; observedFrom: number; items: TimelineItem[] }

const NS_PER_MS = 1e6
const MIN = 60e3, HOUR = 3600e3, DAY = 24 * HOUR
/** Gap overlap from this much on makes a duration a lower bound (core/history). */
const AT_LEAST_OVERLAP = MIN
/** A gap ending this close before an outage's start explains it (core/history BoundarySlack). */
const BOUNDARY_SLACK = 2 * MIN
/** Gaps of these kinds mean the app was not watching: «почалось, поки додаток не працював». */
const APP_OFF = new Set(['app_not_running', 'pc_sleep', 'app_update'])
const GAP_LABEL: Record<string, string> = {
  app_not_running: 'gap.label.appOff', pc_sleep: 'gap.label.pcSleep', no_link: 'gap.label.noLink',
  likely_dead: 'gap.label.likelyOut', app_update: 'gap.label.update',
}

const NONE = () => t('common.value.none')
const ms = (iso: string | null | undefined) => (iso ? Date.parse(iso) : null)
const dur = (ns: number) => ns / NS_PER_MS
const atLeast = (text: string, yes: boolean) => (yes ? t('outages.duration.atLeast', { duration: text }) : text)
const pctOr = (soc: number | null | undefined) => (soc == null ? t('outages.table.soc.unknown') : `${Math.round(soc)}%`)
const gapEnd = (g: Gap, now: number) => ms(g.End) ?? now
const overlap = (g: Gap, from: number, to: number, now: number) => Math.max(0, Math.min(gapEnd(g, now), to) - Math.max(Date.parse(g.Start), from))

/** «сьогодні», «вчора», else «19.11». */
function dayWord(at: number, now: number): string {
  const ago = daysBefore(at, now)
  return ago === 0 ? t('date.today', { time: '' }).trim() : ago === 1 ? t('date.yesterday', { time: '' }).trim() : dateShort(at)
}

/** «сьогодні 14:05», «вчора 14:05», «19.11 14:05» with the given time text. */
function when(at: number, now: number, time: string): string {
  const ago = daysBefore(at, now)
  if (ago === 0) return t('date.today', { time })
  if (ago === 1) return t('date.yesterday', { time })
  return t('date.shortTime', { date: dateShort(at), time })
}

/** «Ср» (tooltip titles start a sentence). */
const weekdayCap = (at: number) => { const w = weekdayShort(at); return w.charAt(0).toUpperCase() + w.slice(1) }
/** «18 листопада», en «18 November». */
const dateLong = (at: number) => new Intl.DateTimeFormat(INTL[getLocale()], { day: 'numeric', month: 'long' }).format(new Date(at))
/** The axis date: «15.11», en «15». */
const axisDate = (at: number) => (getLocale() === 'en' ? String(new Date(at).getDate()) : dateShort(at))
/** {from}, {to}[, {end}] of a start that lay in a gap: the gap's start (core
 *  Bounds.StartFrom), the first observation and the end, each day named once
 *  relative to today (dayTimes). */
function gapWindow(from: string, start: number, now: number, end?: number): Record<string, string> {
  if (end == null) { const [f, s] = dayTimes([Date.parse(from), start], now); return { from: f, to: s } }
  const [f, s, e] = dayTimes([Date.parse(from), start, end], now)
  return { from: f, to: s, end: e }
}

function rowOf(v: View, r: Report, now: number, opts: ModelOpts): OutageRow {
  const o = v.Outage, b = v.Bounds
  const start = Date.parse(o.Start)
  const asOf = Date.parse(b.AsOf)
  // A start that lay in a gap: the window it lies in (owner 2026-11-23), else «≈».
  const startTime = b.StartFrom != null ? t('outages.table.start.between', gapWindow(b.StartFrom, start, now))
    : b.StartApprox ? t('outages.table.start.approx', { time: clock(start) }) : clock(start)
  const row: OutageRow = {
    id: o.ID,
    start: b.StartFrom != null ? startTime : when(start, now, startTime),
    startTime,
    // A window names its days itself (dayTimes).
    startDay: b.StartFrom != null ? '' : dayWord(start, now),
    startTone: b.StartApprox ? 'muted' : undefined,
    end: '',
    duration: atLeast(hmLong(dur(b.Known)), b.AtLeast),
    soc: `${pctOr(o.SOCStart)} → ${pctOr(o.SOCEnd)}`,
    note: '',
    highlighted: opts.highlight != null && o.ID === opts.highlight,
    info: {},
  }
  if (b.StartApprox) row.info.start = { key: 'info.common.approx' }
  if (b.AtLeast) row.info.duration = { key: 'info.common.atLeast' }
  const end = ms(o.End)
  if (end == null) {
    if (b.Frozen) {
      Object.assign(row, { end: t('outages.table.end.linkLost'), endKind: 'linkLost', endTone: 'muted' })
      row.info.end = { key: 'info.outages.linkLost' }
    }
    else {
      Object.assign(row, { end: t('outages.table.end.ongoing'), endKind: 'ongoing' })
      // Live and going on: the charge now, from the live snapshot (unknown otherwise).
      if (opts.liveSoc != null) Object.assign(row, { soc: `${pctOr(o.SOCStart)} → ${pctOr(opts.liveSoc)}`, socLive: true })
    }
  } else {
    // An end found only after a gap: the grid was back by then, not necessarily then.
    const shown = b.EndApprox ? ms(b.NotAfter) ?? end : end
    row.end = b.EndApprox ? t('outages.table.end.notAfter', { time: clock(shown) }) : clock(shown)
    if (b.EndApprox) {
      row.endTone = 'muted'
      row.info.end = { key: 'info.common.notAfter' }
    }
    if (midnight(shown) !== midnight(start)) row.endDay = dayWord(shown, now)
  }
  if (b.Frozen) row.asOf = t('outages.table.asOf', { time: clock(asOf) })

  // One note, the most important: ran out › started while the app was off › gap › frozen.
  const died = ms(o.DiedAt)
  const gaps = r.GapSpans ?? []
  const afterAppOff = b.StartApprox && gaps.some(g => {
    const e = gapEnd(g, now)
    return APP_OFF.has(g.Reason) && e <= start && start - e <= BOUNDARY_SLACK
  })
  const gapInside = b.StartApprox || b.EndApprox || (asOf - start) - dur(b.Known) >= AT_LEAST_OVERLAP
  const note = (key: string, info: string) => { row.note = t(key, { time: died != null ? clock(died) : '' }); row.info.note = { key: info } }
  if (died != null) { note('outages.note.stationOut', 'info.outages.stationOut'); row.noteTone = 'bad' }
  else if (afterAppOff) note('outages.note.startedWhileOff', 'info.outages.startedWhileOff')
  else if (gapInside) note('outages.note.gap', 'info.outages.gapRow')
  else if (b.Frozen) Object.assign(row, { note: t('outages.note.frozen'), noteTone: 'muted' })
  return row
}

function gridRing(r: Report): RingProps {
  const ring: RingProps = { key: 'grid', title: t('outages.ring.grid.label'), value: NONE(), sub: NONE(), fill: 0, tone: 'good', stale: false, info: { key: 'info.outages.grid' } }
  if (r.GridPct == null) return ring
  // Any outage keeps the ring below 100%, however small (or hidden in a gap).
  const pct = r.Off > 0 || r.Count > 0 ? Math.min(99, Math.round(r.GridPct)) : Math.round(r.GridPct)
  ring.value = t('outages.ring.grid.value', { percent: pct })
  ring.fill = Math.min(1, Math.max(0, r.GridPct / 100))
  ring.sub = r.Count > 0
    ? t('outages.ring.grid.sub', { duration: atLeast(hmLong(dur(r.Off)), r.OffAtLeast) })
    : t('outages.ring.grid.sub.observed', { duration: hmLong(dur(r.Observed)) })
  return ring
}

function coverageRing(r: Report): RingProps {
  const ring: RingProps = {
    key: 'coverage', title: t('outages.ring.coverage.label'), value: NONE(), sub: t('outages.ring.coverage.noData'), fill: 0, tone: 'muted', stale: false,
    info: { key: 'info.outages.coverage' },
  }
  if (r.Coverage == null) {
    // Only an ongoing outage so far: there was one, it just has not ended.
    if ((r.Rows ?? []).some(v => v.Outage.End == null)) ring.sub = t('outages.ring.coverage.ongoing')
    return ring
  }
  const percent = Math.round(r.Coverage)
  ring.value = t(r.CoverageApprox ? 'outages.ring.coverage.value.approx' : 'outages.ring.coverage.value', { percent })
  ring.sub = t('outages.ring.coverage.sub', { count: r.RanOut })
  ring.fill = Math.min(1, Math.max(0, r.Coverage / 100))
  ring.tone = 'good'
  // The percent once, with «≈» when some ends lay in a gap.
  ring.aria = `${ring.title}. ${t('outages.ring.coverage.a11y.likely', { percent: ring.value, count: r.RanOut })}`
  return ring
}

function stats(r: Report): OutagesModel['stats'] {
  const n = r.Rows?.length ?? 0
  const longestAt = ms(r.LongestAt)
  // Nothing observed in the period: a count of 0 would be an invented fact.
  const seen = r.ObservedSince != null && r.Observed > 0
  // The rate is per observed day: time in gaps and before monitoring is not counted.
  const days = Math.max(1, dur(r.Observed) / DAY)
  return {
    count: seen ? num(r.Count) : NONE(),
    countMeta: seen && r.Count > 0 ? t('outages.stat.count.meta', { rate: num(r.Count / days, 1) }) : '',
    average: n > 0 ? hmLong(dur(r.Average)) : NONE(),
    longest: n > 0 ? hmLong(dur(r.Longest)) : NONE(),
    longestMeta: n > 0 && longestAt != null ? t('outages.stat.longest.meta', { date: dayShort(longestAt), time: clock(longestAt) }) : '',
  }
}

/** The days of the period as «15.11 – 21.11» (To is exclusive). */
function periodLabel(r: Report): string {
  return rangeLabel({ from: ymd(Date.parse(r.Period.From)), to: ymd(Date.parse(r.Period.To) - 1) })
}

function hint(r: Report, now: number): string {
  const since = ms(r.ObservedSince)
  const range = periodLabel(r)
  if (since == null) return t('outages.hint', { range })
  const from = Math.max(Date.parse(r.Period.From), since), to = Date.parse(r.Period.To)
  // The same threshold as the tooltips: a gap of a few seconds is not «пропуск».
  const count = (r.GapSpans ?? []).filter(g => overlap(g, from, to, now) >= MIN).length
  return count > 0 ? t('outages.hint.gaps', { range, count }) : t('outages.hint', { range })
}

function empty(r: Report): OutagesModel['empty'] {
  const since = ms(r.ObservedSince)
  const firstUse = { title: t('outages.empty.firstUse.title'), body: t('outages.empty.firstUse.body.v1') }
  if (since == null) return firstUse
  if (Date.parse(r.Period.To) <= since) return { body: t('outages.empty.beforeHistory', { date: dateShort(since) }), action: 'fromStart' }
  if (r.Count > 0) return undefined
  // The period holds all of the monitoring, and there was nothing.
  if (Date.parse(r.Period.From) <= since) return firstUse
  if (dur(r.Gaps) >= MIN) return { body: t('outages.empty.periodWithGaps', { duration: hmLong(dur(r.Gaps)) }) }
  return { body: t('outages.empty.period') }
}

/** The axis label of a column: «пн 16.11» (en «Mon 16»), a week «16–22.11».
 *  Shared with the Energy tab, whose columns are the same days and weeks. */
export function bucketLabel(b: Pick<Bucket, 'Start' | 'End'>, weeks: boolean): string {
  const s = Date.parse(b.Start), last = Date.parse(b.End) - 1
  if (!weeks) return t('outages.chart.axis.day', { weekday: weekdayShort(s), date: axisDate(s) })
  const sameMonth = new Date(s).getMonth() === new Date(last).getMonth()
  return t('outages.chart.axis.week', { start: sameMonth ? String(new Date(s).getDate()) : dateShort(s), end: dateShort(last) })
}

/** The reason of the gap that covers most of [from, to). */
function gapReason(r: Report, from: number, to: number, now: number): string | undefined {
  let best: Gap | undefined, most = 0
  for (const g of r.GapSpans ?? []) {
    const o = overlap(g, from, to, now)
    if (o > most) { best = g; most = o }
  }
  return best && GAP_LABEL[best.Reason]
}

const line = (text: string, cls = '') => `<div class="tip__line${cls ? ' ' + cls : ''}">${esc(text)}</div>`

/** The tooltip title of a column: «Ср, 18 листопада», «… (сьогодні)», «Тиждень 16.11 – 22.11». */
export function bucketTitle(b: Pick<Bucket, 'Start' | 'End'>, weeks: boolean, now: number): string {
  const s = Date.parse(b.Start), e = Date.parse(b.End)
  return weeks
    ? t('outages.tooltip.title.week', { start: dateShort(s), end: dateShort(e - 1) })
    : t(daysBefore(s, now) === 0 ? 'outages.tooltip.title.today' : 'outages.tooltip.title', { weekdayLong: weekdayCap(s), dateLong: dateLong(s) })
}

function tipHTML(b: Bucket, r: Report, weeks: boolean, now: number): string {
  const s = Date.parse(b.Start), e = Date.parse(b.End)
  let html = `<div class="tip__title">${esc(bucketTitle(b, weeks, now))}</div>`
  if (b.NoData) {
    const reason = gapReason(r, s, e, now)
    return html + line(reason ? `${t('outages.chart.day.noData')} · ${t(reason)}` : t('outages.chart.day.noData')) +
      `<div class="tip__foot">${esc(t('info.outages.noDataDay'))}</div>`
  }
  const gaps = dur(b.Gaps) >= MIN ? line(t('outages.tooltip.gaps', { duration: hmLong(dur(b.Gaps)) })) : ''
  if (b.Count === 0) return html + gaps + `<div class="tip__foot">${esc(t('outages.tooltip.none'))}</div>`
  html += line(t(b.Ongoing ? 'outages.tooltip.count.ongoing' : 'outages.tooltip.count', { count: b.Count }))
  const percent = e > s ? Math.round(dur(b.Off) / (e - s) * 100) : 0
  html += line(t(weeks ? 'outages.tooltip.total.week' : 'outages.tooltip.total', { duration: atLeast(hmLong(dur(b.Off)), b.OffAtLeast), percent }))
  const l = b.Count > 1 ? b.Longest : null
  if (l) {
    const end = ms(l.Outage.End)
    const start = Date.parse(l.Outage.Start)
    const args = { duration: atLeast(hmLong(dur(l.Bounds.Known)), l.Bounds.AtLeast), end: end != null ? clock(end) : t('outages.table.end.ongoing') }
    html += line(l.Bounds.StartFrom != null
      ? t('outages.tooltip.longest.between', { ...args, ...gapWindow(l.Bounds.StartFrom, start, now, end ?? undefined) })
      : t('outages.tooltip.longest', { ...args, start: clock(start) }))
  }
  if (b.MinSOC != null) {
    const soc = Math.round(b.MinSOC)
    html += b.RanOut ? line(t('outages.tooltip.minSoc.out', { soc }), 't-bad') : line(t('outages.tooltip.minSoc', { soc }))
  }
  return html + gaps + `<div class="tip__foot">${esc(t('outages.tooltip.hint'))}</div>`
}

function barItem(b: Bucket, r: Report, weeks: boolean, now: number): BarItem {
  const s = Date.parse(b.Start)
  const kind: BarItem['kind'] = b.BeforeObs ? 'before' : b.NoData ? 'noData' : b.Count === 0 ? 'zero' : 'value'
  return {
    key: ymd(s),
    label: bucketLabel(b, weeks),
    hours: dur(b.Off) / HOUR,
    kind,
    died: b.RanOut,
    tip: kind === 'before' ? '' : tipHTML(b, r, weeks, now),
    atLeast: b.OffAtLeast,
    today: s <= now && now < Date.parse(b.End) + 1,
  }
}

function inBucket(v: View, b: Bucket): boolean {
  const start = Date.parse(v.Outage.Start), end = ms(v.Outage.End) ?? Infinity
  return start < Date.parse(b.End) && end > Date.parse(b.Start)
}

/** nextMidnight: the local midnight after the day that starts at dayStart. */
function nextMidnight(dayStart: number): number {
  const d = new Date(dayStart)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1).getTime()
}

function timeline(r: Report, now: number, rows: Map<number, OutageRow>): TimelineModel {
  const start = midnight(now), end = nextMidnight(start)
  const items: TimelineItem[] = []
  for (const v of r.Rows ?? []) {
    const o = v.Outage, b = v.Bounds, row = rows.get(o.ID)
    if (!row) continue
    // An end found only after a gap is drawn to «не пізніше»; without one the
    // outage runs to now, or, frozen, to the last data.
    const to = ms(o.End) != null ? (b.EndApprox ? ms(b.NotAfter) : null) ?? ms(o.End)! : b.Frozen ? Date.parse(b.AsOf) : now
    const a = Math.max(Date.parse(o.Start), start), z = Math.min(to, end)
    if (z <= a) continue
    const endText = o.End == null && !b.Frozen ? t('outages.timeline.now') : clock(to)
    const band = { start: row.startTime, end: endText, duration: row.duration }
    // A start that lay in a gap: «між {from} і {to} – {end}», not «між … і …–{end}».
    const gw = b.StartFrom != null ? { ...band, ...gapWindow(b.StartFrom, Date.parse(o.Start), now, o.End == null && !b.Frozen ? undefined : to) } : null
    const title = gw ? t('overview.chart.tooltip.band.between', gw) : t('overview.chart.tooltip.band', band)
    const tone = row.noteTone === 'bad' ? 't-bad' : row.noteTone === 'muted' ? 't-subtle' : ''
    items.push({
      key: `o${o.ID}`, kind: 'outage', start: a, end: z, label: gw ? t('outages.timeline.band.between', gw) : t('outages.timeline.band', band), aria: title,
      tip: `<div class="tip__title">${esc(title)}</div>` + (row.note ? line(row.note, tone) : ''),
    })
  }
  for (const g of r.GapSpans ?? []) {
    const a = Math.max(Date.parse(g.Start), start), z = Math.min(gapEnd(g, now), end, now)
    if (z - a < MIN) continue
    const reason = GAP_LABEL[g.Reason]
    const title = reason ? `${t('outages.chart.day.noData')} · ${t(reason)}` : t('outages.chart.day.noData')
    const text = t(g.Reason === 'app_not_running' ? 'gap.tooltip.appOff.v1' : 'gap.tooltip', { start: clock(a), end: clock(z) })
    items.push({ key: `g${g.ID}`, kind: 'gap', start: a, end: z, aria: title, tip: `<div class="tip__title">${esc(title)}</div>` + line(text) })
  }
  // In time order: the order of the spans for Tab.
  items.sort((x, y) => x.start - y.start)
  return { start, end, now, observedFrom: Math.min(Math.max(start, ms(r.ObservedSince) ?? now), now), items }
}

/** outagesModel turns a report into what the tab shows at `now` (epoch ms).
 *  filterDay is the key of a clicked column; highlight an outage id. */
export function outagesModel(r: Report, now: number, opts: ModelOpts): OutagesModel {
  const weeks = r.Period.Weeks
  const buckets = r.Buckets ?? []
  const items = buckets.map(b => barItem(b, r, weeks, now))
  const m: OutagesModel = {
    empty: empty(r),
    rings: [gridRing(r), coverageRing(r)],
    stats: stats(r),
    hint: hint(r, now),
    bars: { title: t(weeks ? 'outages.chart.title.weeks' : 'outages.chart.title.days'), sub: t('outages.chart.axis.y'), items },
    rows: [],
  }
  if (r.Uncertain > 0) m.footnote = t('outages.stats.footnote', { count: r.Uncertain })
  const firstSeen = items.findIndex(i => i.kind !== 'before')
  const since = ms(r.ObservedSince)
  if (firstSeen > 0 && since != null) m.bars.observedSince = { index: firstSeen, label: t('chart.observedSince', { date: dayShort(since) }) }

  let views = r.Rows ?? []
  const i = opts.filterDay ? items.findIndex(it => it.key === opts.filterDay) : -1
  if (i >= 0) {
    views = views.filter(v => inBucket(v, buckets[i]))
    m.filter = { day: items[i].key, label: t('outages.chart.filterActive', { date: weeks ? items[i].label : dayShort(Date.parse(buckets[i].Start)) }) }
  }
  m.rows = views.map(v => rowOf(v, r, now, opts))
  // «Сьогодні»: one day, today, is shown by the hour.
  if (!weeks && buckets.length === 1 && daysBefore(Date.parse(buckets[0].Start), now) === 0) {
    const rows = new Map((r.Rows ?? []).map(v => [v.Outage.ID, rowOf(v, r, now, opts)]))
    m.timeline = timeline(r, now, rows)
    m.bars.title = t('outages.timeline.title')
    m.bars.sub = t('outages.timeline.sub')
  }
  return m
}

/** hasLiveRow: the report has an ongoing, not frozen row, whose end shows the live charge. */
export const hasLiveRow = (r: Report) => (r.Rows ?? []).some(v => v.Outage.End == null && !v.Bounds.Frozen)

/** rangeAround: 7 days that hold the local day of `at`, ending no later than
 *  today, to show an outage from outside the current period. */
export function rangeAround(at: number, now: number): DayRange {
  const d = new Date(at), today = midnight(now)
  const to = Math.min(new Date(d.getFullYear(), d.getMonth(), d.getDate() + 6).getTime(), today)
  const e = new Date(to)
  return { from: ymd(new Date(e.getFullYear(), e.getMonth(), e.getDate() - 6).getTime()), to: ymd(to) }
}
