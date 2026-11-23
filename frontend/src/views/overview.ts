import type { SeriesResponse } from '../../bindings/github.com/econ11/marstek-monitor-code/app/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { renderGauges, skeletonGaugesHTML } from '../ui/gauge'
import { icon } from '../ui/icons'
import { infoTipHTML, keepInfoTip } from '../ui/infoTip'
import { loadingHTML } from '../ui/loading'
import { syncSeg } from '../ui/period'
import { clearChartPointer, renderSocChart, type ChartBand, type ChartPoint, type ChartRange } from '../ui/socChart'
import { escapeHTML as esc } from '../ui/tooltip'
import { chartNote, type OverviewFeed, type OverviewModel, type RightRow } from './overviewModel'

// The «Огляд» tab: DOM after hi-fi 01-overview.html and 09-states.html.
// Its layout is built on mount (again after a language switch); every model
// of the feed (a snapshot, a one-second tick) rewrites only the parts that
// changed. The header, the grid rail and the
// window title belong to the shell (shell.ts).

export type OverviewDeps = {
  series(range: ChartRange): Promise<SeriesResponse>
  dismissGapCard(id: number): Promise<void>
  /** The chart range and dismissed gap cards, kept across visits to the tab. */
  memory: { range: ChartRange; dismissed: Set<number> }
  /** The snapshots and their model, computed once per tick for the shell and this tab. */
  feed: OverviewFeed
  now?: () => number
}

const RANGES: ChartRange[] = ['24h', '7d', '30d']
const CHART_REFRESH_MS = 60e3
const CHART_HEIGHT = { full: 236, withBanner: 200 }

function layoutHTML(range: ChartRange): string {
  const ranges = RANGES.map(r => `<button class="seg__item" aria-pressed="${r === range}" data-value="${r}">${esc(t(`common.chart.range.${r}`))}</button>`).join('')
  return `<div class="ov-grid">
    <div data-slot="notice" class="ov-notice" hidden></div>
    <section class="panel ov-gauges" data-slot="gauges-panel"><div class="gauges" data-slot="gauges"></div></section>
    <aside class="panel panel--flush ov-side" aria-label="${esc(t('overview.side.a11y'))}"><div class="status" data-slot="status"></div></aside>
    <section class="panel ov-chart" aria-labelledby="soc-title">
      <div class="panel__head">
        <h2 class="panel__title" id="soc-title">${esc(t('overview.chart.title'))}</h2>
        <div class="seg seg--sm" role="group" aria-label="${esc(t('overview.chart.range.a11y'))}" data-slot="range"><span class="seg__thumb"></span>${ranges}</div>
      </div>
      <div class="chart" data-slot="chart"></div>
      <div class="legend soc-legend" data-slot="legend"></div>
      <p class="note ov-chart-empty" data-slot="chart-empty" hidden>${esc(t('overview.chart.empty'))}</p>
    </section>
  </div>`
}

/** The chart legend, with the ⓘ of «без світла», «немає даних» and the station minimum line. */
function legendHTML(m: OverviewModel): string {
  const items: [string, string][] = [
    ['<i class="band"></i>', t('overview.chart.legend.outage')], ['<i class="dash"></i>', t('gap.label')], ['<span class="th"></span>', t('overview.chart.legend.floor')],
  ]
  return `<span><span class="ln"></span>${esc(t('overview.chart.legend.soc'))}</span>` +
    items.map(([mark, label], i) => `<span>${mark}${esc(label)}${m.legend?.[i] ? infoTipHTML(label, m.legend[i]) : ''}</span>`).join('')
}

function noticeHTML(m: OverviewModel): string {
  let html = ''
  if (m.variant === 'skeleton') {
    html += loadingHTML('', t(m.station ? 'onboarding.firstData.waiting' : 'offline.reconnecting'))
  }
  if (m.banner) {
    const cls = { neutral: '', alert: '', danger: ' banner--danger', quiet: ' banner--quiet banner--info' }[m.bannerKind ?? 'neutral']
    const role = m.bannerKind === 'alert' || m.bannerKind === 'danger' ? 'alert' : 'status'
    const lines = (m.bannerLines ?? []).map(l => `<div class="banner__d">${esc(l)}</div>`).join('')
    const title = `<div class="banner__t${m.bannerKind === 'quiet' ? ' banner__t--quiet' : ''}">${esc(m.banner)}</div>`
    html += `<div class="banner${cls}" role="${role}">${icon(m.bannerIcon ?? 'clock')}<div>${title}${lines}</div></div>`
  }
  if (m.gapCard) {
    const info = m.gapCard.info ? infoTipHTML(m.gapCard.title ?? '', m.gapCard.info) : ''
    html += `<div class="card-gap">${icon('history')}<div><div class="t">${esc(m.gapCard.title ?? '')}${info}</div><div class="d">${esc(m.gapCard.text)}</div></div>` +
      `<button class="iconbtn" data-dismiss-gap="${m.gapCard.id}" aria-label="${esc(t('gapCard.dismiss.a11y'))}">${icon('x')}</button></div>`
  }
  return html
}

function rowHTML(r: RightRow): string {
  const value = r.live ? `<span class="live">${esc(r.value)}</span>` : esc(r.value)
  const info = r.info ? infoTipHTML(r.label, r.info) : ''
  // A row about a tab is a link to it (the router navigates on [data-view]):
  // the label is the link, stretched over the row, so the ⓘ beside it is a
  // button of its own, not one inside a link. The link says the whole row.
  const label = r.view
    ? `<a class="status__link" href="#${r.view}" data-view="${r.view}" aria-label="${esc(`${r.label}: ${r.value}`)}">${esc(r.label)}</a>`
    : esc(r.label)
  return `<div class="status__row${r.hero ? ' status__row--hero' : ''}${r.view ? ' status__row--link' : ''}">` +
    `<span class="status__icon"${r.tone ? ` data-tone="${r.tone}"` : ''}>${icon(r.icon ?? 'clock')}</span>` +
    `<span class="status__body"><span class="label">${label}${info}</span><span class="status__value"${r.view ? ' aria-hidden="true"' : ''}>${value}</span>` +
    (r.meta ? `<span class="status__meta">${esc(r.meta)}</span>` : '') + `</span></div>`
}

const SKELETON_ROWS = [[60, 170], [140, 190], [70, 110]]
const skeletonRowsHTML = () => SKELETON_ROWS.map(([label, value]) =>
  `<div class="status__row" aria-hidden="true"><span class="skel skel-icon"></span><span class="status__body"><span class="skel skel-line skel-line--label" style="width:${label}px"></span><span class="skel skel-line" style="width:${value}px"></span></span></div>`).join('')

/** overviewScreen is the tab as a router Screen: mount() builds it in host,
 *  draws every model of the feed, unmount() stops its listeners. */
export function overviewScreen(deps: OverviewDeps): Screen {
  const now = deps.now ?? Date.now
  const mem = deps.memory
  let root: HTMLElement
  const slot = (name: string) => root.querySelector<HTMLElement>(`[data-slot="${name}"]`)!
  const query = () => ({
    notice: slot('notice'), gaugesPanel: slot('gauges-panel'), gauges: slot('gauges'),
    range: slot('range'), chart: slot('chart'), legend: slot('legend'), chartEmpty: slot('chart-empty'), status: slot('status'),
  })
  let el: ReturnType<typeof query>

  let snapshot: Snapshot | null = null
  let model: OverviewModel | null = null
  let series: { points: ChartPoint[]; bands: ChartBand[] } | null = null
  let seriesAt = 0
  let seriesKey = ''
  let loading = false
  /** A refresh came due while the window was hidden: it runs when shown. */
  let due = false
  let mounted = false
  const html = new Map<HTMLElement, string>()
  const setHTML = (target: HTMLElement, markup: string) => {
    if (html.get(target) === markup) return
    html.set(target, markup)
    target.innerHTML = markup
  }

  function drawChart() {
    if (!mounted || !model || !snapshot) return
    const skeleton = model.variant === 'skeleton'
    const lastAt = snapshot.Last ? Date.parse(snapshot.Last.At) : undefined
    renderSocChart(el.chart, series?.points ?? [], series?.bands ?? [], {
      range: mem.range, now: now(), skeleton, lastData: lastAt,
      tail: model.variant === 'stale' ? 'stale' : model.variant === 'nolink' ? 'nolink' : model.variant === 'likelyDead' ? 'dead' : null,
      observedSince: snapshot.ObservedSince ? Date.parse(snapshot.ObservedSince) : undefined,
      floor: snapshot.Profile.MinSOC,
      height: skeleton || model.banner || model.gapCard ? CHART_HEIGHT.withBanner : CHART_HEIGHT.full,
    })
    // Nothing to draw yet (also a loaded series without a measurement): say so.
    const note = chartNote(model.variant, series ? series.points.filter(p => p.soc != null).length : null)
    el.chartEmpty.hidden = !note
    if (note) el.chartEmpty.textContent = note
  }

  async function loadSeries() {
    if (loading || !mounted || !snapshot?.Station?.ID) return
    // Nobody sees a hidden window: no fetch until it is shown again.
    if (document.visibilityState === 'hidden') { due = true; return }
    due = false
    loading = true
    const asked = mem.range
    try {
      const res = await deps.series(asked)
      if (asked !== mem.range) return
      series = {
        points: (res.Points ?? []).map(p => ({ t: Date.parse(p.At), soc: p.SOC })),
        bands: (res.Bands ?? []).map(b => ({ start: Date.parse(b.Start), end: Date.parse(b.End), kind: b.Kind === 'gap' ? 'gap' : 'outage', label: b.Label })),
      }
      drawChart()
    } catch (err) {
      console.warn('SOC series', err)
    } finally {
      // Also after a failure: the next try is a refresh interval away, not a second.
      seriesAt = now()
      loading = false
      if (asked !== mem.range) void loadSeries()
    }
  }

  function render() {
    const c = deps.feed.current()
    if (!mounted || !c) return
    const s = snapshot = c.snapshot
    // The model is shared with the shell: a dismissed card is left out of a copy.
    const m = model = c.model.gapCard && mem.dismissed.has(c.model.gapCard.id) ? { ...c.model, gapCard: undefined } : c.model
    const skeleton = m.variant === 'skeleton'

    root.setAttribute('aria-busy', String(skeleton))

    const notice = noticeHTML(m)
    keepInfoTip(() => setHTML(el.notice, notice))
    el.notice.hidden = notice === ''

    el.gaugesPanel.classList.toggle('is-dim', m.variant === 'nolink' || m.variant === 'likelyDead')
    el.gaugesPanel.setAttribute('aria-label', t(m.variant === 'live' ? 'overview.rings.a11y' : 'overview.rings.a11y.stale'))
    if (skeleton) {
      el.gaugesPanel.setAttribute('aria-hidden', 'true')
      setHTML(el.gauges, skeletonGaugesHTML())
    } else {
      el.gaugesPanel.removeAttribute('aria-hidden')
      html.delete(el.gauges)
      renderGauges(el.gauges, m.rings)
    }

    el.range.hidden = el.legend.hidden = skeleton
    keepInfoTip(() => {
      setHTML(el.legend, legendHTML(m))
      setHTML(el.status, (skeleton ? skeletonRowsHTML() : '') + m.right.map(rowHTML).join(''))
    })

    // The chart follows the state at once and otherwise refreshes once a minute.
    const key = `${s.Station?.ID}|${m.variant}|${m.outageRail}|${m.gapCard?.id ?? ''}|${m.banner ? 1 : 0}`
    if (key !== seriesKey) {
      seriesKey = key
      drawChart()
      void loadSeries()
    } else if (now() - seriesAt > CHART_REFRESH_MS) {
      void loadSeries()
    }
  }

  function onRange(e: Event) {
    const item = (e.target as Element).closest<HTMLElement>('.seg__item')
    if (!item || item.dataset.value === mem.range) return
    el.range.querySelectorAll('.seg__item').forEach(b => b.setAttribute('aria-pressed', String(b === item)))
    syncSeg(el.range)
    mem.range = item.dataset.value as ChartRange
    series = null
    drawChart()
    void loadSeries()
  }
  function onNotice(e: Event) {
    const btn = (e.target as Element).closest<HTMLElement>('[data-dismiss-gap]')
    if (!btn) return
    // Hidden at once on success; a failure leaves the card in place.
    const id = Number(btn.dataset.dismissGap)
    deps.dismissGapCard(id).then(() => { mem.dismissed.add(id); render() }, err => console.warn('dismiss gap card', err))
  }
  function onVisibility() {
    // The pointer is not over a hidden window, whatever pointerleave missed.
    if (document.visibilityState === 'hidden') clearChartPointer(el.chart)
    if (document.visibilityState !== 'hidden' && (due || now() - seriesAt > CHART_REFRESH_MS)) void loadSeries()
  }
  const resize = new ResizeObserver(() => { if (mounted) { syncSeg(el.range); drawChart() } })

  // build (re)creates the layout in the current language; the snapshot, range
  // and loaded series are kept, so a language switch redraws without a fetch.
  function build() {
    resize.disconnect()
    root.innerHTML = layoutHTML(mem.range)
    el = query()
    html.clear()
    el.range.addEventListener('click', onRange)
    el.notice.addEventListener('click', onNotice)
    resize.observe(el.chart)
    syncSeg(el.range)
  }

  let offLocale = () => {}
  let offFeed = () => {}
  return {
    mount(host) {
      root = host
      mounted = true
      build()
      offLocale = onLocaleChange(() => {
        build()
        render()
        // render() draws the chart only when its state key changes, and a language
        // switch does not change it: the new, empty chart element is drawn here
        // once, from the kept series (no fetch).
        drawChart()
      })
      void document.fonts?.ready.then(() => { if (mounted) syncSeg(el.range) })
      document.addEventListener('visibilitychange', onVisibility)
      // Ages and durations move on with the feed's tick.
      offFeed = deps.feed.on(render)
      render()
    },
    // The feed brings the same snapshots (main.ts), with their model.
    update() {},
    unmount() {
      mounted = false
      offLocale()
      offFeed()
      document.removeEventListener('visibilitychange', onVisibility)
      resize.disconnect()
      root.removeAttribute('aria-busy')
    },
  }
}
