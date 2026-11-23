import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import { setLocale } from '../lib/i18n'
import { FakeEl, host, installFakeDom } from '../testing/fakeDom'
import type { Entry } from '../../bindings/github.com/econ11/marstek-monitor-code/core/journal/models'
import { errorsScreen, type ErrorsDeps, type JournalPage } from './errors'
import { fakeDocument } from '../testing/fakeDom'

// The Errors screen on a stand-in DOM (src/testing/fakeDom.ts). What it
// draws is tested in errorsModel.test.ts.

const noDeps: ErrorsDeps = { page: async () => ({ Entries: [], More: false, Counts: { All: 0, Errors: 0, Warnings: 0, Connection: 0 } }), since: async () => null, markSeen: async () => {} }
const now = new Date(2026, 10, 21, 15, 17).getTime()

beforeEach(() => {
  vi.useFakeTimers({ now })
  installFakeDom()
  vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => {
  setLocale('uk')
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

test('a fetch error survives a language switch', async () => {
  const screen = errorsScreen({ ...noDeps, page: () => Promise.reject(new Error("db")), markSeen: async () => {} })
  const root = host()
  screen.mount(root)
  screen.update({ ObservedSince: new Date(now - 864e5).toISOString(), NewEvents: 0, JournalSeenAt: null } as unknown as Snapshot)
  await vi.advanceTimersByTimeAsync(0)
  const slot = (name: string) => (root as unknown as FakeEl).querySelector(`[data-slot="${name}"]`)!
  expect(slot('error').hidden).toBe(false)
  setLocale('en')
  expect(slot('error').hidden).toBe(false)
  expect(slot('loading').hidden).toBe(true)
  expect(root.innerHTML).toContain('Try again')
})

test('the «Рівень» column has its ⓘ; the header keeps its own name', () => {
  const screen = errorsScreen({ ...noDeps, markSeen: async () => {} })
  const root = host()
  screen.mount(root)
  expect(root.innerHTML).toMatch(/<th scope="col" aria-label="Рівень">Рівень<button type="button" class="infotip" data-info="info.errors.level"/)
})

// Entries newest first: id n at now - n minutes.
const mk = (n: number): Entry => ({ ID: `conn.lanRestored:${n}`, At: new Date(now - n * 6e4).toISOString(), Level: 'info', Group: 'connection', Key: 'conn.event.lanRestored', TodoKey: '', Args: null, Code: '' } as unknown as Entry)
const range = (a: number, b: number) => Array.from({ length: b - a }, (_, i) => mk(a + i))
const snap = { ObservedSince: new Date(now - 864e5).toISOString(), NewEvents: 0, JournalSeenAt: null } as unknown as Snapshot
const counts = { All: 450, Errors: 0, Warnings: 0, Connection: 450 }
const slotOf = (root: HTMLElement, name: string) => (root as unknown as FakeEl).querySelector(`[data-slot="${name}"]`)!
const click = (root: HTMLElement, el: unknown) => (root as unknown as FakeEl).dispatch('click', { target: { closest: () => el } })

test('the first load asks for 200; «Показати ще» appends the next page and keeps focus on the button', async () => {
  const calls: unknown[][] = []
  const pages: Record<string, JournalPage> = {
    '': { Entries: range(1, 201), More: true, Counts: counts },
    [mk(200).At]: { Entries: range(201, 401), More: true, Counts: counts },
  }
  const screen = errorsScreen({
    ...noDeps,
    page: async (...a) => { calls.push(a); return pages[a[2]] },
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  expect(calls).toHaveLength(1)
  expect(calls[0].slice(2)).toEqual(['', '', 200, 'all', ''])
  expect(slotOf(root, 'more').hidden).toBe(false)
  expect(slotOf(root, 'showing').textContent).toBe('Показано 200 з 450')
  expect(slotOf(root, 'rows').innerHTML.match(/<tr /g)).toHaveLength(200)

  // The focus is on the button when it is clicked.
  const btn = new FakeEl()
  btn.dataset.action = 'more'
  fakeDocument.activeElement = btn
  click(root, btn)
  await vi.advanceTimersByTimeAsync(0)
  expect(calls).toHaveLength(2)
  expect(calls[1].slice(2)).toEqual([mk(200).At, mk(200).ID, 200, 'all', ''])
  expect(slotOf(root, 'rows').innerHTML.match(/<tr /g)).toHaveLength(400)
  expect(slotOf(root, 'showing').textContent).toBe('Показано 400 з 450')
  expect(fakeDocument.activeElement).toBe((root as unknown as FakeEl).querySelector('[data-action="more"]'))
})

test('the last page takes the button away: no button, the focus moves to the first new row', async () => {
  const screen = errorsScreen({
    ...noDeps,
    page: async (_f, _t, beforeAt) => beforeAt === ''
      ? { Entries: range(1, 3), More: true, Counts: { ...counts, All: 4, Connection: 4 } }
      : { Entries: range(3, 5), More: false, Counts: { ...counts, All: 4, Connection: 4 } },
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  const btn = new FakeEl()
  btn.dataset.action = 'more'
  fakeDocument.activeElement = btn
  click(root, btn)
  await vi.advanceTimersByTimeAsync(0)
  expect(slotOf(root, 'more').hidden).toBe(true)
  expect(fakeDocument.activeElement).toBe((root as unknown as FakeEl).querySelector(`tr[data-id="${mk(3).ID}"]`))
})

test('the 60 s refresh asks for what is newer than the newest row and puts it on top', async () => {
  const since: unknown[][] = []
  const markSeen = vi.fn(async () => {})
  const screen = errorsScreen({
    page: async () => ({ Entries: range(2, 5), More: false, Counts: { ...counts, All: 3, Connection: 3 } }),
    since: async (...a) => { since.push(a); return since.length === 1 ? { Entries: [mk(1)], Counts: { ...counts, All: 4, Connection: 4 } } : null },
    markSeen,
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  expect(slotOf(root, 'rows').innerHTML.match(/<tr /g)).toHaveLength(3)
  await vi.advanceTimersByTimeAsync(60e3)
  expect(since).toHaveLength(1)
  expect(since[0].slice(2)).toEqual([new Date(now - 2 * 6e4 - 600e3 - 1).toISOString(), '', 'all', ''])
  const html = slotOf(root, 'rows').innerHTML
  expect(html.match(/<tr /g)).toHaveLength(4)
  expect(html.indexOf(mk(1).ID)).toBeLessThan(html.indexOf(mk(2).ID))
  // Only a warning marks again (newEvents); an info row does not: still the first mark.
  expect(markSeen).toHaveBeenCalledTimes(1)
  expect(markSeen).toHaveBeenCalledWith(mk(2).At)
  await vi.advanceTimersByTimeAsync(60e3) // nothing new: no change
  expect(slotOf(root, 'rows').innerHTML.match(/<tr /g)).toHaveLength(4)
})

test('the filter chips show the counts of the whole period', async () => {
  const screen = errorsScreen({
    ...noDeps,
    page: async () => ({ Entries: range(1, 3), More: true, Counts: { All: 450, Errors: 0, Warnings: 45, Connection: 450 } }),
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  // The stand-in DOM keeps the markup of the segmented control as inserted.
  expect(slotOf(root, 'filters').innerHTML).toContain('<span class="n">450</span>')
  expect(slotOf(root, 'filters').innerHTML).toContain('<span class="n">45</span>')
})

const gapAt = (id: number, startAgoMin: number, endAgoMin: number): Entry => ({
  ID: `app.gap:${id}`, At: new Date(now - startAgoMin * 6e4).toISOString(), Level: 'info', Group: 'app', Key: 'journal.pcSleep', TodoKey: '',
  Args: { start: new Date(now - startAgoMin * 6e4).toISOString(), end: new Date(now - endAgoMin * 6e4).toISOString() }, Code: '',
} as unknown as Entry)
const rowIDs = (root: HTMLElement) => Array.from(slotOf(root, 'rows').innerHTML.matchAll(/data-id="([^"]+)"/g), m => m[1])

test('a filter is asked of the server: page 1 of that filter replaces the rows, with the filtered total', async () => {
  const calls: unknown[][] = []
  const warn = (n: number) => ({ ...mk(n), ID: `conn.apiSilent:${n}`, Level: 'warning' }) as Entry
  const screen = errorsScreen({
    ...noDeps,
    page: async (...a) => {
      calls.push(a)
      return a[5] === 'warnings'
        ? { Entries: [warn(300), warn(301)], More: false, Counts: { All: 252, Errors: 0, Warnings: 2, Connection: 252, New: 2 } }
        : { Entries: range(1, 201), More: true, Counts: { All: 252, Errors: 0, Warnings: 2, Connection: 252, New: 2 } }
    },
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  expect(slotOf(root, 'more').hidden).toBe(false)
  // Pick «попередження».
  click(root, { dataset: { filter: 'warning' }, getAttribute: () => null })
  await vi.advanceTimersByTimeAsync(0)
  expect(calls[1].slice(2)).toEqual(['', '', 200, 'warnings', ''])
  expect(rowIDs(root)).toEqual(['conn.apiSilent:300', 'conn.apiSilent:301'])
  expect(slotOf(root, 'more').hidden).toBe(true)
  expect(slotOf(root, 'hint').textContent).toContain('2 нові')
})

test('the refresh re-reads an open gap and a gap stored after a wake, and replaces rows by ID', async () => {
  const since: unknown[][] = []
  const open = gapAt(7, 30, 0) // ends now: still open
  const first = [mk(1), open, mk(40)]
  const screen = errorsScreen({
    ...noDeps,
    page: async () => ({ Entries: first, More: false, Counts: { All: 3, Errors: 0, Warnings: 0, Connection: 3 } }),
    since: async (...a) => {
      since.push(a)
      // the open gap closed; a gap stored after the wake, older than the newest row
      return { Entries: [gapAt(7, 30, 20), gapAt(8, 8, 5)], Counts: { All: 4, Errors: 0, Warnings: 0, Connection: 4 } }
    },
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  await vi.advanceTimersByTimeAsync(60e3)
  // The read starts at the open gap row (30 min), not at the newest.
  expect(since[0][2]).toBe(new Date(now - 30 * 6e4 - 1).toISOString())
  expect(rowIDs(root)).toEqual(['conn.lanRestored:1', 'app.gap:8', 'app.gap:7', 'conn.lanRestored:40'])
  expect(slotOf(root, 'rows').innerHTML).toContain('app.gap:7')
  // A gap that is closed is no longer re-read from its start: next time the window is the newest − 10 min.
  await vi.advanceTimersByTimeAsync(60e3)
  expect(since[1][2]).toBe(new Date(now - 1 * 6e4 - 600e3 - 1).toISOString())
})

test('focus: stays on the same row after a refresh; a page does not steal focus the user moved', async () => {
  const screen = errorsScreen({
    ...noDeps,
    page: async (_f, _t, beforeAt) => beforeAt === ''
      ? { Entries: range(2, 4), More: true, Counts: { All: 4, Errors: 0, Warnings: 0, Connection: 4 } }
      : { Entries: range(4, 6), More: false, Counts: { All: 4, Errors: 0, Warnings: 0, Connection: 4 } },
    since: async () => ({ Entries: [mk(1)], Counts: { All: 5, Errors: 0, Warnings: 0, Connection: 5 } }),
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  // Focus on a row: after the refresh it is on that same row.
  const row = new FakeEl()
  row.dataset.id = mk(3).ID
  fakeDocument.activeElement = row
  await vi.advanceTimersByTimeAsync(60e3)
  expect(fakeDocument.activeElement).toBe((root as unknown as FakeEl).querySelector(`tr[data-id="${mk(3).ID}"]`))
  // The user clicks the button, then moves the focus elsewhere before the page arrives.
  const btn = new FakeEl()
  btn.dataset.action = 'more'
  fakeDocument.activeElement = btn
  click(root, btn)
  const elsewhere = new FakeEl()
  fakeDocument.activeElement = elsewhere
  await vi.advanceTimersByTimeAsync(0)
  expect(fakeDocument.activeElement).toBe(elsewhere)
})

test('«зв\'язок» with 3 pages loaded: a new link-drop row is prepended, pages are kept, counts follow the server', async () => {
  const mkPage = (a: number, b: number, more: boolean): JournalPage => ({ Entries: range(a, b), More: more, Counts: { All: 7, Errors: 0, Warnings: 0, Connection: 7, New: 0 } })
  const pages: Record<string, JournalPage> = { '': mkPage(10, 12, true), [mk(11).At]: mkPage(12, 14, true), [mk(13).At]: mkPage(14, 16, false) }
  let calls = 0
  const screen = errorsScreen({
    ...noDeps,
    page: async (_f, _t, beforeAt) => pages[beforeAt],
    since: async () => (++calls === 1
      ? { Entries: [mk(2)], Counts: { All: 8, Errors: 0, Warnings: 0, Connection: 8, New: 0 } }
      // a non-matching row (another group) only moves the chips
      : { Entries: [], Counts: { All: 9, Errors: 0, Warnings: 0, Connection: 8, New: 0 } }),
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  const btn = new FakeEl()
  btn.dataset.action = 'more'
  for (let i = 0; i < 2; i++) { fakeDocument.activeElement = btn; click(root, btn); await vi.advanceTimersByTimeAsync(0) }
  click(root, { dataset: { filter: 'connection' }, getAttribute: () => null })
  await vi.advanceTimersByTimeAsync(0)
  for (let i = 0; i < 2; i++) { fakeDocument.activeElement = btn; click(root, btn); await vi.advanceTimersByTimeAsync(0) }
  expect(rowIDs(root)).toHaveLength(6)
  const row = new FakeEl()
  row.dataset.id = mk(12).ID
  fakeDocument.activeElement = row
  await vi.advanceTimersByTimeAsync(60e3)
  expect(rowIDs(root)).toHaveLength(7)
  expect(rowIDs(root)[0]).toBe(mk(2).ID)
  expect(fakeDocument.activeElement).toBe((root as unknown as FakeEl).querySelector(`tr[data-id="${mk(12).ID}"]`))
  expect(slotOf(root, 'filters').innerHTML).toContain('<span class="n">8</span>')
  await vi.advanceTimersByTimeAsync(60e3)
  expect(rowIDs(root)).toHaveLength(7)
  expect(slotOf(root, 'filters').innerHTML).toContain('<span class="n">9</span>')
})

test('a failed filter change goes back to the old filter and rows, with a quiet line; the chip retries', async () => {
  let fail = true
  const screen = errorsScreen({
    ...noDeps,
    page: async (_f, _t, _b, _i, _l, filter) => {
      if (filter === 'warnings' && fail) throw new Error('db')
      return { Entries: filter === 'warnings' ? [{ ...mk(5), Level: 'warning' } as Entry] : range(1, 3), More: false, Counts: { All: 2, Errors: 0, Warnings: 1, Connection: 2 } }
    },
  })
  const root = host()
  screen.mount(root)
  screen.update(snap)
  await vi.advanceTimersByTimeAsync(0)
  click(root, { dataset: { filter: 'warning' }, getAttribute: () => null })
  await vi.advanceTimersByTimeAsync(0)
  expect(slotOf(root, 'loaderr').hidden).toBe(false)
  expect(slotOf(root, 'error').hidden).toBe(true)
  expect(rowIDs(root)).toEqual([mk(1).ID, mk(2).ID])
  expect(slotOf(root, 'filters').innerHTML).toContain('data-filter="all"')
  expect(slotOf(root, 'filters').innerHTML).toMatch(/aria-pressed="true" data-filter="all"/)
  fail = false
  click(root, { dataset: { filter: 'warning' }, getAttribute: () => null })
  await vi.advanceTimersByTimeAsync(0)
  expect(slotOf(root, 'loaderr').hidden).toBe(true)
  expect(rowIDs(root)).toEqual([mk(5).ID])
})
