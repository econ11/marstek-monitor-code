import type { SettingsView, UpdateView } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Settings } from '../../bindings/github.com/econ11/marstek-monitor-code/core/settings/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { icon } from '../ui/icons'
import { infoTipHTML, keepInfoTip } from '../ui/infoTip'
import { loadingHTML } from '../ui/loading'
import { morphChildren } from '../ui/morph'
import { mountPeriod, periodRange, syncSeg, type DayRange, type PeriodKey } from '../ui/period'
import { escapeHTML as esc } from '../ui/tooltip'
import {
  isSection, panelGroups, retentionChange, sectionIcon, settingsModel, splitAction, validateMinSOC, validateName, validTime, withChange,
  type RowAction, type RowNote, type SettingRow, type SettingsSection,
} from './settingsModel'

// The Settings screen (hi-fi 07-settings.html): «←» back to the previous tab
// (Esc too), the section menu on the left, the section's rows on the right.
// Every change is saved at once; a failed save puts the control back and
// says so under the row.

export type SettingsDeps = {
  get(): Promise<SettingsView>
  save(s: Settings): Promise<SettingsView>
  setAutostart(on: boolean): Promise<SettingsView>
  rename(name: string): Promise<void>
  sendTest(): Promise<void>
  openNotifySettings(): Promise<void>
  /** Windows blocks the app's notifications (asked on mount and on focus). */
  notificationsOff(): Promise<boolean>
  /** Days "YYYY-MM-DD", inclusive; null or [] = the folder dialog was cancelled. */
  exportCSV(fromDay: string, toDay: string): Promise<string[] | null>
  showInFolder(path: string): Promise<void>
  pruneNow(): Promise<void>
  openURL(url: string): Promise<void>
  /** The update state (UpdatesService.State), the manual check (Check) and
   *  its event (updates:changed; returns the unsubscribe). */
  updateState(): Promise<UpdateView>
  checkUpdates(): Promise<UpdateView>
  onUpdates(fn: (v: UpdateView) => void): () => void
  /** The one-click update (Task 5 sets it to UpdatesService.Install). While
   *  absent, «Оновити» is shown disabled. */
  install?(): Promise<unknown>
  back(): void
  section?: string
  now?: () => number
}

/** A row's transient state: an error, or a result line (sent, saved). */
type Overlay = { error?: string; notes?: RowNote[] }

const fileDir = (path: string) => {
  const parts = path.split(/[\\/]/)
  return parts.length >= 2 ? parts[parts.length - 2] : path
}

/** The row's label (named o-<id> for its control) and its ⓘ beside it, not
 *  inside it, so the control's name stays the label. */
function labelHTML(r: SettingRow): string {
  const label = `<span id="o-${r.id}">${esc(r.label)}</span>`
  return r.info ? `<span>${label}${infoTipHTML(r.label, r.info, r.id, ` data-focus="${r.id}:info"`)}</span>` : label
}

function notesHTML(notes: RowNote[] | undefined, id: string): string {
  if (!notes?.length) return ''
  return `<ul class="opt__notes">${notes.map((n, i) => `<li class="opt__note" data-tone="${n.tone ?? 'muted'}">` +
    `${n.tone === 'warn' ? icon('alert', 'icon--sm') : n.tone === 'good' ? icon('check-circle', 'icon--sm') : ''}<span>${esc(n.text)}` +
    (n.action ? ` <button type="button" class="link" data-act="note" data-id="${id}" data-note="${i}" data-focus="${id}:note:${i}">${esc(n.actionLabel ?? '')}</button>` : '') +
    '</span></li>').join('')}</ul>`
}

function segHTML(r: SettingRow, labelledBy: string, small = false): string {
  const items = (r.options ?? []).map(o => `<button type="button" class="seg__item" aria-pressed="${o.value === r.value}" data-act="seg" ` +
    `data-id="${r.id}" data-value="${esc(o.value)}" data-focus="${r.id}:${esc(o.value)}"${r.disabled ? ' disabled' : ''}>${esc(o.label)}</button>`).join('')
  return `<div class="seg${small ? ' seg--sm' : ''}" role="group" aria-labelledby="${labelledBy}"><span class="seg__thumb"></span>${items}</div>`
}

function errorHTML(r: SettingRow): string {
  // Inserted already filled, a status region is often not read out: an
  // error goes in as an alert. Keyed by its text, so a redraw in place
  // (morphChildren) also puts a new error in as a new node.
  return `<div class="opt__err" id="e-${r.id}" data-key="e-${r.id}${r.error ? `:${esc(r.error)}` : ''}"${r.error ? ' role="alert"' : ''}>${r.error ? esc(r.error) : ''}</div>`
}

/** A sub-control inside its switch's row (threshold, minutes, quiet range). */
function subHTML(r: SettingRow): string {
  if (r.control === 'time2') {
    const [a, b] = r.value as [string, string]
    const inv = r.error ? ' aria-invalid="true"' : ''
    const dis = r.disabled ? ' disabled' : ''
    return `<div class="opt__sub"><span class="timerange">` +
      `<input class="input mono" data-act="time" data-id="${r.id}" data-part="0" data-focus="${r.id}:0" value="${esc(a)}" inputmode="numeric" maxlength="5" ` +
      `aria-label="${esc(t('settings.notify.quiet.start.a11y'))}" aria-describedby="e-${r.id}"${inv}${dis}> – ` +
      `<input class="input mono" data-act="time" data-id="${r.id}" data-part="1" data-focus="${r.id}:1" value="${esc(b)}" inputmode="numeric" maxlength="5" ` +
      `aria-label="${esc(t('settings.notify.quiet.end.a11y'))}" aria-describedby="e-${r.id}"${inv}${dis}></span>` +
      `<span class="opt__hint">${esc(r.desc ?? '')}</span></div>${errorHTML(r)}`
  }
  return `<div class="opt__sub">${labelHTML(r)}${segHTML(r, `o-${r.id}`, true)}</div>`
}

function rowHTML(r: SettingRow, subs: SettingRow[]): string {
  const ic = r.icon ? `<span class="opt__icon">${icon(r.icon)}</span>` : ''
  const plain = r.icon ? '' : ' opt--plain'
  const desc = r.desc ? `<div class="opt__d" id="d-${r.id}">${esc(r.desc)}</div>` : ''
  const head = r.info ? `<div class="opt__t">${labelHTML(r)}</div>${desc}` : `<div class="opt__t" id="o-${r.id}">${esc(r.label)}</div>${desc}`
  const tail = subs.map(subHTML).join('') + notesHTML(r.notes, r.id)
  switch (r.control) {
    case 'toggle': {
      const off = subs.length && !r.value ? ' is-off' : ''
      return `<div class="opt${plain}${off}" data-row="${r.id}">${ic}<div class="opt__main">${head}${tail}${errorHTML(r)}</div>` +
        `<button type="button" class="switch" role="switch" aria-checked="${r.value === true}" aria-labelledby="o-${r.id}"` +
        `${r.desc ? ` aria-describedby="d-${r.id}"` : ''} data-act="toggle" data-id="${r.id}" data-focus="${r.id}"${r.disabled ? ' disabled' : ''}></button></div>`
    }
    case 'seg':
      return `<div class="opt${plain} opt--wide" data-row="${r.id}">${ic}<div class="opt__main">${head}${tail}${errorHTML(r)}</div>` +
        `<div class="opt__ctl">${segHTML(r, `o-${r.id}`)}</div></div>`
    case 'text': {
      const inv = r.error ? ' aria-invalid="true"' : ''
      const field = `<input class="input input--text${r.suffix ? ' input--num' : ''}" id="in-${r.id}" data-act="text" data-id="${r.id}" data-focus="${r.id}" ` +
        `value="${esc(String(r.value ?? ''))}"${r.placeholder ? ` placeholder="${esc(r.placeholder)}"` : ''} aria-labelledby="o-${r.id}" aria-describedby="${r.desc ? `d-${r.id} ` : ''}e-${r.id}"${inv}` +
        `${r.suffix ? ' inputmode="numeric" maxlength="2"' : ' spellcheck="false"'}>`
      return `<div class="opt${plain}" data-row="${r.id}">${ic}<div class="opt__main">${head}${tail}${errorHTML(r)}</div>` +
        `<div class="opt__ctl">${field}${r.suffix ? `<span class="opt__suffix">${esc(r.suffix)}</span>` : ''}</div></div>`
    }
    case 'button': {
      if (r.href) {
        return `<div class="opt opt--plain" data-row="${r.id}"><div class="opt__main"><button type="button" class="link" data-act="open" ` +
          `data-id="${r.id}" data-focus="${r.id}">${icon('external', 'icon--sm')}${esc(r.label)}</button></div></div>`
      }
      // The export: the period, then the folder.
      const dis = r.disabled ? ` aria-disabled="true"${r.tip ? ` data-tip="${esc(r.tip)}"` : ''}` : ''
      return `<div class="opt${plain}" data-row="${r.id}">${ic}<div class="opt__main">${head}` +
        `<div class="opt__sub"><div data-slot="period"></div><button type="button" class="btn btn--secondary" data-act="export" ` +
        `data-id="${r.id}" data-focus="${r.id}"${dis}>${icon('download')}${esc(t('settings.data.export.action'))}</button></div>` +
        `${tail}${errorHTML(r)}</div></div>`
    }
    case 'actions': {
      // The update row; its state line is read out when it changes, except
      // while the download counts percents (the progress bar carries them).
      const acts = (r.actions ?? []).map(a => actionHTML(r.id, a)).join('')
      const live = r.progress !== undefined && r.progress < 1 ? 'off' : 'polite'
      return `<div class="opt opt--plain" data-row="${r.id}"><div class="opt__main"><div class="opt__t" id="o-${r.id}" aria-live="${live}">${esc(r.label)}</div>` +
        `${progressHTML(r)}${tail}${errorHTML(r)}</div><div class="opt__ctl opt__acts">${acts}</div></div>`
    }
    default:
      return `<div class="opt${plain} opt--info" data-row="${r.id}">${ic}<div class="opt__main"><div class="opt__t opt__t--info">${esc(r.label)}</div>${tail}</div></div>`
  }
}

/** The one-click update's progress bar (row.progress 0..1), named by the
 *  row's state line. */
export function progressHTML(r: SettingRow): string {
  if (r.progress === undefined) return ''
  const pct = Math.round(Math.min(1, Math.max(0, r.progress)) * 100)
  return `<div class="opt__bar" role="progressbar" aria-labelledby="o-${r.id}" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${pct}">` +
    `<i style="width:${pct}%"></i></div>`
}

/** A button of an 'actions' row. A disabled one is aria-disabled, not
 *  disabled (as the export button): «Перевірити зараз» keeps the keyboard
 *  focus while the check runs; the click handler ignores it. */
export function actionHTML(id: string, a: RowAction): string {
  return `<button type="button" class="btn btn--${a.primary ? 'primary' : 'secondary'}" data-act="${a.act}" ` +
    `data-id="${id}" data-focus="${id}:${a.act}"${a.disabled ? ' aria-disabled="true"' : ''}>${esc(a.label)}</button>`
}

/** The section's body: heading, intro (with its head button), then the
 *  rows, grouped into panels by row.group. */
function bodyHTML(section: SettingsSection, rows: SettingRow[]): string {
  const subsOf = (id: string) => rows.filter(r => r.parent === id)
  const intro = rows.find(r => r.id === 'intro')
  const heads = rows.filter(r => r.head && r.control !== 'banner')
  const banners = rows.filter(r => r.head && r.control === 'banner')
  let html = `<h2 class="sh" id="settings-h" tabindex="-1">${esc(t(`settings.section.${section}`))}</h2>`
  if (intro || heads.length) {
    html += `<div class="sd-row"><p class="sd">${esc(intro?.label ?? '')}</p>` +
      heads.map(h => `<button type="button" class="btn btn--secondary" data-act="${h.id}" data-id="${h.id}" data-focus="${h.id}" ` +
        `title="${esc(h.desc ?? '')}">${h.icon ? icon(h.icon) : ''}${esc(h.label)}</button>`).join('') + '</div>'
    html += heads.map(h => notesHTML(h.notes, h.id) + (h.error ? `<p class="opt__err" role="alert">${esc(h.error)}</p>` : '')).join('')
  }
  // In place of the head button (the test notification): Windows blocks them.
  html += banners.map(b => `<div class="banner banner--warn set-banner" role="status">${icon(b.icon ?? 'alert')}` +
    `<div class="banner__t">${esc(b.label)}</div><button type="button" class="btn btn--secondary" data-act="openNotify" ` +
    `data-id="${b.id}" data-focus="${b.id}">${icon('external')}${esc(b.actionLabel ?? '')}</button></div>`).join('')
  // One panel per group, keyed (panelGroups) so a redraw keeps it.
  html += panelGroups(rows).map(g => `<div class="grp" data-key="${esc(g.key)}">${g.label ? `<span class="label">${esc(g.label)}</span>` : ''}<div class="panel">` +
    g.rows.map(r => rowHTML(r, subsOf(r.id))).join('') + '</div></div>').join('')
  return html
}

/** confirmDialog: a modal with a safe and a destructive button; Esc and the
 *  safe one resolve false. Focus starts on the safe one and stays inside. */
function confirmDialog(title: string, body: string, keep: string, act: string, restore: () => void): Promise<boolean> {
  return new Promise(resolve => {
    const scrim = document.createElement('div')
    scrim.className = 'scrim settings-scrim'
    const dlg = document.createElement('div')
    dlg.className = 'dialog settings-dialog'
    dlg.setAttribute('role', 'alertdialog')
    dlg.setAttribute('aria-modal', 'true')
    dlg.setAttribute('aria-labelledby', 'confirm-t')
    dlg.setAttribute('aria-describedby', 'confirm-d')
    dlg.innerHTML = `<div class="dialog__head"><div><h2 class="dialog__title" id="confirm-t">${esc(title)}</h2>` +
      `<p class="dialog__desc" id="confirm-d">${esc(body)}</p></div></div><div class="dialog__body"></div>` +
      `<div class="dialog__foot"><button type="button" class="btn btn--secondary" data-r="0">${esc(keep)}</button>` +
      `<button type="button" class="btn btn--danger" data-r="1">${esc(act)}</button></div>`
    document.body.append(scrim, dlg)
    const buttons = Array.from(dlg.querySelectorAll<HTMLButtonElement>('button'))
    const done = (ok: boolean) => {
      document.removeEventListener('keydown', onKey, true)
      scrim.remove()
      dlg.remove()
      // By key, not by element: a redraw while open replaced the opener.
      restore()
      resolve(ok)
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        done(false)
      } else if (e.key === 'Tab') {
        e.preventDefault()
        const i = buttons.indexOf(document.activeElement as HTMLButtonElement)
        buttons[(i + (e.shiftKey ? buttons.length - 1 : 1)) % buttons.length].focus()
      }
    }
    dlg.addEventListener('click', e => {
      const b = (e.target as Element).closest<HTMLElement>('[data-r]')
      if (b) done(b.dataset.r === '1')
    })
    scrim.addEventListener('click', () => done(false))
    document.addEventListener('keydown', onKey, true)
    buttons[0].focus()
  })
}

export function settingsScreen(deps: SettingsDeps): Screen {
  const now = deps.now ?? Date.now
  let host: HTMLElement | null = null
  let view: SettingsView | null = null
  let snap: Snapshot | null = null
  let snapKey = ''
  let section: SettingsSection = isSection(deps.section) ? deps.section : 'general'
  let loadError = false
  // Windows blocks the app's notifications (banner §11.2); asked again when
  // the window gains focus (the user may have just switched them on).
  let notifyOff = false
  // The update state (Про програму), kept current by updates:changed.
  let upd: UpdateView | null = null
  let offUpdates = () => {}
  const overlay = new Map<string, Overlay>()
  let period: { unmount(): void } | null = null
  let periodSlot: HTMLElement | null = null
  /** The body last drawn and its section: the next render patches it. */
  let drawn: { body: HTMLElement; section: SettingsSection } | null = null
  let range: { key: PeriodKey; r?: DayRange } = { key: '7d' }
  let exportRange: DayRange | null = null
  let exportPaths: string[] = []
  let busy = false
  let leaving = false
  let offLocale = () => {}
  // Whether a tooltip was open when Esc went down (the tooltip engine closes
  // it in the bubble phase, before onKey runs).
  let tipWasOpen = false
  const noteTip = (e: KeyboardEvent) => { tipWasOpen = e.key === 'Escape' && !!document.querySelector('.tip.is-open') }

  const setOverlay = (id: string, o: Overlay | null) => { if (o) overlay.set(id, o); else overlay.delete(id) }

  function rows(): SettingRow[] {
    return settingsModel(view!, snap, section, { notificationsOff: notifyOff, update: upd, canInstall: !!deps.install }).rows.map(r => {
      const o = overlay.get(r.id)
      return o ? { ...r, error: o.error ?? r.error, notes: [...(r.notes ?? []), ...(o.notes ?? [])] } : r
    })
  }

  function frame() {
    host!.innerHTML = `<div class="settings">` +
      `<div class="settings__bar"><button type="button" class="iconbtn" data-act="back" aria-label="${esc(t('settings.back.a11y'))}">${icon('chevron-left', 'icon--lg')}</button>` +
      `<h1 class="settings__title">${esc(t('settings.title'))}</h1></div>` +
      `<div class="set"><nav class="snav" aria-label="${esc(t('settings.sections.a11y'))}"></nav><section class="set__body" aria-labelledby="settings-h"></section></div></div>`
  }

  function renderMenu() {
    const nav = host!.querySelector<HTMLElement>('.snav')!
    const focused = (document.activeElement as HTMLElement | null)?.dataset?.section
    const m = settingsModel(view!, snap, section).menu
    nav.innerHTML = m.map(i => `<button type="button" data-section="${i.key}"${i.active ? ' aria-current="page"' : ''}>` +
      `${icon(sectionIcon(i.key))}<span>${esc(i.label)}</span></button>`).join('') +
      `<div class="ver">Marstek Monitor ${esc(view!.Version)}</div>`
    if (focused) nav.querySelector<HTMLElement>(`[data-section="${focused}"]`)?.focus()
  }

  /** render redraws the body, keeping the focus on the same control. The
   *  same section is patched in place (morphChildren): only what changed
   *  changes, so only the control the user used animates. Another section,
   *  or a new frame (language), is drawn anew. */
  function render() {
    if (!host || !view) return
    const body = host.querySelector<HTMLElement>('.set__body')!
    const focusKey = (document.activeElement as HTMLElement | null)?.closest?.<HTMLElement>('.set__body [data-focus]')?.dataset.focus
    const html = bodyHTML(section, rows())
    const patch = drawn?.body === body && drawn.section === section
    keepInfoTip(() => {
      if (!patch) { body.innerHTML = html; return }
      const fresh = document.createElement('template')
      fresh.innerHTML = html
      morphChildren(body, fresh.content)
    })
    drawn = { body, section }
    body.querySelectorAll<HTMLElement>('.seg').forEach(syncSeg)
    // The export's period stays mounted while its slot does.
    const slot = body.querySelector<HTMLElement>('[data-slot="period"]')
    if (slot !== periodSlot) {
      period?.unmount()
      period = null
      periodSlot = slot
      if (slot) {
        period = mountPeriod(slot, {
          initial: range.key, range: range.r, now,
          onChange: (r, key) => { range = { key, r: key === 'custom' ? r : undefined }; exportRange = r },
        })
      }
    }
    if (focusKey && !body.contains(document.activeElement)) body.querySelector<HTMLElement>(`[data-focus="${CSS.escape(focusKey)}"]`)?.focus()
  }

  function renderAll() {
    if (!host) return
    if (!view) {
      host.innerHTML = loadError
        ? `<div class="settings"><p class="opt__err" role="alert">${esc(t('settings.save.error'))}</p></div>`
        : loadingHTML()
      return
    }
    // A language switch rebuilds everything: the focus stays where it was.
    const active = document.activeElement as HTMLElement | null
    const keep = host.contains(active) ? active!.dataset.focus ?? active!.dataset.section ?? active!.dataset.act : undefined
    frame()
    renderMenu()
    render()
    if (keep) {
      const k = CSS.escape(keep)
      host.querySelector<HTMLElement>(`[data-focus="${k}"], [data-section="${k}"], [data-act="${k}"]`)?.focus()
    }
  }

  /** accept takes the view a change returned. A partial one (the change was
   *  saved, the rest unknown) keeps the known fields and refreshes later. */
  function accept(v: SettingsView) {
    if (!v.Partial || !view) {
      view = v
      return
    }
    view = { ...view, S: v.S?.Notify ? v.S : view.S, Lang: v.Lang || view.Lang }
    deps.get().then(full => { view = full; render() }, err => console.warn('settings refresh', err))
  }

  async function save(id: string, next: Settings): Promise<boolean> {
    try {
      accept(await deps.save(next))
      setOverlay(id, null)
      return true
    } catch (err) {
      console.error('settings save', err)
      setOverlay(id, { error: t('settings.save.error') })
      return false
    } finally {
      render()
    }
  }

  async function change(id: string, value: unknown) {
    if (!view || busy) return
    if (id === 'autostart') {
      try {
        const v = await deps.setAutostart(value as boolean)
        view = { ...view, Autostart: v.Autostart }
        if (v.Partial) deps.get().then(full => { view = full; render() }, err => console.warn('settings refresh', err))
        setOverlay(id, null)
      } catch (err) {
        console.error('autostart', err)
        setOverlay(id, { error: t('settings.save.error') })
      }
      render()
      return
    }
    /** The dialog, with focus back on the control that opened it (by key). */
    const confirm = async (title: string, body: string, keep: string, act: string) => {
      busy = true
      const key = (document.activeElement as HTMLElement | null)?.dataset?.focus
      const restore = () => {
        const el = key ? host?.querySelector<HTMLElement>(`[data-focus="${CSS.escape(key)}"]`) : null
        ;(el ?? host?.querySelector<HTMLElement>('#settings-h'))?.focus()
      }
      const ok = await confirmDialog(title, body, keep, act, restore)
      busy = false
      return ok
    }
    let prune = false
    if (id === 'retention') {
      const c = retentionChange(view.S.RetentionDays, Number(value), now())
      if (c.confirm) {
        const ok = await confirm(t('settings.data.retention.confirm', { period: c.period }),
          t('settings.data.retention.confirm.body', { date: c.date }),
          t('settings.data.retention.confirm.keep'), t('settings.data.retention.confirm.delete'))
        if (!ok) { render(); return }
        prune = true
      }
    }
    busy = true
    const ok = await save(id, withChange(view.S, id, value))
    busy = false
    // Save applied the new retention synchronously: PruneNow uses it.
    if (ok && prune) deps.pruneNow().catch(err => console.error('prune', err))
  }

  async function commitText(input: HTMLInputElement) {
    if (leaving || !view) return
    const id = input.dataset.id!
    const value = input.value
    if (id === 'name') {
      const current = snap?.Station?.Name ?? ''
      // Unchanged is no edit: a station without a name (shown by its model)
      // keeps the empty field without an error.
      if (value.trim() === current) { setOverlay(id, null); render(); return }
      const err = validateName(value)
      if (err) { setOverlay(id, { error: err }); render(); return }
      try {
        await deps.rename(value.trim())
        setOverlay(id, null)
      } catch (e) {
        console.error('rename', e)
        setOverlay(id, { error: t('settings.save.error') })
      }
      render()
    } else if (id === 'minSOC') {
      const err = validateMinSOC(value)
      if (err) { setOverlay(id, { error: err }); render(); return }
      if (Number(value) === view.S.StationMinSOC) { setOverlay(id, null); render(); return }
      await change(id, value.trim())
    }
  }

  function commitTime(input: HTMLInputElement) {
    if (leaving || !view) return
    const both = Array.from(input.closest('.timerange')!.querySelectorAll<HTMLInputElement>('input')).map(i => i.value.trim())
    if (!both.every(validTime)) { setOverlay('quiet.range', { error: t('settings.notify.quiet.error') }); render(); return }
    const n = view.S.Notify
    if (both[0] === n.QuietStart && both[1] === n.QuietEnd) { setOverlay('quiet.range', null); render(); return }
    void change('quiet.range', both)
  }

  async function runExport(btn: HTMLElement) {
    if (btn.getAttribute('aria-disabled') === 'true' || busy) return
    // mountPeriod reports changes only; before one, the initial period.
    const r = exportRange ?? (range.key === 'custom' && range.r ? range.r : periodRange(range.key === 'custom' ? '7d' : range.key, now()))
    busy = true
    try {
      const paths = await deps.exportCSV(r.from, r.to)
      // Cancelled: no error, no message.
      if (!paths?.length) { busy = false; return }
      exportPaths = paths
      const [text, action] = splitAction(t('settings.data.export.done', { fileName: fileDir(paths[0]) }), ' · ')
      setOverlay('export', { notes: [{ text, tone: 'good', action: 'showInFolder', actionLabel: action }] })
    } catch (err) {
      console.error('export', err)
      setOverlay('export', { notes: [{ text: t('settings.data.export.error'), tone: 'warn' }] })
    }
    busy = false
    render()
  }

  async function sendTest() {
    try {
      await deps.sendTest()
      const [text, action] = splitAction(t('settings.notify.test.sent'), '? ', true)
      setOverlay('test', { notes: [{ text, tone: 'muted', action: 'openNotify', actionLabel: action }] })
    } catch (err) {
      console.error('test notification', err)
      setOverlay('test', { notes: [{ text: t('settings.notify.test.error'), tone: 'warn' }] })
    }
    render()
  }

  function checkNotify() {
    deps.notificationsOff().then(off => {
      if (off === notifyOff || !host) return
      notifyOff = off
      // Not under the user's typing: the next redraw shows it.
      if (document.activeElement instanceof HTMLInputElement && host.contains(document.activeElement)) return
      if (section === 'notifications') render()
    }, err => console.warn('notification setting', err))
  }

  /** A new update state: redrawn when «Про програму» is open. */
  function setUpdate(v: UpdateView) {
    upd = v
    if (host && view && section === 'about') render()
  }

  /** «Перевірити зараз»: the result (an error included) comes back in the
   *  view; a call that fails outright counts as a failed check. */
  function checkUpdates() {
    if (upd?.Checking) return
    deps.checkUpdates().then(setUpdate, err => {
      console.error('update check', err)
      setUpdate({ ...(upd ?? { Current: view?.Version ?? '', Latest: '', Available: false, PageURL: '', Progress: 0, Phase: '' }), Checking: false, Err: 'check_failed' })
    })
  }

  function noteAction(id: string, i: number) {
    const note = rows().find(r => r.id === id)?.notes?.[i]
    if (!note?.action) return
    if (note.action === 'openNotify') deps.openNotifySettings().catch(err => console.error('notification settings', err))
    else if (note.action === 'showInFolder' && exportPaths[0]) deps.showInFolder(exportPaths[0]).catch(err => console.error('show in folder', err))
  }

  function onClick(e: Event) {
    const el = (e.target as Element).closest<HTMLElement>('[data-act], [data-section]')
    if (!el || !host?.contains(el)) return
    if (el.dataset.section) {
      if (el.dataset.section !== section && isSection(el.dataset.section)) {
        section = el.dataset.section
        renderMenu()
        render()
        host.querySelector<HTMLElement>('.set__body')!.scrollTop = 0
      }
      return
    }
    const id = el.dataset.id ?? ''
    switch (el.dataset.act) {
      case 'back': deps.back(); break
      case 'toggle': void change(id, el.getAttribute('aria-checked') !== 'true'); break
      case 'seg': if (el.getAttribute('aria-pressed') !== 'true') void change(id, el.dataset.value); break
      case 'test': void sendTest(); break
      case 'openNotify': deps.openNotifySettings().catch(err => console.error('notification settings', err)); break
      case 'export': void runExport(el); break
      case 'open': {
        const href = rows().find(r => r.id === id)?.href
        if (href) deps.openURL(href).catch(err => console.error('open url', err))
        break
      }
      case 'note': noteAction(id, Number(el.dataset.note)); break
      case 'check': if (el.getAttribute('aria-disabled') !== 'true') checkUpdates(); break
      case 'download': if (upd?.PageURL) deps.openURL(upd.PageURL).catch(err => console.error('open url', err)); break
      // «Оновити»: its phases and a failure come back as updates:changed.
      case 'install': if (el.getAttribute('aria-disabled') !== 'true') deps.install?.().catch(err => console.error('update install', err)); break
    }
  }

  function onKey(e: KeyboardEvent) {
    const el = e.target as HTMLElement
    // An Esc that only closed a tooltip stays on the screen.
    if (e.key === 'Escape' && tipWasOpen) { tipWasOpen = false; return }
    if (e.key === 'Escape' && !e.defaultPrevented) {
      // An edit in progress is dropped, then Esc leaves the screen.
      leaving = true
      e.preventDefault()
      deps.back()
      return
    }
    if (el.dataset?.section && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      const items = Array.from(host!.querySelectorAll<HTMLElement>('.snav [data-section]'))
      const i = items.indexOf(el)
      items[(i + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length].focus()
      e.preventDefault()
      return
    }
    if (e.key === 'Enter' && el instanceof HTMLInputElement) {
      e.preventDefault()
      if (el.dataset.act === 'text') void commitText(el)
      else if (el.dataset.act === 'time') commitTime(el)
    }
  }

  function onFocusOut(e: FocusEvent) {
    const el = e.target as HTMLElement
    if (!(el instanceof HTMLInputElement)) return
    // Between the two quiet-hours fields the range is not committed yet.
    if (el.dataset.act === 'time' && (e.relatedTarget as HTMLElement | null)?.dataset?.act === 'time') return
    if (el.dataset.act === 'text') void commitText(el)
    else if (el.dataset.act === 'time') commitTime(el)
  }

  /** Typing clears a shown error as soon as the value is good. */
  function onInput(e: Event) {
    const el = e.target as HTMLInputElement
    const id = el.dataset.id
    if (!id || !overlay.get(id)?.error) return
    const fine = el.dataset.act === 'time'
      ? Array.from(el.closest('.timerange')!.querySelectorAll<HTMLInputElement>('input')).every(i => validTime(i.value.trim()))
      : id === 'name' ? validateName(el.value) === null : validateMinSOC(el.value) === null
    if (fine) {
      setOverlay(id, null)
      const err = host!.querySelector<HTMLElement>(`#e-${CSS.escape(id)}`)
      if (err) err.textContent = ''
      el.closest('.opt')?.querySelectorAll('[aria-invalid]').forEach(i => i.removeAttribute('aria-invalid'))
    }
  }

  /** What of the snapshot the screen shows: redraw only when it changes, and
   *  never under the user's typing. */
  function keyOf(s: Snapshot): string {
    return JSON.stringify([s.Station?.ID, s.Station?.Name])
  }

  return {
    mount(h) {
      host = h
      renderAll()
      host.addEventListener('click', onClick)
      host.addEventListener('focusout', onFocusOut)
      host.addEventListener('input', onInput)
      document.addEventListener('keydown', noteTip, true)
      document.addEventListener('keydown', onKey)
      offLocale = onLocaleChange(renderAll)
      window.addEventListener('focus', checkNotify)
      checkNotify()
      offUpdates = deps.onUpdates(setUpdate)
      deps.updateState().then(v => { if (!upd) setUpdate(v) }, err => console.warn('update state', err))
      deps.get().then(v => {
        view = v
        renderAll()
        // The heading takes the focus (read out); Tab goes on into the section.
        host?.querySelector<HTMLElement>('#settings-h')?.focus()
      }, err => {
        console.error('settings', err)
        loadError = true
        renderAll()
      })
    },
    update(s) {
      snap = s
      const k = keyOf(s)
      if (k === snapKey || !view) return
      // Not under the user's typing: the key stays old, so the next snapshot
      // (or leaving the field) draws it.
      if (document.activeElement instanceof HTMLInputElement && host?.contains(document.activeElement)) return
      snapKey = k
      renderMenu()
      render()
    },
    unmount() {
      leaving = true
      period?.unmount()
      period = periodSlot = drawn = null
      offLocale()
      offUpdates()
      window.removeEventListener('focus', checkNotify)
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('keydown', noteTip, true)
      host?.removeEventListener('click', onClick)
      host?.removeEventListener('focusout', onFocusOut)
      host?.removeEventListener('input', onInput)
      host = null
    },
  }
}
