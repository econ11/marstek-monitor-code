/* Sample data for the mockups — ONE consistent story, simulated, so every number on
   every screen agrees (Overview, Outages, Energy, tray, toasts).
   Station: Venus E 3.0, 5.12 kWh, max charge 2500 W (we charge at 1750 → 600 W above 90%),
   max discharge 2500 W (off-grid), output stops at 10%.
   Time axis: hours since Sun 15.11.2026 00:00 (local). "Now" = Sat 21.11 15:17. */
(function () {
  const CAP = 5.12, MIN = 10, STEP = 5 / 60;          // kWh, %, h
  const DAY0 = new Date(2026, 10, 15);                  // 15 Nov 2026 (month is 0-based)
  const NOW = 6 * 24 + 15 + 17 / 60;
  const h = (day, hh, mm = 0) => day * 24 + hh + mm / 60;

  // [start, end|null(ongoing), avg discharge W, flags]
  const fixed = [
    [h(0, 9), h(0, 11), 420], [h(0, 19), h(0, 21), 400],
    [h(1, 5, 30), h(1, 7, 30), 380], [h(1, 13), h(1, 15), 450], [h(1, 20), h(1, 22), 430],
    [h(2, 16), h(2, 19), 410],
    [h(3, 13, 45), h(3, 16), 432, { cloud: true }],
    [h(4, 7), h(4, 9), 435], [h(4, 19, 30), h(5, 2, 40), 790],
    [h(5, 18), h(5, 20, 30), 430],
    [h(6, 6, 10), h(6, 8, 10), 410], [h(6, 11, 50), null, 415],
  ];
  // 23 earlier days (for the 30-day view) — seeded, so the page is stable
  let seed = 7;
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  const early = [];
  for (let d = -23; d < 0; d++) {
    const n = rnd() < 0.15 ? 0 : 1 + Math.floor(rnd() * 3);
    const slots = [6, 11, 16, 20].sort(() => rnd() - 0.5).slice(0, n).sort((a, b) => a - b);
    for (const s of slots) {
      const st = h(d, s, Math.floor(rnd() * 4) * 15), len = [2, 2, 2.5, 3, 4][Math.floor(rnd() * 5)];
      early.push([st, st + len, 360 + Math.floor(rnd() * 100)]);
    }
  }
  const outages = early.concat(fixed).map(([s, e, p, f]) => ({ start: s, end: e, p, ongoing: e == null, cloud: !!(f && f.cloud) }));
  const gaps = [{ start: h(6, 3, 10), end: h(6, 4, 20), kind: 'nodata' }];   // PC off, cloud had nothing

  // ---- simulate 5-min samples ----
  const samples = [];
  let soc = 100, dead = null;
  for (let t = h(-23, 0); t <= NOW + 1e-9; t += STEP) {
    const o = outages.find(o => t >= o.start && t < (o.end ?? Infinity));
    const g = gaps.find(g => t >= g.start && t < g.end);
    let p = 0;
    if (o) {
      if (dead == null) {
        const jitter = Math.sin(t * 7.3) * 40 + Math.sin(t * 2.1) * 25;
        p = -(o.p + jitter);
        soc += (p / 1000) * STEP / CAP * 100;
        if (soc <= MIN) { soc = MIN; dead = t; o.diedAt = t; }
      }
    } else {
      dead = null;
      if (soc < 100) { p = soc < 90 ? 1750 : 600; soc = Math.min(100, soc + (p / 1000) * STEP / CAP * 100); }
    }
    const grid = !o;
    if (g || (o && dead != null && t > dead)) samples.push({ t, soc: null, p: null, grid, gap: g ? 'nodata' : 'dead' });
    else samples.push({ t, soc: Math.round(soc * 10) / 10, p: Math.round(p), grid });
  }
  // soc at start/end of each outage, min soc
  for (const o of outages) {
    const inside = samples.filter(s => s.t >= o.start && s.t <= (o.end ?? NOW) && s.soc != null);
    o.socStart = Math.round(inside[0]?.soc ?? 100);
    o.socEnd = Math.round(inside[inside.length - 1]?.soc ?? MIN);
    o.minSoc = Math.round(Math.min(...inside.map(s => s.soc)));
    if (o.diedAt) o.socEnd = MIN;
  }

  const DOW = ['нд', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб'];
  const DOW_LONG = ['Неділя', 'Понеділок', 'Вівторок', 'Середа', 'Четвер', "П'ятниця", 'Субота'];
  const MON_GEN = ['січня', 'лютого', 'березня', 'квітня', 'травня', 'червня', 'липня', 'серпня', 'вересня', 'жовтня', 'листопада', 'грудня'];
  const pad = n => String(n).padStart(2, '0');
  const dateOf = t => { const d = new Date(DAY0); d.setMinutes(d.getMinutes() + Math.round(t * 60)); return d; };
  const fmt = {
    time: t => { const d = dateOf(t); return `${pad(d.getHours())}:${pad(d.getMinutes())}`; },
    dayShort: t => { const d = dateOf(t); return `${DOW[d.getDay()]} ${pad(d.getDate())}.${pad(d.getMonth() + 1)}`; },
    dm: t => { const d = dateOf(t); return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}`; },
    dow: t => DOW[dateOf(t).getDay()],
    dayTip: t => { const d = dateOf(t); const w = DOW[d.getDay()]; return `${w[0].toUpperCase() + w.slice(1)}, ${d.getDate()} ${MON_GEN[d.getMonth()]}`; },
    dayLong: t => { const d = dateOf(t); return `${DOW_LONG[d.getDay()]}, ${d.getDate()} ${MON_GEN[d.getMonth()]}`; },
    dur: hrs => { const m = Math.round(hrs * 60); return `${Math.floor(m / 60)} год ${pad(m % 60)} хв`; },
    durShort: hrs => { const m = Math.round(hrs * 60); return `${Math.floor(m / 60)}:${pad(m % 60)}`; },
    kwh: v => v.toFixed(1).replace('.', ','),
    dayIndex: t => Math.floor(t / 24),
  };
  // NB: Ukrainian decimal separator is a comma (3,7 кВт·год). kwh() follows that; see README.

  // ---- per-day aggregates ----
  function day(d) {
    const s0 = d * 24, s1 = Math.min(s0 + 24, NOW);
    const day = { d, t: s0, count: 0, off: 0, longest: null, minSoc: 100, died: false, cloud: false, ongoing: false, chg: 0, dis: 0, peak: null, disOutage: 0 };
    for (const o of outages) {
      const e = o.end ?? NOW, a = Math.max(o.start, s0), b = Math.min(e, s1);
      if (b > a) {
        day.off += b - a;
        if (o.minSoc < day.minSoc) day.minSoc = o.minSoc;
        if (o.diedAt && o.diedAt >= s0 && o.diedAt < s1 + 24) day.died = day.died || (o.start >= s0 && o.start < s1);
        if (o.cloud) day.cloud = true;
      }
      if (o.start >= s0 && o.start < s0 + 24) {
        day.count++; if (o.ongoing) day.ongoing = true;
        const len = e - o.start;
        if (!day.longest || len > day.longest.len) day.longest = { len, o };
        if (o.diedAt) day.died = true;
      }
    }
    for (const s of samples) if (s.t >= s0 && s.t < s1 && s.p != null) {
      if (s.p > 0) day.chg += s.p / 1000 * STEP;
      if (s.p < 0) { day.dis += -s.p / 1000 * STEP; if (!s.grid) day.disOutage += -s.p / 1000 * STEP; if (!day.peak || -s.p > day.peak.w) day.peak = { w: -s.p, t: s.t }; }
    }
    day.span = s1 - s0;
    return day;
  }
  const days = []; for (let d = -23; d <= 6; d++) days.push(day(d));

  window.MMData = { CAP, MIN, NOW, h, outages, gaps, samples, days, fmt, dateOf };
})();
