import type { Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/history/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { ymd } from '../ui/calendar'
import { dayBarsSVG } from '../ui/dayBars'
import { dayTimelineSVG } from '../ui/dayTimeline'
import { renderGauges } from '../ui/gauge'
import { icon } from '../ui/icons'
import { infoTipHTML, keepInfoTip, type Info } from '../ui/infoTip'
import { loadingHTML } from '../ui/loading'
import { mountPeriod, periodRange, type DayRange } from '../ui/period'
import { escapeHTML as esc, hideTip, showTip } from '../ui/tooltip'
import { hasLiveRow, outagesModel, rangeAround, type OutageRow, type OutagesModel } from './outagesModel'
import type { PeriodMemory } from './shell'

// The «Відключення» tab: DOM after hi-fi 02-outages.html and the outages
// fragment of 09-states.html. The report is fetched for the period on mount,
// on a period change, once a minute and when the outage state changes; a
// language switch redraws from the kept report. A click on a column filters
// the table to that day (again, or ×, clears it); a toast's outage id
// highlights its row.

export type OutagesDeps = {
  report(fromDay: string, toDay: string): Promise<Report>
  /** The period, kept across visits to the tab. */
  memory: PeriodMemory
  /** The outage to highlight (a toast click). */
  outageId?: number
  now?: () => number
}

const REFRESH_MS = 60e3

const COLS = ['start', 'end', 'duration', 'soc', 'note'] as const

function layoutHTML(): string {
  const head = COLS.map(c => `<th scope="col"${c === 'duration' ? ' class="num"' : ''}>${esc(t(`outages.table.col.${c}`))}</th>`).join('')
  return `<div class="toolbar"><div data-slot="period"></div><span class="toolbar__hint" data-slot="hint"></span></div>
  ${loadingHTML('data-slot="loading"')}
  <div class="out-error" data-slot="error" role="alert" hidden><p class="note">${esc(t('error.generic'))}</p><button type="button" class="btn btn--secondary" data-action="retry">${esc(t('common.action.retry'))}</button></div>
  <div class="out-body" data-slot="body" hidden>
    <div class="out-top">
      <section class="panel out-summary" aria-label="${esc(t('outages.summary.a11y'))}">
        <div class="gauges" data-slot="gauges"></div>
        <div class="stats" data-slot="stats"></div>
        <p class="note out-footnote" data-slot="footnote" hidden></p>
      </section>
      <section class="panel" aria-labelledby="out-bars-title">
        <div class="panel__head">
          <h2 class="panel__title" id="out-bars-title" data-slot="bars-title"></h2>
          <span class="panel__sub" data-slot="bars-sub"></span>
          <div class="legend">
            <span><i style="background:var(--color-warn)"></i>${esc(t('overview.chart.legend.outage'))}</span>
            <span><i class="dash"></i>${esc(t('outages.chart.day.noData'))}${infoTipHTML(t('outages.chart.day.noData'), { key: 'info.outages.noDataDay' })}</span>
            <span><i class="tick"></i>${esc(t('outages.chart.day.zero'))}</span>
            <span><i class="dot"></i>${esc(t('state.stationOut'))}</span>
          </div>
        </div>
        <div class="chart bars" data-slot="bars" tabindex="-1"></div>
      </section>
    </div>
    <section class="panel panel--flush" aria-labelledby="out-ev-title">
      <div class="panel__head">
        <h2 class="panel__title" id="out-ev-title">${esc(t('outages.table.title'))}</h2>
        <span class="panel__sub" data-slot="ev-sub"></span>
        <span data-slot="chip"></span>
      </div>
      <div class="out-empty" data-slot="empty" hidden></div>
      <div class="table-wrap" data-slot="table"><table class="table"><thead><tr>${head}</tr></thead><tbody data-slot="rows"></tbody></table></div>
    </section>
  </div>`
}

function statsHTML(m: OutagesModel): string {
  const s = m.stats
  const stat = (label: string, value: string, meta: string, aria: string) =>
    `<div class="stat" role="group" aria-label="${esc(aria)}"><span class="label">${esc(label)}</span><span class="stat__v">${esc(value)}</span><span class="stat__m">${esc(meta)}</span></div>`
  return stat(t('outages.stat.count.label'), s.count, s.countMeta, t('outages.stat.count', { count: s.count })) +
    stat(t('outages.stat.average.label'), s.average, s.average === t('common.value.none') ? '' : t('outages.stat.average.meta'), t('outages.stat.average', { duration: s.average })) +
    stat(t('outages.stat.longest.label'), s.longest, s.longestMeta, s.longestMeta ? t('outages.stat.longest', { duration: s.longest, date: s.longestMeta }) : `${t('outages.stat.longest.label')}: ${s.longest}`)
}

/** The ⓘ of a cell, named after the mark it explains; at: the row and cell, for a redraw. */
const cellInfo = (r: OutageRow, cell: keyof OutageRow['info'], label: string) => {
  const info: Info | undefined = r.info[cell]
  return info ? infoTipHTML(label, info, `${r.id}:${cell}`) : ''
}

function endHTML(r: OutageRow): string {
  if (r.endKind === 'ongoing') return `<span class="pill" data-tone="warn">${icon('plug-off')}${esc(r.end)}</span>`
  if (r.endKind === 'linkLost') return `<span class="pill pill--outline">${icon('wifi-off')}${esc(r.end)}</span>${cellInfo(r, 'end', r.end)}`
  const end = `<span><span${r.endTone ? ' class="t-muted"' : ''}>${esc(r.end)}</span>${cellInfo(r, 'end', r.end)}</span>`
  return r.endDay ? `<span class="when">${end}<small>${esc(r.endDay)}</small></span>` : end
}

function socHTML(soc: string, live = false): string {
  const [a, b] = soc.split(' → ')
  const side = (v: string) => (v === t('outages.table.soc.unknown') ? `<span class="t-subtle">${esc(v)}</span>` : esc(v))
  return b == null ? esc(soc) : `<span class="arrow-soc">${side(a)} <i>→</i> ${live ? `<span class="t-warn">${esc(b)}</span>` : side(b)}</span>`
}

function rowHTML(r: OutageRow): string {
  const tone = r.noteTone === 'bad' ? 't-bad' : r.noteTone === 'muted' ? 't-subtle' : ''
  const note = r.note ? `<span class="${tone}">${esc(r.note)}</span>${cellInfo(r, 'note', r.note)}` : `<span class="t-subtle">${esc(t('common.value.none'))}</span>`
  return `<tr data-id="${r.id}"${r.highlighted ? ' class="is-hit"' : ''}>` +
    `<td class="nowrap"><span class="when"><span><span${r.startTone ? ' class="t-muted"' : ''}>${esc(r.startTime)}</span>${cellInfo(r, 'start', r.startTime)}</span><small>${esc(r.startDay)}</small></span></td>` +
    `<td class="nowrap">${endHTML(r)}</td>` +
    `<td class="num nowrap">${esc(r.duration)}${cellInfo(r, 'duration', r.duration)}${r.asOf ? `<br><small class="t-subtle">${esc(r.asOf)}</small>` : ''}</td>` +
    `<td>${socHTML(r.soc, r.socLive)}</td>` +
    `<td>${note}</td></tr>`
}

function emptyHTML(e: NonNullable<OutagesModel['empty']>): string {
  return (e.title ? `<h3 class="out-empty__t">${esc(e.title)}</h3>` : '') +
    `<p class="out-empty__d">${esc(e.body)}</p>` +
    (e.action === 'fromStart' ? `<button type="button" class="btn btn--secondary" data-action="fromStart">${esc(t('outages.empty.fromStart'))}</button>` : '')
}

/** The filter chip: the catalogue text ends in its action («… · Показати всі»), which becomes the button. */
function chipHTML(label: string): string {
  const cut = label.lastIndexOf(' · ')
  const text = cut > 0 ? label.slice(0, cut) : label
  const action = cut > 0 ? label.slice(cut + 3) : ''
  return `<span class="filter-chip">${esc(text)} · <button type="button" class="link-btn" data-action="clearFilter">${esc(action)}${icon('x')}</button></span>`
}

/** outagesScreen is the tab as a router Screen. */
export function outagesScreen(deps: OutagesDeps): Screen {
  const now = deps.now ?? Date.now
  const mem = deps.memory
  let root: HTMLElement
  const slot = (name: string) => root.querySelector<HTMLElement>(`[data-slot="${name}"]`)!
  let period: { unmount(): void } | null = null
  let report: Report | null = null
  let model: OutagesModel | null = null
  let filter: string | undefined
  let highlight = deps.outageId
  let scrolled = false
  let mounted = false
  let seq = 0
  let stateKey = ''
  let barsWidth = 0
  /** The charge now, while the link is LIVE (an ongoing row's end). */
  let liveSoc: number | undefined
  let snapshot: Snapshot | null = null
  /** The highlighted outage was looked for outside the period (once). */
  let located = false
  /** Bumped on every period the user picks: a pending locate then gives way. */
  let periodPicks = 0
  /** The load failed with nothing to show: kept across a language switch. */
  let failed = false

  const range = (): DayRange => (mem.key === 'custom' && mem.range ? mem.range : periodRange(mem.key === 'custom' ? '7d' : mem.key, now()))

  async function load() {
    if (!mounted) return
    const id = ++seq
    const r = range()
    root.setAttribute('aria-busy', 'true')
    try {
      const rep = await deps.report(r.from, r.to)
      if (id !== seq || !mounted) return
      report = rep
      failed = false
      slot('error').hidden = true
      render()
      void locate(rep)
    } catch (err) {
      console.warn('outages report', err)
      // Nothing to show yet: an error with a retry, not an endless «Завантажуємо…».
      if (id === seq && mounted && !report) {
        failed = true
        showError()
      }
    } finally {
      if (id === seq && mounted) root.setAttribute('aria-busy', 'false')
    }
  }

  function showError() {
    slot('loading').hidden = true
    slot('error').hidden = false
  }

  /** locate: a toast's outage outside the period switches the tab to 7 days around it. */
  async function locate(rep: Report) {
    if (highlight == null || located) return
    located = true
    const id = highlight
    const picks = periodPicks
    if ((rep.Rows ?? []).some(v => v.Outage.ID === id)) return
    let start = [snapshot?.Current, snapshot?.Previous].find(o => o?.ID === id)?.Start
    if (!start) {
      const r30 = periodRange('30d', now())
      try {
        start = (await deps.report(r30.from, r30.to)).Rows?.find(v => v.Outage.ID === id)?.Outage.Start
      } catch (err) {
        console.warn('outages report', err)
      }
    }
    if (!start || !mounted || highlight !== id || picks !== periodPicks) return
    mem.key = 'custom'
    mem.range = rangeAround(Date.parse(start), now())
    filter = undefined
    mountPeriodSwitcher()
    void load()
  }

  /** The key of the focused column, if focus is on one. */
  const focusedBar = () => (document.activeElement as Element | null)?.closest?.<HTMLElement>('.bar-g')?.dataset.key
  /** focusBar puts focus back on a column after a redraw (or on the chart). */
  function focusBar(key: string | undefined) {
    const host = slot('bars')
    const g = key ? Array.from(host.querySelectorAll<HTMLElement>('.bar-g[tabindex="0"]')).find(el => el.dataset.key === key) : undefined
    ;(g ?? host).focus()
  }

  function drawBars() {
    if (!model) return
    const host = slot('bars')
    barsWidth = host.clientWidth
    // The redraw replaces the columns: the tooltip and focus go with them, so
    // the tooltip closes and focus returns to the same column.
    const focused = focusedBar()
    hideTip()
    // «Сьогодні» is the day by the hour; other periods are columns.
    host.innerHTML = model.timeline
      ? dayTimelineSVG(barsWidth || 600, model.timeline)
      : dayBarsSVG(barsWidth || 600, model.bars.items, {
        selected: model.filter?.day,
        observedSince: model.bars.observedSince,
        label: t(report?.Period.Weeks ? 'outages.chart.a11y.weeks' : 'outages.chart.a11y.days'),
      })
    host.classList.toggle('has-selection', model.filter != null)
    if (focused) focusBar(focused)
  }

  function render() {
    if (!mounted || !report) return
    const before = model?.filter?.day
    const chipFocused = !!document.activeElement?.closest?.('[data-slot="chip"]')
    const m = model = outagesModel(report, now(), { filterDay: filter, highlight, liveSoc })
    slot('loading').hidden = true
    slot('body').hidden = false
    slot('hint').textContent = m.hint
    renderGauges(slot('gauges'), m.rings)
    slot('stats').innerHTML = statsHTML(m)
    const foot = slot('footnote')
    foot.hidden = !m.footnote
    foot.innerHTML = m.footnote ? `${icon('info', 'icon--sm')}<span>${esc(m.footnote)}</span>` : ''
    slot('bars-title').textContent = m.bars.title
    slot('bars-sub').textContent = m.bars.sub
    drawBars()
    slot('ev-sub').textContent = t('outages.table.sub', { count: m.rows.length })
    slot('chip').innerHTML = m.filter ? chipHTML(m.filter.label) : ''
    // The chip is redrawn too: keep focus on it, or, when it is gone, on the column it named.
    if (chipFocused) {
      const btn = slot('chip').querySelector<HTMLElement>('button')
      if (btn) btn.focus()
      else focusBar(before)
    }
    const empty = slot('empty')
    // A filtered day always has rows: the empty states are about the period.
    const e = m.filter ? undefined : m.empty
    empty.hidden = !e
    empty.innerHTML = e ? emptyHTML(e) : ''
    slot('table').hidden = !!e
    keepInfoTip(() => { slot('rows').innerHTML = m.rows.map(rowHTML).join('') })
    if (highlight != null && !scrolled) {
      const tr = slot('rows').querySelector<HTMLElement>('tr.is-hit')
      if (tr) { scrolled = true; tr.scrollIntoView({ block: 'nearest' }) }
    }
  }

  function setFilter(key: string | undefined) {
    filter = filter === key ? undefined : key
    hideTip()
    render()
  }

  function mountPeriodSwitcher() {
    period?.unmount()
    period = mountPeriod(slot('period'), {
      initial: mem.key, range: mem.range, now,
      onChange: (r, key) => {
        periodPicks++
        mem.key = key
        if (key === 'custom') mem.range = r
        filter = undefined
        void load()
      },
    })
  }

  function barAt(e: Event): { g: HTMLElement; i: number } | null {
    const g = (e.target as Element | null)?.closest?.<HTMLElement>('.bar-g')
    return g ? { g, i: Number(g.dataset.i) } : null
  }
  function onBarShow(e: Event) {
    const b = barAt(e)
    const item = b && (model?.timeline ? model.timeline.items[b.i] : model?.bars.items[b.i])
    if (!b || !item?.tip) return
    const shape = b.g.querySelector('.bar') ?? b.g.querySelector('.bar-hit')!
    showTip(b.g, item.tip, { rect: shape.getBoundingClientRect() })
  }
  function onBarHide(e: Event) {
    const b = barAt(e)
    const to = (e as PointerEvent).relatedTarget
    if (b && !(to instanceof Node && b.g.contains(to))) hideTip()
  }
  function onBarClick(e: Event) {
    const b = barAt(e)
    // A span of the timeline only has a tooltip: the table is already that day.
    const item = b && !model?.timeline ? model?.bars.items[b.i] : undefined
    if (item?.kind === 'value') { highlight = undefined; setFilter(item.key) }
  }
  function onBarKey(e: KeyboardEvent) {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onBarClick(e)
  }
  function onAction(e: Event) {
    const a = (e.target as Element | null)?.closest?.<HTMLElement>('[data-action]')?.dataset.action
    if (a === 'clearFilter') setFilter(undefined)
    if (a === 'retry') {
      failed = false
      slot('error').hidden = true
      slot('loading').hidden = false
      void load()
    }
    if (a === 'fromStart' && report?.ObservedSince) {
      periodPicks++
      mem.key = 'custom'
      mem.range = { from: ymd(Date.parse(report.ObservedSince)), to: ymd(now()) }
      filter = undefined
      mountPeriodSwitcher()
      void load()
    }
  }

  const resize = new ResizeObserver(() => { if (mounted && slot('bars').clientWidth !== barsWidth) drawBars() })

  function build() {
    resize.disconnect()
    root.innerHTML = layoutHTML()
    const bars = slot('bars')
    bars.addEventListener('pointerover', onBarShow)
    bars.addEventListener('pointerout', onBarHide)
    bars.addEventListener('focusin', onBarShow)
    bars.addEventListener('focusout', hideTip)
    bars.addEventListener('click', onBarClick)
    bars.addEventListener('keydown', onBarKey)
    root.addEventListener('click', onAction)
    resize.observe(bars)
    mountPeriodSwitcher()
  }

  let offLocale = () => {}
  let tick = 0
  return {
    mount(host) {
      root = host
      mounted = true
      build()
      offLocale = onLocaleChange(() => {
        root.removeEventListener('click', onAction)
        build()
        if (report) render()
        else if (failed) showError()
      })
      tick = window.setInterval(() => void load(), REFRESH_MS)
      void load()
    },
    update(s: Snapshot) {
      snapshot = s
      const soc = s.Link === 'LIVE' && s.Last?.SOC != null ? s.Last.SOC : undefined
      if (soc !== liveSoc) {
        liveSoc = soc
        // Only an ongoing live row shows the charge: redraw just the table rows,
        // so an open tooltip and the focus elsewhere stay put.
        if (mounted && report && model && hasLiveRow(report)) {
          model = outagesModel(report, now(), { filterDay: filter, highlight, liveSoc })
          const rows = model.rows.map(rowHTML).join('')
          keepInfoTip(() => { slot('rows').innerHTML = rows })
        }
      }
      // A new, ended or frozen outage changes the report: fetch it again.
      const c = s.Current
      const key = `${s.Station?.ID ?? ''}|${c?.ID ?? ''}|${c?.End ?? ''}|${s.Link}|${s.Grid}`
      if (key === stateKey) return
      const first = stateKey === ''
      stateKey = key
      if (!first) void load()
    },
    unmount() {
      mounted = false
      seq++
      offLocale()
      clearInterval(tick)
      resize.disconnect()
      period?.unmount()
      period = null
      hideTip()
      root.removeEventListener('click', onAction)
      root.removeAttribute('aria-busy')
    },
  }
}
