import { expect, test } from 'vitest'
import { createRouter, type Screen, type View } from './router'

// The router needs only a few DOM calls; a small stand-in keeps the test
// free of a DOM library.
class FakeEl {
  attrs = new Map<string, string>()
  dataset: Record<string, string> = {}
  children = 0
  getAttribute(n: string) { return this.attrs.get(n) ?? null }
  setAttribute(n: string, v: string) { this.attrs.set(n, v) }
  removeAttribute(n: string) { this.attrs.delete(n) }
  replaceChildren() { this.children = 0 }
  addEventListener() {}
}

const VIEWS: View[] = ['overview', 'outages', 'errors', 'battery', 'energy', 'settings', 'firstRun']

function setup() {
  const tabs = ['overview', 'outages', 'errors', 'battery', 'energy', 'settings'].map(v => {
    const el = new FakeEl()
    el.dataset.view = v
    return el
  })
  const slot = new FakeEl()
  const host = {
    querySelectorAll: () => tabs,
    querySelector: () => slot,
    addEventListener() {},
  } as unknown as HTMLElement
  const log: string[] = []
  const screens = Object.fromEntries(VIEWS.map(v => [v, (): Screen => ({
    mount: () => { log.push(`mount ${v}`) },
    update: () => { log.push(`update ${v}`) },
    unmount: () => { log.push(`unmount ${v}`) },
  })])) as Record<View, () => Screen>
  return { router: createRouter(host, screens), tabs, log }
}

test('router marks the current tab', () => {
  const { router, tabs, log } = setup()
  router.navigate('overview')
  log.length = 0
  router.navigate('outages')
  expect(router.current()).toBe('outages')
  const current = tabs.filter(t => t.getAttribute('aria-current') === 'page').map(t => t.dataset.view)
  expect(current).toEqual(['outages'])
  // The previous screen is unmounted first, the new one mounted once.
  expect(log).toEqual(['unmount overview', 'mount outages'])
  router.navigate('outages') // already there: nothing happens
  expect(log).toEqual(['unmount overview', 'mount outages'])
})

// Task 20c (Task 7 review): a screen mounted after a snapshot gets the last
// one at once; the same tab asked to show an outage or a section is mounted
// again with those options.
test('router replays the last snapshot and remounts for options', () => {
  const tabs: FakeEl[] = []
  const slot = new FakeEl()
  const host = { querySelectorAll: () => tabs, querySelector: () => slot, addEventListener() {} } as unknown as HTMLElement
  const log: string[] = []
  const screens = Object.fromEntries(VIEWS.map(v => [v, (o: { outageId?: number; section?: string }): Screen => ({
    mount: () => { log.push(`mount ${v}${o.outageId ? ` #${o.outageId}` : ''}${o.section ? ` ${o.section}` : ''}`) },
    update: s => { log.push(`update ${v} ${(s as unknown as { Seq: number }).Seq}`) },
    unmount: () => { log.push(`unmount ${v}`) },
  })])) as Record<View, (o: { outageId?: number; section?: string }) => Screen>
  const router = createRouter(host, screens)
  router.navigate('overview')
  router.update({ Seq: 7 } as never)
  router.navigate('outages', { outageId: 42 })
  router.navigate('outages', { outageId: 43 })
  router.navigate('settings', { section: 'notifications' })
  router.navigate('settings', { section: 'data' })
  expect(log).toEqual([
    'mount overview', 'update overview 7',
    'unmount overview', 'mount outages #42', 'update outages 7',
    'unmount outages', 'mount outages #43', 'update outages 7',
    'unmount outages', 'mount settings notifications', 'update settings 7',
    'unmount settings', 'mount settings data', 'update settings 7',
  ])
})
