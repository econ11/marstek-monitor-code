import type { Device, Diagnosis, SettingsView } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Settings } from '../../bindings/github.com/econ11/marstek-monitor-code/core/settings/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { getLocale, onLocaleChange, setLocale, t, type Locale } from '../lib/i18n'
import { icon } from '../ui/icons'
import { spinnerHTML } from '../ui/loading'
import { escapeHTML as esc } from '../ui/tooltip'
import {
  ADD_TIMEOUT_MS, NETWORK_SETTINGS_URL, PROBE_GUARD_MS, firstRunModel, firstRunStep, probeCode, readmeURL, validIPv4,
  type FRAction, type FRContext, type FREvent, type FRState, type FRView,
} from './firstRunModel'
import { validateName, withChange } from './settingsModel'

// The first-run screen (hi-fi 10-first-run.html, flows §1.3): the window
// «Додати станцію» over the whole window, one line of welcome, the method
// tiles, and the result area under them, the only part that changes. The
// search starts by itself. The main window shows it while the database has
// no station (OnboardService.HasStation); DONE → «Перейти до огляду».

export type FirstRunDeps = {
  scan(): Promise<void>
  probe(ip: string): Promise<Device>
  add(d: Device): Promise<void>
  allowFirewall(): Promise<void>
  /** Subscribes to scan.device and scan.done; returns the unsubscribe. */
  onScan(device: (d: Device) => void, done: (dx: Diagnosis) => void): () => void
  getSettings(): Promise<SettingsView>
  saveSettings(s: Settings): Promise<SettingsView>
  setAutostart(on: boolean): Promise<SettingsView>
  rename(name: string): Promise<void>
  openURL(url: string): Promise<void>
  copyText(text: string): Promise<unknown>
  /** «Перейти до огляду» (and «Відкрити» on a station already added). */
  finish(): void
  now?: () => number
  /** Called after every change of state (the scripted check, firstRunE2E.ts). */
  onState?: (st: FRState, ctx: FRContext) => void
}

/** What the screen offers a script: the same actions a click runs. */
export type FirstRunScreen = Screen & {
  act(key: string, arg?: string): void
  state(): FRState
  context(): Readonly<FRContext>
}

const APP_NAME = 'Marstek Monitor'
/** After «already being added», how long the firewall button waits busy. */
const FIREWALL_RETRY_MS = 15_000
const GRID = { on: 0, off: 1 }

/** The name Add stores (app/onboard.go stationName): the model, ≤ 24. */
const defaultName = (model: string) => Array.from(model.trim()).slice(0, 24).join('').trim() || 'Marstek'

function actionHTML(a: FRAction, attrs = ''): string {
  const cls = a.kind === 'link' ? 'link' : `btn btn--${a.kind}`
  const lead = a.kind === 'link' ? '' : a.busy ? spinnerHTML() : a.icon ? icon(a.icon) : ''
  const tail = a.external ? icon('external', 'icon--sm') : ''
  const state = (a.disabled || a.busy ? ' aria-disabled="true"' : '') + (a.busy ? ' aria-busy="true"' : '')
  return `<button type="button" class="${cls}" data-act="${a.key}" data-focus="${esc(a.key)}${attrs ? `:${esc(attrs)}` : ''}"` +
    `${attrs ? ` data-arg="${esc(attrs)}"` : ''}${state}>${lead}${esc(a.label)}${tail}</button>`
}

function headHTML(v: FRView): string {
  const h = v.head
  if (!h) return ''
  const lead = v.back
    ? `<button type="button" class="iconbtn result__back" data-act="back" data-focus="back" aria-label="${esc(t('settings.back.a11y'))}">${icon('chevron-left', 'icon--lg')}</button>`
    : `<span class="result__ic"${h.tone ? ` data-tone="${h.tone}"` : ''}>${h.busy ? spinnerHTML() : icon(h.icon)}</span>`
  return `<div class="result__h">${lead}<div><h2 id="fr-h" tabindex="-1" data-focus="heading">${esc(h.title)}</h2>${h.body ? `<p>${esc(h.body)}</p>` : ''}</div></div>`
}

function rowsHTML(v: FRView, scanning: boolean): string {
  if (v.rowsTitle === undefined) return ''
  const items = v.rows.map(r => `<li class="fr-row"><span class="fr-row__t mono">${esc(r.text)}</span>` +
    (r.untested ? `<span class="pill pill--outline">${esc(r.untested)}</span>` : '') +
    (r.added ? `<span class="pill" data-tone="good">${icon('check')}${esc(r.added)}</span>` : '') +
    actionHTML(r.action, r.key) + '</li>')
  // Nothing yet: a skeleton row while the search runs (flows §1.3).
  if (!items.length && scanning) items.push('<li class="fr-row" aria-hidden="true"><span class="skel skel-line"></span></li>')
  return `<h2 class="fr-rows__h" id="fr-h" tabindex="-1" data-focus="heading">${esc(v.rowsTitle)}</h2><ul class="fr-rows">${items.join('')}</ul>`
}

function dxHTML(v: FRView): string {
  return v.dx.map(x => `<div class="dx" data-dx="${x.key}">${icon('info')}<div><b>${esc(x.title)}</b><br>${esc(x.body)}` +
    (x.action ? `<div class="dx__act">${actionHTML(x.action)}${x.note ? `<span class="dx__note" role="status">${esc(x.note)}</span>` : ''}</div>` : '') +
    (x.error ? `<div class="dx__err" role="alert">${esc(x.error)}</div>` : '') + '</div></div>').join('')
}

function ipHTML(v: FRView): string {
  const f = v.ipForm
  if (!f) return ''
  const inv = f.error ? ' aria-invalid="true"' : ''
  return `<div class="field field--wide"><label for="fr-ip">${esc(f.label)}</label>` +
    `<div class="field__row"><input class="input mono" id="fr-ip" data-focus="ip" value="${esc(f.value)}" placeholder="${esc(f.placeholder)}" ` +
    `inputmode="decimal" spellcheck="false" autocomplete="off" maxlength="15" aria-describedby="fr-ip-help fr-ip-err"${inv}${f.busy ? ' readonly' : ''}>` +
    `${actionHTML(f.submit)}</div><small id="fr-ip-help">${esc(f.help)}</small>` +
    `<div class="field__err" id="fr-ip-err">${f.error ? esc(f.error) : ''}${f.extra ? `<div class="field__extra">${esc(f.extra)}</div>` : ''}</div>` +
    (f.busy ? `<p class="note loading">${spinnerHTML()}<span>${esc(f.busy)}</span></p>` : '') + '</div>'
}

function guideHTML(v: FRView): string {
  let html = v.steps ? `<ol class="steps">${v.steps.map(s => `<li><span>${esc(s)}</span></li>`).join('')}</ol>` : ''
  if (v.template) {
    html += `<p class="letter">${esc(v.template.body)}</p>`
    if (v.template.copied) html += `<p class="letter__done" role="status">${esc(v.template.copied)}</p>`
  }
  return html
}

/** DONE: the name field (in the result panel) and the switches with the
 *  tray tip (a panel of their own under it). */
function doneHTML(v: FRView): { name: string; extras: string } {
  const d = v.done
  if (!d) return { name: '', extras: '' }
  const n = d.name
  const name = `<div class="field"><label for="fr-name">${esc(n.label)}</label>` +
    `<input class="input" id="fr-name" data-focus="name" value="${esc(n.value)}" maxlength="40" spellcheck="false" ` +
    `aria-describedby="fr-name-help fr-name-err"${n.error ? ' aria-invalid="true"' : ''}>` +
    `<small id="fr-name-help">${esc(n.help)}</small><div class="field__err" id="fr-name-err"${n.error ? ' role="alert"' : ''}>${n.error ? esc(n.error) : ''}</div></div>`
  const rows = d.toggles.map(x => `<div class="row2"><span id="fr-t-${x.key}">${esc(x.label)}</span>` +
    (x.error ? `<span class="row2__err" role="alert">${esc(x.error)}</span>` : '') +
    `<button type="button" class="switch" role="switch" aria-checked="${x.on}" aria-labelledby="fr-t-${x.key}" data-act="toggle" data-arg="${x.key}" data-focus="toggle:${x.key}"></button></div>`)
  if (d.trayHint) {
    rows.push(`<div class="row2">${icon('info')}<span class="t-muted">${esc(d.trayHint.text)}</span>` +
      `<button type="button" class="iconbtn" style="margin-left:auto" data-act="trayHint" data-focus="trayHint" aria-label="${esc(d.trayHint.dismiss)}">${icon('x')}</button></div>`)
  }
  return { name, extras: rows.length ? `<div class="panel panel--flush">${rows.join('')}</div>` : '' }
}

export function firstRunScreen(deps: FirstRunDeps): FirstRunScreen {
  const now = deps.now ?? Date.now
  let host: HTMLElement | null = null
  let st: FRState = 'discovering'
  const ctx: FRContext = { devices: [] }
  let view: SettingsView | null = null
  let scanning = false
  let probeRun = 0
  let probeGuard = 0
  let addTimer = 0
  let lastAnnounce = ''
  let offScan = () => {}
  let offLocale = () => {}
  let sample: Snapshot | null = null
  let editingName = false
  /** The state LAN_GUIDE was opened from (its «←» goes back there). */
  let guideFrom: FRState | undefined
  let firewallRetry = 0

  function found(): number { return ctx.devices.length }

  /** step applies an event. focus: where the focus goes after the user's own
   *  action (default: the new result's heading); 'keep' for what the station
   *  or a timer did (the focus stays where it is, unless its control went). */
  function step(ev: FREvent, focus?: string) {
    const next = firstRunStep(st, ev, found(), guideFrom)
    const changed = next !== st
    if (changed && next === 'lanGuide') guideFrom = st
    st = next
    if (changed) enter(next)
    render(focus ?? (changed ? 'heading' : undefined))
    if (changed) deps.onState?.(st, ctx)
  }

  function enter(s: FRState) {
    if (s !== 'adding') clearTimeout(addTimer)
    if (s === 'done') {
      ctx.trayHint = true
      deps.getSettings().then(v => {
        view = v
        ctx.platform = v.Platform
        ctx.settings = { autostart: v.Autostart, minimizeToTray: v.S.MinimizeToTray }
        render()
      }, err => console.error('settings', err))
    }
  }

  function startScan() {
    // A scan still running goes on (the service starts no second one): its
    // rows stay. A new one starts empty.
    if (!scanning) {
      ctx.devices = []
      ctx.dx = null
    }
    ctx.addError = false
    scanning = true
    deps.scan().catch(err => {
      console.error('scan', err)
      scanDone({ PublicNetwork: false, FirewallMayBlock: false, NoLAN: false, Cancelled: false })
    })
  }

  function scanDone(dx: Diagnosis) {
    // Cancelled (a probe or the handover stopped it): says nothing about the network.
    if (dx.Cancelled) {
      scanning = false
      // The search tile was chosen while a probe was stopping the old scan:
      // the service started no new one then (one at a time). Start it now.
      if (st === 'discovering') startScan()
      return
    }
    scanning = false
    ctx.dx = dx
    step({ type: 'scanDone', dx }, 'keep')
  }

  function onDevice(d: Device) {
    if (!scanning || ctx.devices.some(x => x.BLEMac === d.BLEMac)) return
    ctx.devices = [...ctx.devices, d]
    step({ type: 'device', d }, 'keep')
  }

  function rescan() {
    if (st === 'adding' || st === 'done') return
    probeRun++
    clearTimeout(probeGuard)
    startScan()
    step({ type: 'rescan' })
  }

  function probe() {
    const run = ++probeRun
    const ip = (ctx.ip ?? '').trim()
    clearTimeout(probeGuard)
    const answer = (result: Device | { error: ReturnType<typeof probeCode> }) => {
      if (run !== probeRun || st !== 'probingIp') return
      clearTimeout(probeGuard)
      if ('error' in result) {
        ctx.ipError = result.error === 'cancelled' ? undefined : result.error
        step({ type: 'probe', result }, 'ip')
        return
      }
      step({ type: 'probe', result })
      startAdd(result)
    }
    probeGuard = window.setTimeout(() => { answer({ error: 'noAnswer' }); probeRun++ }, PROBE_GUARD_MS)
    deps.probe(ip).then(d => answer(d), err => answer({ error: probeCode(err) }))
  }

  /** submitIp sends the address given (a script), else the field's. */
  function submitIp(given?: string) {
    const input = host?.querySelector<HTMLInputElement>('#fr-ip')
    ctx.ip = given ?? input?.value ?? ctx.ip
    const ip = (ctx.ip ?? '').trim()
    if (!validIPv4(ip)) {
      ctx.ipError = 'format'
      // IP_NO_ANSWER: editing the address is MANUAL_IP (with the format error).
      if (st === 'ipNoAnswer') step({ type: 'editIp' }, 'ip')
      else render('ip')
      return
    }
    ctx.ipError = undefined
    step({ type: 'submitIp', ip }, 'ip')
    if (st === 'probingIp') probe()
  }

  function startAdd(d: Device) {
    ctx.added = d
    ctx.name = defaultName(d.Model)
    ctx.first = null
    ctx.addError = false
    // 20 s from «Додати», also when the call itself hangs (flows §1.4).
    clearTimeout(addTimer)
    addTimer = window.setTimeout(() => step({ type: 'addTimeout' }, 'keep'), ADD_TIMEOUT_MS)
    deps.add(d).then(() => {
      if (st === 'adding' && sample) takeSample(sample)
    }, err => {
      console.error('add', err)
      ctx.addError = true
      step({ type: 'addFailed' })
    })
  }

  /** The first sample of the added station: ADDING → DONE, or DONE «чекаємо»
   *  gets its figures. */
  function takeSample(s: Snapshot) {
    const d = ctx.added
    if (!d || s.Station?.ID !== d.BLEMac || s.Last?.SOC == null) return
    if (s.Station.Name && !editingName) ctx.name = s.Station.Name
    if (ctx.first) return
    const start = s.Current && !s.Current.End ? Date.parse(s.Current.Start) : NaN
    ctx.first = {
      soc: s.Last.SOC,
      grid: s.Grid === GRID.on ? 'on' : s.Grid === GRID.off ? 'off' : 'unknown',
      offFor: Number.isFinite(start) ? Math.max(0, now() - start) : 0,
    }
    if (st === 'adding') step({ type: 'firstSample' }, 'keep')
    else if (st === 'done') render()
  }

  async function allowFirewall() {
    if (ctx.firewall === 'pending') return
    ctx.firewall = 'pending'
    render('firewall')
    try {
      await deps.allowFirewall()
      ctx.firewall = 'idle'
      // The rule is there: search again at once.
      rescan()
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      // «No» on the UAC prompt is the user's answer, not a failure. «Already
      // being added»: an earlier prompt is still open (or its PowerShell still
      // runs after the wait ran out): pending, and the button comes back
      // after a while, as nothing reports when that one ends.
      if (/already being added/.test(msg)) {
        ctx.firewall = 'pending'
        clearTimeout(firewallRetry)
        firewallRetry = window.setTimeout(() => { if (ctx.firewall === 'pending') { ctx.firewall = 'idle'; render() } }, FIREWALL_RETRY_MS)
      } else {
        ctx.firewall = /cancelled by user/.test(msg) ? 'idle' : 'failed'
      }
      if (ctx.firewall === 'failed') console.error('firewall', err)
      render('firewall')
    }
  }

  async function commitName(input: HTMLInputElement) {
    editingName = false
    const value = input.value
    const err = validateName(value)
    if (err) { ctx.nameError = err; render('name'); return }
    if (value.trim() === ctx.name) { ctx.nameError = null; render(); return }
    try {
      await deps.rename(value.trim())
      ctx.name = value.trim()
      ctx.nameError = null
    } catch (e) {
      console.error('rename', e)
      ctx.nameError = t('settings.save.error')
    }
    render()
  }

  async function toggle(key: 'autostart' | 'minimizeToTray') {
    const s = ctx.settings
    if (!s || !view) return
    ctx.toggleError = undefined
    try {
      if (key === 'autostart') {
        const v = await deps.setAutostart(!s.autostart)
        ctx.settings = { ...s, autostart: v.Autostart }
      } else {
        view = await deps.saveSettings(withChange(view.S, 'minimizeToTray', !s.minimizeToTray))
        ctx.settings = { ...s, minimizeToTray: view.S.MinimizeToTray }
      }
    } catch (err) {
      console.error(key, err)
      ctx.toggleError = key
    }
    render(`toggle:${key}`)
  }

  async function setLanguage(l: Locale) {
    setLocale(l)
    try {
      const v = view ?? await deps.getSettings()
      view = await deps.saveSettings(withChange(v.S, 'language', l))
    } catch (err) {
      console.error('language', err)
    }
  }

  /** act runs an action by its key, as a click does. */
  function act(key: string, arg?: string) {
    switch (key) {
      case 'tile': if (arg === 'lan') rescan(); else if (arg === 'ip') step({ type: 'enterIp' }, 'ip'); break
      case 'rescan': rescan(); break
      case 'enterIp': step({ type: 'enterIp' }, 'ip'); break
      case 'howTo': step({ type: 'howTo' }); break
      case 'back': step({ type: 'back' }); break
      case 'retry': if (st === 'ipNoAnswer') { ctx.ipError = undefined; step({ type: 'retry' }, 'ip'); probe() } break
      case 'submitIp': if (st !== 'probingIp') submitIp(arg); break
      case 'add': {
        const d = ctx.devices.find(x => x.BLEMac === arg)
        if (d && st === 'found') { step({ type: 'add', d }); startAdd(d) }
        break
      }
      case 'open': case 'overview': deps.finish(); break
      case 'firewall': void allowFirewall(); break
      case 'netSettings': deps.openURL(NETWORK_SETTINGS_URL).catch(err => console.error('network settings', err)); break
      case 'readme': deps.openURL(readmeURL()).catch(err => console.error('open url', err)); break
      case 'copyTemplate': {
        const body = firstRunModel(st, ctx).template?.body
        if (body) deps.copyText(body).then(() => { ctx.copied = true; render('copyTemplate') }, err => console.error('copy', err))
        break
      }
      case 'toggle': if (arg === 'autostart' || arg === 'minimizeToTray') void toggle(arg); break
      case 'trayHint': ctx.trayHint = false; render('overview'); break
    }
  }

  function tilesHTML(v: FRView): string {
    if (!v.tiles) return ''
    return `<div class="tiles" role="radiogroup" aria-label="${esc(t('onboarding.tiles.a11y'))}">` + v.tiles.map(x =>
      `<button type="button" class="tile" role="radio" aria-checked="${x.checked}" tabindex="${x.checked ? 0 : -1}" data-act="tile" data-arg="${x.key}" data-focus="tile:${x.key}">` +
      `<span class="tile__icon">${icon(x.icon)}</span><b>${esc(x.title)}</b><span>${esc(x.body)}</span></button>`).join('') + '</div>'
  }

  function topHTML(v: FRView): string {
    const l = getLocale()
    const opt = (value: Locale) => `<option value="${value}"${l === value ? ' selected' : ''}>${esc(t(`settings.general.language.${value}`))}</option>`
    return `<div class="fr__top"><div><h1>${APP_NAME}</h1><p>${esc(v.welcome)}</p></div>` +
      `<select class="select lang" data-act="lang" data-focus="lang" aria-label="${esc(t('onboarding.languageSwitch.a11y'))}">${opt('uk')}${opt('en')}</select></div>`
  }

  /** renderTop draws the welcome line and the language switch: only on
   *  mount and a language switch, so a redraw for a found row or a sample
   *  never closes the open language list. */
  function renderTop() {
    const top = host?.querySelector<HTMLElement>('[data-slot="top"]')
    if (!top) return
    const focused = top.contains(document.activeElement)
    top.innerHTML = topHTML(firstRunModel(st, ctx))
    if (focused) top.querySelector<HTMLElement>('[data-act="lang"]')?.focus()
  }

  /** render redraws the page. The focus stays on the same control; focus
   *  names where it goes after the user's own action ('heading' = the
   *  result's heading, read out). */
  function render(focus?: string) {
    if (!host) return
    const v = firstRunModel(st, ctx)
    const active = document.activeElement as HTMLElement | null
    const keep = host.contains(active) ? active!.dataset.focus : undefined
    const input = active instanceof HTMLInputElement && host.contains(active) ? active : null
    const caret = input?.selectionStart ?? null
    const done = doneHTML(v)
    const result = headHTML(v) + rowsHTML(v, scanning) + dxHTML(v) + ipHTML(v) + guideHTML(v) +
      (v.notListed ? `<p class="notlisted">${esc(v.notListed.text)} ${actionHTML(v.notListed.action)}</p>` : '') +
      (v.progress ? `<p class="note loading fr-busy">${spinnerHTML()}<span>${esc(v.progress)}</span></p>` : '') +
      (v.error ? `<p class="fr-error" role="alert">${esc(v.error)}</p>` : '') +
      (v.actions.length && !v.done ? `<div class="actions">${v.actions.map(a => actionHTML(a)).join('')}</div>` : '') +
      done.name
    const page = host.querySelector<HTMLElement>('.fr')!
    page.querySelector('[data-slot="body"]')!.innerHTML = tilesHTML(v) +
      `<section class="panel result${v.head ? '' : ' result--flush'}" ${v.label ? `aria-label="${esc(v.label)}"` : 'aria-labelledby="fr-h"'}>${result}</section>` + done.extras +
      (v.done ? `<div class="fr-foot">${v.actions.map(a => actionHTML(a)).join('')}</div>` : '')
    const live = page.querySelector<HTMLElement>('[data-slot="announce"]')!
    if (v.announce !== lastAnnounce) live.textContent = lastAnnounce = v.announce
    const want = focus === 'heading' ? undefined : focus === undefined || focus === 'keep' ? keep : focus
    let el = want ? page.querySelector<HTMLElement>(`[data-focus="${CSS.escape(want)}"]`) : null
    if (!el && focus === 'heading') el = page.querySelector<HTMLElement>('#fr-h')
    // The control the focus was on is gone: the result's heading.
    if (!el && keep) el = page.querySelector<HTMLElement>('#fr-h')
    el?.focus()
    if (el instanceof HTMLInputElement) {
      // A redraw under the user's typing keeps the name typed (the address
      // is ctx.ip, kept on every keystroke).
      if (input && el.id === 'fr-name' && input.id === 'fr-name') el.value = input.value
      const at = caret ?? el.value.length
      el.setSelectionRange(at, at)
    }
  }

  function onClick(e: Event) {
    const el = (e.target as Element).closest<HTMLElement>('[data-act]')
    if (!el || !host?.contains(el) || el.dataset.act === 'lang') return
    if (el.getAttribute('aria-disabled') === 'true') return
    if (el.dataset.act === 'tile' && el.getAttribute('aria-checked') === 'true') return
    act(el.dataset.act!, el.dataset.arg)
  }

  function onKey(e: KeyboardEvent) {
    const el = e.target as HTMLElement
    if (el.id === 'fr-ip' && e.key === 'Enter') { e.preventDefault(); if (st !== 'probingIp') submitIp(); return }
    if (el.id === 'fr-name' && e.key === 'Enter') { e.preventDefault(); void commitName(el as HTMLInputElement); return }
    if (e.key === 'Escape' && st === 'lanGuide') { e.preventDefault(); act('back'); return }
    // The tiles are a radio group: the arrows move between them and choose.
    if (el.dataset.act === 'tile' && ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) {
      e.preventDefault()
      const other = host!.querySelector<HTMLElement>(`.tile[data-arg="${el.dataset.arg === 'lan' ? 'ip' : 'lan'}"]`)
      if (other) { act('tile', other.dataset.arg); host!.querySelector<HTMLElement>(`[data-focus="tile:${other.dataset.arg}"]`)?.focus() }
    }
  }

  function onInput(e: Event) {
    const el = e.target as HTMLInputElement
    if (el.id === 'fr-ip') {
      ctx.ip = el.value
      // Editing the address clears its error (IP_NO_ANSWER → MANUAL_IP).
      if (ctx.ipError) {
        ctx.ipError = undefined
        if (st === 'ipNoAnswer') step({ type: 'editIp' }, 'ip')
        else render('ip')
      }
    } else if (el.id === 'fr-name') {
      editingName = true
      if (ctx.nameError && validateName(el.value) === null) {
        ctx.nameError = null
        const err = host!.querySelector<HTMLElement>('#fr-name-err')
        if (err) { err.textContent = ''; err.removeAttribute('role') }
        el.removeAttribute('aria-invalid')
      }
    }
  }

  function onChange(e: Event) {
    const el = e.target as HTMLSelectElement
    if (el.dataset.act === 'lang' && (el.value === 'uk' || el.value === 'en')) void setLanguage(el.value)
  }

  function onFocusOut(e: FocusEvent) {
    const el = e.target as HTMLElement
    if (el.id === 'fr-name' && st === 'done') void commitName(el as HTMLInputElement)
  }

  return {
    mount(h) {
      host = h
      host.innerHTML = '<div class="fr"><div data-slot="top" style="display:contents"></div><div data-slot="body" style="display:contents"></div><div class="sr-only" aria-live="polite" data-slot="announce"></div></div>'
      host.addEventListener('click', onClick)
      host.addEventListener('keydown', onKey)
      host.addEventListener('input', onInput)
      host.addEventListener('change', onChange)
      host.addEventListener('focusout', onFocusOut)
      offLocale = onLocaleChange(() => { renderTop(); render() })
      renderTop()
      offScan = deps.onScan(onDevice, scanDone)
      startScan()
      render()
      // The platform picks the Mac texts (the diagnosis, the guide, DONE).
      deps.getSettings().then(v => { ctx.platform = v.Platform; render() }, err => console.error('settings', err))
      deps.onState?.(st, ctx)
    },
    update(s) {
      sample = s
      takeSample(s)
    },
    unmount() {
      clearTimeout(addTimer)
      clearTimeout(probeGuard)
      clearTimeout(firewallRetry)
      probeRun++
      offScan()
      offLocale()
      host?.removeEventListener('click', onClick)
      host?.removeEventListener('keydown', onKey)
      host?.removeEventListener('input', onInput)
      host?.removeEventListener('change', onChange)
      host?.removeEventListener('focusout', onFocusOut)
      host = null
    },
    act,
    state: () => st,
    context: () => ctx,
  }
}
