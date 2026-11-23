import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Outage } from '../../bindings/github.com/econ11/marstek-monitor-code/core/store/models'
import { age, clock, dateShort, dayTimes, daysBefore, hm, hmLong, num, numFixed, spoken, weekdayShort } from '../lib/format'
import { onLocaleChange, t } from '../lib/i18n'
import { stationLabel } from '../lib/station'
import type { Info } from '../ui/infoTip'

// Everything the Overview tab shows, derived from one Snapshot (spec §5.0,
// §5.1; flows §2.6, §3.1). Pure: the view only turns this into DOM.
//
// The brief's types are extended with optional fields only (aria, meta, icon…).

export type RingProps = {
  key: 'soc' | 'power' | 'toEmpty' | 'toFull' | 'lastsFor' | 'noLight' | 'grid' | 'coverage' | 'split' | 'health' | 'cycles'
  title: string
  value: string
  unit?: string
  sub: string
  fill: number
  tone: 'good' | 'warn' | 'info' | 'bad' | 'muted'
  stale: boolean
  /** Full sentence for the screen reader. */
  aria?: string
  /** The ⓘ next to the title (copy §15). */
  info?: Info
}

export type RightRow = {
  label: string
  value: string
  tone?: string
  meta?: string
  icon?: string
  /** The tinted «Світло» row while the grid is off. */
  hero?: boolean
  /** Pulsing dot: data is arriving. */
  live?: boolean
  /** The row is a link to this tab. */
  view?: 'errors'
  /** The ⓘ next to the label (copy §15). */
  info?: Info
}

export type OverviewModel = {
  variant: 'skeleton' | 'live' | 'stale' | 'nolink' | 'likelyDead'
  banner?: string
  bannerLines?: string[]
  bannerKind?: 'neutral' | 'alert' | 'danger' | 'quiet'
  bannerIcon?: string
  rings: RingProps[]
  right: RightRow[]
  /** «Джерело даних»: the compact status in the app bar (owner request, Task 21). */
  source: RightRow
  /** 'on' = the amber rail is lit (no grid), 'off' = grid present, no rail. */
  outageRail: 'on' | 'off' | 'unknown'
  gapCard?: { id: number; text: string; title?: string; info?: Info }
  /** The ⓘ of the chart legend: «без світла», «немає даних», the station minimum line. */
  legend?: Info[]
  /** Station name for the header; model when unnamed. */
  station?: { name: string; model: string }
  /** Goes after «Marstek Monitor · » in the window title. */
  titleState?: string
}

// Values of the Go enums (core/link, core/detect, core/model). Kept here as
// literals so this module does not need the generated bindings at run time.
const LINK = { connecting: 'CONNECTING', live: 'LIVE', stale: 'STALE', noLink: 'NO_LINK', likelyDead: 'LIKELY_DEAD', resyncing: 'RESYNCING' }
const GRID = { on: 0, off: 1 }
const MODE = { unknown: 0, idle: 1, charging: 2, discharging: 3, cutoff: 4 }
const SIGNAL_PRESENT = 1
const DIAG_PC_OFFLINE = 'pc_offline'

const NS_PER_MS = 1e6
const MIN = 60e3, HOUR = 3600e3
/** Battery dead zone, W (global constraint: ±20 W). */
const DEADBAND_W = 20
/** A gap today longer than this replaces the outage count (copy §4.1). */
const GAPS_NOTE_AFTER = 15 * MIN

const NONE = () => t('common.value.none')
const ms = (iso: string | null | undefined) => (iso ? Date.parse(iso) : null)
const clamp01 = (v: number) => (Number.isFinite(v) ? Math.min(1, Math.max(0, v)) : 0)

type Ctx = {
  s: Snapshot
  now: number
  variant: OverviewModel['variant']
  /** Time of the last measurement, if there ever was one. */
  lastAt: number | null
  /** Battery mode: the core's while fresh, else read off the last measurement. */
  mode: number
  /** Stale-value suffix for accessible names, '' while live. */
  ariaSuffix: string
}

function variantOf(s: Snapshot): OverviewModel['variant'] {
  if (!s.Last) return s.Link === LINK.noLink || s.Link === LINK.likelyDead ? 'nolink' : 'skeleton'
  switch (s.Link) {
    case LINK.live:
    case LINK.resyncing:
      return 'live'
    case LINK.noLink:
      return 'nolink'
    case LINK.likelyDead:
      return 'likelyDead'
  }
  return 'stale'
}

function modeOf(s: Snapshot): number {
  if (s.Mode !== MODE.unknown) return s.Mode
  const w = s.Last?.BatteryW
  if (w == null) return MODE.unknown
  return w > DEADBAND_W ? MODE.charging : w < -DEADBAND_W ? MODE.discharging : MODE.idle
}

function socRing(c: Ctx): RingProps {
  const { s, variant } = c
  const ring: RingProps = { key: 'soc', title: t('overview.ring.soc.label'), value: NONE(), sub: NONE(), fill: 0, tone: 'muted', stale: variant !== 'live', info: { key: 'info.overview.soc' } }
  const soc = s.Last?.SOC
  if (soc == null) return ring
  const shown = Math.round(soc)
  ring.value = t('overview.ring.soc.value', { soc: shown })
  ring.fill = clamp01(soc / 100)
  ring.tone = soc < s.LowThreshold ? 'bad' : 'good'
  if (s.Profile.NominalWh > 0) {
    const args = { soc: shown, energy: numFixed(((soc / 100) * s.Profile.NominalWh) / 1000, 2), capacity: numFixed(s.Profile.NominalWh / 1000, 2) }
    ring.sub = t('overview.ring.soc.sub', args)
    ring.aria = t('overview.ring.soc.a11y', args) + c.ariaSuffix
  }
  if ((variant === 'nolink' || variant === 'likelyDead') && c.lastAt != null) {
    ring.unit = t('overview.ring.soc.lastAt', { time: clock(c.lastAt) })
    if (variant === 'likelyDead') ring.sub = t('overview.ring.soc.likelyOut.sub', { time: clock(c.lastAt) })
  }
  return ring
}

/** The power ⓘ with the caption's limit (idle: the model's own, the caption
 *  has none); without a limit, the text without «з {max} Вт макс.». */
const powerInfo = (max: number) => (max > 0 ? { key: 'info.overview.power', args: { max: num(max) } } : { key: 'info.overview.power.noMax' })

function powerRing(c: Ctx): RingProps {
  const { s, variant, mode } = c
  const state = mode === MODE.discharging ? 'discharging' : mode === MODE.charging ? 'charging' : 'idle'
  const max = state === 'discharging' ? s.Profile.MaxDischargeW : state === 'charging' ? s.Profile.MaxChargeW : 0
  const ring: RingProps = {
    key: 'power', title: t('overview.ring.power.label'), value: NONE(), unit: t('unit.w'), fill: 0, stale: variant !== 'live',
    tone: state === 'discharging' ? 'warn' : state === 'charging' ? 'good' : 'info',
    sub: state === 'idle' ? NONE() : max > 0 ? t(`overview.ring.power.sub.${state}`, { max: num(max) }) : NONE(),
    info: powerInfo(state === 'idle' ? Math.max(s.Profile.MaxChargeW, s.Profile.MaxDischargeW) : max),
  }
  const w = s.Last?.BatteryW
  // Without a link the last power is not shown at all (flows §2.6): only «—».
  if (w == null || mode === MODE.unknown || variant === 'nolink' || variant === 'likelyDead') return ring
  const abs = state === 'idle' ? 0 : Math.round(Math.abs(w))
  ring.value = num(abs)
  ring.unit = t(`overview.ring.power.unit.${state}`)
  if (state === 'idle') ring.sub = t('overview.ring.power.sub.idle')
  ring.fill = variant === 'live' ? clamp01(s.Fills.Power) : max > 0 ? clamp01(abs / max) : 0
  ring.aria = t('overview.ring.power.a11y', { state, watts: num(abs), max: num(max) }) + c.ariaSuffix
  return ring
}

function forecastRing(c: Ctx): RingProps {
  const { s, now, variant, mode } = c
  const key = mode === MODE.discharging ? 'toEmpty' : mode === MODE.charging ? 'toFull' : 'lastsFor'
  const label = key === 'lastsFor' ? 'lasts' : key
  const ring: RingProps = {
    key, title: t(`overview.ring.${label}.label`), value: NONE(), unit: t('overview.ring.eta.unit.hm'), sub: NONE(), fill: 0, tone: 'info', stale: variant !== 'live',
    info: key === 'toEmpty' ? { key: 'info.overview.toEmpty', args: { floor: num(s.Profile.MinSOC) } } : { key: `info.overview.${label}` },
  }
  // A forecast is never extended past fresh data: «—» (flows §2.6).
  if (variant !== 'live') return ring
  const ns = key === 'toEmpty' ? s.F.ToEmpty : key === 'toFull' ? s.F.ToFull : s.F.LastsFor
  if (ns == null) {
    // No value, no fill: a filled ring behind «—» would read as a forecast.
    if (s.F.Warming) Object.assign(ring, { sub: t('overview.ring.toEmpty.noData.sub'), info: { key: 'info.overview.calculating' } })
    else if (key === 'lastsFor') Object.assign(ring, { sub: t('overview.ring.lasts.noData.sub'), info: { key: 'info.overview.lasts.noData' } })
    return ring
  }
  ring.fill = clamp01(s.Fills.Time)
  const dur = ns / NS_PER_MS
  if (dur < 24 * HOUR) ring.value = hm(dur)
  else {
    ring.value = t('overview.ring.eta.value.h', { hours: Math.floor(dur / HOUR) })
    ring.unit = t('overview.ring.eta.unit.h')
  }
  const duration = spoken(dur)
  if (key === 'toEmpty') {
    const at = ms(s.F.EmptyAt) ?? now + dur
    const floor = num(s.Profile.MinSOC)
    ring.sub = daysBefore(at, now) === 0
      ? t('overview.ring.toEmpty.sub', { floor, time: clock(at) })
      : t('overview.ring.toEmpty.sub.otherDay', { floor, weekday: weekdayShort(at), time: clock(at) })
    ring.aria = t('overview.ring.toEmpty.a11y', { floor, duration, time: clock(at) })
  } else if (key === 'toFull') {
    const time = clock(ms(s.F.FullAt) ?? now + dur)
    ring.sub = t('overview.ring.toFull.sub', { time })
    ring.aria = t('overview.ring.toFull.a11y', { duration, time })
  } else {
    // The snapshot carries the duration, not the average outage load it was
    // computed from; the load is the usable energy over that duration.
    const soc = s.Last?.SOC
    if (soc != null && dur > 0) {
      const usableWh = (Math.max(0, soc - s.Profile.MinSOC) / 100) * s.Profile.NominalWh
      const watts = num(Math.round(usableWh / (dur / HOUR)))
      ring.sub = t('overview.ring.lasts.sub', { watts })
      ring.aria = t('overview.ring.lasts.a11y', { watts, duration })
    }
  }
  return ring
}

function noLightRing(c: Ctx): RingProps {
  const { s, variant } = c
  const ring: RingProps = {
    key: 'noLight', title: t('overview.ring.offToday.label'), value: NONE(), unit: t('overview.ring.offToday.unit'), sub: NONE(), fill: 0, tone: 'warn', stale: variant !== 'live',
    info: { key: 'info.overview.offToday' },
  }
  // Nothing was ever measured: «0:00 · без відключень» would be an invented fact.
  if (c.lastAt == null) return ring
  const dur = s.NoLightToday / NS_PER_MS
  // Time in a data gap is unknown (core/history): it is not counted, so with
  // gaps today the count of outages could be wrong and is not claimed.
  const gaps = (s.GapsToday ?? 0) / NS_PER_MS > GAPS_NOTE_AFTER
  // Without fresh data, or with gaps today, the time is only a lower bound.
  const atLeast = variant !== 'live' || s.NoLightAtLeast || gaps
  const value = hm(dur)
  ring.value = atLeast ? t('overview.ring.offToday.value.atLeast', { value }) : value
  // Gaps replace the ring's own ⓘ (copy §15); any other «≥» keeps it.
  if (gaps) ring.info = { key: 'info.overview.offToday.gaps' }
  ring.sub = gaps ? t('overview.ring.offToday.sub.gaps')
    : s.OutagesToday === 0 ? t('overview.ring.offToday.sub.none')
      : t('overview.ring.offToday.sub', { count: s.OutagesToday })
  ring.fill = clamp01(s.Fills.NoLight)
  const duration = atLeast ? t('duration.atLeast', { duration: spoken(dur) }) : spoken(dur)
  ring.aria = t('overview.ring.offToday.a11y', { duration, count: s.OutagesToday })
  return ring
}

const openOutage = (s: Snapshot): Outage | null => (s.Current && s.Current.End == null ? s.Current : null)

function gridRow(c: Ctx): RightRow {
  const { s, now, variant, lastAt } = c
  // «невідомо», «далі невідомо» have their own ⓘ.
  const row: RightRow = { label: t('overview.side.grid.label'), value: t('overview.side.grid.unknown'), icon: 'plug', info: { key: 'info.overview.grid.unknown' } }
  const known = { key: 'info.overview.grid' }
  const open = openOutage(s)
  const since = open ? Date.parse(open.Start) : 0
  const offKey = open?.Partial || s.CurrentBounds?.StartApprox ? 'overview.side.grid.offApprox' : 'overview.side.grid.off'
  // A start that lay in a gap: the window it lies in (owner 2026-11-23), its
  // duration from the first observation, so a lower bound.
  const gapFrom = open ? ms(s.CurrentBounds?.StartFrom) : null
  const between = (upTo: number) => {
    const [from, to] = dayTimes([gapFrom!, since], now)
    return t('overview.side.grid.offBetween', { from, to, duration: hmLong(upTo - since) })
  }
  if (variant === 'live') {
    // An unconfirmed change (the core's 30 s check): shown neutrally, without
    // the confirmed state's tint; the ⓘ stays that of the confirmed state.
    if (s.GridPendingSince != null && (s.GridPending === GRID.on || s.GridPending === GRID.off)) {
      const info = s.Grid === GRID.off || s.Grid === GRID.on ? known : row.info
      return { ...row, value: t(s.Grid !== GRID.on && s.Grid !== GRID.off ? 'overview.side.grid.pendingUnknown' : s.GridPending === GRID.on ? 'overview.side.grid.pendingOn' : 'overview.side.grid.pendingOff'), info }
    }
    if (s.Grid === GRID.off) {
      // The rail is lit, so the row says «немає» even before the outage record is there.
      const value = gapFrom != null ? between(now) : open ? t(offKey, { time: clock(since), duration: hmLong(now - since) }) : t('overview.side.grid.offNoStart')
      return { ...row, value, icon: 'plug-off', tone: 'warn', hero: true, info: known }
    }
    if (s.Grid === GRID.on) {
      row.tone = 'good'
      row.info = known
      row.value = t('overview.side.grid.on')
      const back = ms(s.Previous?.End)
      if (back != null && daysBefore(back, now) === 0) row.value = t('overview.side.grid.onSince', { time: clock(back), duration: hmLong(now - back) })
    }
    return row
  }
  if (lastAt == null) return row
  // Not current: the last known state, frozen at the last measurement.
  const lastOff = gapFrom != null ? between(lastAt) : open ? t('overview.side.grid.offAtLeast', { time: clock(since), duration: hmLong(lastAt - since) }) : ''
  if (variant === 'likelyDead' && open) {
    return { ...row, value: lastOff, meta: t('overview.side.grid.frozen', { time: clock(lastAt) }), icon: 'plug-off', tone: 'warn', hero: true, info: known }
  }
  if (open) row.value = t('overview.side.grid.unknownLast', { lastState: lastOff })
  else if (s.Last?.Grid === SIGNAL_PRESENT) row.value = t('overview.side.grid.unknownLast', { lastState: t('overview.side.grid.on') })
  return row
}

function prevOutageRow(c: Ctx): RightRow {
  const p = c.s.Previous
  const row: RightRow = { label: t('overview.side.prevOutage.label'), value: t('overview.side.prevOutage.none'), icon: 'history', info: { key: 'info.overview.prevOutage' } }
  const start = ms(p?.Start), end = ms(p?.End)
  if (!p || start == null || end == null) return row
  // Only what was observed (core/history): time in a gap is not counted, so
  // the duration is then a lower bound; a start right after a gap is approximate.
  const b = c.s.PreviousBounds
  const known = b ? hmLong(b.Known / NS_PER_MS) : hmLong(end - start)
  const duration = b?.AtLeast ? t('duration.atLeast.short', { duration: known }) : known
  // An end found only after a gap: the grid was back by then, not necessarily then.
  const notAfter = b?.EndApprox ? ms(b.NotAfter) : null
  // A start that lay in a gap: the window it lies in (owner 2026-11-23).
  const gapFrom = ms(b?.StartFrom)
  if (gapFrom != null) {
    // No {date}: each day is named once, on the first time that falls on it.
    const [f, s, e] = dayTimes([gapFrom, start, notAfter ?? end], c.now)
    const endText = notAfter != null ? t('outages.table.end.notAfter', { time: e }) : e
    row.value = t('overview.side.prevOutage.value.between', { from: f, to: s, end: endText, duration })
  } else {
    const ago = daysBefore(start, c.now)
    // date.today / date.yesterday are «сьогодні {time}» templates; the day word alone is what the row needs.
    const date = ago === 0 ? t('date.today', { time: '' }).trim() : ago === 1 ? t('date.yesterday', { time: '' }).trim() : dateShort(start)
    const from = b?.StartApprox ? t('time.approx', { time: clock(start) }) : clock(start)
    const to = notAfter != null ? t('outages.table.end.notAfter', { time: clock(notAfter) }) : clock(end)
    row.value = t('overview.side.prevOutage.value', { date, start: from, end: to, duration })
  }
  const died = ms(p.DiedAt)
  if (died != null) row.meta = `${t('state.stationOut')} ${t('date.at', { time: clock(died) })}`
  else if (p.SOCStart != null && p.SOCEnd != null) row.meta = `${Math.round(p.SOCStart)}% → ${Math.round(p.SOCEnd)}%`
  return row
}

/** «Помилки»: journal warnings since the user last opened the tab (Task 10 NewEvents). */
function errorsRow(c: Ctx): RightRow {
  const count = c.s.NewEvents ?? 0
  const row: RightRow = { label: t('overview.side.errors.label'), value: t('overview.side.errors.value', { count }), view: 'errors', info: { key: 'info.errors.new' } }
  return count > 0 ? { ...row, icon: 'alert', tone: 'bad' } : { ...row, icon: 'check-circle', tone: 'good' }
}

function sourceRow(c: Ctx): RightRow {
  const { s, now, variant, lastAt } = c
  const row: RightRow = {
    label: t('overview.side.source.label'), value: t('overview.side.source.connecting', { source: t('source.lan') }), icon: 'loader',
    info: { key: 'info.overview.source' },
  }
  if (lastAt == null) {
    return variant === 'nolink' ? { ...row, value: t('source.none'), icon: 'wifi-off', tone: 'bad' } : row
  }
  const fresh = t('overview.side.source.value', { source: t('source.lan'), age: age(now - lastAt) })
  switch (variant) {
    case 'live':
      return { ...row, value: fresh, icon: 'wifi', tone: 'good', live: true }
    case 'stale':
      return s.Link === LINK.stale ? { ...row, value: fresh, icon: 'clock' } : { ...row, meta: t('overview.side.source.lastAt', { time: clock(lastAt) }) }
  }
  return { ...row, value: t('source.none'), meta: t('overview.side.source.lastAt', { time: clock(lastAt) }), icon: 'wifi-off', tone: 'bad' }
}

function banner(c: Ctx): Pick<OverviewModel, 'banner' | 'bannerLines' | 'bannerKind' | 'bannerIcon'> {
  const { s, now, variant, lastAt } = c
  const time = lastAt != null ? clock(lastAt) : NONE()
  switch (variant) {
    case 'stale':
      if (s.Link !== LINK.stale) return { banner: t('offline.reconnecting'), bannerKind: 'quiet', bannerIcon: 'loader' }
      return {
        banner: lastAt != null && daysBefore(lastAt, now) > 0
          ? t('stale.banner.otherDay', { date: dateShort(lastAt), time })
          : t('stale.banner.today', { time }),
        bannerLines: [t('stale.banner.reason.reconnecting')], bannerKind: 'neutral', bannerIcon: 'clock',
      }
    case 'nolink': {
      const lines = [
        s.Diagnosis === DIAG_PC_OFFLINE ? t('link.dx.pcOffline.v1')
          : lastAt != null ? t('link.dx.lanSilent.v1', { time }) : t('offline.reconnecting'),
      ]
      // The router hint only when the last known state is «без світла».
      if (openOutage(s)) lines.push(t('offline.none.hint'))
      return { banner: t('offline.none.title'), bannerLines: lines, bannerKind: 'alert', bannerIcon: 'wifi-off' }
    }
    case 'likelyDead': {
      const open = openOutage(s)
      const soc = s.Last?.SOC
      const body = open && soc != null
        ? t('link.likelyOut.body', { time, soc: Math.round(soc), gridOffSince: clock(Date.parse(open.Start)) })
        : t('overview.side.source.lastAt', { time })
      return { banner: t('link.likelyOut.title'), bannerLines: [body], bannerKind: 'danger', bannerIcon: 'battery' }
    }
  }
  return {}
}

function gapCard(c: Ctx): OverviewModel['gapCard'] {
  const { s, now } = c
  const g = s.GapCard
  if (!g) return undefined
  const start = Date.parse(g.Start), end = ms(g.End) ?? now
  const from = daysBefore(start, now) === 0 ? clock(start) : `${dateShort(start)} ${clock(start)}`
  // v1 has no cloud to fill the gap: either an outage that began before the
  // gap ended is confirmed still on, or what happened in between is unknown.
  // An outage open in the DB is not enough: right after the gap the grid is
  // unknown until confirmed (it may have come back in the night).
  const open = openOutage(s)
  const text = open && Date.parse(open.Start) <= end && s.Grid === GRID.off
    ? t('gapCard.stillOff', { duration: hmLong(now - Date.parse(open.Start)) })
    : t('gapCard.unknown.v1')
  return { id: g.ID, title: t('gapCard.title', { start: from, end: clock(end) }), text, info: { key: 'info.overview.gapCard' } }
}

function titleState(c: Ctx, m: OverviewModel): string | undefined {
  if (c.variant === 'live') {
    const open = openOutage(c.s)
    return m.outageRail === 'on' && open
      ? t('tray.tooltip.line3.off.noEta', { duration: hmLong(c.now - Date.parse(open.Start)) })
      : undefined
  }
  return c.variant === 'skeleton' || m.bannerKind === 'quiet' ? undefined : m.banner
}

/** overviewModel turns a snapshot into what the Overview tab shows at `now` (epoch ms). */
export function overviewModel(s: Snapshot, now: number): OverviewModel {
  const variant = variantOf(s)
  const lastAt = ms(s.Last?.At)
  const stale = variant !== 'live' && lastAt != null
  const c: Ctx = {
    s, now, variant, lastAt, mode: modeOf(s),
    ariaSuffix: stale ? ' ' + t('stale.ring.a11ySuffix', { time: clock(lastAt) }) : '',
  }
  const station = s.Station?.ID ? { name: stationLabel(s.Station), model: s.Station.Name ? s.Station.Model : '' } : undefined

  if (variant === 'skeleton') {
    return { variant, rings: [], right: [], source: sourceRow(c), outageRail: 'unknown', station }
  }
  const m: OverviewModel = {
    variant,
    ...banner(c),
    rings: [socRing(c), powerRing(c), forecastRing(c), noLightRing(c)],
    right: [gridRow(c), prevOutageRow(c), errorsRow(c)],
    source: sourceRow(c),
    outageRail: variant !== 'live' ? 'unknown' : s.Grid === GRID.off ? 'on' : s.Grid === GRID.on ? 'off' : 'unknown',
    gapCard: gapCard(c),
    // The station minimum as the chart draws its line (Snapshot.Profile.MinSOC, the Settings value applied).
    legend: [{ key: 'info.overview.chart.outage' }, { key: 'info.overview.chart.noData' }, { key: 'info.overview.chart.floor', args: { floor: num(s.Profile.MinSOC) } }],
    station,
  }
  m.titleState = titleState(c, m)
  return m
}

/** chartNote is the note under the charge chart when it has nothing to draw:
 *  no data yet, or a loaded series without a single measurement. measured is
 *  the number of measurements in the series, null until it has loaded. */
export function chartNote(variant: OverviewModel['variant'], measured: number | null): string | undefined {
  return variant === 'skeleton' || measured === 0 ? t('overview.chart.empty') : undefined
}

export type OverviewFeed = ReturnType<typeof overviewFeed>

/** overviewFeed keeps the newest snapshot and its model, computed once per
 *  snapshot and once per tick() for every listener (the shell and the
 *  Overview tab), and again after a language switch (before the listeners
 *  added later redraw in the new language). Snapshots can arrive out of order
 *  (the first bound call racing an event); one with a lower Seq than the kept
 *  one is dropped: update() then returns false. */
export function overviewFeed(now: () => number = Date.now, model: typeof overviewModel = overviewModel) {
  let last: { snapshot: Snapshot; model: OverviewModel } | null = null
  const listeners = new Set<(m: OverviewModel, s: Snapshot) => void>()
  const publish = (s: Snapshot) => {
    last = { snapshot: s, model: model(s, now()) }
    for (const fn of Array.from(listeners)) fn(last.model, s)
  }
  onLocaleChange(() => { if (last) publish(last.snapshot) })
  return {
    update(s: Snapshot): boolean {
      if (last && s.Seq < last.snapshot.Seq) return false
      publish(s)
      return true
    },
    /** tick recomputes the model at now (ages and durations move on). */
    tick: () => { if (last) publish(last.snapshot) },
    /** on calls fn with every new model; returns the unsubscribe. */
    on(fn: (m: OverviewModel, s: Snapshot) => void): () => void {
      listeners.add(fn)
      return () => { listeners.delete(fn) }
    },
    current: () => last,
  }
}
