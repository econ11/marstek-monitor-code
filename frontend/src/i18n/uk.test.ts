import { expect, test } from 'vitest'
import { en } from './en'
import { uk } from './uk'

// v1 is LAN-only: no string the app can show may mention the cloud. The
// catalogue keys that do stay in uk.ts and en.ts for later, but nothing may
// use them.

const CLOUD = /хмар|cloud/i

// Every source file of the app as text, without the tests and the catalogues.
const files = import.meta.glob('../**/*.ts', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const code = Object.entries(files)
  .filter(([path]) => !path.endsWith('.test.ts') && !path.endsWith('/uk.ts') && !path.endsWith('/en.ts'))
  .map(([, text]) => text)
  .join('\n')

// Every key the code can pass to t(): quoted keys anywhere in the sources
// (some reach t() through a variable), and the prefix of each template key.
function usedKeys(): string[] {
  const literal = Array.from(code.matchAll(/['"]([a-zA-Z][\w]*(?:\.[\w]+)+)['"]/g), m => m[1])
  const prefixes = Array.from(code.matchAll(/`([a-zA-Z][\w.]*\.)\$\{/g), m => m[1])
  // Chart band and gap labels arrive from the backend as keys (core/monitor).
  prefixes.push('gap.label', 'overview.chart.band.')
  // So do the journal's «what happened» and «what to do» (core/journal).
  prefixes.push('journal.', 'conn.event.')
  return Object.keys(uk).filter(k => literal.includes(k) || prefixes.some(p => k.startsWith(p)))
}

test('the scan finds the keys the code uses', () => {
  const keys = usedKeys()
  expect(keys.length).toBeGreaterThan(80)
  for (const k of ['overview.ring.soc.label', 'overview.ring.power.unit.charging', 'gap.label.pcSleep', 'link.dx.pcOffline.v1', 'gap.tooltip.appOff.v1',
    'conn.event.apiSilent.todo', 'journal.cutoff', 'errors.empty.noLogSource']) {
    expect(keys).toContain(k)
  }
})

test('no string used in v1 mentions the cloud, in either language', () => {
  const keys = usedKeys()
  expect(keys.filter(k => CLOUD.test(uk[k]))).toEqual([])
  expect(keys.filter(k => CLOUD.test(en[k]))).toEqual([])
})

test('the catalogue texts with a cloud are kept, unused, for a later version', () => {
  const keys = usedKeys()
  for (const k of ['link.dx.lanSilentNoCloud', 'link.dx.pcOffline', 'gapCard.noCloud', 'gap.tooltip.appOff']) {
    expect(uk[k]).toMatch(CLOUD)
    expect(en[k]).toMatch(CLOUD)
    expect(keys).not.toContain(k)
  }
})

// «Станція розрядилась» is always a conclusion (LIKELY_DEAD, flows §2.2),
// never an observed fact: every text that says so must say «ймовірно».
test('every text saying the station ran out says «ймовірно»', () => {
  // «вимкнула вихід» is the same kind of conclusion (journal.cutoff).
  const ranOut = Object.keys(uk).filter(k => /розрядилась|вимкнула вихід/i.test(uk[k]))
  expect(ranOut).toEqual(expect.arrayContaining([
    'gap.label.likelyOut', 'overview.ring.soc.likelyOut.sub', 'state.stationOut', 'gapCard.stationOut', 'link.likelyOut.title',
    'journal.likelyOut', 'journal.cutoff', 'info.overview.chart.noData', 'info.outages.stationOut',
  ]))
  expect(ranOut.filter(k => !/ймовірно/i.test(uk[k]))).toEqual([])
})

// «most likely» is the same hedge (info.outages.stationOut, copy §15).
test('every en text saying the station ran out says «probably» or «most likely»', () => {
  const ranOut = Object.keys(en).filter(k => /ran out|turned its output off/i.test(en[k]))
  expect(ranOut).toEqual(expect.arrayContaining([
    'gap.label.likelyOut', 'overview.ring.soc.likelyOut.sub', 'state.stationOut', 'gapCard.stationOut', 'link.likelyOut.title',
    'journal.likelyOut', 'journal.cutoff', 'info.overview.chart.noData', 'info.outages.stationOut',
  ]))
  expect(ranOut.filter(k => !/probably|most likely/i.test(en[k]))).toEqual([])
})
