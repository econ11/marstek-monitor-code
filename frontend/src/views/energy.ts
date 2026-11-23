import type { PowerSeries, Report } from '../../bindings/github.com/econ11/marstek-monitor-code/core/energy/models'
import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'
import type { Screen } from '../app/router'
import { onLocaleChange, t } from '../lib/i18n'
import { renderGauges } from '../ui/gauge'
import { groupedBarsSVG } from '../ui/groupedBars'
import { icon } from '../ui/icons'
import { infoTipHTML, keepInfoTip } from '../ui/infoTip'
import { loadingHTML, spinnerHTML } from '../ui/loading'
import { ymd } from '../ui/calendar'
import { mountPeriod, periodRange, type DayRange } from '../ui/period'
import { powerChart, type PowerChart } from '../ui/powerChart'
import { escapeHTML as esc, hideTip, showTip } from '../ui/tooltip'
import { energyModel, type EnergyModel } from './energyModel'
import { powerModel } from './powerModel'
import type { PeriodMemory } from './shell'

// The «Енергія» tab: DOM after hi-fi 05-energy.html. The totals are numbers,
// not rings (sums, not shares); the «Мережа / сонце» ring is drawn only when
// the station reports solar charging. The report is fetched for the period on
// mount, on a period change, once a minute and when the station changes; a
// language switch redraws from the kept report.

export type EnergyDeps = {
  report(fromDay: string, toDay: string): Promise<Report>
  /** The battery power chart of the same days (Task 21c). */
  power(fromDay: string, toDay: string): Promise<PowerSeries>
  /** The period, kept across visits to the tab. */
  memory: PeriodMemory
  now?: () => number
}

const REFRESH_MS = 60e3
/** While past days are being counted, the totals grow every few seconds. */
const BACKFILL_REFRESH_MS = 5e3

function layoutHTML(): string {
  const total = (key: string, tone: string, extra = '') =>
    `<div class="total"><span class="label">${esc(t(`energy.total.${key}.label`))}${infoTipHTML(t(`energy.total.${key}.label`), { key: `info.energy.${key}` })}</span>` +
    `<span class="total__v t-${tone}" data-slot="${key}"></span><span class="total__m" data-slot="${key}-meta" hidden></span>${extra}</div>`
  return `<div class="toolbar"><div data-slot="period"></div></div>
  ${loadingHTML('data-slot="loading"')}
  <div class="out-error" data-slot="error" role="alert" hidden><p class="note">${esc(t('error.generic'))}</p><button type="button" class="btn btn--secondary" data-action="retry">${esc(t('common.action.retry'))}</button></div>
  <div class="en" data-slot="body" hidden>
    <div class="en-side">
      <section class="panel en-pv" data-slot="pv" hidden><div class="gauges" data-slot="gauges"></div></section>
      <section class="panel totals" aria-label="${esc(t('energy.totals.a11y'))}">
        ${total('charged', 'good')}${total('discharged', 'warn')}${total('efficiency', 'info')}
        <p class="note en-note" data-slot="incomplete" hidden></p>
      </section>
    </div>
    <section class="panel" aria-labelledby="en-bars-title">
      <div class="panel__head">
        <h2 class="panel__title" id="en-bars-title" data-slot="bars-title"></h2>
        <div class="legend">
          <span><i style="background:var(--color-good)"></i>${esc(t('energy.chart.legend.charge'))}</span>
          <span><i class="en-dis"></i>${esc(t('energy.chart.legend.discharge'))}</span>
          <span><i class="dash"></i>${esc(t('outages.chart.day.noData'))}</span>
        </div>
      </div>
      <p class="note en-backfill" data-slot="backfill" role="status" hidden></p>
      <p class="note en-empty" data-slot="empty" hidden></p>
      <div class="chart bars gb" data-slot="bars" tabindex="-1"></div>
    </section>
    <section class="panel en-power" aria-labelledby="en-power-title">
      <div class="panel__head">
        <h2 class="panel__title" id="en-power-title">${esc(t('energy.power.title'))}<small class="en-power__note" data-slot="power-note" hidden></small></h2>
        <div class="legend">
          <span><i style="background:var(--color-good)"></i>${esc(t('energy.chart.legend.charge'))}</span>
          <span><i style="background:var(--color-warn)"></i>${esc(t('energy.chart.legend.discharge'))}</span>
          <span><i class="band"></i>${esc(t('overview.chart.legend.outage'))}</span>
        </div>
      </div>
      <p class="note en-peak" data-slot="power-peak" hidden></p>
      ${loadingHTML('data-slot="power-loading" hidden')}
      <div class="out-error" data-slot="power-error" role="alert" hidden><p class="note">${esc(t('error.generic'))}</p><button type="button" class="btn btn--secondary" data-action="retry-power">${esc(t('common.action.retry'))}</button></div>
      <p class="note en-empty" data-slot="power-empty" hidden></p>
      <div class="chart pw" data-slot="power-chart" tabindex="0" role="group"></div>
      <span class="sr-only" aria-live="polite" data-slot="power-live"></span>
    </section>
  </div>`
}

/** «29,2 кВт·год» → 29,2<small>кВт·год</small>; «88%» → 88<small>%</small>. */
function valueHTML(v: string): string {
  if (v.endsWith('%')) return `${esc(v.slice(0, -1))}<small>%</small>`
  // The unit follows the last plain space (the number's own groups use a no-break space).
  const cut = v.lastIndexOf(' ')
  return cut > 0 ? `${esc(v.slice(0, cut))}<small>${esc(v.slice(cut + 1))}</small>` : esc(v)
}

/** energyScreen is the tab as a router Screen. */
export function energyScreen(deps: EnergyDeps): Screen {
  const now = deps.now ?? Date.now
  const mem = deps.memory
  let root: HTMLElement
  const slot = (name: string) => root.querySelector<HTMLElement>(`[data-slot="${name}"]`)!
  let period: { unmount(): void } | null = null
  let report: Report | null = null
  let model: EnergyModel | null = null
  let mounted = false
  let seq = 0
  let stationKey = ''
  /** The first snapshot was seen: '' is «no station», not «not seen yet», so A → none → B reloads B. */
  let seen = false
  let barsWidth = 0
  /** The chart's last markup: an unchanged refresh is not redrawn. */
  let drawn = ''
  /** The load failed with nothing to show: kept across a language switch. */
  let failed = false
  let power: PowerSeries | null = null
  /** The power chart's load failed with nothing to show. */
  let powerFailed = false
  let pseq = 0
  let pchart: PowerChart | null = null
  let powerWidth = 0
  /** The station and days `power` is for: another one starts the chart afresh. */
  let powerKey = ''

  const range = (): DayRange => (mem.key === 'custom' && mem.range ? mem.range : periodRange(mem.key === 'custom' ? '7d' : mem.key, now()))

  async function loadPower(r: DayRange) {
    const id = ++pseq
    try {
      const s = await deps.power(r.from, r.to)
      if (id !== pseq || !mounted) return
      power = s
      powerFailed = false
      renderPower()
    } catch (err) {
      console.warn('energy power', err)
      if (id === pseq && mounted && !power) {
        powerFailed = true
        renderPower()
      }
    }
  }

  /** renderPower draws the power chart from the kept series, its loader or its error. */
  function renderPower() {
    if (!mounted || !pchart) return
    slot('power-error').hidden = !(powerFailed && !power)
    slot('power-loading').hidden = !!power || powerFailed
    const m = power ? powerModel(power, now()) : null
    const note = slot('power-note')
    note.hidden = !m?.note
    note.textContent = m?.note ? ` · ${m.note}` : ''
    const peak = slot('power-peak')
    peak.hidden = !m?.peak
    peak.textContent = m?.peak?.caption ?? ''
    const empty = slot('power-empty')
    empty.hidden = !m?.empty
    empty.textContent = m?.empty ?? ''
    const host = slot('power-chart')
    host.hidden = !m || !!m.empty
    powerWidth = host.clientWidth
    pchart.draw(m)
  }

  /** backfillTick: called by the 5 s refresh while past days are counted. */
  async function load(backfillTick = false) {
    if (!mounted) return
    const id = ++seq
    const r = range()
    const key = `${stationKey}|${r.from}|${r.to}`
    if (key !== powerKey) {
      // Another period or station: never the old chart under the new title.
      powerKey = key
      power = null
      powerFailed = false
      renderPower()
      void loadPower(r)
    } else if (!(backfillTick && power && r.to < ymd(now()))) {
      // A past period already drawn is not re-read every 5 s of the backfill;
      // the minute refresh reads it again once the backfill is done.
      void loadPower(r)
    }
    root.setAttribute('aria-busy', 'true')
    try {
      const rep = await deps.report(r.from, r.to)
      if (id !== seq || !mounted) return
      report = rep
      failed = false
      slot('error').hidden = true
      render()
    } catch (err) {
      console.warn('energy report', err)
      // Nothing to show yet: an error with a retry, not an endless «Завантажуємо…».
      if (id === seq && mounted && !report) {
        failed = true
        slot('loading').hidden = true
        slot('error').hidden = false
      }
    } finally {
      if (id === seq && mounted) root.setAttribute('aria-busy', 'false')
    }
  }

  /** The key of the focused column, if focus is on one. */
  const focusedBar = () => (document.activeElement as Element | null)?.closest?.<HTMLElement>('.bar-g')?.dataset.key

  function drawBars() {
    if (!model) return
    const host = slot('bars')
    barsWidth = host.clientWidth
    const svg = groupedBarsSVG(barsWidth || 600, model.bars, {
      label: t(report?.Period.Weeks ? 'energy.chart.a11y.weeks' : 'energy.chart.a11y.days'),
    })
    // A refresh with the same columns keeps them, and an open tooltip with them.
    if (svg === drawn && host.firstElementChild) return
    drawn = svg
    // The redraw replaces the columns: the tooltip goes with them, and focus
    // returns to the same day (or to the chart).
    const focused = focusedBar()
    hideTip()
    host.innerHTML = svg
    if (focused) {
      const g = Array.from(host.querySelectorAll<HTMLElement>('.bar-g[tabindex]')).find(el => el.dataset.key === focused)
      ;(g ?? host).focus()
    }
  }

  function setTotal(key: string, value: string, meta?: string) {
    slot(key).innerHTML = valueHTML(value)
    const m = slot(`${key}-meta`)
    m.hidden = !meta
    m.textContent = meta ?? ''
  }

  function render() {
    if (!mounted || !report) return
    const m = model = energyModel(report, now())
    slot('loading').hidden = true
    slot('body').hidden = false
    const pv = slot('pv')
    pv.hidden = !m.pvRing
    if (m.pvRing) renderGauges(slot('gauges'), [m.pvRing])
    else slot('gauges').innerHTML = ''
    setTotal('charged', m.totals.charged)
    setTotal('discharged', m.totals.discharged, m.totals.dischargedMeta)
    setTotal('efficiency', m.totals.efficiency, m.totals.efficiencyMeta)
    const note = slot('incomplete')
    note.hidden = !m.incomplete
    const info = m.incomplete ? infoTipHTML(m.incomplete, { key: 'info.energy.incomplete' }) : ''
    keepInfoTip(() => { note.innerHTML = m.incomplete ? `${icon('info', 'icon--sm')}<span>${esc(m.incomplete)}${info}</span>` : '' })
    slot('bars-title').textContent = m.title
    const fill = slot('backfill')
    fill.hidden = !m.backfilling
    fill.innerHTML = m.backfilling ? `${spinnerHTML()}<span>${esc(m.backfilling)}</span>` : ''
    schedule(m.backfilling ? BACKFILL_REFRESH_MS : REFRESH_MS)
    const empty = slot('empty')
    empty.hidden = !m.empty
    empty.textContent = m.empty ?? ''
    drawBars()
  }

  function mountPeriodSwitcher() {
    period?.unmount()
    period = mountPeriod(slot('period'), {
      initial: mem.key, range: mem.range, now,
      onChange: (r, key) => {
        mem.key = key
        if (key === 'custom') mem.range = r
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
    const item = b && model?.bars[b.i]
    if (!b || !item?.tip) return
    // Above the higher column of the pair, centred on the pair (hi-fi 05-energy).
    const shapes = Array.from(b.g.querySelectorAll('.bar')).map(el => el.getBoundingClientRect())
    const hit = b.g.querySelector('.bar-hit')!.getBoundingClientRect()
    if (!shapes.length) shapes.push(new DOMRect(hit.left + hit.width / 2, hit.bottom, 0, 0))
    const left = Math.min(...shapes.map(r => r.left)), right = Math.max(...shapes.map(r => r.right))
    const top = Math.min(...shapes.map(r => r.top))
    showTip(b.g, item.tip, { rect: { left, top, bottom: top + 10, width: right - left } })
  }
  function onBarHide(e: Event) {
    const b = barAt(e)
    const to = (e as PointerEvent).relatedTarget
    if (b && !(to instanceof Node && b.g.contains(to))) hideTip()
  }
  function onAction(e: Event) {
    const a = (e.target as Element | null)?.closest?.<HTMLElement>('[data-action]')?.dataset.action
    if (a === 'retry') {
      slot('error').hidden = true
      slot('loading').hidden = false
      void load()
    }
    if (a === 'retry-power') {
      powerFailed = false
      renderPower()
      void loadPower(range())
    }
  }

  const resize = new ResizeObserver(() => {
    if (!mounted) return
    if (slot('bars').clientWidth !== barsWidth) drawBars()
    if (slot('power-chart').clientWidth !== powerWidth) renderPower()
  })

  function build() {
    resize.disconnect()
    root.innerHTML = layoutHTML()
    const bars = slot('bars')
    bars.addEventListener('pointerover', onBarShow)
    bars.addEventListener('pointerout', onBarHide)
    bars.addEventListener('focusin', onBarShow)
    bars.addEventListener('focusout', hideTip)
    root.addEventListener('click', onAction)
    resize.observe(bars)
    resize.observe(slot('power-chart'))
    pchart = powerChart(slot('power-chart'), slot('power-live'))
    mountPeriodSwitcher()
  }

  let offLocale = () => {}
  let tick = 0
  let every = 0
  /** schedule sets the refresh interval (faster while backfilling). */
  function schedule(ms: number) {
    if (!mounted || ms === every) return
    clearInterval(tick)
    every = ms
    tick = window.setInterval(() => void load(ms === BACKFILL_REFRESH_MS), ms)
  }
  return {
    mount(host) {
      root = host
      mounted = true
      build()
      offLocale = onLocaleChange(() => {
        // The rebuild replaces the power chart: focus and its point come back.
        const powerFocus = document.activeElement === slot('power-chart')
        const powerSel = pchart?.selected() ?? null
        root.removeEventListener('click', onAction)
        build()
        drawn = ''
        render()
        renderPower()
        if (powerFocus) {
          pchart?.select(powerSel)
          slot('power-chart').focus()
        }
        if (failed && !report) {
          slot('loading').hidden = true
          slot('error').hidden = false
        }
      })
      schedule(REFRESH_MS)
      void load()
    },
    update(s: Snapshot) {
      // Another station: its energy.
      const key = s.Station?.ID ?? ''
      if (seen && key === stationKey) return
      const first = !seen
      seen = true
      stationKey = key
      // The first snapshot only names the station the mounted load was for:
      // the chart drawn for it stays (no reset on the next refresh).
      if (first) powerKey = powerKey.replace(/^[^|]*/, key)
      else void load()
    },
    unmount() {
      mounted = false
      seq++
      pseq++
      offLocale()
      clearInterval(tick)
      every = 0
      resize.disconnect()
      period?.unmount()
      period = null
      hideTip()
      root.removeEventListener('click', onAction)
      root.removeAttribute('aria-busy')
    },
  }
}
