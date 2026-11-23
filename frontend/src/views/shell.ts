import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { icon } from '../ui/icons'
import { infoTipHTML, keepInfoTip } from '../ui/infoTip'
import { laterHTML } from '../ui/loading'
import { mountPeriod, type DayRange, type PeriodKey } from '../ui/period'
import { escapeHTML as esc, hideTip, tipAnchor } from '../ui/tooltip'
import { DONATE_URL } from './settingsModel'
import type { OverviewFeed, OverviewModel, RightRow } from './overviewModel'

// The window frame around every tab (hi-fi 01-overview.html): the header with
// the station, the five tabs and ⚙, the grid rail under it, the window title,
// and the content slot the router mounts screens into.

export type ShellDeps = {
  setTitle(title: string): void
  /** The snapshots and their model (shared with the Overview tab). */
  feed: OverviewFeed
  /** Opens a URL in the default browser (Browser.OpenURL). */
  openURL?(url: string): Promise<void>
}

const APP_NAME = 'Marstek Monitor'
const TABS = {
  overview: 'nav.tab.overview', outages: 'nav.tab.outages', errors: 'nav.tab.errors',
  battery: 'nav.tab.battery', energy: 'nav.tab.energy',
} as const

export function frameHTML(): string {
  const tabs = Object.entries(TABS).map(([v, key]) => `<a class="tab" href="#${v}" data-view="${v}"><span data-label="${key}"></span>` +
    (v === 'errors' ? '<span class="tab__count" data-slot="errors-badge" hidden></span>' : '') + '</a>').join('')
  return `<header class="appbar" data-grid="none">
  <div class="station-btn" data-slot="station"></div>
  <nav class="tabs" data-label-aria="nav.tabs.a11y">${tabs}</nav>
  <div class="appbar__end"><span class="appbar__source"><span data-slot="source"></span><span data-slot="source-info"></span></span><button type="button" class="iconbtn" data-act="donate">${icon('heart', 'icon--lg')}</button><a class="iconbtn" href="#settings" data-view="settings" data-label-aria="nav.settings.a11y">${icon('settings', 'icon--lg')}</a></div>
</header>
<div class="content" data-slot="view"></div>`
}

/** «● LAN · 7 с тому»: the dot and value of the source row, its label for the
 *  screen reader, the whole row in the tooltip (owner request, Task 21). */
function sourceHTML(r: RightRow): string {
  const full = `${r.label}: ${r.value}${r.meta ? ` · ${r.meta}` : ''}`
  return `<span class="src${r.live ? ' live' : ''}" data-tone="${r.tone ?? 'muted'}" title="${esc(full)}">` +
    `<span class="sr-only">${esc(r.label)}: </span><span class="src__text">${esc(r.value)}</span></span>`
}

function stationHTML(m: OverviewModel, s: Snapshot): string {
  const soc = s.Last?.SOC
  const known = m.variant !== 'skeleton' && soc != null
  const color = m.variant !== 'live' ? 'var(--color-text-subtle)' : soc != null && soc < s.LowThreshold ? 'var(--color-bad)' : 'var(--color-good)'
  const dial = `<span class="minidial" aria-hidden="true"><svg viewBox="0 0 48 48"><circle class="track" cx="24" cy="24" r="21.5"/>` +
    (known && soc > 0 ? `<circle class="fill" cx="24" cy="24" r="21.5" pathLength="100" stroke-dasharray="${Math.min(soc, 100)} 100" transform="rotate(-90 24 24)" style="stroke:${color}"/>` : '') +
    `</svg><b>${known ? `<span>${Math.round(soc)}</span>` : ''}</b></span>`
  const name = m.station?.name ?? APP_NAME
  const model = m.station?.model
  return dial + `<span class="station-btn__text"><span class="station-btn__name">${esc(name)}</span>${model ? `<span class="station-btn__model">${esc(model)}</span>` : ''}</span>`
}

/** mountShell builds the frame into root (once; a language switch only
 *  replaces its texts, so the router's tab marks stay). It draws the station,
 *  the rail and the window title from every model of the feed. */
export function mountShell(root: HTMLElement, deps: ShellDeps) {
  root.innerHTML = frameHTML()
  const appbar = root.querySelector<HTMLElement>('.appbar')!
  const station = root.querySelector<HTMLElement>('[data-slot="station"]')!
  const source = root.querySelector<HTMLElement>('[data-slot="source"]')!
  const sourceInfo = root.querySelector<HTMLElement>('[data-slot="source-info"]')!
  const errorsBadge = root.querySelector<HTMLElement>('[data-slot="errors-badge"]')!
  const errorsTab = errorsBadge.closest<HTMLElement>('.tab')!
  const donate = root.querySelector<HTMLElement>('[data-act="donate"]')
  let stationMarkup = ''
  let sourceMarkup = ''
  let sourceInfoMarkup = ''
  let title = ''
  /** The badge's count, drawn again in a new language. */
  let newCount = 0

  function texts() {
    root.querySelectorAll<HTMLElement>('[data-label]').forEach(el => { el.textContent = t(el.dataset.label!) })
    if (donate) { donate.setAttribute('aria-label', t('support.author')); donate.setAttribute('data-tip', t('support.author')) }
    root.querySelectorAll<HTMLElement>('[data-label-aria]').forEach(el => el.setAttribute('aria-label', t(el.dataset.labelAria!)))
  }

  function render(m: OverviewModel, s: Snapshot) {
    appbar.dataset.grid = m.variant === 'skeleton' ? 'none' : m.outageRail === 'on' ? 'off' : m.outageRail === 'off' ? 'on' : 'unknown'
    const markup = stationHTML(m, s)
    if (markup !== stationMarkup) station.innerHTML = stationMarkup = markup
    const src = sourceHTML(m.source)
    if (src !== sourceMarkup) source.innerHTML = sourceMarkup = src
    // The ⓘ apart from the age, which moves every second: it is redrawn only when its text changes.
    const info = m.source.info ? infoTipHTML(m.source.label, m.source.info) : ''
    if (info !== sourceInfoMarkup) keepInfoTip(() => { sourceInfo.innerHTML = sourceInfoMarkup = info })
    const next = m.titleState ? `${APP_NAME} · ${m.titleState}` : APP_NAME
    if (next !== title) deps.setTitle(title = next)
    badge(s.NewEvents ?? 0)
  }

  /** «нові» on the Errors tab: journal warnings since it was last opened.
   *  The number is for the eye; the tab's name says it for screen readers. */
  function badge(count: number) {
    newCount = count
    errorsBadge.hidden = count <= 0
    errorsBadge.textContent = count > 0 ? String(count) : ''
    errorsBadge.setAttribute('aria-hidden', 'true')
    // What «нові» counts (copy §15 info.errors.new) is the tab link's own app
    // tooltip: it opens on hover and on focus and describes the link (the
    // badge itself is aria-hidden).
    if (count > 0) {
      errorsTab.setAttribute('aria-label', `${t('nav.tab.errors')}, ${t('errors.new.badge.a11y', { count })}`)
      errorsTab.setAttribute('data-tip', t('info.errors.new'))
    } else {
      errorsTab.removeAttribute('aria-label')
      errorsTab.removeAttribute('data-tip')
      // Without data-tip the pointer and focus handlers no longer see the tab:
      // its open tooltip (the count cleared under it) is closed here.
      if (tipAnchor() === errorsTab) hideTip()
    }
  }

  donate?.addEventListener('click', () => {
    deps.openURL?.(DONATE_URL)?.catch(err => console.error('open url', err))
  })
  texts()
  // The feed redraws in the new language too; the tab's tooltip does not wait for it.
  onLocaleChange(() => { texts(); badge(newCount) })
  deps.feed.on(render)
}

/** PeriodMemory keeps a tab's period until the window closes. */
export type PeriodMemory = { key: PeriodKey; range?: DayRange }

/** placeholderScreen is a tab whose content comes in a later task: a neutral
 *  «Ця вкладка з'явиться згодом», never a loader (nothing is loading), under
 *  the period switcher when the tab has one. */
export function placeholderScreen(opts: { period?: PeriodMemory; now?: () => number } = {}): Screen {
  let off = () => {}
  let period: { unmount(): void } | null = null
  return {
    mount(host) {
      const note = document.createElement('div')
      const text = () => { note.innerHTML = laterHTML() }
      text()
      off = onLocaleChange(text)
      const mem = opts.period
      if (mem) {
        const bar = document.createElement('div')
        bar.className = 'toolbar'
        host.appendChild(bar)
        period = mountPeriod(bar, {
          initial: mem.key, range: mem.range, now: opts.now ?? Date.now,
          onChange: (r, key) => { mem.key = key; if (key === 'custom') mem.range = r },
        })
      }
      host.appendChild(note)
    },
    update() {},
    unmount() {
      off()
      period?.unmount()
    },
  }
}
