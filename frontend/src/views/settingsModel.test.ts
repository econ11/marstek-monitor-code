import { afterEach, expect, test } from 'vitest'
import type { SettingsView, UpdateView } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import { setLocale } from '../lib/i18n'
import { stationLabel } from '../lib/station'
import { DONATE_URL, SECTIONS, notifyTestSent, panelGroups, retentionChange, settingsModel, splitAction, validateName, withChange, type SettingRow } from './settingsModel'

afterEach(() => setLocale('uk'))

const S_NS = 1e9, MIN_NS = 60 * S_NS

const view = {
  S: {
    Language: '', MinimizeToTray: true,
    Notify: { LowThreshold: 20, QuietEnabled: false, QuietStart: '23:00', QuietEnd: '07:00', NoLinkAfter: 10 * MIN_NS, Enabled: { no_link: false } },
    PollInterval: 5 * S_NS, RetentionDays: 0, StationMinSOC: 10,
  },
  Lang: 'uk', Autostart: false, Version: '1.0.0', Platform: 'windows', DataPath: 'C:\\Users\\me\\AppData\\Local\\MarstekMonitor',
  HasHistory: true, Partial: false,
} as unknown as SettingsView

function snapWith(o: Record<string, unknown> = {}): Snapshot {
  return {
    Station: { ID: 'st1', Name: 'Дім', Model: 'VenusE 3.0', BLEMac: '', WifiMac: '', IP: '192.168.1.47', Firmware: 151, CreatedAt: '' },
    Link: 'LIVE', Battery: { Cycles: null, HealthPct: null, HealthApprox: false }, ...o,
  } as unknown as Snapshot
}
const snap = snapWith()
const noStation = snapWith({ Station: { ID: '', Name: '', Model: '', Firmware: 0 } })

function row(m: { rows: SettingRow[] }, id: string): SettingRow {
  const r = m.rows.find(r => r.id === id)
  if (!r) throw new Error(`no row ${id} in ${m.rows.map(r => r.id).join(', ')}`)
  return r
}

// The brief's tests (plan 2 Task 17, Step 1).
test('error toggle is hidden in v1', () => expect(settingsModel(view, snap, 'notifications').rows.map(r => r.id)).not.toContain('error'))
test('no-link minutes disabled while off', () => expect(row(settingsModel(view, snap, 'notifications'), 'noConnection.after').disabled).toBe(true))
test('connection section has no cloud', () => expect(JSON.stringify(settingsModel(view, snap, 'connection'))).not.toMatch(/хмар|cloud|Bluetooth/i))
test('rename validation', () => expect(validateName('x'.repeat(25))).toBe('Назва — до 24 символів. Зараз 25.'))
test('export disabled without history', () => expect(row(settingsModel({ ...view, HasHistory: false } as SettingsView, snap, 'data'), 'export').disabled).toBe(true))
test('retention shortening asks first', () => {
  const now = new Date(2026, 10, 21, 14, 5).getTime()
  expect(retentionChange(0, 365, now)).toEqual({ confirm: true, period: '1 рік', date: '21.11.2025' })
  expect(retentionChange(1825, 730, now)).toEqual({ confirm: true, period: '2 роки', date: '21.11.2024' })
  expect(retentionChange(365, 1825, now)).toEqual({ confirm: false })
  expect(retentionChange(365, 0, now)).toEqual({ confirm: false })
  expect(retentionChange(365, 365, now)).toEqual({ confirm: false })
  setLocale('en')
  expect(retentionChange(0, 1825, now)).toEqual({ confirm: true, period: '5 years', date: '22 Nov 2021' }) // 1825 days, one of them 29.02.2024
})

test('menu lists the six sections and marks the current one', () => {
  const m = settingsModel(view, snap, 'data')
  expect(m.menu.map(i => i.key)).toEqual(['general', 'notifications', 'stations', 'connection', 'data', 'about'])
  expect(m.menu.map(i => i.label)).toEqual(['Загальні', 'Сповіщення', 'Станції', 'Підключення', 'Дані та історія', 'Про програму'])
  expect(m.menu.filter(i => i.active).map(i => i.key)).toEqual(['data'])
})

test('general: language, autostart, tray, theme', () => {
  const m = settingsModel(view, snap, 'general')
  expect(m.rows.map(r => r.id)).toEqual(['language', 'autostart', 'minimizeToTray', 'theme'])
  expect(row(m, 'language')).toMatchObject({ control: 'seg', value: '', label: 'Мова', desc: 'За замовчуванням — мова Windows.' })
  expect(row(m, 'language').options).toEqual([{ value: '', label: 'Як у Windows' }, { value: 'uk', label: 'Українська' }, { value: 'en', label: 'English' }])
  expect(row(m, 'autostart')).toMatchObject({ control: 'toggle', value: false, label: 'Запускати з Windows' })
  expect(row(m, 'minimizeToTray')).toMatchObject({ control: 'toggle', value: true })
  expect(row(m, 'theme')).toMatchObject({ control: 'seg', value: 'dark', disabled: true, desc: "Світла тема з'явиться пізніше." })
  setLocale('en')
  expect(row(settingsModel(view, snap, 'general'), 'language').options!.map(o => o.label)).toEqual(['Same as Windows', 'Українська', 'English'])
})

test('notifications: groups, defaults and dependent controls', () => {
  const m = settingsModel(view, snap, 'notifications')
  expect(m.rows.map(r => r.id)).toEqual(['intro', 'test', 'gridLost', 'gridBack', 'low', 'low.threshold', 'full', 'likelyOut',
    'noConnection', 'noConnection.after', 'quiet', 'quiet.range'])
  expect(row(m, 'gridLost')).toMatchObject({ control: 'toggle', value: true, group: 'Світло', label: 'Зникло світло' })
  expect(row(m, 'low.threshold')).toMatchObject({ control: 'seg', value: '20', parent: 'low', disabled: false })
  expect(row(m, 'low.threshold').options!.map(o => o.label)).toEqual(['10%', '20%', '30%', '40%', '50%'])
  expect(row(m, 'likelyOut').label).toBe('Станція, ймовірно, розрядилась')
  expect(row(m, 'noConnection')).toMatchObject({ value: false, group: "Зв'язок",
    desc: 'Якщо станція не відповідає довше, ніж вказано. Вимкнено за замовчуванням: без світла роутер теж може вимкнутись.' })
  expect(row(m, 'noConnection.after').options!.map(o => o.label)).toEqual(['5 хв', '10 хв', '15 хв', '30 хв', '60 хв'])
  expect(row(m, 'noConnection.after').value).toBe(String(10 * MIN_NS))
  expect(row(m, 'quiet.range')).toMatchObject({ control: 'time2', value: ['23:00', '07:00'], disabled: true })
  expect(row(m, 'test')).toMatchObject({ control: 'button', head: true, label: 'Надіслати тестове сповіщення' })
  expect(JSON.stringify(m)).not.toMatch(/хмар|cloud/i)
})

test('a switch turned off disables its sub-control', () => {
  const off = { ...view, S: { ...view.S, Notify: { ...view.S.Notify, QuietEnabled: true, Enabled: { low_charge: false, no_link: true } } } } as SettingsView
  const m = settingsModel(off, snap, 'notifications')
  expect(row(m, 'low')).toMatchObject({ value: false })
  expect(row(m, 'low.threshold').disabled).toBe(true)
  expect(row(m, 'noConnection.after').disabled).toBe(false)
  expect(row(m, 'quiet.range').disabled).toBe(false)
})

// Task 20c (copy §11.2): Windows blocks the app's notifications → the
// banner settings.notify.osDisabled instead of the test button.
test('notifications off in Windows: a banner instead of the test button', () => {
  expect(settingsModel(view, snap, 'notifications', { notificationsOff: false }).rows.map(r => r.id)).toContain('test')
  const m = settingsModel(view, snap, 'notifications', { notificationsOff: true })
  expect(m.rows.map(r => r.id)).not.toContain('test')
  expect(row(m, 'osDisabled')).toMatchObject({ control: 'banner', head: true,
    label: 'Сповіщення для Marstek Monitor вимкнено в Windows.', actionLabel: 'Відкрити налаштування Windows' })
  setLocale('en')
  expect(row(settingsModel(view, snap, 'notifications', { notificationsOff: true }), 'osDisabled')).toMatchObject({
    label: 'Notifications for Marstek Monitor are off in Windows.', actionLabel: 'Open Windows settings' })
  // Only the notifications section shows it.
  expect(settingsModel(view, snap, 'general', { notificationsOff: true }).rows.map(r => r.id)).not.toContain('osDisabled')
})

test('splitAction keeps or drops the separator', () => {
  expect(splitAction("Надіслано. Не з'явилось? Відкрити", '? ', true)).toEqual(["Надіслано. Не з'явилось?", 'Відкрити'])
  expect(splitAction('Збережено: X · Показати в папці', ' · ')).toEqual(['Збережено: X', 'Показати в папці'])
  expect(splitAction('Без дії', ' · ')).toEqual(['Без дії', ''])
})

test('withChange builds the settings to save', () => {
  const s = view.S
  expect(withChange(s, 'language', 'en').Language).toBe('en')
  expect(withChange(s, 'low', false).Notify.Enabled).toEqual({ no_link: false, low_charge: false })
  expect(withChange(s, 'noConnection', true).Notify.Enabled).toEqual({ no_link: true })
  expect(withChange(s, 'low.threshold', '30').Notify.LowThreshold).toBe(30)
  expect(withChange(s, 'noConnection.after', String(30 * MIN_NS)).Notify.NoLinkAfter).toBe(30 * MIN_NS)
  expect(withChange(s, 'quiet.range', ['22:30', '06:45']).Notify).toMatchObject({ QuietStart: '22:30', QuietEnd: '06:45' })
  expect(withChange(s, 'poll', String(2 * S_NS)).PollInterval).toBe(2 * S_NS)
  expect(withChange(s, 'retention', '730').RetentionDays).toBe(730)
  expect(withChange(s, 'minSOC', '15').StationMinSOC).toBe(15)
  // The battery data reads were removed (owner decision 2026-11-23).
  expect(() => withChange(s, 'batteryDetails', 'on')).toThrow()
  expect(withChange(s, 'minimizeToTray', false).MinimizeToTray).toBe(false)
  // The original is never changed.
  expect(s.Notify.Enabled).toEqual({ no_link: false })
  expect(s.Language).toBe('')
})

test('stations: rename in place, discharge floor, tray', () => {
  const m = settingsModel(view, snap, 'stations')
  expect(m.rows.map(r => r.id)).toEqual(['intro', 'name', 'minSOC', 'trayShows'])
  expect(row(m, 'name')).toMatchObject({ control: 'text', value: 'Дім', label: 'Назва станції' })
  expect(row(m, 'minSOC')).toMatchObject({ control: 'text', value: '10' })
  expect(row(m, 'minSOC').desc).toMatch(/ймовірно/)
  expect(row(m, 'trayShows').label).toBe('У треї показується: Дім')
  expect(settingsModel(view, noStation, 'stations').rows.map(r => r.id)).toEqual(['intro', 'none', 'minSOC'])
})

test('name validation follows settings.ValidName', () => {
  expect(validateName('  ')).toBe('Дайте станції назву — так її буде видно в сповіщеннях.')
  expect(validateName('x'.repeat(24))).toBeNull()
  expect(validateName('  Дача  ')).toBeNull()
  // Code points, not UTF-16 units: 24 emoji fit.
  expect(validateName('🔋'.repeat(24))).toBeNull()
  expect(validateName('🔋'.repeat(25))).toBe('Назва — до 24 символів. Зараз 25.')
  setLocale('en')
  expect(validateName('x'.repeat(30))).toBe('Up to 24 characters. This one is 30.')
})

test('connection: polling and the LAN source only', () => {
  const m = settingsModel(view, snap, 'connection')
  expect(m.rows.map(r => r.id)).toEqual(['poll', 'station'])
  expect(row(m, 'poll').options!.map(o => o.label)).toEqual(['2 с', '5 с', '10 с', '30 с'])
  expect(row(m, 'poll').value).toBe(String(5 * S_NS))
  expect(row(m, 'station').label).toBe('Дім: LAN')
})

// Owner decision 2026-11-23 (Ruling 57): the app never reads the battery data
// (Bat.GetStatus), so Connection has no such setting, note or button.
test('connection: no battery data setting', () => {
  for (const lang of ['uk', 'en'] as const) {
    setLocale(lang)
    const text = JSON.stringify(settingsModel(view, snap, 'connection'))
    expect(text).not.toMatch(/батаре|battery|прошивк|firmware|Спробувати|Try now/i)
  }
})

test('data: retention, export, location', () => {
  const m = settingsModel(view, snap, 'data')
  expect(m.rows.map(r => r.id)).toEqual(['retention', 'export', 'location'])
  expect(row(m, 'retention').options).toEqual([{ value: '0', label: 'завжди' }, { value: '365', label: '1 рік' },
    { value: '730', label: '2 роки' }, { value: '1825', label: '5 років' }])
  expect(row(m, 'export')).toMatchObject({ control: 'button', disabled: false })
  expect(row({ rows: settingsModel({ ...view, HasHistory: false } as SettingsView, snap, 'data').rows }, 'export').tip).toBe('ще немає даних')
  expect(row(m, 'location').label).toBe('Дані зберігаються тільки на цьому комп\'ютері: C:\\Users\\me\\AppData\\Local\\MarstekMonitor')
})

test('about: version, privacy without the cloud, GitHub', () => {
  const m = settingsModel(view, snap, 'about')
  expect(m.rows.map(r => r.id)).toEqual(['version', 'updateCheck', 'update', 'description', 'privacy', 'github', 'donate'])
  expect(row(m, 'version').label).toBe('Версія 1.0.0')
  expect(row(m, 'privacy').label).toBe('Додаток не надсилає ваші дані нікуди, крім станції. Телеметрії немає.')
  expect(row(m, 'github').href).toBe('https://github.com/econ11/marstek-monitor-code')
  expect(row(m, 'donate')).toMatchObject({ control: 'button', href: DONATE_URL, label: 'Підтримати автора' })
  expect(DONATE_URL).toBe('https://donatello.to/econ11')
  expect(JSON.stringify(m)).not.toMatch(/хмар|cloud/i)
})

test('about: a dev build says so, a stamped one shows its number', () => {
  const dev = settingsModel({ ...view, Version: 'dev' } as SettingsView, snap, 'about')
  expect(row(dev, 'version').label).toBe('Версія dev (збірка розробника)')
  const none = settingsModel({ ...view, Version: '' } as SettingsView, snap, 'about')
  expect(row(none, 'version').label).toBe('Версія dev (збірка розробника)')
  const rel = settingsModel({ ...view, Version: '1.2.3' } as SettingsView, snap, 'about')
  expect(row(rel, 'version').label).toBe('Версія 1.2.3')
})

test('every section in en has no missing key', () => {
  setLocale('en')
  for (const sec of ['general', 'notifications', 'stations', 'connection', 'data', 'about'] as const) {
    // The station's own name and «Українська» stay as they are.
    const text = JSON.stringify(settingsModel(view, snap, sec)).replaceAll('Дім', '').replaceAll('Українська', '')
    expect(text).not.toMatch(/"(settings|addStation)\.[\w.]+"/)
    expect(text).not.toMatch(/[а-яіїєґ]{3}/i)
  }
})

// Review round 1, Important 2: with tray minimizing off, × quits, and the
// row says what that costs (flows §5.2).
test('minimize to tray off explains that outages stop being recorded', () => {
  expect(row(settingsModel(view, snap, 'general'), 'minimizeToTray').desc).toBe('Вікно ховається, а додаток далі стежить за станцією.')
  const off = { ...view, S: { ...view.S, MinimizeToTray: false } } as SettingsView
  expect(row(settingsModel(off, snap, 'general'), 'minimizeToTray').desc).toBe('Поки додаток закритий, відключення не записуються.')
  setLocale('en')
  expect(row(settingsModel(off, snap, 'general'), 'minimizeToTray').desc).toBe("While the app is closed, outages aren't recorded.")
})

// ⓘ (copy §15).
test('the rows of §15 have their ⓘ, the others none', () => {
  const infos = (section: Parameters<typeof settingsModel>[2]) =>
    Object.fromEntries(settingsModel(view, snap, section).rows.filter(r => r.info).map(r => [r.id, r.info!.key]))
  expect(infos('notifications')).toEqual({
    'low.threshold': 'info.settings.lowThreshold', noConnection: 'info.settings.noLinkAfter', quiet: 'info.settings.quiet',
  })
  expect(infos('connection')).toEqual({ poll: 'info.settings.poll' })
  expect(infos('data')).toEqual({ retention: 'info.settings.retention', export: 'info.settings.export' })
  expect(infos('general')).toEqual({})
})

// Final review I1: a station added before plan 2 has Name ''. Settings names
// it as the tray does (app/tray.go stationLabel): the name, else the model,
// else «Marstek»; the name field keeps the raw (empty) name and shows the
// label as its placeholder.
test('a station without a name is shown by its model', () => {
  expect(stationLabel({ Name: 'Дім', Model: 'VenusE 3.0' })).toBe('Дім')
  expect(stationLabel({ Name: '', Model: 'VenusE 3.0' })).toBe('VenusE 3.0')
  expect(stationLabel({ Name: '', Model: '' })).toBe('Marstek')
  expect(stationLabel(null)).toBe('Marstek')
  const unnamed = snapWith({ Station: { ID: 'st1', Name: '', Model: 'VenusE 3.0', Firmware: 151 } })
  const st = settingsModel(view, unnamed, 'stations')
  expect(row(st, 'name')).toMatchObject({ value: '', placeholder: 'VenusE 3.0' })
  expect(row(st, 'trayShows').label).toBe('У треї показується: VenusE 3.0')
  expect(row(settingsModel(view, unnamed, 'connection'), 'station').label).toBe('VenusE 3.0: LAN')
  // A named one: its name in all three.
  expect(row(settingsModel(view, snap, 'stations'), 'name')).toMatchObject({ value: 'Дім', placeholder: 'Дім' })
  expect(row(settingsModel(view, snap, 'connection'), 'station').label).toBe('Дім: LAN')
})

test('panelGroups: the stations panel keeps its key when the first snapshot arrives', () => {
  const key = (s: Snapshot | null) => panelGroups(settingsModel(view, s, 'stations').rows).map(g => g.key)
  expect(key(noStation)).toEqual(key(snap))
  expect(key(null)).toEqual(key(snap))
  const notify = panelGroups(settingsModel(view, snap, 'notifications').rows)
  expect(new Set(notify.map(g => g.key)).size).toBe(notify.length)
  expect(notify.flatMap(g => g.rows).some(r => r.parent || r.head || r.id === 'intro')).toBe(false)
})

// Plan 3 Task 4: the update rows of «Про програму».
const upd = (o: Partial<UpdateView> = {}): UpdateView =>
  ({ Current: '1.0.0', Latest: '', Available: false, PageURL: '', Checking: false, Err: '', Progress: 0, Phase: '', ...o })
const withUpdates = { ...view, S: { ...view.S, UpdateCheck: true } } as SettingsView

test('about: the update switch, its row and «Перевірити зараз»', () => {
  const m = settingsModel(withUpdates, snap, 'about', { update: upd() })
  expect(m.rows.map(r => r.id)).toEqual(['version', 'updateCheck', 'update', 'description', 'privacy', 'github', 'donate'])
  expect(row(m, 'updateCheck')).toMatchObject({ control: 'toggle', value: true, label: 'Перевіряти оновлення автоматично', desc: 'Раз на день запит до GitHub, без ваших даних.' })
  const off = settingsModel({ ...view, S: { ...view.S, UpdateCheck: false } } as SettingsView, snap, 'about', { update: upd() })
  expect(row(off, 'updateCheck').value).toBe(false)
  const r = row(m, 'update')
  // Never checked: only the button; the version row says what runs.
  expect(r).toMatchObject({ control: 'actions', label: '' })
  expect(r.actions).toEqual([{ act: 'check', label: 'Перевірити зараз' }])
  expect(withChange(withUpdates.S, 'updateCheck', false).UpdateCheck).toBe(false)
  // Before the view is known (no extras): the row still offers the check.
  expect(row(settingsModel(withUpdates, snap, 'about'), 'update').actions?.map(a => a.act)).toEqual(['check'])
})

test('about: checking, then «Доступна версія» with «Оновити» and «Завантажити»', () => {
  const checking = row(settingsModel(withUpdates, snap, 'about', { update: upd({ Checking: true }) }), 'update')
  expect(checking.label).toBe('Перевіряємо…')
  expect(checking.actions).toEqual([{ act: 'check', label: 'Перевірити зараз', disabled: true }])

  const avail = upd({ Latest: '1.2.0', Available: true, PageURL: 'https://github.com/econ11/marstek-monitor/releases/tag/v1.2.0' })
  const r = row(settingsModel(withUpdates, snap, 'about', { update: avail, canInstall: true }), 'update')
  expect(r.label).toBe('Доступна версія 1.2.0.')
  expect(r.actions).toEqual([
    { act: 'install', label: 'Оновити', primary: true },
    { act: 'download', label: 'Завантажити' },
    { act: 'check', label: 'Перевірити зараз' },
  ])
  // Without the install hook (canInstall false), «Оновити» is shown disabled.
  const noInstall = row(settingsModel(withUpdates, snap, 'about', { update: avail }), 'update')
  expect(noInstall.actions?.[0]).toEqual({ act: 'install', label: 'Оновити', primary: true, disabled: true })
  // Final review M7: only Windows has the installer, even with the hook.
  const mac = row(settingsModel({ ...withUpdates, Platform: 'darwin' } as SettingsView, snap, 'about', { update: avail, canInstall: true }), 'update')
  expect(mac.actions?.map(a => [a.act, !!a.disabled])).toEqual([['install', true], ['download', false], ['check', false]])
})

// Final review M2: a dev build never says «остання версія» and offers no check.
test('about: a dev build says updates are not checked', () => {
  for (const Version of ['dev', '']) {
    const dev = { ...withUpdates, Version } as SettingsView
    for (const u of [upd({ Current: Version }), upd({ Current: Version, Latest: '1.2.0' }), null]) {
      const r = row(settingsModel(dev, snap, 'about', { update: u, canInstall: true }), 'update')
      expect(r).toMatchObject({ control: 'info', label: 'Збірка розробника — оновлення не перевіряються.' })
      expect(r.actions).toBeUndefined()
      expect(r.error).toBeUndefined()
    }
  }
  setLocale('en')
  expect(row(settingsModel({ ...withUpdates, Version: 'dev' } as SettingsView, snap, 'about', { update: upd() }), 'update').label)
    .toBe("Developer build — updates aren't checked.")
})

test('about: «остання версія» when up to date, the error after a failed manual check', () => {
  const latest = row(settingsModel(withUpdates, snap, 'about', { update: upd({ Latest: '1.0.0' }) }), 'update')
  expect(latest.label).toBe('У вас остання версія.')
  expect(latest.error).toBeUndefined()
  const failed = row(settingsModel(withUpdates, snap, 'about', { update: upd({ Err: 'check_failed' }) }), 'update')
  expect(failed.error).toBe('Не вдалося перевірити оновлення. Спробуйте пізніше.')
  expect(failed.label).toBe('')
  // A failure after an earlier success keeps what is known and adds the error.
  const both = row(settingsModel(withUpdates, snap, 'about', { update: upd({ Latest: '1.2.0', Available: true, Err: 'check_failed' }) }), 'update')
  expect(both.label).toBe('Доступна версія 1.2.0.')
  expect(both.error).toBe('Не вдалося перевірити оновлення. Спробуйте пізніше.')
  setLocale('en')
  expect(row(settingsModel(withUpdates, snap, 'about', { update: upd({ Err: 'check_failed' }) }), 'update').error).toBe("Couldn't check for updates. Try again later.")
  const en = row(settingsModel(withUpdates, snap, 'about', { update: upd({ Latest: '1.2.0', Available: true }), canInstall: true }), 'update')
  expect(en.label).toBe('Version 1.2.0 is available.')
  expect(en.actions?.map(a => a.label)).toEqual(['Update', 'Download', 'Check now'])
})

// Fix round 1: in a simulated run the button says the check is off there.
test('about: the manual check in a simulated run', () => {
  const r = row(settingsModel(withUpdates, snap, 'about', { update: upd({ Err: 'simulated' }) }), 'update')
  expect(r.error).toBe('Перевірка оновлень вимкнена в імітації.')
  setLocale('en')
  expect(row(settingsModel(withUpdates, snap, 'about', { update: upd({ Err: 'simulated' }) }), 'update').error).toBe('Update checks are off in a simulated run.')
})

// Task 5: «Оновити» runs — the phase text and the progress bar, then a
// failure's reason with «Оновити» and «Завантажити» still offered.
test('about: the one-click update phases and its failures', () => {
  const at = (o: Partial<UpdateView>) => row(settingsModel(withUpdates, snap, 'about',
    { update: upd({ Latest: '1.2.0', Available: true, PageURL: 'https://github.com/x', ...o }), canInstall: true }), 'update')
  const dl = at({ Phase: 'downloading', Progress: 0.426 })
  expect(dl.label).toBe('Завантажуємо 1.2.0… 43%')
  expect(dl.progress).toBeCloseTo(0.426)
  expect(dl.actions).toEqual([
    { act: 'install', label: 'Оновити', primary: true, disabled: true },
    { act: 'download', label: 'Завантажити' },
  ])
  expect(dl.error).toBeUndefined()
  expect(at({ Phase: 'verifying', Progress: 1 }).label).toBe('Перевіряємо файл…')
  expect(at({ Phase: 'backingUp', Progress: 1 }).label).toBe('Зберігаємо копію бази даних…')
  expect(at({ Phase: 'starting', Progress: 1 })).toMatchObject({ label: 'Запускаємо інсталятор. Програма закриється й відкриється знову.', progress: 1 })

  const reasons: Record<string, string> = {
    download: 'Не вдалося завантажити оновлення. Спробуйте ще раз або завантажте зі сторінки релізу.',
    checksum: 'Завантажений файл не збігся з опублікованою сумою SHA-256, тому його видалено. Спробуйте завантажити ще раз.',
    backup: 'Не вдалося зберегти копію бази даних, тому оновлення не встановлено. Спробуйте ще раз або завантажте зі сторінки релізу.',
    start: 'Не вдалося запустити інсталятор. Спробуйте ще раз або завантажте зі сторінки релізу.',
  }
  for (const [code, text] of Object.entries(reasons)) {
    const r = at({ Err: code })
    expect(r.error).toBe(text)
    expect(r.progress).toBeUndefined()
    expect(r.label).toBe('Доступна версія 1.2.0.')
    expect(r.actions?.map(a => [a.act, !!a.disabled])).toEqual([['install', false], ['download', false], ['check', false]])
  }
  setLocale('en')
  expect(at({ Phase: 'downloading', Progress: 0.5 }).label).toBe('Downloading 1.2.0… 50%')
  expect(at({ Err: 'checksum' }).error).toBe("The downloaded file didn't match the published SHA-256 checksum, so it was deleted. Try downloading again.")
  expect(at({ Phase: 'backingUp', Progress: 1 }).label).toBe('Backing up the database…')
})

// macOS port, Task 2 (Review Focus 5): on a Mac no text of Settings names
// Windows, the tray, the firewall or UAC.
const WINDOWS_WORDS = /Windows|тре[йюї]|tray|брандмауер|firewall|UAC/i
/** Every text a value shows: its strings, without ids, keys and links. */
function shown(x: unknown, out: string[] = []): string[] {
  if (typeof x === 'string') out.push(x)
  else if (Array.isArray(x)) x.forEach(v => shown(v, out))
  else if (x && typeof x === 'object') {
    for (const [k, v] of Object.entries(x)) if (!['id', 'parent', 'act', 'icon', 'href', 'value', 'key'].includes(k)) shown(v, out)
  }
  return out
}

test('TestNoWindowsWordOnMac: settings', () => {
  const mac = { ...view, Platform: 'darwin', DataPath: '/Users/me/Library/Application Support/MarstekMonitor' } as SettingsView
  const avail = { Available: true, Latest: '1.1.0' } as UpdateView
  for (const l of ['uk', 'en'] as const) {
    setLocale(l)
    const texts = [
      ...SECTIONS.flatMap(sec => shown(settingsModel(mac, snap, sec, { update: avail, canInstall: true }))),
      ...shown(settingsModel(mac, snap, 'notifications', { notificationsOff: true })),
      ...notifyTestSent('darwin'),
    ]
    expect(texts.filter(s => WINDOWS_WORDS.test(s))).toEqual([])
  }
})

test('on Windows the settings texts stay the Windows ones', () => {
  expect(row(settingsModel(view, snap, 'general'), 'autostart').label).toBe('Запускати з Windows')
  expect(notifyTestSent('windows')).toEqual(["Надіслано. Не з'явилось?", 'Відкрити налаштування сповіщень Windows'])
  const mac = { ...view, Platform: 'darwin' } as SettingsView
  expect(row(settingsModel(mac, snap, 'general'), 'autostart').label).toBe('Відкривати під час входу в систему')
  expect(row(settingsModel(mac, snap, 'stations'), 'trayShows').label).toBe('У рядку меню показується: Дім')
  expect(notifyTestSent('darwin')).toEqual(["Надіслано. Не з'явилось?", 'Відкрити налаштування сповіщень macOS'])
})
