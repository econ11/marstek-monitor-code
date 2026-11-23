import type { Summary } from '../../bindings/github.com/econ11/marstek-monitor-code/core/health/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { capChartSVG } from '../ui/capChart'
import { renderGauges } from '../ui/gauge'
import { icon } from '../ui/icons'
import { loadingHTML } from '../ui/loading'
import { escapeHTML as esc, hideTip, showTip } from '../ui/tooltip'
import { batteryModel, type BatteryModel } from './batteryModel'

// The «Батарея» tab: DOM after hi-fi 04-battery.html. Two rings, health and
// cycles, the row centred (flows §3). The summary is fetched on mount, every
// 10 minutes (the capacity pass runs daily, the cycles move slowly) and when
// the station changes; a language switch redraws from the kept summary.

export type BatteryDeps = {
  summary(): Promise<Summary>
}

const REFRESH_MS = 10 * 60e3

function layoutHTML(): string {
  return `${loadingHTML('data-slot="loading"')}
  <div class="out-error" data-slot="error" role="alert" hidden><p class="note">${esc(t('error.generic'))}</p><button type="button" class="btn btn--secondary" data-action="retry">${esc(t('common.action.retry'))}</button></div>
  <div data-slot="body" hidden>
    <section class="panel bat-gauges" aria-label="${esc(t('battery.rings.a11y'))}">
      <div class="gauges" data-slot="gauges"></div>
    </section>
    <section class="panel" aria-labelledby="cap-title">
      <div class="panel__head">
        <h2 class="panel__title" id="cap-title">${esc(t('battery.chart.title'))}</h2>
        <div class="legend" data-slot="legend">
          <span data-slot="legend-est"><span class="ln cap-ln"></span>${esc(t('battery.chart.legend.estimate'))}</span>
          <span data-slot="legend-approx"><span class="ln cap-ln cap-ln--approx"></span>${esc(t('battery.chart.legend.approx'))}</span>
        </div>
      </div>
      <div class="out-empty bat-empty" data-slot="empty" hidden></div>
      <div class="chart cap" data-slot="chart" tabindex="-1"></div>
      <p class="note bat-note">${icon('info')}<span>${esc(t('battery.chart.note'))}</span></p>
    </section>
  </div>`
}

/** batteryScreen is the tab as a router Screen. */
export function batteryScreen(deps: BatteryDeps): Screen {
  let root: HTMLElement
  const slot = (name: string) => root.querySelector<HTMLElement>(`[data-slot="${name}"]`)!
  let summary: Summary | null = null
  let model: BatteryModel | null = null
  let mounted = false
  let seq = 0
  let stationKey = ''
  /** The first snapshot was seen: '' is «no station», not «not seen yet», so A → none → B reloads B. */
  let seen = false
  let chartWidth = 0
  /** The chart's last markup: an unchanged refresh is not redrawn. */
  let drawn = ''
  /** The load failed with nothing to show: kept across a language switch. */
  let failed = false

  async function load() {
    if (!mounted) return
    const id = ++seq
    root.setAttribute('aria-busy', 'true')
    try {
      const s = await deps.summary()
      if (id !== seq || !mounted) return
      summary = s
      failed = false
      slot('error').hidden = true
      render()
    } catch (err) {
      console.warn('battery summary', err)
      // Nothing to show yet: an error with a retry, not an endless «Завантажуємо…».
      if (id === seq && mounted && !summary) showError()
    } finally {
      if (id === seq && mounted) root.setAttribute('aria-busy', 'false')
    }
  }

  function showError() {
    failed = true
    slot('loading').hidden = true
    slot('error').hidden = false
  }

  /** The focused month, if focus is on a point. */
  const focusedPoint = () => (document.activeElement as Element | null)?.closest?.<HTMLElement>('.hitpt')?.dataset.i

  function drawChart() {
    if (!model) return
    const host = slot('chart')
    const months = model.chart.months
    host.hidden = !months.length
    if (!months.length) { host.innerHTML = drawn = ''; return }
    chartWidth = host.clientWidth
    const svg = capChartSVG(chartWidth || 600, months, model.chart.nominal, { label: t('battery.chart.a11y') })
    // A refresh with the same months keeps them, and an open tooltip with them.
    if (svg === drawn && host.firstElementChild) return
    drawn = svg
    // The redraw replaces the points: the tooltip goes with them, and focus
    // returns to the same month (or to the chart).
    const focused = focusedPoint()
    hideTip()
    host.innerHTML = svg
    if (focused != null) (host.querySelector<HTMLElement>(`.hitpt[data-i="${focused}"]`) ?? host).focus()
  }

  function render() {
    if (!mounted || !summary) return
    const m = model = batteryModel(summary)
    slot('loading').hidden = true
    slot('body').hidden = false
    renderGauges(slot('gauges'), m.rings)
    // The legend goes with the chart.
    slot('legend').hidden = !!m.empty
    const empty = slot('empty')
    empty.hidden = !m.empty
    empty.innerHTML = m.empty
      ? `<p class="out-empty__t">${esc(m.empty.title)}</p><p class="out-empty__d">${esc(m.empty.body)}</p>` +
        (m.empty.progress ? `<p class="out-empty__d">${esc(m.empty.progress)}</p>` : '')
      : ''
    drawChart()
  }

  function pointAt(e: Event): { el: Element; i: number } | null {
    const el = (e.target as Element | null)?.closest?.('.hitpt')
    return el ? { el, i: Number((el as HTMLElement).dataset.i) } : null
  }
  function onPointShow(e: Event) {
    const p = pointAt(e)
    const month = p && model?.chart.months[p.i]
    if (!p || !month) return
    // Anchored on the dot, not on the larger hit circle (hi-fi 04-battery).
    const r = p.el.getBoundingClientRect()
    showTip(p.el, month.tip, { rect: { left: r.left + 14, top: r.top + 12, bottom: r.bottom - 12, width: r.width - 28 } })
  }
  function onPointHide(e: Event) {
    const p = pointAt(e)
    const to = (e as PointerEvent).relatedTarget
    if (p && !(to instanceof Node && p.el.contains(to))) hideTip()
  }
  function onAction(e: Event) {
    const a = (e.target as Element | null)?.closest?.<HTMLElement>('[data-action]')?.dataset.action
    if (a === 'retry') {
      slot('error').hidden = true
      slot('loading').hidden = false
      void load()
    }
  }

  const resize = new ResizeObserver(() => { if (mounted && slot('chart').clientWidth !== chartWidth) drawChart() })

  function build() {
    resize.disconnect()
    root.removeEventListener('click', onAction)
    root.innerHTML = layoutHTML()
    const chart = slot('chart')
    chart.addEventListener('pointerover', onPointShow)
    chart.addEventListener('pointerout', onPointHide)
    chart.addEventListener('focusin', onPointShow)
    chart.addEventListener('focusout', hideTip)
    root.addEventListener('click', onAction)
    resize.observe(chart)
  }

  let offLocale = () => {}
  let tick = 0
  return {
    mount(host) {
      root = host
      mounted = true
      build()
      offLocale = onLocaleChange(() => {
        build()
        drawn = ''
        render()
        if (failed && !summary) showError()
      })
      tick = window.setInterval(() => void load(), REFRESH_MS)
      void load()
    },
    update(s: Snapshot) {
      // Another station: its battery. The old station's summary is dropped
      // first, so its rings never show with the new station's data.
      const key = s.Station?.ID ?? ''
      if (!seen || key !== stationKey) {
        const first = !seen
        seen = true
        stationKey = key
        if (!first) {
          summary = model = null
          drawn = ''
          if (mounted) {
            slot('body').hidden = slot('error').hidden = true
            slot('loading').hidden = false
          }
          void load()
        }
      }
    },
    unmount() {
      mounted = false
      seq++
      offLocale()
      clearInterval(tick)
      resize.disconnect()
      hideTip()
      root.removeEventListener('click', onAction)
      root.removeAttribute('aria-busy')
    },
  }
}
