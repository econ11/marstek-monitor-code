import type { FRContext, FRState } from './firstRunModel'
import type { FirstRunScreen } from './firstRun'

// The scripted first run (-simulate-e2e only, Wails flag e2e=firstrun): with
// no input allowed on the owner's PC, the screen drives itself through the
// same actions a click runs, and says where it is in the window title, which
// an agent reads (GetWindowText) and photographs (PrintWindow) between steps:
//   found (the simulator's row) → IP 127.0.0.2 (nothing there: no answer,
//   other subnet) → «1.2.3» (format) → search again → Add → done → overview.
// Each pause leaves time for a capture.

const HOLD_MS = 6_000

type Step = { st: FRState; ctx: Readonly<FRContext> }

export function runFirstRunE2E(screen: FirstRunScreen, setTitle: (title: string) => void, finish: () => void) {
  const log: string[] = []
  const say = (line: string) => {
    log.push(line)
    setTitle(`Marstek Monitor [e2e] ${line}`)
    console.info('[e2e]', line)
  }
  const sleep = (ms: number) => new Promise(r => setTimeout(r, ms))
  const now = (): Step => ({ st: screen.state(), ctx: screen.context() })
  async function until(ok: (s: Step) => boolean, ms: number, what: string): Promise<Step> {
    const end = Date.now() + ms
    while (Date.now() < end) {
      const s = now()
      if (ok(s)) return s
      await sleep(100)
    }
    throw new Error(`timed out waiting for ${what} (state ${screen.state()})`)
  }

  async function script() {
    let s = await until(x => x.st === 'found' || x.st === 'noneFound', 20_000, 'the scan')
    say(`1 ${s.st} rows=${s.ctx.devices.length} ${s.ctx.devices.map(d => `${d.Model}/${d.ModelCode}/${d.IP}/v${d.Firmware}`).join(',')}`)
    if (s.st !== 'found') throw new Error('the simulator was not found')
    await sleep(HOLD_MS)

    screen.act('enterIp')
    screen.act('submitIp', '127.0.0.2')
    s = await until(x => x.st !== 'probingIp' && x.st !== 'manualIp' || x.ctx.ipError !== undefined, 15_000, 'the probe')
    say(`2 probe 127.0.0.2 → ${s.st} code=${s.ctx.ipError ?? '-'}`)
    await sleep(HOLD_MS)

    screen.act('submitIp', '1.2.3')
    s = now()
    say(`3 submit 1.2.3 → ${s.st} code=${s.ctx.ipError ?? '-'}`)
    await sleep(2_000)

    screen.act('tile', 'lan')
    s = await until(x => x.st === 'found' && x.ctx.devices.length > 0, 20_000, 'the second scan')
    const d = s.ctx.devices[0]
    say(`4 rescan → found; add ${d.Model}`)
    screen.act('add', d.BLEMac)
    s = await until(x => x.st === 'done', 30_000, 'done')
    const f = s.ctx.first
    say(`5 done name=${s.ctx.name} first=${f ? `soc ${f.soc} grid ${f.grid}` : 'waiting'}`)
    await sleep(HOLD_MS + 4_000)

    finish()
    say('6 overview')
  }

  script().catch(err => say(`FAIL ${err instanceof Error ? err.message : String(err)} after: ${log.join(' | ')}`))
}
