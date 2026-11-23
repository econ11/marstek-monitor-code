import { afterEach, expect, test } from 'vitest'
import type { Entry } from '../../bindings/github.com/econ11/marstek-monitor-code/core/journal/models'
import { setLocale } from '../lib/i18n'
import { compareEntries, errorsModel, seenUpTo } from './errorsModel'

afterEach(() => setLocale('uk'))

const M = 60e3
const at = (h: number, m = 0, day = 21) => new Date(2026, 10, day, h, m, 0).getTime()
const iso = (ms: number) => new Date(ms).toISOString()
const now = at(15, 17)
const obs = at(9, 0, 17)
const seenAt = at(12, 0)

function entry(id: string, when: number, level: string, group: string, key: string, todo = '', args: Record<string, string> | null = null): Entry {
  return { ID: id, At: iso(when), Level: level, Group: group, Key: key, TodoKey: todo, Args: args, Code: '' } as unknown as Entry
}

// Newest first, as MonitorService.Journal returns them.
const entries: Entry[] = [
  entry('conn.ipChanged:7', at(14, 30), 'info', 'connection', 'conn.event.ipChanged', '', { ip: '192.168.1.20' }),
  entry('conn.apiSilent:5', at(14, 0), 'warning', 'connection', 'conn.event.apiSilent', 'conn.event.apiSilent.todo'),
  entry('station.lowCharge:6', at(13, 0), 'warning', 'station', 'journal.lowCharge', 'journal.lowCharge.todo', { soc: '19' }),
  entry('conn.lanRestored:4', at(2, 40, 19), 'info', 'connection', 'conn.event.lanRestored', '', { duration_ms: String(80 * M) }),
  entry('app.gap:3', at(1, 20, 19), 'info', 'app', 'journal.appOff', '', { start: iso(at(1, 20, 19)), end: iso(at(2, 40, 19)) }),
  entry('station.likelyOut:2', at(12, 25, 18), 'warning', 'station', 'journal.likelyOut', 'journal.likelyOut.todo', { at: iso(at(12, 20, 18)), soc: '11' }),
]
const restored = entry('conn.lanRestored:9', at(14, 0), 'info', 'connection', 'conn.event.lanRestored', '', { duration_ms: String(6 * M) })

test('filters and counts', () => {
  const m = errorsModel(entries, 'connection', 0, obs, now)
  expect(m.filters.find(f => f.key === 'error')!.disabled).toBe(true)
  expect(m.rows.every(r => ['conn', 'app'].some(p => r.id.startsWith(p)))).toBe(true)
  expect(m.filters.map(f => [f.key, f.label, f.count, f.disabled])).toEqual([
    ['all', 'усі', 6, false],
    ['error', 'помилки', 0, true],
    ['warning', 'попередження', 3, false],
    ['connection', "зв'язок", 4, false],
  ])
  expect(m.rows).toHaveLength(4)
  expect(errorsModel(entries, 'warning', 0, obs, now).rows.map(r => r.id)).toEqual(['conn.apiSilent:5', 'station.lowCharge:6', 'station.likelyOut:2'])
})

test('the errors filter is off: the station sends no error log over LAN', () => {
  const m = errorsModel(entries, 'all', 0, obs, now)
  const text = 'Станція не передає журнал помилок через LAN. Тут поки лише події зв\'язку додатка.'
  expect(m.strip).toBe(text)
  expect(m.filters.find(f => f.key === 'error')!.tip).toBe(text)
  expect(m.filters.filter(f => f.key !== 'error').every(f => f.tip === undefined)).toBe(true)
})

test('new since seen', () => expect(errorsModel(entries, 'all', seenAt, obs, now).rows.filter(r => r.isNew).length).toBe(2))

test('only warnings and errors are new, as the badge counts them', () => {
  const rows = errorsModel(entries, 'all', seenAt, obs, now).rows
  expect(rows.filter(r => r.isNew).map(r => r.id)).toEqual(['conn.apiSilent:5', 'station.lowCharge:6'])
  // Never seen: every warning is new.
  expect(errorsModel(entries, 'all', 0, obs, now).rows.filter(r => r.isNew)).toHaveLength(3)
})

test('restored row has duration', () => expect(errorsModel([restored], 'all', 0, obs, now).rows[0].what).toBe('Зв\'язок по локальній мережі відновлено (не було 6 хв)'))

test('rows: when, level, icon, what and advice', () => {
  const rows = errorsModel(entries, 'all', seenAt, obs, now).rows
  expect(rows[0]).toMatchObject({ when: '14:30', whenSub: 'сьогодні', level: 'info', levelLabel: 'інфо', icon: 'network', what: 'Станція змінила адресу: тепер 192.168.1.20', todo: '' })
  expect(rows[1]).toMatchObject({
    level: 'warning', levelLabel: 'попередження', icon: 'wifi-off', what: 'Локальний API станції перестав відповідати',
    todo: 'Це відома проблема деяких прошивок. Додаток спробує знову; допоможе перезапуск станції.', isNew: true,
  })
  expect(rows[2]).toMatchObject({ icon: 'alert', what: 'Низький заряд — 19%', todo: 'Зменште навантаження, щоб заряду вистачило довше.' })
  expect(rows[3]).toMatchObject({ when: '02:40', whenSub: 'чт 19.11', icon: 'wifi', what: "Зв'язок по локальній мережі відновлено (не було 1 год 20 хв)" })
  expect(rows[4]).toMatchObject({ when: '01:20', icon: 'clock', what: 'Додаток не працював з 01:20 до 02:40', isNew: false })
  expect(rows[5]).toMatchObject({ whenSub: 'ср 18.11', what: 'Станція, ймовірно, розрядилась ≈ о 12:20 (заряд перед цим 11%)' })
})

test('a time on another day than the row carries its date', () => {
  const night = entry('app.gap:8', at(23, 0, 20), 'info', 'app', 'journal.pcSleep', '', { start: iso(at(23, 0, 20)), end: iso(at(7, 30)) })
  expect(errorsModel([night], 'all', 0, obs, now).rows[0]).toMatchObject({ whenSub: 'вчора', icon: 'moon', what: 'ПК спав з 23:00 до 21.11 07:30' })
})

test('missing or bad args do not break a row', () => {
  const bare = entry('station.cutoff:1', at(14, 0), 'warning', 'station', 'journal.cutoff', 'journal.cutoff.todo', null)
  const row = errorsModel([bare], 'all', 0, obs, now).rows[0]
  expect(row.what).toBe('Заряд дійшов до нижньої межі {soc}% — станція, ймовірно, вимкнула вихід')
  expect(row.todo).toBe('Зменште навантаження або дочекайтесь світла.')
  const bad = entry('conn.lanRestored:2', at(14, 0), 'info', 'connection', 'conn.event.lanRestored', '', { duration_ms: 'x' })
  expect(errorsModel([bad], 'all', 0, obs, now).rows[0].what).toBe("Зв'язок по локальній мережі відновлено (не було {duration})")
})

test('args are kept as text: the view escapes them', () => {
  const ip = entry('conn.ipChanged:3', at(14, 0), 'info', 'connection', 'conn.event.ipChanged', '', { ip: '<img src=x onerror=alert(1)>' })
  expect(errorsModel([ip], 'all', 0, obs, now).rows[0].what).toBe('Станція змінила адресу: тепер <img src=x onerror=alert(1)>')
})

test('empty filter', () => expect(errorsModel([], 'warning', 0, obs, now).empty).toBe('Немає подій цього типу. Показати всі'))

test('empty journal', () => {
  const m = errorsModel([], 'all', 0, obs, now)
  expect(m.empty).toBe('За цей час станція не повідомляла про помилки.')
  expect(m.rows).toEqual([])
  expect(errorsModel(entries, 'all', 0, obs, now).empty).toBeUndefined()
  // The errors filter can't be picked, but an empty one says so all the same.
  expect(errorsModel(entries, 'error', 0, obs, now).empty).toBe('Немає подій цього типу. Показати всі')
})

test('hint: new rows of this visit, and since when the app records', () => {
  expect(errorsModel(entries, 'all', seenAt, obs, now).hint).toBe('2 нові · записуємо з вт 17.11')
  expect(errorsModel(entries, 'all', now, obs, now).hint).toBe('Нових немає · записуємо з вт 17.11')
  expect(errorsModel([], 'all', 0, null, now).hint).toBe('Нових немає')
})

test('en', () => {
  setLocale('en')
  const m = errorsModel(entries, 'all', seenAt, obs, now)
  expect(m.strip).toBe("The station doesn't send its error log over LAN. Only the app's connection events are shown for now.")
  expect(m.filters.map(f => f.label)).toEqual(['all', 'errors', 'warnings', 'connection'])
  expect(m.rows[1]).toMatchObject({ whenSub: 'today', levelLabel: 'warning', what: "The station's local API stopped responding" })
  expect(m.rows[3]).toMatchObject({ whenSub: 'Thu 19', what: 'Local connection restored (lost for 1 h 20 min)' })
  expect(m.rows[5].what).toBe('The station probably ran out at ≈ 12:20 (charge before that 11%)')
  expect(m.hint).toBe('2 new · recording since Tue 17')
})

test('seen up to the newest entry, wherever it is in the list', () => {
  expect(seenUpTo(entries)).toBe(iso(at(14, 30)))
  expect(seenUpTo([...entries].reverse())).toBe(iso(at(14, 30)))
  expect(seenUpTo([restored, entries[5]])).toBe(iso(at(14, 0)))
  expect(seenUpTo([])).toBeNull()
})

test('a short link drop: seconds under a minute, else the short form, times to the second', () => {
  const drop = (secs: number) => entry('conn.linkDropped:1', at(12, 0, 21) + secs * 1000, 'info', 'connection', 'journal.linkDropped', '',
    { from: iso(at(12, 0) + 10e3), to: iso(at(12, 0) + 10e3 + secs * 1000), seconds: String(secs) })
  expect(errorsModel([drop(40)], 'all', 0, obs, now).rows[0].what).toBe('Станція не відповідала 40 с (12:00:10–12:00:50)')
  expect(errorsModel([drop(150)], 'all', 0, obs, now).rows[0].what).toBe('Станція не відповідала 2 хв (12:00:10–12:02:40)')
  setLocale('en')
  expect(errorsModel([drop(40)], 'all', 0, obs, now).rows[0].what).toBe("The station didn't respond for 40 s (12:00:10–12:00:50)")
  expect(errorsModel([drop(150)], 'all', 0, obs, now).rows[0].what).toBe("The station didn't respond for 2 min (12:00:10–12:02:40)")
  // Info: never «new».
  expect(errorsModel([drop(40)], 'all', 0, obs, now).rows[0].isNew).toBe(false)
})

test('filter chips show the counts of the whole period, rows what is loaded', () => {
  const counts = { All: 450, Errors: 0, Warnings: 45, Connection: 450 }
  const m = errorsModel(entries, 'warning', 0, obs, now, { counts, more: true })
  expect(m.filters.map(f => [f.key, f.count])).toEqual([['all', 450], ['error', 0], ['warning', 45], ['connection', 450]])
  expect(m.rows).toHaveLength(3)
  expect(m.more).toBe(true)
  expect(m.showing).toBe('Показано 3 з 45')
})

test('no more pages: no button, no «shown of»', () => {
  const m = errorsModel(entries, 'all', 0, obs, now, { counts: { All: 6, Errors: 0, Warnings: 3, Connection: 4 }, more: false })
  expect(m.more).toBe(false)
  expect(m.showing).toBe('')
})

test('paging copy in en', () => {
  setLocale('en')
  const m = errorsModel(entries, 'all', 0, obs, now, { counts: { All: 450, Errors: 0, Warnings: 45, Connection: 450 }, more: true })
  expect(m.showing).toBe('Showing 6 of 450')
})

test('the «new» hint is the core count over the whole period when the page has it', () => {
  const m = errorsModel(entries, 'all', seenAt, obs, now, { counts: { All: 450, Errors: 0, Warnings: 45, Connection: 450, New: 7 }, more: true })
  expect(m.hint).toBe('7 нових · записуємо з вт 17.11')
})

test('compareEntries: newest first, the same instant by number within a code', () => {
  const a = entry('conn.ipChanged:10', at(12), 'info', 'connection', 'k')
  const b = entry('conn.ipChanged:9', at(12), 'info', 'connection', 'k')
  const c = entry('conn.lanRestored:2', at(12), 'info', 'connection', 'k')
  const d = entry('app.gap:1', at(13), 'info', 'app', 'k')
  expect([b, a, d, c].sort(compareEntries).map(e => e.ID)).toEqual(['app.gap:1', 'conn.lanRestored:2', 'conn.ipChanged:10', 'conn.ipChanged:9'])
})

// Task 5 fix: the restart gap of the one-click update.
test('an app update gap', () => {
  const upd = entry('app.gap:9', at(10, 0), 'info', 'app', 'journal.appUpdate', '', { start: iso(at(10, 0)), end: iso(at(10, 2)) })
  expect(errorsModel([upd], 'all', 0, obs, now).rows[0]).toMatchObject({ icon: 'download', what: 'Додаток оновлювався з 10:00 до 10:02' })
  expect(errorsModel([upd], 'connection', 0, obs, now).rows).toHaveLength(1)
})
