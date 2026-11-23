import { afterEach, expect, test } from 'vitest'
import type { Device, Diagnosis } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import { setLocale } from '../lib/i18n'
import {
  FR_STATES, firstRunModel, firstRunStep, guessPlatform, probeCode, readmeURL, toastNavigation, validIPv4,
  type FRContext, type FREvent, type FRState,
} from './firstRunModel'

afterEach(() => setLocale('uk'))

const d: Device = { Model: 'VenusE 3.0', ModelCode: 'VNSE3-0', IP: '192.168.1.47', BLEMac: '0a1b2c3d4e5f', Firmware: 151, Tested: true, Added: false }
const dx: Diagnosis = { PublicNetwork: false, FirewallMayBlock: false, NoLAN: false, Cancelled: false }
const ctx: FRContext = { devices: [] }

test('scan with no devices ends in noneFound', () => expect(firstRunStep('discovering', { type: 'scanDone', dx }, 0)).toBe('noneFound'))

test('device during scan stays discovering, then found', () => {
  expect(firstRunStep('discovering', { type: 'device', d }, 1)).toBe('discovering')
  expect(firstRunStep('discovering', { type: 'scanDone', dx }, 1)).toBe('found')
})

test('probe results', () => {
  expect(firstRunStep('probingIp', { type: 'probe', result: { error: 'noAnswer' } }, 0)).toBe('ipNoAnswer')
  expect(firstRunStep('probingIp', { type: 'probe', result: { error: 'notMarstek' } }, 0)).toBe('manualIp')
  expect(firstRunStep('probingIp', { type: 'probe', result: { error: 'otherSubnet' } }, 0)).toBe('ipNoAnswer')
  expect(firstRunStep('probingIp', { type: 'probe', result: { error: 'format' } }, 0)).toBe('manualIp')
  expect(firstRunStep('probingIp', { type: 'probe', result: d }, 0)).toBe('adding')
})

test('a cancelled probe shows nothing: back to the field, no error state', () => {
  expect(firstRunStep('probingIp', { type: 'probe', result: { error: 'cancelled' } }, 0)).toBe('manualIp')
  const v = firstRunModel('manualIp', { devices: [], ip: '192.168.1.47', ipError: 'cancelled' })
  expect(v.ipForm?.error).toBeUndefined()
  expect(v.ipForm?.extra).toBeUndefined()
})

test('a cancelled scan.done is ignored (ProbeIP or the handover stopped the scan)', () => {
  const cancelled = { ...dx, Cancelled: true }
  expect(firstRunStep('discovering', { type: 'scanDone', dx: cancelled }, 0)).toBe('discovering')
  expect(firstRunStep('probingIp', { type: 'scanDone', dx: cancelled }, 0)).toBe('probingIp')
  // A late scan.done never pulls the screen out of another state.
  expect(firstRunStep('manualIp', { type: 'scanDone', dx }, 0)).toBe('manualIp')
})

test('a probe answer after the user moved on changes nothing', () => {
  expect(firstRunStep('discovering', { type: 'probe', result: d }, 0)).toBe('discovering')
  expect(firstRunStep('manualIp', { type: 'probe', result: { error: 'noAnswer' } }, 0)).toBe('manualIp')
})

test('a typed address is checked before it is sent', () => {
  expect(firstRunStep('manualIp', { type: 'submitIp', ip: '192.168.1' }, 0)).toBe('manualIp')
  expect(firstRunStep('manualIp', { type: 'submitIp', ip: ' 192.168.1.47 ' }, 0)).toBe('probingIp')
  expect(firstRunStep('ipNoAnswer', { type: 'submitIp', ip: '10.0.0.5' }, 0)).toBe('probingIp')
  for (const bad of ['', 'abc', '192.168.1.256', '1.2.3.4.5', '01.2.3.4', '0.0.0.0', '255.255.255.255', '192.168.1.255', '192.168.1.0', '224.0.0.1', '1.2.3.-4'])
    expect(validIPv4(bad), bad).toBe(false)
  for (const good of ['192.168.1.47', '10.0.0.5', '172.16.4.1', '127.0.0.1'])
    expect(validIPv4(good), good).toBe(true)
})

test('the tiles switch the method from every state but adding and done', () => {
  for (const st of ['found', 'noneFound', 'lanGuide', 'ipNoAnswer', 'manualIp', 'probingIp'] as FRState[])
    expect(firstRunStep(st, { type: 'rescan' }, 0), st).toBe('discovering')
  for (const st of ['discovering', 'found', 'noneFound', 'lanGuide'] as FRState[])
    expect(firstRunStep(st, { type: 'enterIp' }, 0), st).toBe('manualIp')
  expect(firstRunStep('adding', { type: 'rescan' }, 1)).toBe('adding')
  expect(firstRunStep('done', { type: 'enterIp' }, 1)).toBe('done')
})

test('the guide goes back where it came from', () => {
  expect(firstRunStep('noneFound', { type: 'howTo' }, 0)).toBe('lanGuide')
  expect(firstRunStep('found', { type: 'howTo' }, 1)).toBe('lanGuide')
  expect(firstRunStep('ipNoAnswer', { type: 'howTo' }, 0)).toBe('lanGuide')
  expect(firstRunStep('lanGuide', { type: 'back' }, 0)).toBe('noneFound')
  expect(firstRunStep('lanGuide', { type: 'back' }, 1)).toBe('found')
})

test('the guide goes back to the state it was opened from', () => {
  expect(firstRunStep('lanGuide', { type: 'back' }, 0, 'ipNoAnswer')).toBe('ipNoAnswer')
  expect(firstRunStep('lanGuide', { type: 'back' }, 1, 'found')).toBe('found')
  expect(firstRunStep('lanGuide', { type: 'back' }, 0, 'noneFound')).toBe('noneFound')
  // Rows gone since (a rescan found none): not back to an empty «found».
  expect(firstRunStep('lanGuide', { type: 'back' }, 0, 'found')).toBe('noneFound')
  // Only a state the guide can be opened from.
  expect(firstRunStep('lanGuide', { type: 'back' }, 0, 'adding')).toBe('noneFound')
})

test('an Add refused after the 20 s timeout takes DONE back (no false success)', () => {
  expect(firstRunStep('done', { type: 'addFailed' }, 1)).toBe('found')
  expect(firstRunStep('done', { type: 'addFailed' }, 0)).toBe('manualIp')
})

test('a toast click during the first run', () => {
  // A toast needs a running monitor: the station is stored. Leave the first
  // run (header back), then open the toast's tab.
  expect(toastNavigation('done')).toBe('leave')
  expect(toastNavigation('adding')).toBe('leave')
  // No station yet: nothing a toast could be about; stay.
  expect(toastNavigation('noneFound')).toBe('ignore')
  expect(toastNavigation('discovering')).toBe('ignore')
  // Not in the first run: the usual navigation.
  expect(toastNavigation(null)).toBe('go')
})

test('adding ends in done on the first sample or after 20 s; a refused add goes back', () => {
  expect(firstRunStep('found', { type: 'add', d }, 1)).toBe('adding')
  expect(firstRunStep('adding', { type: 'firstSample' }, 1)).toBe('done')
  expect(firstRunStep('adding', { type: 'addTimeout' }, 1)).toBe('done')
  expect(firstRunStep('adding', { type: 'addFailed' }, 1)).toBe('found')
  expect(firstRunStep('adding', { type: 'addFailed' }, 0)).toBe('manualIp')
  expect(firstRunStep('ipNoAnswer', { type: 'retry' }, 0)).toBe('probingIp')
  expect(firstRunStep('ipNoAnswer', { type: 'editIp' }, 0)).toBe('manualIp')
})

test('every state has an exit', () => {
  // flows §10 «без тупиків»: each state has an event that leads elsewhere.
  // done's exit is the screen's «Перейти до огляду», not a state.
  const events: FREvent[] = [
    { type: 'device', d }, { type: 'scanDone', dx }, { type: 'howTo' }, { type: 'rescan' }, { type: 'enterIp' }, { type: 'back' },
    { type: 'editIp' }, { type: 'retry' }, { type: 'submitIp', ip: '192.168.1.47' }, { type: 'probe', result: d },
    { type: 'add', d }, { type: 'firstSample' }, { type: 'addTimeout' }, { type: 'addFailed' },
  ]
  for (const st of FR_STATES) {
    if (st === 'done') {
      expect(firstRunModel('done', { devices: [d], added: d }).actions.map(a => a.key)).toContain('overview')
      continue
    }
    const exits = events.flatMap(ev => [0, 1].map(n => firstRunStep(st, ev, n))).filter(to => to !== st)
    expect(exits.length, st).toBeGreaterThan(0)
  }
})

test('none found primary action is the guide in v1', () => {
  const v = firstRunModel('noneFound', ctx)
  expect(v.actions[0].key).toBe('howTo')
  expect(v.actions[0].kind).toBe('primary')
  expect(v.actions.map(a => a.key)).toEqual(['howTo', 'enterIp', 'rescan'])
  expect(v.actions[2].kind).toBe('link')
  expect(v.head?.title).toBe('Станцій у локальній мережі не знайдено')
})

test('the diagnosis blocks, the firewall button and its pending state', () => {
  const all = { PublicNetwork: true, FirewallMayBlock: true, NoLAN: true, Cancelled: false }
  const v = firstRunModel('noneFound', { devices: [], dx: all })
  expect(v.dx.map(x => x.key)).toEqual(['noLan', 'netPublic', 'firewall'])
  expect(v.dx[0].body).toBe('Підключіть його до того самого Wi-Fi, що й станцію.')
  expect(v.dx[1].action).toMatchObject({ key: 'netSettings', label: 'Як це зробити' })
  expect(v.dx[2].action).toMatchObject({ key: 'firewall', label: 'Дозволити (потрібні права адміністратора)' })
  expect(v.dx[2].action?.busy).toBeFalsy()
  const pending = firstRunModel('noneFound', { devices: [], dx: all, firewall: 'pending' })
  expect(pending.dx[2].action?.busy).toBe(true)
  expect(pending.dx[2].note).toBeTruthy()
  expect(firstRunModel('noneFound', { devices: [], dx: all, firewall: 'failed' }).dx[2].error).toBeTruthy()
  expect(firstRunModel('noneFound', { devices: [], dx }).dx).toEqual([])
})

test('found rows: the row text, untested and already added', () => {
  const untested = { ...d, BLEMac: 'aa', IP: '192.168.1.48', Tested: false }
  const added = { ...d, BLEMac: 'bb', IP: '192.168.1.49', Added: true }
  const v = firstRunModel('found', { devices: [d, untested, added] })
  expect(v.rows.map(r => r.text)).toEqual([
    'VenusE 3.0 · 192.168.1.47 · VNSE3-0 · прошивка v151',
    'VenusE 3.0 · 192.168.1.48 · VNSE3-0 · прошивка v151',
    'VenusE 3.0 · 192.168.1.49 · VNSE3-0 · прошивка v151',
  ])
  expect(v.rows.map(r => r.untested)).toEqual([undefined, 'може працювати, не тестувалося', undefined])
  expect(v.rows.map(r => r.action.key)).toEqual(['add', 'add', 'open'])
  expect(v.rows.map(r => r.action.label)).toEqual(['Додати', 'Додати', 'Відкрити'])
  expect(v.rows[2].added).toBe('вже додано')
  expect(v.rowsTitle).toBe('Знайдено в мережі:')
  expect(v.notListed?.action.key).toBe('howTo')
  expect(v.announce).toContain('VenusE 3.0 · 192.168.1.47')
})

test('a model code that is only the device string again is not repeated', () => {
  const v = firstRunModel('found', { devices: [{ ...d, ModelCode: d.Model }] })
  expect(v.rows[0].text).toBe('VenusE 3.0 · 192.168.1.47 · прошивка v151')
})

test('during the scan the rows come at once, with the progress line and no button yet', () => {
  const v0 = firstRunModel('discovering', ctx)
  expect(v0.progress).toBe('Шукаємо станції у вашому Wi-Fi…')
  expect(v0.rows).toEqual([])
  const v1 = firstRunModel('discovering', { devices: [d] })
  expect(v1.rows).toHaveLength(1)
  expect(v1.rows[0].action.disabled).toBe(true)
  expect(v1.tiles?.find(x => x.checked)?.key).toBe('lan')
})

test('the IP errors', () => {
  const ip = '192.168.1.60'
  const m = (st: FRState, e: FRContext['ipError']) => firstRunModel(st, { devices: [], ip, ipError: e }).ipForm!
  expect(m('manualIp', 'format').error).toBe('Введіть адресу у форматі 192.168.1.47.')
  expect(m('manualIp', 'notMarstek').error).toBe('За адресою 192.168.1.60 відповів пристрій, але це не станція Marstek.')
  expect(m('ipNoAnswer', 'noAnswer').error).toBe('За адресою 192.168.1.60 станція не відповідає. Перевірте адресу і чи ввімкнено на станції локальний API.')
  expect(m('ipNoAnswer', 'noAnswer').extra).toBeUndefined()
  expect(m('ipNoAnswer', 'otherSubnet').error).toBe(m('ipNoAnswer', 'noAnswer').error)
  expect(m('ipNoAnswer', 'otherSubnet').extra).toBe('Станція може бути в іншій підмережі (VPN, другий роутер). Спробуйте ввести її IP вручну.')
  expect(m('probingIp', undefined).busy).toBe('Перевіряємо зв\'язок зі станцією…')
  expect(firstRunModel('ipNoAnswer', { devices: [], ip, ipError: 'noAnswer' }).actions.map(a => a.key)).toEqual(['retry', 'howTo'])
  expect(firstRunModel('manualIp', { devices: [], ip: '' }).tiles?.find(x => x.checked)?.key).toBe('ip')
})

test('the guide: the help texts, the support letter with a placeholder serial, no cloud block', () => {
  const v = firstRunModel('lanGuide', ctx)
  expect(v.head?.title).toBe('Як увімкнути локальний API')
  expect(v.steps).toEqual([
    'Станція і цей комп\'ютер мають бути в одній мережі Wi-Fi.',
    'Якщо Windows запитає дозвіл для Marstek Monitor, дозвольте доступ у приватних мережах.',
    'Після ввімкнення натисніть «Шукати знову» — тут же, нікуди не переходячи.',
  ])
  expect(v.template?.body).toContain('[серійний номер]')
  expect(v.actions.map(a => a.key)).toEqual(['rescan', 'readme', 'copyTemplate'])
  expect(v.back).toBe(true)
  setLocale('en')
  expect(firstRunModel('lanGuide', { devices: [d] }).template?.body).toBe(
    'Hello, please enable the local API (Open API) on my VenusE 3.0 station, serial number [serial number]. Thank you.')
})

test('done: the title, the summary with the first data, the name, the switches', () => {
  const base: FRContext = { devices: [d], added: d, name: 'VenusE 3.0', settings: { autostart: true, minimizeToTray: false }, trayHint: true }
  const v = firstRunModel('done', { ...base, first: { soc: 72.4, grid: 'off', offFor: 3 * 3600e3 + 27 * 60e3 } })
  expect(v.head?.title).toBe('Станцію «VenusE 3.0» додано')
  expect(v.head?.body).toBe('VenusE 3.0 · 192.168.1.47 · LAN. Перші дані: 72%, без світла 3 год 27 хв.')
  expect(v.done?.name.value).toBe('VenusE 3.0')
  expect(v.done?.toggles).toEqual([
    { key: 'autostart', label: 'Запускати з Windows', on: true },
    { key: 'minimizeToTray', label: 'Згортати в трей при закритті', on: false },
  ])
  expect(v.done?.trayHint).toBeTruthy()
  expect(v.tiles).toBeNull()
  expect(v.actions.map(a => [a.key, a.kind])).toEqual([['overview', 'primary']])
  expect(v.announce).toBe('Станцію «VenusE 3.0» додано.')
  expect(firstRunModel('done', { ...base, first: { soc: 50, grid: 'on' } }).head?.body).toContain('Перші дані: 50%, світло є.')
  expect(firstRunModel('done', { ...base, first: { soc: 50, grid: 'unknown' } }).head?.body).toContain('світло: невідомо')
  expect(firstRunModel('done', { ...base, first: null }).head?.body).toBe('VenusE 3.0 · LAN. Чекаємо перших даних — це може зайняти до хвилини.')
  expect(firstRunModel('done', { ...base, trayHint: false }).done?.trayHint).toBeUndefined()
  expect(firstRunModel('done', { ...base, nameError: 'x' }).done?.name.error).toBe('x')
})

// Every view of every state, with every option on, in both languages.
function allStatesViews(): unknown[] {
  const dxAll = { PublicNetwork: true, FirewallMayBlock: true, NoLAN: true, Cancelled: false }
  const ctxs: FRContext[] = [
    { devices: [] },
    { devices: [d, { ...d, BLEMac: 'x', Tested: false, Added: true }], dx: dxAll, firewall: 'pending', copied: true, addError: true },
    { devices: [], dx: dxAll, firewall: 'failed' },
    ...(['format', 'noAnswer', 'notMarstek', 'otherSubnet', 'cancelled'] as const).map(e => ({ devices: [], ip: '192.168.1.5', ipError: e })),
    { devices: [d], added: d, name: 'VenusE 3.0', first: { soc: 10, grid: 'off' as const, offFor: 60e3 }, settings: { autostart: true, minimizeToTray: true }, trayHint: true },
    { devices: [d], added: d, name: 'VenusE 3.0', first: null, settings: null, trayHint: true },
  ]
  return FR_STATES.flatMap(st => ctxs.map(c => firstRunModel(st, c)))
}

test('first run has no cloud', () => {
  expect(JSON.stringify(allStatesViews())).not.toMatch(/хмар|cloud|Акаунт/i)
  setLocale('en')
  expect(JSON.stringify(allStatesViews())).not.toMatch(/хмар|cloud|Акаунт|account/i)
})

test('no text of any state is a missing key', () => {
  for (const l of ['uk', 'en'] as const) {
    setLocale(l)
    // A missing key comes back as itself: «onboarding.x.y».
    expect(JSON.stringify(allStatesViews())).not.toMatch(/"(?:onboarding|addStation|help|state|source|common|settings|ip)\.[\w.]+"/)
  }
})

test('the welcome line and the tiles, without the account tile', () => {
  const v = firstRunModel('noneFound', ctx)
  expect(v.welcome).toBe("Показує, що зараз із зарядною станцією, на скільки її вистачить і як часто не було світла. Історія зберігається на цьому комп'ютері.")
  expect(v.tiles?.map(x => x.key)).toEqual(['lan', 'ip'])
  expect(v.tiles?.map(x => x.title)).toEqual(['Пошук у локальній мережі', 'IP вручну'])
})

test('the ProbeError code comes from the Wails error: cause.Code, else the message', () => {
  // Wails v3: a Go error from a bound method rejects with a RuntimeError
  // whose message is err.Error() and cause the JSON of the error value.
  expect(probeCode(Object.assign(new Error('noAnswer'), { cause: { Code: 'noAnswer' } }))).toBe('noAnswer')
  expect(probeCode(Object.assign(new Error('x'), { cause: { Code: 'otherSubnet' } }))).toBe('otherSubnet')
  expect(probeCode(new Error('notMarstek'))).toBe('notMarstek')
  expect(probeCode(new Error('format'))).toBe('format')
  expect(probeCode(new Error('cancelled'))).toBe('cancelled')
  // Anything else (a lost call): the station did not answer.
  expect(probeCode(new Error('boom'))).toBe('noAnswer')
  expect(probeCode('weird')).toBe('noAnswer')
})

// Final review M8: the guide's link opens the local-API section of the user
// README in the app's language, not the top of the page (the anchors are
// GitHub's slugs of release-repo/README.md's headings).
test('readmeURL opens the local-API section', () => {
  expect(decodeURIComponent(readmeURL())).toBe('https://github.com/econ11/marstek-monitor#як-увімкнути-локальний-api-станції')
  setLocale('en')
  expect(readmeURL()).toBe('https://github.com/econ11/marstek-monitor#enabling-the-stations-local-api')
})

// macOS port, Task 2 (Review Focus 5): on a Mac no text of the first run
// names Windows, the tray, the firewall or UAC. The Mac never reports a
// public network (netdiag_darwin), and its firewall action opens settings.
const WINDOWS_WORDS = /Windows|тре[йюїя]|панел|tray|брандмауер|firewall|UAC|інсталятор|installer/i
function shown(x: unknown, out: string[] = []): string[] {
  if (typeof x === 'string') out.push(x)
  else if (Array.isArray(x)) x.forEach(v => shown(v, out))
  else if (x && typeof x === 'object') for (const [k, v] of Object.entries(x)) if (!['key', 'icon'].includes(k)) shown(v, out)
  return out
}

test('TestNoWindowsWordOnMac: first run', () => {
  const dxMac = { PublicNetwork: false, FirewallMayBlock: true, NoLAN: true, Cancelled: false }
  const ctxs: FRContext[] = [
    { devices: [], platform: 'darwin' },
    { devices: [d], dx: dxMac, copied: true, addError: true, platform: 'darwin' },
    { devices: [], dx: dxMac, firewall: 'pending', platform: 'darwin' },
    { devices: [], dx: dxMac, firewall: 'failed', platform: 'darwin' },
    { devices: [], dx: { ...dxMac, PublicNetwork: true }, platform: 'darwin' },
    { devices: [d], added: d, name: 'VenusE 3.0', settings: { autostart: false, minimizeToTray: false }, trayHint: true, platform: 'darwin' },
    { devices: [d], added: d, name: 'VenusE 3.0', first: { soc: 10, grid: 'off' as const, offFor: 60e3 }, settings: { autostart: true, minimizeToTray: true }, trayHint: true, platform: 'darwin' },
  ]
  for (const l of ['uk', 'en'] as const) {
    setLocale(l)
    const texts = FR_STATES.flatMap(st => ctxs.flatMap(c => shown(firstRunModel(st, c))))
    expect(texts.filter(s => WINDOWS_WORDS.test(s))).toEqual([])
  }
})

test('on a Mac: the firewall block opens settings, no tray tip', () => {
  const v = firstRunModel('noneFound', { devices: [], dx: { ...dx, FirewallMayBlock: true }, platform: 'darwin' })
  expect(v.dx[0]).toMatchObject({ key: 'firewall', title: 'Немає дозволу на локальну мережу', action: { key: 'firewall', label: 'Відкрити налаштування' } })
  const done = firstRunModel('done', { devices: [d], added: d, name: 'VenusE 3.0', settings: { autostart: true, minimizeToTray: true }, trayHint: true, platform: 'darwin' })
  expect(done.done?.trayHint).toBeUndefined()
  expect(done.done?.toggles.map(x => x.label)).toEqual(['Відкривати під час входу в систему', 'Згортати в рядок меню при закритті'])
  expect(firstRunModel('lanGuide', { devices: [], platform: 'darwin' }).steps?.[1]).toBe('Якщо macOS запитає дозвіл на доступ до локальної мережі, натисніть «Дозволити».')
})

test('on a Mac: the firewall note and error name the settings, not a prompt', () => {
  const at = (firewall: FRContext['firewall'], platform?: string) =>
    firstRunModel('noneFound', { devices: [], dx: { ...dx, FirewallMayBlock: true }, firewall, platform }).dx[0]
  expect(at('pending', 'darwin').note).toBe('Відкриваю налаштування…')
  expect(at('failed', 'darwin').error).toBe('Не вдалося відкрити налаштування. Відкрийте «Системні параметри» → «Конфіденційність і безпека» → «Локальна мережа».')
  expect(at('pending').note).toBe('Підтвердьте запит Windows…')
  setLocale('en')
  expect(at('pending', 'darwin').note).toBe('Opening settings…')
  expect(at('failed', 'darwin').error).toBe("Couldn't open settings. Open System Settings → Privacy & Security → Local Network.")
})

test('on a Mac a public network is never shown, even if reported', () => {
  const mac = firstRunModel('noneFound', { devices: [], dx: { ...dx, PublicNetwork: true }, platform: 'darwin' })
  expect(mac.dx.map(x => x.key)).toEqual([])
  const win = firstRunModel('noneFound', { devices: [], dx: { ...dx, PublicNetwork: true }, platform: 'windows' })
  expect(win.dx.map(x => x.key)).toEqual(['netPublic'])
})

// Before getSettings resolves (or when it fails) the webview guesses.
test('guessPlatform: darwin on a Mac webview, else unknown', () => {
  expect(guessPlatform({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel' })).toBe('darwin')
  expect(guessPlatform({ userAgent: 'Mozilla/5.0', platform: 'MacIntel' })).toBe('darwin')
  expect(guessPlatform({ userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Edg/120.0', platform: 'Win32' })).toBe('')
  expect(guessPlatform({})).toBe('')
  expect(guessPlatform(undefined)).toBe('')
})
