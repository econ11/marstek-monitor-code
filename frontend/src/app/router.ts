import type { Snapshot } from '../../bindings/github.com/econ11/marstek-monitor-code/core/monitor/models'

// The window's tabs. One screen is mounted at a time in the shell's view slot;
// the tabs and the ⚙ button are the shell's [data-view] elements.

export type View = 'overview' | 'outages' | 'errors' | 'battery' | 'energy' | 'settings' | 'firstRun'
export const VIEWS: readonly View[] = ['overview', 'outages', 'errors', 'battery', 'energy', 'settings', 'firstRun']
export type NavOpts = { outageId?: number; section?: string }
export type Screen = { mount(host: HTMLElement): void; update(s: Snapshot): void; unmount(): void }

export const isView = (v: unknown): v is View => VIEWS.includes(v as View)

/** createRouter mounts screens into host's [data-slot="view"] and marks the
 *  [data-view] element of the current one with aria-current="page"; a click
 *  on any [data-view] element navigates. Each screen factory gets the
 *  navigation options (an outage to highlight, a Settings section).
 *  update() hands every snapshot to the current screen and keeps the last
 *  one for the next screen mounted. */
export function createRouter(host: HTMLElement, screens: Record<View, (o: NavOpts) => Screen>) {
  let view: View | null = null
  let screen: Screen | null = null
  let last: Snapshot | null = null

  function mark() {
    host.querySelectorAll<HTMLElement>('[data-view]').forEach(el => {
      if (el.dataset.view === view) el.setAttribute('aria-current', 'page')
      else el.removeAttribute('aria-current')
    })
  }

  function navigate(v: View, o?: NavOpts) {
    // The same tab again is a no-op, unless it is asked to show something.
    if (v === view && !o?.outageId && !o?.section) return
    screen?.unmount()
    const slot = host.querySelector<HTMLElement>('[data-slot="view"]')!
    slot.replaceChildren()
    slot.scrollTop = 0
    view = v
    screen = screens[v](o ?? {})
    mark()
    screen.mount(slot)
    if (last) screen.update(last)
  }

  host.addEventListener('click', e => {
    const el = (e.target as Element | null)?.closest?.<HTMLElement>('[data-view]')
    if (!el || !isView(el.dataset.view)) return
    e.preventDefault()
    navigate(el.dataset.view)
  })

  return {
    navigate,
    current: (): View => view ?? 'overview',
    update(s: Snapshot) {
      last = s
      screen?.update(s)
    },
  }
}
