import type { Entry } from '../../bindings/github.com/econ11/marstek-monitor-code/core/journal/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { ymd } from '../ui/calendar'
import { icon } from '../ui/icons'
import { infoTipHTML } from '../ui/infoTip'
import { loadingHTML } from '../ui/loading'
import { syncSeg } from '../ui/period'
import { escapeHTML as esc, hideTip } from '../ui/tooltip'
import { compareEntries, errorsModel, seenUpTo, type ErrorFilter, type ErrorRow, type ErrorsModel, type JournalCounts } from './errorsModel'

// The «Помилки» tab: DOM after hi-fi 03-errors. The journal (from the first
// observed day to today) comes in pages of 200, newest first: the first page
// once the first snapshot arrives, older ones on «Показати ще». Once a minute,
// and when new warnings come in while the tab is open, only what is newer than
// the newest row on screen is fetched and put on top. A language switch
// redraws from the kept entries.
//
// «Нові»: on opening, the tab first keeps the snapshot's JournalSeenAt (the
// last visit), then marks the journal seen up to the newest entry it drew
// (a warning logged after the fetch stays new). Rows are new
// against the kept time for the whole visit, so they stay marked while the
// badge and the Overview row drop to zero.

/** One page of the journal, as MonitorService.JournalPage returns it. */
export type JournalPage = { Entries: Entry[] | null; More: boolean; Counts: JournalCounts }

/** What MonitorService.JournalSince returns: the newer entries and the whole-period counts. */
export type JournalSince = { Entries: Entry[] | null; Counts: JournalCounts }

export type ErrorsDeps = {
  /** Up to `limit` entries of the filter after the cursor (beforeAt '' = from the newest); newSince: the last visit (RFC 3339, '' = never). */
  page(fromDay: string, toDay: string, beforeAt: string, beforeID: string, limit: number, filter: string, newSince: string): Promise<JournalPage | null>
  /** The entries newer than the cursor, newest first. */
  since(fromDay: string, toDay: string, afterAt: string, afterID: string, filter: string, newSince: string): Promise<JournalSince | null>
  /** Marks the journal seen up to upTo, the At of the newest entry shown. */
  markSeen(upTo: string): Promise<void>
  now?: () => number
}

const REFRESH_MS = 60e3
export const PAGE_SIZE = 200
/** The server's names of the filters. */
const FILTER_ARG: Record<ErrorFilter, string> = { all: 'all', error: 'errors', warning: 'warnings', connection: 'connection' }
/** An open gap row ends at the time it was read; this is how close counts as open. */
const OPEN_GAP_MS = 120e3
/** The refresh reads this far behind the newest row: a gap stored after a wake starts earlier. */
const REFRESH_BACK_MS = 10 * 60e3
const COLS = ['when', 'level', 'what', 'todo'] as const

function layoutHTML(): string {
  // «Рівень» has an ⓘ (copy §15); the header is named by its word alone.
  const head = COLS.map(c => {
    const label = t(`errors.table.col.${c}`)
    return c === 'level'
      ? `<th scope="col" aria-label="${esc(label)}">${esc(label)}${infoTipHTML(label, { key: 'info.errors.level' })}</th>`
      : `<th scope="col">${esc(label)}</th>`
  }).join('')
  return `<div class="toolbar"><div class="seg" role="group" aria-label="${esc(t('errors.filter.a11y'))}" data-slot="filters"><span class="seg__thumb"></span></div><span class="toolbar__hint" data-slot="hint"></span></div>
  <div class="banner banner--quiet banner--info err-strip">${icon('info')}<div class="banner__d" data-slot="strip"></div></div>
  ${loadingHTML('data-slot="loading"')}
  <div class="out-error" data-slot="error" role="alert" hidden><p class="note">${esc(t('error.generic'))}</p><button type="button" class="btn btn--secondary" data-action="retry">${esc(t('common.action.retry'))}</button></div>
  <p class="note t-subtle" data-slot="loaderr" role="alert" hidden>${esc(t('errors.loadFailed'))}</p>
  <section class="panel panel--flush" aria-label="${esc(t('errors.table.a11y'))}" data-slot="body" hidden>
    <div class="table-wrap">
      <table class="table err-table" data-slot="table" tabindex="-1"><thead><tr>${head}</tr></thead><tbody data-slot="rows"></tbody></table>
      <div class="err-empty" data-slot="empty" hidden></div>
    </div>
    <div class="err-more" data-slot="more" hidden><span class="t-subtle" data-slot="showing"></span><button type="button" class="btn btn--secondary" data-action="more">${esc(t('errors.more'))}</button></div>
  </section>`
}

function filterHTML(f: ErrorsModel['filters'][number], on: boolean): string {
  // A filter that can't be picked stays focusable, so its tooltip says why.
  const off = f.disabled ? ` aria-disabled="true" data-tip="${esc(f.tip ?? '')}"` : ''
  return `<button type="button" class="seg__item" aria-pressed="${on}" data-filter="${f.key}"${off}>${esc(f.label)} <span class="n">${f.count}</span></button>`
}

function pillHTML(r: ErrorRow): string {
  // Error: red fill; warning: red outline; info: neutral. Icon and word always.
  const cls = r.level === 'warning' ? 'pill pill--bad-outline' : 'pill'
  const tone = r.level === 'error' ? ' data-tone="bad"' : ''
  return `<span class="${cls}"${tone}>${icon(r.icon)}${esc(r.levelLabel)}</span>`
}

function rowHTML(r: ErrorRow): string {
  const fresh = r.isNew ? `<span class="sr-only">${esc(t('errors.row.new.a11y'))}</span>` : ''
  const todo = r.todo ? esc(r.todo) : `<span class="t-subtle">${esc(t('common.value.none'))}</span>`
  // tabindex -1: a row can take the focus when the last page removes «Показати ще».
  return `<tr${r.isNew ? ' class="is-new"' : ''} data-id="${esc(r.id)}" tabindex="-1">` +
    `<td class="nowrap"><span class="when">${esc(r.when)}<small>${esc(r.whenSub)}</small></span>${fresh}</td>` +
    `<td class="lvl">${pillHTML(r)}</td>` +
    `<td><span class="what"><b>${esc(r.what)}</b>${r.whatSub ? `<small>${esc(r.whatSub)}</small>` : ''}</span></td>` +
    `<td class="advice">${todo}</td></tr>`
}

/** errorsScreen is the «Помилки» tab. */
export function errorsScreen(deps: ErrorsDeps): Screen {
  const now = deps.now ?? Date.now
  let root!: HTMLElement
  let mounted = false
  let entries: Entry[] | null = null
  let more = false
  let counts: JournalCounts = { All: 0, Errors: 0, Warnings: 0, Connection: 0 }
  let loadingMore = false
  /** The last filter change could not load (shown until a read succeeds). */
  let loadErr = false
  /** Gap rows on screen that were still open when read: their end moves. */
  const openGaps = new Set<string>()
  let filter: ErrorFilter = 'all'
  /** The last visit, kept on the first snapshot of this one (null = not yet). */
  let seenAt: number | null = null
  let observedSince: number | null = null
  let newEvents = 0
  let failed = false
  let marked = false
  /** Bumped by a full load and by unmount: an older answer is dropped. */
  let seq = 0
  let tick = 0
  let offLocale = () => {}

  const newSince = () => (seenAt ? new Date(seenAt).toISOString() : '')
  const isGap = (e: Entry) => e.ID.startsWith('app.gap:')
  function noteOpen(list: Entry[], readAt: number) {
    for (const e of list) {
      if (!isGap(e)) continue
      const end = e.Args?.end ? Date.parse(e.Args.end) : NaN
      if (end >= readAt - OPEN_GAP_MS) openGaps.add(e.ID)
      else openGaps.delete(e.ID)
    }
  }

  const slot = (name: string) => root.querySelector<HTMLElement>(`[data-slot="${name}"]`)!

  /** revertTo: the filter to go back to if this read (a filter change) fails. */
  async function load(revertTo?: ErrorFilter) {
    if (!mounted || seenAt == null) return
    const id = ++seq
    loadingMore = false
    const to = now()
    root.setAttribute('aria-busy', 'true')
    try {
      const page = await deps.page(ymd(observedSince ?? to), ymd(to), '', '', PAGE_SIZE, FILTER_ARG[filter], newSince())
      if (id !== seq || !mounted) return
      entries = page?.Entries ?? []
      openGaps.clear()
      noteOpen(entries, to)
      more = page?.More ?? false
      counts = page?.Counts ?? { All: entries.length, Errors: 0, Warnings: 0, Connection: 0 }
      failed = false
      slot('error').hidden = true
      loadErr = false
      render()
      markIfNeeded()
    } catch (err) {
      console.warn('journal', err)
      if (id === seq && mounted && !entries) {
        failed = true
        showError()
      } else if (id === seq && mounted && revertTo !== undefined) {
        // A filter change that could not load: back to the old chip and rows,
        // and a quiet line; clicking the chip again tries again.
        filter = revertTo
        loadErr = true
        render()
      }
    } finally {
      if (id === seq && mounted) root.setAttribute('aria-busy', 'false')
    }
  }

  /** Seen up to the newest entry on screen once it is drawn (and again when
   *  new warnings came in); the snapshot that follows clears the badge. A
   *  warning logged after the fetch stays new for the next load. */
  function markIfNeeded() {
    const upTo = seenUpTo(entries ?? [])
    if (upTo && (!marked || newEvents > 0)) {
      marked = true
      deps.markSeen(upTo).catch(err => console.warn('journal seen', err))
    }
  }

  /** Puts what is newer than the newest row on top, and re-reads the rows that
   *  can still change: an open gap (its end moves) and a gap stored after a
   *  wake, which starts before the newest row (the 60 s refresh, new warnings). */
  async function refresh() {
    if (!mounted || seenAt == null) return
    if (!entries || entries.length === 0) return load()
    const id = seq
    const to = now()
    let from = Date.parse(entries[0].At) - REFRESH_BACK_MS
    for (const e of entries) if (openGaps.has(e.ID)) from = Math.min(from, Date.parse(e.At))
    try {
      // The cursor sits just before `from`: the read starts at that row.
      const view = await deps.since(ymd(observedSince ?? to), ymd(to), new Date(from - 1).toISOString(), '', FILTER_ARG[filter], newSince())
      if (id !== seq || !mounted || !entries) return
      const byID = new Map(entries.map(e => [e.ID, e]))
      const last = entries[entries.length - 1]
      let changed = false
      for (const e of view?.Entries ?? []) {
        const old = byID.get(e.ID)
        if (!old) {
          // Older than the oldest loaded row, with more pages to come: the
          // next page brings it; here it would leave a hole.
          if (more && compareEntries(e, last) > 0) continue
          changed = true
        } else if (JSON.stringify(old) !== JSON.stringify(e)) changed = true
        byID.set(e.ID, e)
      }
      noteOpen(view?.Entries ?? [], to)
      // The chips and «нові» come from the server, so rows that do not match
      // the filter still count.
      if (view?.Counts && JSON.stringify(view.Counts) !== JSON.stringify(counts)) {
        counts = view.Counts
        changed = true
      }
      if (!changed) return
      entries = Array.from(byID.values()).sort(compareEntries)
      render()
      markIfNeeded()
    } catch (err) {
      console.warn('journal since', err) // quiet: the rows stay, the next minute tries again
    }
  }

  /** «Показати ще»: the next page of older rows, appended. */
  async function showMore() {
    if (!mounted || !entries || entries.length === 0 || !more || loadingMore) return
    const id = seq
    const last = entries[entries.length - 1]
    const to = now()
    loadingMore = true
    try {
      const page = await deps.page(ymd(observedSince ?? to), ymd(to), last.At, last.ID, PAGE_SIZE, FILTER_ARG[filter], newSince())
      if (id !== seq || !mounted || !entries) return
      const have = new Set(entries.map(e => e.ID))
      const add = (page?.Entries ?? []).filter(e => !have.has(e.ID))
      noteOpen(add, to)
      entries = [...entries, ...add]
      more = page?.More ?? false
      if (page?.Counts) counts = page.Counts
      render(new Set(add.map(e => e.ID)))
    } catch (err) {
      console.warn('journal page', err) // the button stays: try again
    } finally {
      if (id === seq) loadingMore = false
    }
  }

  function showError() {
    slot('loading').hidden = true
    slot('error').hidden = false
  }

  /** Draws the kept entries. added: the rows a page just appended. */
  function render(added?: Set<string>) {
    if (!mounted || !entries || seenAt == null) return
    const m = errorsModel(entries, filter, seenAt, observedSince, now(), { counts, more })
    const active = document.activeElement as HTMLElement | null
    const focused = active?.dataset?.filter
    const onMore = active?.dataset?.action === 'more'
    const onRow = active?.dataset?.id
    const seg = slot('filters')
    seg.querySelectorAll('.seg__item').forEach(el => el.remove())
    seg.insertAdjacentHTML('beforeend', m.filters.map(f => filterHTML(f, f.key === filter)).join(''))
    if (focused) seg.querySelector<HTMLElement>(`[data-filter="${focused}"]`)?.focus()
    syncSeg(seg)
    slot('hint').textContent = m.hint
    slot('strip').textContent = m.strip
    slot('loading').hidden = true
    slot('body').hidden = false
    slot('rows').innerHTML = m.rows.map(rowHTML).join('')
    slot('table').hidden = m.rows.length === 0
    slot('loaderr').hidden = !loadErr
    slot('more').hidden = !m.more
    slot('showing').textContent = m.showing
    // Focus follows what the user had it on, and only that: the button (or, when
    // the last page took it away, the first visible new row, else the table),
    // or the same row by ID. Focus the user moved elsewhere is left alone.
    if (onMore) {
      const first = added && m.rows.find(r => added.has(r.id))
      if (m.more) root.querySelector<HTMLElement>('[data-action="more"]')?.focus()
      else if (first) root.querySelector<HTMLElement>(`tr[data-id="${first.id.replace(/"/g, '')}"]`)?.focus()
      else slot('table').focus()
    } else if (onRow) {
      root.querySelector<HTMLElement>(`tr[data-id="${onRow.replace(/"/g, '')}"]`)?.focus()
    }
    const empty = slot('empty')
    empty.hidden = !m.empty
    // A filter with nothing in it: the whole sentence is the way back to «усі».
    empty.innerHTML = !m.empty ? ''
      : filter === 'all' ? `${icon('check-circle')}<p>${esc(m.empty)}</p>`
        : `${icon('check-circle')}<p><button type="button" class="link-btn" data-action="showAll">${esc(m.empty)}</button></p>`
  }

  function setFilter(f: ErrorFilter) {
    if (f === filter) return
    const prev = filter
    filter = f
    hideTip()
    // The server filters: page 1 of the new filter replaces the rows.
    void load(prev)
  }

  function onClick(e: Event) {
    const el = (e.target as Element | null)?.closest?.<HTMLElement>('[data-filter], [data-action]')
    if (!el) return
    if (el.dataset.filter) {
      if (el.getAttribute('aria-disabled') !== 'true') setFilter(el.dataset.filter as ErrorFilter)
      return
    }
    if (el.dataset.action === 'more') void showMore()
    if (el.dataset.action === 'showAll') {
      setFilter('all')
      slot('filters').querySelector<HTMLElement>('[data-filter="all"]')?.focus()
    }
    if (el.dataset.action === 'retry') {
      failed = false
      slot('error').hidden = true
      slot('loading').hidden = false
      void load()
    }
  }

  const resize = new ResizeObserver(() => { if (mounted) syncSeg(slot('filters')) })

  function build() {
    root.innerHTML = layoutHTML()
    resize.disconnect()
    resize.observe(slot('filters'))
  }

  return {
    mount(host) {
      root = host
      mounted = true
      build()
      root.addEventListener('click', onClick)
      offLocale = onLocaleChange(() => {
        hideTip()
        build()
        if (entries) render()
        else if (failed) showError()
      })
      tick = window.setInterval(() => void refresh(), REFRESH_MS)
    },
    update(s: Snapshot) {
      if (s.ObservedSince) observedSince = Date.parse(s.ObservedSince)
      const fresh = s.NewEvents ?? 0
      if (seenAt == null) {
        // The first snapshot of this visit: keep the last visit, then load.
        seenAt = s.JournalSeenAt ? Date.parse(s.JournalSeenAt) : 0
        newEvents = fresh
        void load()
        return
      }
      // New warnings while the tab is open: show them (they are new against
      // the kept time) and mark them seen in turn.
      if (fresh > 0 && fresh !== newEvents) void refresh()
      newEvents = fresh
    },
    unmount() {
      mounted = false
      seq++
      offLocale()
      clearInterval(tick)
      resize.disconnect()
      hideTip()
      root.removeEventListener('click', onClick)
      root.removeAttribute('aria-busy')
    },
  }
}
