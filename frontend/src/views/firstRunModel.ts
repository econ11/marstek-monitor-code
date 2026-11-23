import type { Device, Diagnosis } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import { hmLong } from '../lib/format'
import { getLocale, t } from '../lib/i18n'

// The first-run screen's automaton (flows §1.4 without the cloud states) and
// what each state shows (hi-fi 10-first-run.html): one page, the method tiles
// on top, only the result area below them changes. v1 is LAN-only: no
// account tile, no «sign in» action, no cloud offer (global constraints).

export type FRState = 'discovering' | 'found' | 'noneFound' | 'lanGuide' | 'manualIp' | 'probingIp' | 'ipNoAnswer' | 'adding' | 'done'
export const FR_STATES: readonly FRState[] = ['discovering', 'found', 'noneFound', 'lanGuide', 'manualIp', 'probingIp', 'ipNoAnswer', 'adding', 'done']

/** ProbeIP's error codes (app/onboard.go ProbeError). «cancelled»: the
 *  monitor took over or the app is closing; nothing to show. */
export type ProbeCode = 'format' | 'noAnswer' | 'notMarstek' | 'otherSubnet' | 'cancelled'
const PROBE_CODES: readonly ProbeCode[] = ['format', 'noAnswer', 'notMarstek', 'otherSubnet', 'cancelled']

export type FREvent = { type: 'device'; d: Device } | { type: 'scanDone'; dx: Diagnosis } | { type: 'howTo' | 'rescan' | 'enterIp' | 'back' | 'editIp' | 'retry' }
  | { type: 'submitIp'; ip: string } | { type: 'probe'; result: Device | { error: ProbeCode } }
  | { type: 'add'; d: Device } | { type: 'firstSample' } | { type: 'addTimeout' } | { type: 'addFailed' }

/** ADDING waits this long for the first sample, then DONE «чекаємо перших
 *  даних» (flows §1.4, O9): the station is stored, the monitor goes on. */
export const ADD_TIMEOUT_MS = 20_000
/** PROBING_IP: the Go side answers within 5 s once the station is free
 *  (flows §1.4), after up to 1 s of call gap; this guard only ends a call
 *  that never returns, as «no answer». */
export const PROBE_GUARD_MS = 7_000

/** validIPv4 is the field's check before anything is sent (onboard.ValidIPv4
 *  decides again): four decimal parts 0–255 without leading zeros, not
 *  0.0.0.0, broadcast, multicast, or a .0 / .255 host. */
export function validIPv4(s: string): boolean {
  const parts = s.trim().split('.')
  if (parts.length !== 4 || !parts.every(p => /^(0|[1-9]\d{0,2})$/.test(p) && Number(p) <= 255)) return false
  const [a, , , d] = parts.map(Number)
  return a !== 0 && a < 224 && d !== 0 && d !== 255
}

/** probeCode reads a ProbeIP rejection. Wails v3 rejects a bound call whose
 *  Go method returned an error with a RuntimeError: message = err.Error()
 *  (the code, ProbeError.Error), cause = the error value as JSON
 *  ({"Code": "…"}). Anything else (a lost call) counts as no answer. */
export function probeCode(err: unknown): ProbeCode {
  const cause = (err as { cause?: unknown } | null)?.cause
  const fromCause = cause && typeof cause === 'object' ? (cause as { Code?: unknown }).Code : undefined
  const code = typeof fromCause === 'string' ? fromCause : err instanceof Error ? err.message : ''
  return (PROBE_CODES as readonly string[]).includes(code) ? code as ProbeCode : 'noAnswer'
}

const IP_STATES: readonly FRState[] = ['manualIp', 'probingIp', 'ipNoAnswer']

/** toastNavigation: what a toast click does while the window is on the first
 *  run (st; null = not on it). A toast needs a running monitor, so in ADDING
 *  or DONE the station is stored: leave the first run (the header comes back),
 *  then open the toast's tab. Before that nothing can be about a station. */
export function toastNavigation(st: FRState | null): 'go' | 'leave' | 'ignore' {
  if (st === null) return 'go'
  return st === 'adding' || st === 'done' ? 'leave' : 'ignore'
}

/** firstRunStep is flows §1.4 (DISCOVERING…DONE, no CLOUD_*); found is the
 *  number of rows the current scan found; from is the state LAN_GUIDE was
 *  opened from (its «←» goes back there). The tiles (search, IP) switch the
 *  method from every state but ADDING and DONE. A scan.done that a probe or
 *  the handover cancelled says nothing about the network and is ignored. */
export function firstRunStep(st: FRState, ev: FREvent, found: number, from?: FRState): FRState {
  if (st !== 'adding' && st !== 'done') {
    if (ev.type === 'rescan') return 'discovering'
    if (ev.type === 'enterIp' && !IP_STATES.includes(st)) return 'manualIp'
  }
  switch (st) {
    case 'discovering':
      if (ev.type === 'scanDone' && !ev.dx.Cancelled) return found > 0 ? 'found' : 'noneFound'
      break
    case 'found':
      if (ev.type === 'add') return 'adding'
      if (ev.type === 'howTo') return 'lanGuide'
      break
    case 'noneFound':
      if (ev.type === 'howTo') return 'lanGuide'
      break
    case 'lanGuide':
      if (ev.type === 'back') {
        if (from === 'ipNoAnswer' || from === 'noneFound') return from
        return found > 0 ? 'found' : 'noneFound'
      }
      break
    case 'manualIp':
      if (ev.type === 'submitIp' && validIPv4(ev.ip)) return 'probingIp'
      break
    case 'probingIp':
      if (ev.type === 'probe') {
        if (!('error' in ev.result)) return 'adding'
        return ev.result.error === 'noAnswer' || ev.result.error === 'otherSubnet' ? 'ipNoAnswer' : 'manualIp'
      }
      break
    case 'ipNoAnswer':
      if (ev.type === 'retry') return 'probingIp'
      if (ev.type === 'howTo') return 'lanGuide'
      if (ev.type === 'editIp') return 'manualIp'
      if (ev.type === 'submitIp' && validIPv4(ev.ip)) return 'probingIp'
      break
    case 'adding':
      if (ev.type === 'firstSample' || ev.type === 'addTimeout') return 'done'
      if (ev.type === 'addFailed') return found > 0 ? 'found' : 'manualIp'
      break
    case 'done':
      // DONE after the 20 s timeout, then the Add call failed after all:
      // nothing was stored, so no «додано».
      if (ev.type === 'addFailed') return found > 0 ? 'found' : 'manualIp'
      break
  }
  return st
}

/** What the screen knows besides its state. */
export type FRContext = {
  /** The rows of the current scan, in the order they came. */
  devices: Device[]
  /** The last scan's diagnosis (NONE_FOUND). */
  dx?: Diagnosis | null
  /** The address typed, and why it failed (kept while the user edits). */
  ip?: string
  ipError?: ProbeCode
  /** The firewall rule: the UAC prompt is open, or adding the rule failed. */
  firewall?: 'idle' | 'pending' | 'failed'
  /** The support letter was copied. */
  copied?: boolean
  /** The last Add was refused. */
  addError?: boolean
  /** ADDING / DONE: the station, its name, the first sample (null: none yet). */
  added?: Device
  name?: string
  nameError?: string | null
  first?: { soc: number; grid: 'on' | 'off' | 'unknown'; offFor?: number } | null
  /** DONE's switches (null: not loaded); toggleError: the one that failed. */
  settings?: { autostart: boolean; minimizeToTray: boolean } | null
  toggleError?: 'autostart' | 'minimizeToTray'
  /** DONE: the tray tip is still shown (not dismissed). */
  trayHint?: boolean
}

export type FRAction = {
  key: string
  label: string
  kind: 'primary' | 'secondary' | 'ghost' | 'link'
  icon?: string
  /** Waiting for this action (a spinner, no second click). */
  busy?: boolean
  disabled?: boolean
  /** Opens outside the app. */
  external?: boolean
}

export type FRRow = { key: string; text: string; untested?: string; added?: string; action: FRAction }
export type FRDx = { key: 'noLan' | 'netPublic' | 'firewall'; title: string; body: string; action?: FRAction; note?: string; error?: string }

export type FRView = {
  welcome: string
  /** null: no tiles (ADDING, DONE). */
  tiles: { key: 'lan' | 'ip'; title: string; body: string; icon: string; checked: boolean }[] | null
  /** LAN_GUIDE has a «←». */
  back?: boolean
  head?: { icon: string; tone?: 'good'; title: string; body?: string; busy?: boolean }
  /** The result area's accessible name when it has no heading. */
  label?: string
  /** DISCOVERING: the progress line under the rows. */
  progress?: string
  rowsTitle?: string
  rows: FRRow[]
  notListed?: { text: string; action: FRAction }
  dx: FRDx[]
  steps?: string[]
  template?: { body: string; copied?: string }
  ipForm?: { label: string; help: string; placeholder: string; value: string; error?: string; extra?: string; busy?: string; submit: FRAction }
  done?: {
    name: { label: string; help: string; value: string; error?: string }
    toggles: { key: 'autostart' | 'minimizeToTray'; label: string; on: boolean; error?: string }[]
    trayHint?: { text: string; dismiss: string }
  }
  /** The result area's error (an Add that failed). */
  error?: string
  actions: FRAction[]
  /** What a screen reader hears when the state changes (live region). */
  announce: string
}

/** The README section the guide's link opens (help.localApi.link): «Як
 *  увімкнути локальний API станції» / "Enabling the station's local API" of
 *  release-repo/README.md, in the app's language (GitHub's heading slugs;
 *  rename them together). */
export function readmeURL(): string {
  const slug = getLocale() === 'en' ? 'enabling-the-stations-local-api' : 'як-увімкнути-локальний-api-станції'
  return 'https://github.com/econ11/marstek-monitor#' + encodeURIComponent(slug)
}
/** Windows Settings → Network status (onboarding.dx.netPublic + howTo). */
export const NETWORK_SETTINGS_URL = 'ms-settings:network-status'

function rowText(d: Device): string {
  const args = { model: d.Model, ip: d.IP, modelCode: d.ModelCode, firmware: `v${d.Firmware}` }
  const text = t('addStation.scan.found.row', args)
  // Without a model code in the reply's src it is the device string again.
  return d.ModelCode && d.ModelCode !== d.Model ? text : text.replace(` · ${d.ModelCode} · `, ' · ')
}

function rows(ctx: FRContext, actionable: boolean): FRRow[] {
  return ctx.devices.map(d => ({
    key: d.BLEMac,
    text: rowText(d),
    untested: d.Tested ? undefined : t('addStation.scan.found.untested'),
    added: d.Added ? t('addStation.scan.found.added') : undefined,
    action: d.Added
      ? { key: 'open', label: t('addStation.scan.found.added.open'), kind: 'secondary', disabled: !actionable }
      : { key: 'add', label: t('addStation.scan.found.action'), kind: 'secondary', icon: 'plus', disabled: !actionable },
  }))
}

function dxBlocks(ctx: FRContext): FRDx[] {
  const dx = ctx.dx
  if (!dx || dx.Cancelled) return []
  const out: FRDx[] = []
  // No LAN at all explains everything else: first.
  if (dx.NoLAN) out.push({ key: 'noLan', title: t('onboarding.dx.noLan.title'), body: t('onboarding.dx.noLan.body.v1') })
  if (dx.PublicNetwork) {
    out.push({ key: 'netPublic', title: t('onboarding.dx.netPublic.title'), body: t('onboarding.dx.netPublic.body'),
      action: { key: 'netSettings', label: t('common.action.howTo'), kind: 'link', external: true } })
  }
  if (dx.FirewallMayBlock) {
    const pending = ctx.firewall === 'pending'
    out.push({ key: 'firewall', title: t('onboarding.dx.firewall.title'), body: t('onboarding.dx.firewall.body'),
      action: { key: 'firewall', label: t('onboarding.dx.firewall.action'), kind: 'secondary', busy: pending },
      note: pending ? t('onboarding.dx.firewall.pending') : undefined,
      error: ctx.firewall === 'failed' ? t('onboarding.dx.firewall.error') : undefined })
  }
  return out
}

function gridText(f: NonNullable<FRContext['first']>): string {
  if (f.grid === 'on') return t('state.grid.on')
  if (f.grid === 'off') return t('state.grid.off', { duration: hmLong(f.offFor ?? 0) })
  return t('state.grid.unknown')
}

function ipForm(st: FRState, ctx: FRContext): NonNullable<FRView['ipForm']> {
  const ip = (ctx.ip ?? '').trim()
  const e = st === 'probingIp' ? undefined : ctx.ipError
  const error = e === 'format' ? t('addStation.ip.error.format')
    : e === 'notMarstek' ? t('addStation.ip.error.notMarstek', { ip })
    : e === 'noAnswer' || e === 'otherSubnet' ? t('addStation.ip.error.noAnswer', { ip })
    : undefined
  return {
    label: t('addStation.ip.label'), help: t('addStation.ip.help'), placeholder: t('addStation.ip.placeholder'),
    value: ctx.ip ?? '', error,
    extra: e === 'otherSubnet' ? t('onboarding.dx.otherSubnet.body') : undefined,
    busy: st === 'probingIp' ? t('addStation.ip.checking') : undefined,
    submit: { key: 'submitIp', label: t('addStation.scan.found.action'), kind: 'primary', busy: st === 'probingIp' },
  }
}

/** firstRunModel is what the screen shows in a state. */
export function firstRunModel(st: FRState, ctx: FRContext): FRView {
  const method = IP_STATES.includes(st) ? 'ip' : 'lan'
  const view: FRView = {
    welcome: t('onboarding.welcome.body'),
    tiles: st === 'adding' || st === 'done' ? null : [
      { key: 'lan', title: t('addStation.tile.lan.title'), body: t('addStation.tile.lan.body'), icon: 'wifi', checked: method === 'lan' },
      { key: 'ip', title: t('addStation.tile.ip.title'), body: t('addStation.tile.ip.body'), icon: 'keyboard', checked: method === 'ip' },
    ],
    rows: [],
    dx: [],
    actions: [],
    announce: '',
  }
  const error = ctx.addError ? t('onboarding.add.error') : undefined
  const howTo: FRAction = { key: 'howTo', label: t('common.action.howTo'), kind: 'link' }
  switch (st) {
    case 'discovering': {
      view.rowsTitle = t('addStation.scan.found.title')
      view.rows = rows(ctx, false)
      view.progress = t('addStation.scan.progress')
      view.notListed = { text: t('addStation.scan.notListed'), action: howTo }
      view.announce = view.progress
      break
    }
    case 'found': {
      view.rowsTitle = t('addStation.scan.found.title')
      view.rows = rows(ctx, true)
      view.notListed = { text: t('addStation.scan.notListed'), action: howTo }
      view.actions = [{ key: 'rescan', label: t('onboarding.none.action.rescan'), kind: 'link' }]
      view.error = error
      view.announce = [view.rowsTitle, ...view.rows.map(r => r.text)].join(' ')
      break
    }
    case 'noneFound': {
      view.head = { icon: 'search', title: t('onboarding.none.title'), body: t('onboarding.none.body') }
      view.dx = dxBlocks(ctx)
      // v1 has no account: the guide is the main action (flows §1.5).
      view.actions = [
        { key: 'howTo', label: t('onboarding.none.action.howTo'), kind: 'primary' },
        { key: 'enterIp', label: t('onboarding.none.action.ip'), kind: 'secondary', icon: 'keyboard' },
        { key: 'rescan', label: t('onboarding.none.action.rescan'), kind: 'link' },
      ]
      view.announce = [view.head.title, view.head.body, ...view.dx.map(x => x.title)].join(' ')
      break
    }
    case 'lanGuide': {
      view.back = true
      view.head = { icon: 'info', title: t('help.localApi.title'), body: t('help.localApi.body') }
      view.steps = [t('help.localApi.sameWifi'), t('help.localApi.firewall'), t('onboarding.lanGuide.after')]
      view.template = {
        body: t('onboarding.lanGuide.supportTemplate.body', { model: ctx.devices[0]?.Model ?? 'Marstek', serial: t('onboarding.lanGuide.supportTemplate.serial') }),
        copied: ctx.copied ? t('onboarding.lanGuide.supportTemplate.copied') : undefined,
      }
      view.actions = [
        { key: 'rescan', label: t('onboarding.none.action.rescan'), kind: 'primary', icon: 'search' },
        { key: 'readme', label: t('help.localApi.link'), kind: 'link', external: true },
        { key: 'copyTemplate', label: t('onboarding.lanGuide.supportTemplate.action'), kind: 'ghost' },
      ]
      view.announce = view.head.title
      break
    }
    case 'manualIp':
    case 'probingIp':
    case 'ipNoAnswer': {
      // No heading: the chosen tile above already says «IP вручну».
      view.label = t('addStation.tile.ip.title')
      view.ipForm = ipForm(st, ctx)
      if (st === 'ipNoAnswer') {
        view.actions = [
          { key: 'retry', label: t('common.action.retry'), kind: 'secondary' },
          { key: 'howTo', label: t('onboarding.none.action.howTo'), kind: 'link' },
        ]
      }
      view.error = st === 'manualIp' ? error : undefined
      view.announce = view.ipForm.busy ?? view.ipForm.error ?? ''
      break
    }
    case 'adding': {
      view.head = { icon: 'loader', title: t('onboarding.firstData.waiting'), busy: true }
      view.announce = view.head.title
      break
    }
    case 'done': {
      const d = ctx.added
      const station = ctx.name ?? d?.Model ?? ''
      const sources = t('source.lan')
      const f = ctx.first
      view.head = {
        icon: 'check', tone: 'good', title: t('onboarding.done.title', { station }),
        body: f
          ? t('onboarding.done.summary', { model: d?.Model ?? '', address: d?.IP ?? '', sources, soc: Math.round(f.soc), gridState: gridText(f) })
          : t('onboarding.done.summary.waiting', { model: d?.Model ?? '', sources }),
      }
      const s = ctx.settings
      view.done = {
        name: { label: t('addStation.name.label'), help: t('addStation.name.help'), value: station, error: ctx.nameError ?? undefined },
        toggles: s ? [
          { key: 'autostart', label: t('settings.general.autostart'), on: s.autostart },
          { key: 'minimizeToTray', label: t('settings.general.minimizeToTray'), on: s.minimizeToTray },
        ].map(x => ctx.toggleError === x.key ? { ...x, error: t('settings.save.error') } : x) as NonNullable<FRView['done']>['toggles'] : [],
        trayHint: ctx.trayHint ? { text: t('onboarding.done.trayHint'), dismiss: t('onboarding.done.trayHint.dismiss.a11y') } : undefined,
      }
      view.actions = [{ key: 'overview', label: t('onboarding.done.action'), kind: 'primary' }]
      view.announce = t('addStation.added.announce', { station })
      break
    }
  }
  return view
}
