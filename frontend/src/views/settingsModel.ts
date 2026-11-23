import type { SettingsView, UpdateView } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Settings } from '../../bindings/github.com/econ11/marstek-monitor-code/core/settings/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import { pk } from '../i18n/platform'
import { getLocale, INTL, t } from '../lib/i18n'
import { stationLabel } from '../lib/station'
import type { Info } from '../ui/infoTip'

// The Settings screen (hi-fi 07-settings.html, copy §11): a menu of
// sections on the left and the rows of one section on the right. Every
// change is saved at once (SettingsService.Save), there is no «Зберегти».

export type SettingsSection = 'general' | 'notifications' | 'stations' | 'connection' | 'data' | 'about'
export const SECTIONS: readonly SettingsSection[] = ['general', 'notifications', 'stations', 'connection', 'data', 'about']
export const isSection = (v: unknown): v is SettingsSection => SECTIONS.includes(v as SettingsSection)

/** A line under a row: a state (tone), optionally with an action button. */
export type RowNote = { text: string; tone?: 'good' | 'warn' | 'muted'; action?: string; actionLabel?: string }

/** SettingRow is one row of a section. Beyond the brief's fields: icon (a
 *  line icon, notifications only), parent (a sub-control drawn inside its
 *  switch's row), head (drawn next to the section's intro), tip (the reason a
 *  control is disabled), href (an external link), notes (state lines), and
 *  suffix / inputMode for a text field. */
export type SettingRow = {
  id: string
  label: string
  desc?: string
  control: 'toggle' | 'seg' | 'time2' | 'text' | 'button' | 'info' | 'banner' | 'actions'
  value?: unknown
  options?: { value: string; label: string }[]
  disabled?: boolean
  group?: string
  error?: string
  icon?: string
  parent?: string
  head?: boolean
  tip?: string
  href?: string
  notes?: RowNote[]
  suffix?: string
  /** A text field's placeholder (the name field: the station's label). */
  placeholder?: string
  /** A banner's action button (control 'banner'). */
  actionLabel?: string
  /** The ⓘ next to the label (copy §15). */
  info?: Info
  /** The buttons of an 'actions' row (the update row of «Про програму»). */
  actions?: RowAction[]
  /** The one-click update's progress bar (0..1) while it runs. */
  progress?: number
}

/** A button of an 'actions' row; act names what a click does. */
export type RowAction = { act: string; label: string; primary?: boolean; disabled?: boolean }

/** What the Settings screen knows beyond the view and the snapshot:
 *  notificationsOff = Windows blocks the app's toasts (banner §11.2);
 *  update = the update state (UpdatesService); canInstall = the one-click
 *  update binding exists (Task 5); «Оновити» is enabled only with it and on
 *  Windows (SettingsView.Platform). */
export type SettingsExtras = { notificationsOff?: boolean; update?: UpdateView | null; canInstall?: boolean }

/** A catalogue text whose tail is an action's label, split at the last sep:
 *  «Надіслано. Не з'явилось? Відкрити…» at «? » keeps the «?» (keep),
 *  «Збережено: X · Показати в папці» at « · » drops the dot. */
export function splitAction(text: string, sep: string, keep = false): [string, string] {
  const i = text.lastIndexOf(sep)
  if (i < 0) return [text, '']
  return [text.slice(0, keep ? i + sep.trimEnd().length : i).trim(), text.slice(i + sep.length).trim()]
}

const ICON: Record<SettingsSection, string> = {
  general: 'sliders', notifications: 'bell', stations: 'battery', connection: 'network', data: 'database', about: 'info',
}
export const sectionIcon = (s: SettingsSection) => ICON[s]

const S_NS = 1e9, MIN_NS = 60 * S_NS
const LOWS = [10, 20, 30, 40, 50]
const NO_LINK_MIN = [5, 10, 15, 30, 60]
const POLL_S = [2, 5, 10, 30]
const RETENTION = [0, 365, 730, 1825]
const NAME_MAX = 24
export const MIN_SOC_LOW = 5, MIN_SOC_HIGH = 30
export const GITHUB_URL = 'https://github.com/econ11/marstek-monitor-code'
export const DONATE_URL = 'https://donatello.to/econ11'

/** The notification kinds behind the switches (core/notify). */
const KIND: Record<string, string> = {
  gridLost: 'grid_lost', gridBack: 'grid_back', low: 'low_charge', full: 'full', likelyOut: 'likely_dead', noConnection: 'no_link',
}

/** enabled: a missing Enabled entry means on (notify.Policy.enabled). */
function enabled(s: Settings, id: string): boolean {
  return (s.Notify.Enabled as Record<string, boolean> | null)?.[KIND[id]] ?? true
}

/** withChange returns a copy of s with one row's new value (as the row's
 *  control gives it: boolean, the option value string, [start, end], text). */
export function withChange(s: Settings, id: string, value: unknown): Settings {
  const out: Settings = { ...s, Notify: { ...s.Notify, Enabled: { ...(s.Notify.Enabled ?? {}) } } }
  const n = out.Notify
  if (id in KIND) {
    (n.Enabled as Record<string, boolean>)[KIND[id]] = value as boolean
    return out
  }
  switch (id) {
    case 'language': out.Language = value as string; break
    case 'minimizeToTray': out.MinimizeToTray = value as boolean; break
    case 'low.threshold': n.LowThreshold = Number(value); break
    case 'noConnection.after': n.NoLinkAfter = Number(value); break
    case 'quiet': n.QuietEnabled = value as boolean; break
    case 'quiet.range': [n.QuietStart, n.QuietEnd] = value as [string, string]; break
    case 'poll': out.PollInterval = Number(value); break
    case 'retention': out.RetentionDays = Number(value); break
    case 'minSOC': out.StationMinSOC = Number(value); break
    case 'updateCheck': out.UpdateCheck = value as boolean; break
    default: throw new Error(`settings: no setting ${id}`)
  }
  return out
}

/** validateName mirrors settings.ValidName (Go is authoritative): trimmed,
 *  not empty, at most 24 code points. Returns the error text or null. */
export function validateName(name: string): string | null {
  const v = name.trim()
  if (v === '') return t('addStation.name.error.empty')
  const length = Array.from(v).length
  if (length > NAME_MAX) return t('addStation.name.error.tooLong', { max: NAME_MAX, length })
  return null
}

/** validateMinSOC: a whole number 5…30 (settings.validMinSOC), else the error. */
export function validateMinSOC(text: string): string | null {
  const v = text.trim()
  return /^\d{1,2}$/.test(v) && Number(v) >= MIN_SOC_LOW && Number(v) <= MIN_SOC_HIGH ? null : t('settings.stations.minSOC.error')
}

/** validTime: «HH:MM», 00:00…23:59, two digits each (settings.validHHMM). */
export function validTime(v: string): boolean {
  const m = /^(\d\d):(\d\d)$/.exec(v)
  return !!m && Number(m[1]) <= 23 && Number(m[2]) <= 59
}

const retentionLabel = (days: number) => days === 0
  ? t('settings.data.retention.option.forever')
  : t('settings.data.retention.option.years', { years: Math.round(days / 365) })

/** «21.11.2025»; en «21 Nov 2025». */
function dateLong(at: Date): string {
  if (getLocale() === 'en') return new Intl.DateTimeFormat(INTL.en, { day: 'numeric', month: 'short', year: 'numeric' }).format(at)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(at.getDate())}.${pad(at.getMonth() + 1)}.${at.getFullYear()}`
}

/** retentionChange: a shorter retention deletes history, so it is confirmed
 *  first with the period and the first day kept (monitor.RetentionBound:
 *  local midnight − days). Longer, the same or «always» need no confirm. */
export function retentionChange(from: number, to: number, now: number): { confirm: false } | { confirm: true; period: string; date: string } {
  if (to <= 0 || (from > 0 && to >= from)) return { confirm: false }
  const d = new Date(now)
  const bound = new Date(d.getFullYear(), d.getMonth(), d.getDate() - to)
  return { confirm: true, period: retentionLabel(to), date: dateLong(bound) }
}

/** pt: the text of key for the platform (its .mac variant on a Mac). */
const pt = (key: string, platform: string, args?: Record<string, string | number>) => t(pk(key, platform), args)

/** The «Надіслано…» note under the test button: its text and its action. */
export function notifyTestSent(platform: string): [string, string] {
  return splitAction(pt('settings.notify.test.sent', platform), '? ', true)
}

const toggle = (id: string, key: string, value: boolean, o: Partial<SettingRow> = {}): SettingRow =>
  ({ id, label: t(key), desc: t(`${key}.desc`), control: 'toggle', value, ...o })

function general(v: SettingsView): SettingRow[] {
  return [
    {
      id: 'language', label: t('settings.general.language'), desc: pt('settings.general.language.desc', v.Platform), control: 'seg', value: v.S.Language,
      options: [{ value: '', label: pt('settings.general.language.system', v.Platform) }, { value: 'uk', label: t('settings.general.language.uk') },
        { value: 'en', label: t('settings.general.language.en') }],
    },
    toggle('autostart', 'settings.general.autostart', v.Autostart, { label: pt('settings.general.autostart', v.Platform) }),
    // Off: × quits, so say what that costs (flows §5.2, instead of a quit dialog).
    toggle('minimizeToTray', 'settings.general.minimizeToTray', v.S.MinimizeToTray, {
      label: pt('settings.general.minimizeToTray', v.Platform), ...v.S.MinimizeToTray ? {} : { desc: t('settings.general.minimizeToTray.off.help') },
    }),
    {
      id: 'theme', label: t('settings.general.theme'), desc: t('settings.general.theme.desc'), control: 'seg', value: 'dark', disabled: true,
      options: [{ value: 'dark', label: t('settings.general.theme.dark') }],
    },
  ]
}

function notifications(v: SettingsView, off: boolean): SettingRow[] {
  const s = v.S, n = s.Notify
  const grid = t('settings.notify.group.grid'), battery = t('settings.notify.group.battery')
  const conn = t('settings.notify.group.connection'), quiet = t('settings.notify.quiet')
  const low = enabled(s, 'low'), noLink = enabled(s, 'noConnection')
  // Windows blocks the toasts: a test would show nothing (hi-fi 07-settings).
  const [blocked, open] = splitAction(pt('settings.notify.osDisabled', v.Platform), '. ', true)
  return [
    { id: 'intro', label: t('settings.notify.intro'), control: 'info' },
    off
      ? { id: 'osDisabled', label: blocked, control: 'banner', head: true, icon: 'alert', actionLabel: open }
      : { id: 'test', label: t('settings.notify.test'), desc: pt('settings.notify.test.desc', v.Platform), control: 'button', head: true, icon: 'bell' },
    toggle('gridLost', 'settings.notify.gridLost', enabled(s, 'gridLost'), { group: grid, icon: 'plug-off' }),
    toggle('gridBack', 'settings.notify.gridBack', enabled(s, 'gridBack'), { group: grid, icon: 'plug' }),
    toggle('low', 'settings.notify.low', low, { group: battery, icon: 'battery' }),
    {
      id: 'low.threshold', label: t('settings.notify.low.threshold'), control: 'seg', value: String(n.LowThreshold), parent: 'low', group: battery, info: { key: 'info.settings.lowThreshold' },
      disabled: !low, options: LOWS.map(p => ({ value: String(p), label: `${p}%` })),
    },
    toggle('full', 'settings.notify.full', enabled(s, 'full'), { group: battery, icon: 'zap' }),
    toggle('likelyOut', 'settings.notify.likelyOut', enabled(s, 'likelyOut'), { group: battery, icon: 'battery' }),
    // settings.notify.error is hidden: v1 has no station errors (spec §9).
    // One ⓘ for the switch and its «Після».
    toggle('noConnection', 'settings.notify.noConnection', noLink, { group: conn, icon: 'wifi-off', desc: t('settings.notify.noConnection.desc.v1'), info: { key: 'info.settings.noLinkAfter' } }),
    {
      id: 'noConnection.after', label: t('settings.notify.noConnection.after'), control: 'seg', value: String(n.NoLinkAfter), parent: 'noConnection',
      group: conn, disabled: !noLink, options: NO_LINK_MIN.map(m => ({ value: String(m * MIN_NS), label: t('duration.short.m', { m }) })),
    },
    toggle('quiet', 'settings.notify.quiet', n.QuietEnabled, { group: quiet, icon: 'moon', info: { key: 'info.settings.quiet' } }),
    {
      id: 'quiet.range', label: t('settings.notify.quiet.range', { start: n.QuietStart, end: n.QuietEnd }), desc: t('settings.notify.quiet.range.desc'),
      control: 'time2', value: [n.QuietStart, n.QuietEnd], parent: 'quiet', group: quiet, disabled: !n.QuietEnabled,
    },
  ]
}

const hasStation = (s: Snapshot | null) => !!s?.Station?.ID

function stations(v: SettingsView, s: Snapshot | null): SettingRow[] {
  const rows: SettingRow[] = [{ id: 'intro', label: t('settings.stations.intro'), control: 'info' }]
  if (hasStation(s)) {
    rows.push({
      id: 'name', label: t('addStation.name.label'), desc: pt('addStation.name.help', v.Platform), control: 'text',
      value: s!.Station.Name, placeholder: stationLabel(s!.Station),
    })
  } else {
    rows.push({ id: 'none', label: t('settings.stations.none'), control: 'info' })
  }
  rows.push({
    id: 'minSOC', label: t('settings.stations.minSOC'), desc: t('settings.stations.minSOC.desc'), control: 'text',
    value: String(v.S.StationMinSOC), suffix: '%',
  })
  if (hasStation(s)) rows.push({ id: 'trayShows', label: pt('settings.stations.trayShows', v.Platform, { station: stationLabel(s!.Station) }), control: 'info' })
  return rows
}

function connection(v: SettingsView, s: Snapshot | null): SettingRow[] {
  const rows: SettingRow[] = [{
    id: 'poll', label: t('settings.conn.poll'), desc: t('settings.conn.poll.desc'), control: 'seg', value: String(v.S.PollInterval), info: { key: 'info.settings.poll' },
    options: POLL_S.map(seconds => ({ value: String(seconds * S_NS), label: t('settings.conn.poll.option', { seconds }) })),
  }]
  if (hasStation(s)) rows.push({ id: 'station', label: t('settings.conn.station.row', { station: stationLabel(s!.Station), sources: t('source.lan') }), control: 'info' })
  return rows
}

function data(v: SettingsView): SettingRow[] {
  return [
    {
      id: 'retention', label: t('settings.data.retention'), control: 'seg', value: String(v.S.RetentionDays), info: { key: 'info.settings.retention' },
      options: RETENTION.map(d => ({ value: String(d), label: retentionLabel(d) })),
    },
    {
      id: 'export', label: t('settings.data.export'), desc: t('settings.data.export.desc'), control: 'button', disabled: !v.HasHistory, info: { key: 'info.settings.export' },
      tip: v.HasHistory ? undefined : t('settings.data.export.noData'),
    },
    { id: 'location', label: t('settings.data.location', { path: v.DataPath }), control: 'info' },
  ]
}

/** The update row: what the last check found, its error, and the buttons —
 *  «Оновити» (Task 5's one-click update), «Завантажити» (PageURL) and
 *  «Перевірити зараз». Before the first check there is no state line (the
 *  version row says what runs). A failed automatic check never sets Err.
 *  While «Оновити» runs (u.Phase) the row shows the phase and a progress bar;
 *  a failed update shows its reason, with «Оновити» (again) and
 *  «Завантажити» (the release page) still there. */
function updateRow(u: UpdateView | null | undefined, canInstall: boolean): SettingRow {
  const check: RowAction = { act: 'check', label: t('settings.about.update.checkNow') }
  const download: RowAction = { act: 'download', label: t('update.download') }
  const install: RowAction = { act: 'install', label: t('settings.about.update.install'), primary: true }
  if (u?.Phase) {
    return {
      id: 'update', label: phaseText(u), control: 'actions', progress: Math.min(1, Math.max(0, u.Progress)),
      actions: [{ ...install, disabled: true }, download],
    }
  }
  if (u?.Checking) return { id: 'update', label: t('settings.about.update.checking'), control: 'actions', actions: [{ ...check, disabled: true }] }
  const r: SettingRow = { id: 'update', label: '', control: 'actions', actions: [check] }
  if (u?.Available && u.Latest) {
    r.label = splitAction(t('settings.about.update.available', { version: u.Latest }), '. ', true)[0]
    r.actions = [canInstall ? install : { ...install, disabled: true }, download, check]
  } else if (u?.Latest) {
    r.label = t('settings.about.update.latest')
  }
  if (u?.Err) r.error = t(UPDATE_ERR[u.Err] ?? 'settings.about.update.failed.v1')
  return r
}

/** UpdateView.Err → its text: the simulated run, then the one-click update's
 *  failures (app/updater.go); anything else is a failed check. */
const UPDATE_ERR: Record<string, string> = {
  simulated: 'settings.about.update.simulated',
  download: 'update.failed.download',
  checksum: 'update.failed.integrity',
  backup: 'update.failed.backup',
  start: 'update.failed.start',
}

function phaseText(u: UpdateView): string {
  switch (u.Phase) {
    case 'downloading': return t('update.downloading', { version: u.Latest, percent: Math.round(Math.min(1, Math.max(0, u.Progress)) * 100) })
    case 'verifying': return t('update.verifying')
    case 'backingUp': return t('update.backingUp')
    default: return t('update.starting')
  }
}

function about(v: SettingsView, x: SettingsExtras): SettingRow[] {
  const dev = v.Version === 'dev' || !v.Version
  return [
    { id: 'version', label: dev ? t('settings.about.version.dev') : t('settings.about.version', { version: v.Version }), control: 'info' },
    toggle('updateCheck', 'settings.about.autoUpdates', v.S.UpdateCheck),
    // A dev build has no version to compare (UpdatesService checks nothing):
    // it never claims «остання версія». The installer exists on Windows only.
    dev ? { id: 'update', label: t('settings.about.update.dev'), control: 'info' }
      : updateRow(x.update, !!x.canInstall && v.Platform === 'windows'),
    { id: 'description', label: t('settings.about.description'), control: 'info' },
    { id: 'privacy', label: t('settings.about.privacy.v1'), control: 'info' },
    { id: 'github', label: t('settings.about.github'), control: 'button', href: GITHUB_URL },
    { id: 'donate', label: t('support.author'), control: 'button', href: DONATE_URL },
  ]
}

/** settingsModel: the menu and the rows of one section, in the current
 *  language. s may be null before the first snapshot. */
export function settingsModel(v: SettingsView, s: Snapshot | null, section: SettingsSection, x: SettingsExtras = {}): { menu: { key: SettingsSection; label: string; active: boolean }[]; rows: SettingRow[] } {
  const menu = SECTIONS.map(key => ({ key, label: t(`settings.section.${key}`), active: key === section }))
  const rows = section === 'general' ? general(v)
    : section === 'notifications' ? notifications(v, !!x.notificationsOff)
      : section === 'stations' ? stations(v, s)
        : section === 'connection' ? connection(v, s)
          : section === 'data' ? data(v)
            : about(v, x)
  return { menu, rows }
}

/** panelGroups: the body's panels — consecutive top-level rows of one group
 *  share one (rows without a group, too). key names the panel for a redraw
 *  in place: the group's label, or for a panel without one its place, so it
 *  stays when its rows change (Stations: «none» becomes «name» with the first
 *  snapshot). */
export function panelGroups(rows: SettingRow[]): { key: string; label?: string; rows: SettingRow[] }[] {
  const groups: { key: string; label?: string; rows: SettingRow[] }[] = []
  for (const r of rows) {
    if (r.id === 'intro' || r.head || r.parent) continue
    const last = groups[groups.length - 1]
    if (last && last.label === r.group) last.rows.push(r)
    else groups.push({ key: r.group !== undefined ? `grp:${r.group}` : `grp:#${groups.length}`, label: r.group, rows: [r] })
  }
  return groups
}
