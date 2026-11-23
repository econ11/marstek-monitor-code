import './styles/tokens.css'
import './styles/components.css'
import './styles/app.css'

import { Browser, Clipboard, Events, Flags, Window } from '@wailsio/runtime'
import { MonitorService, NavigateService, OnboardService, SettingsService, UpdatesService } from '../bindings/github.com/econ11/marstek-monitor-code/app'
import type { Device, Diagnosis, SettingsView, UpdateView } from '../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Snapshot } from '../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Navigate } from '../bindings/github.com/econ11/marstek-monitor-code/app/models'
import { createRouter, isView, type NavOpts, type Screen, type View } from './app/router'
import { getLocale, onLocaleChange, setLocale, type Locale } from './lib/i18n'
import { sprite } from './ui/icons'
import type { ChartRange } from './ui/socChart'
import { initInfoTips } from './ui/infoTip'
import { initTooltips } from './ui/tooltip'
import { batteryScreen } from './views/battery'
import { energyScreen } from './views/energy'
import { errorsScreen } from './views/errors'
import { firstRunScreen, type FirstRunScreen } from './views/firstRun'
import { runFirstRunE2E } from './views/firstRunE2E'
import { toastNavigation } from './views/firstRunModel'
import { outagesScreen } from './views/outages'
import { overviewScreen } from './views/overview'
import { overviewFeed } from './views/overviewModel'
import { settingsScreen } from './views/settings'
import { mountShell, type PeriodMemory } from './views/shell'

// Name of the Go → JS event that carries a monitor.Snapshot (app/service.go).
const SNAPSHOT_EVENT = 'snapshot'
// Name of the Go → JS event that carries a SettingsView after every change
// (app/settings.go).
const SETTINGS_EVENT = 'settings'
// Name of the Go → JS event that carries an UpdateView (app/updates.go).
const UPDATES_EVENT = 'updates:changed'
// Name of the Go → JS event that opens a tab: a toast click (app/toast.go).
const NAVIGATE_EVENT = 'navigate'
// The first-run screen's scan events (app/onboard.go).
const SCAN_DEVICE_EVENT = 'scan.device'
const SCAN_DONE_EVENT = 'scan.done'

const asLocale = (lang: string): Locale => lang === 'uk' ? 'uk' : 'en'

async function start() {
  // The language comes first, so the window never flashes the other one.
  try {
    setLocale(asLocale((await SettingsService.Get()).Lang))
  } catch (err) {
    console.error('settings', err)
  }
  document.documentElement.lang = getLocale()
  onLocaleChange(() => { document.documentElement.lang = getLocale() })
  document.body.insertAdjacentHTML('afterbegin', sprite)
  initTooltips()
  initInfoTips()
  const root = document.getElementById('app')!
  // The newest snapshot and its model, once per snapshot and second for the
  // shell and the Overview tab. The first bound call can race the event: an
  // older snapshot never replaces a newer one.
  const feed = overviewFeed()
  setInterval(feed.tick, 1000)
  mountShell(root, {
    feed,
    openURL: url => Browser.OpenURL(url),
    setTitle: title => {
      document.title = title
      void Window.SetTitle(title).catch(err => console.warn('window title', err))
    },
  })
  // What each tab keeps until the window closes: the chart range, the
  // dismissed gap cards, and the periods of Outages and Energy (each its own).
  const overview = { range: '24h' as ChartRange, dismissed: new Set<number>() }
  const outages: PeriodMemory = { key: '7d' }
  const energy: PeriodMemory = { key: '7d' }
  // Settings → «←» (and Esc) go back to the tab it was opened from.
  let lastTab: View = 'overview'
  const tab = (v: View, f: (o: NavOpts) => Screen) => (o: NavOpts) => { lastTab = v; return f(o) }
  const router = createRouter(root, {
    overview: tab('overview', () => overviewScreen({
      series: range => MonitorService.SOCSeries(range),
      dismissGapCard: id => MonitorService.DismissGapCard(id),
      memory: overview,
      feed,
    })),
    outages: tab('outages', o => outagesScreen({
      report: (from, to) => MonitorService.OutagesReport(from, to),
      memory: outages,
      outageId: o.outageId,
    })),
    errors: tab('errors', () => errorsScreen({
      page: (from, to, beforeAt, beforeID, limit, filter, newSince) => MonitorService.JournalPage(from, to, beforeAt, beforeID, limit, filter, newSince),
      since: (from, to, afterAt, afterID, filter, newSince) => MonitorService.JournalSince(from, to, afterAt, afterID, filter, newSince),
      markSeen: upTo => MonitorService.MarkJournalSeen(upTo),
    })),
    battery: tab('battery', () => batteryScreen({ summary: () => MonitorService.Battery() })),
    energy: tab('energy', () => energyScreen({
      report: (from, to) => MonitorService.EnergyReport(from, to),
      power: (from, to) => MonitorService.PowerSeries(from, to),
      memory: energy,
    })),
    settings: o => settingsScreen({
      get: () => SettingsService.Get(),
      save: s => SettingsService.Save(s),
      setAutostart: on => SettingsService.SetAutostart(on),
      rename: name => SettingsService.RenameStation(name),
      sendTest: () => SettingsService.SendTestNotification(),
      openNotifySettings: () => SettingsService.OpenWindowsNotificationSettings(),
      notificationsOff: () => SettingsService.NotificationsOff(),
      exportCSV: (from, to) => SettingsService.ExportCSV(from, to),
      showInFolder: path => SettingsService.ShowInFolder(path),
      pruneNow: () => SettingsService.PruneNow(),
      openURL: url => Browser.OpenURL(url),
      updateState: () => UpdatesService.State(),
      checkUpdates: () => UpdatesService.Check(),
      onUpdates: fn => Events.On(UPDATES_EVENT, ev => fn(ev.data as UpdateView)),
      // «Оновити»: progress and failures come back as updates:changed.
      install: () => UpdatesService.Install(),
      back: () => {
        router.navigate(lastTab)
        root.querySelector<HTMLElement>('.appbar [data-view="settings"]')?.focus()
      },
      section: o.section,
    }),
    firstRun: () => {
      const screen = firstRunScreen({
        scan: () => OnboardService.Scan(),
        probe: ip => OnboardService.ProbeIP(ip),
        add: d => OnboardService.Add(d),
        allowFirewall: () => OnboardService.AllowFirewall(),
        onScan: (device, done) => {
          const offs = [
            Events.On(SCAN_DEVICE_EVENT, ev => device(ev.data as Device)),
            Events.On(SCAN_DONE_EVENT, ev => done(ev.data as Diagnosis)),
          ]
          return () => offs.forEach(off => off())
        },
        getSettings: () => SettingsService.Get(),
        saveSettings: s => SettingsService.Save(s),
        setAutostart: on => SettingsService.SetAutostart(on),
        rename: name => SettingsService.RenameStation(name),
        openURL: url => Browser.OpenURL(url),
        copyText: text => Clipboard.SetText(text),
        finish: leaveFirstRun,
      })
      if (e2e) runFirstRunE2E(screen, title => void Window.SetTitle(title).catch(() => {}), leaveFirstRun)
      firstRun = screen
      return screen
    },
  })
  // The scripted first run (-simulate-e2e only, main.go frontendFlags).
  let e2e = false
  // The first-run screen while it is shown (null after «Перейти до огляду»).
  let firstRun: FirstRunScreen | null = null
  try {
    e2e = Flags.GetFlag('e2e') === 'firstrun'
  } catch {
    // No flags: not scripted.
  }
  // No station in the database: the first run, also after a restart in the
  // middle of it. A stored station whose monitor failed to start is not a
  // first run: the overview says what is wrong.
  function leaveFirstRun() {
    firstRun = null
    delete root.dataset.firstrun
    router.navigate('overview')
    root.querySelector<HTMLElement>('.appbar [data-view="overview"]')?.focus()
  }
  // The header stays hidden until the answer, so a first run never flashes
  // the tabs (an empty window for a moment instead).
  root.dataset.firstrun = ''
  let hasStation = true
  try {
    hasStation = await OnboardService.HasStation()
  } catch (err) {
    console.error('has station', err)
  }
  if (hasStation) {
    delete root.dataset.firstrun
    router.navigate('overview')
  } else router.navigate('firstRun')
  const update = (s: Snapshot) => { if (feed.update(s)) router.update(s) }
  Events.On(SNAPSHOT_EVENT, ev => update(ev.data as Snapshot))
  // Settings → Language: the screen redraws in the new language at once.
  Events.On(SETTINGS_EVENT, ev => setLocale(asLocale((ev.data as SettingsView).Lang)))
  // A toast click: the tab it is about (flows §2.8).
  function onNavigate(n: Navigate) {
    if (!isView(n.View)) return
    // During the first run: leave it first (the header comes back), or stay
    // when nothing can be about a station yet.
    const how = toastNavigation(firstRun ? firstRun.state() : null)
    if (how === 'ignore') return
    if (how === 'leave') leaveFirstRun()
    router.navigate(n.View, { outageId: n.OutageID || undefined, section: n.Section || undefined })
  }
  Events.On(NAVIGATE_EVENT, ev => onNavigate(ev.data as Navigate))
  // Listening now: a click made before the page was up (--background, or
  // Windows starting the app for it) waited in Go and comes once.
  NavigateService.Ready().then(n => { if (n) onNavigate(n) }, err => console.error('navigate ready', err))
  MonitorService.Snapshot().then(update, err => console.error('first snapshot', err))
}

void start()
