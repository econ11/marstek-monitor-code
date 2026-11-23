/* SOC chart (Overview). One renderer for every connection state.
   opts: { range: 24|168|720, now: t, lastData: t (last fresh measurement; default = now),
           tail: null | 'stale' | 'nolink' | 'dead'  — what is drawn between lastData and now,
           skeleton: bool, observedSince: t (axis left of it is empty, no frame) } */
(function () {
  const D = MMData, F = D.fmt;
  function render(host, opts = {}) {
    const range = opts.range || 24, now = opts.now ?? D.NOW, last = opts.lastData ?? now, tail = opts.tail || null;
    const W = host.clientWidth, H = opts.height || 236;
    const P = { l: 40, r: 8, t: 26, b: 26 };
    const t1 = now, t0 = t1 - range;
    const x = t => P.l + (t - t0) / (t1 - t0) * (W - P.l - P.r);
    const y = v => P.t + (1 - v / 100) * (H - P.t - P.b);
    const step = range <= 24 ? 1 : range <= 168 ? 3 : 6;
    const since = opts.observedSince ?? -Infinity;
    const pts = D.samples.filter((s, i) => s.t >= Math.max(t0, since) - 0.01 && s.t <= last + 1e-6 && i % step === 0);
    let s = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img" aria-label="Заряд станції, % — ${range === 24 ? '24 год' : range === 168 ? '7 днів' : '30 днів'}">`;
    s += `<defs><linearGradient id="socfill-${host.id}" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stop-color="var(--color-good)" stop-opacity=".13"/><stop offset="1" stop-color="var(--color-good)" stop-opacity="0"/></linearGradient><clipPath id="plot-${host.id}"><rect x="${P.l}" y="0" width="${W - P.l - P.r}" height="${H}"/></clipPath></defs>`;
    s += '<g class="grid">';
    for (const v of [0, 25, 50, 75, 100]) s += `<line x1="${P.l}" x2="${W - P.r}" y1="${y(v)}" y2="${y(v)}" class="${v === 0 ? 'base' : ''}"/>`;
    s += '</g><g class="axis">';
    for (const v of [0, 25, 50, 75, 100]) s += `<text x="${P.l - 8}" y="${y(v) + 4}" text-anchor="end">${v}</text>`;
    const ticks = [];
    const every = range === 24 ? 3 : range === 168 ? 24 : 96;
    for (let t = Math.ceil(t0 / every) * every; t <= t1; t += every) ticks.push(t);
    for (const t of ticks) {
      const midnight = Math.abs(t % 24) < 1e-6;
      const label = range === 24 ? (midnight ? F.dayShort(t) : F.time(t)) : F.dayShort(t);
      s += `<text x="${x(t)}" y="${H - 6}" text-anchor="middle" class="${midnight && range === 24 ? 'strong' : ''}">${label}</text>`;
    }
    s += `</g>`;
    if (opts.skeleton) {
      s += `<rect x="${P.l}" y="${P.t}" width="${W - P.l - P.r}" height="${H - P.t - P.b}" rx="6" class="skel-fill"/></svg>`;
      host.innerHTML = s; return;
    }
    if (since > t0) s += `<text class="gap-label" x="${x(since) + 8}" y="${y(50) + 4}">спостереження з ${F.time(since)}</text><line x1="${x(since)}" x2="${x(since)}" y1="${P.t}" y2="${H - P.b}" stroke="var(--color-border-strong)"/>`;
    s += `<g clip-path="url(#plot-${host.id})">`;
    // outage bands (no-grid), labels never collide
    let lastRight = -Infinity;
    for (const o of D.outages) {
      if (o.start >= last) continue;
      const end = Math.min(o.end ?? Infinity, last);
      const a = Math.max(o.start, t0, since), b = Math.min(end, t1);
      if (b <= a) continue;
      const bx = x(a), bw = x(b) - bx;
      s += `<rect class="band" x="${bx}" y="${P.t}" width="${bw}" height="${H - P.t - P.b}"/>`;
      const known = Math.min(o.end ?? last, last) - o.start;
      const ongoing = o.end == null || o.end > last;
      const opts2 = ongoing && tail
        ? [`без світла ≥ ${F.dur(known)}`, `≥ ${F.dur(known)}`]
        : [`без світла ${F.dur(known)}${ongoing ? ', триває' : ''}`, F.dur(known)];
      for (const txt of opts2) {
        const tw = txt.length * 6.1;
        if (bw < 30 && range !== 24) break;
        const left = Math.min(Math.max(bx + bw / 2 - tw / 2, P.l), W - P.r - tw);
        if (left < lastRight + 10) continue;
        s += `<text class="band-label" x="${left}" y="${P.t - 9}">${txt}</text>`;
        lastRight = left + tw; break;
      }
    }
    // gaps inside history
    const gapRuns = []; let run = null;
    const dt = (D.samples[1].t - D.samples[0].t) * step;
    for (const p of pts) {
      if (p.soc == null) { if (!run) run = { a: p.t, kind: p.gap }; run.b = p.t + dt; }
      else if (run) { gapRuns.push(run); run = null; }
    }
    if (run) gapRuns.push(run);
    if (tail === 'nolink' || tail === 'dead') gapRuns.push({ a: last, b: now, kind: tail, open: true });
    for (const g of gapRuns) {
      const gx = x(g.a), gw = Math.max(2, x(g.b) - gx);
      s += `<rect class="gap${g.open ? ' gap--open' : ''}" x="${gx}" y="${P.t + 1}" width="${gw}" height="${H - P.t - P.b - 2}" rx="3"/>`;
      const label = { dead: 'ймовірно, розрядилась', nolink: 'немає даних', nodata: 'немає даних' }[g.kind] || 'немає даних';
      if (gw > label.length * 6 + 8) s += `<text class="gap-label" x="${gx + gw / 2}" y="${y(50) + 4}" text-anchor="middle">${label}</text>`;
    }
    let line = '', area = '', seg = [];
    const flush = () => {
      if (seg.length > 1) {
        const d = seg.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.soc).toFixed(1)}`).join('');
        line += d; area += d + `L${x(seg[seg.length - 1].t).toFixed(1)},${y(0)}L${x(seg[0].t).toFixed(1)},${y(0)}Z`;
      }
      seg = [];
    };
    for (const p of pts) { if (p.soc == null) flush(); else seg.push(p); }
    flush();
    const muted = !!tail;
    const lc = muted ? 'var(--color-text-subtle)' : 'var(--color-good)';
    if (!muted) s += `<path d="${area}" fill="url(#socfill-${host.id})"/>`;
    s += `<line class="threshold" x1="${P.l}" x2="${W - P.r}" y1="${y(20)}" y2="${y(20)}"/>`;
    s += `<path d="${line}" fill="none" stroke="${lc}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`;
    const lp = [...pts].reverse().find(p => p.soc != null);
    if (lp) s += `<circle cx="${x(lp.t)}" cy="${y(lp.soc)}" r="3.5" fill="${muted ? 'var(--color-surface-1)' : lc}" stroke="${muted ? lc : 'var(--color-surface-1)'}" stroke-width="2"/>`;
    if (muted && lp) s += `<text class="gap-label" x="${x(lp.t) - 6}" y="${y(lp.soc) - 10}" text-anchor="end">${lp.soc.toFixed(0)}% о ${F.time(lp.t)}</text>`;
    s += '</g>';
    s += `<text class="threshold-label" x="${W - P.r}" y="${y(20) - 5}" text-anchor="end">20%</text>`;
    s += `<g class="hover" style="display:none"><line class="crosshair" y1="${P.t}" y2="${H - P.b}"/><circle r="4" fill="${lc}" stroke="var(--color-surface-1)" stroke-width="2"/></g>`;
    s += `<rect class="hit" x="${P.l}" y="${P.t}" width="${W - P.l - P.r}" height="${H - P.t - P.b}" fill="transparent"/></svg>`;
    host.innerHTML = s;

    const svg = host.firstChild, hover = svg.querySelector('.hover'), hit = svg.querySelector('.hit');
    hit.addEventListener('pointermove', e => {
      const r = svg.getBoundingClientRect();
      const t = t0 + (e.clientX - r.left - P.l) / (W - P.l - P.r) * (t1 - t0);
      const when = tt => `${F.dayShort(tt)} · ${F.time(tt)}`;
      let html, px, ay, dotOn = false;
      if (t > last && tail) {
        px = x(Math.min(t, now)); ay = y(50);
        html = tail === 'stale'
          ? `<div class="tip__title">Чекаємо даних<small>${when(t)}</small></div><dl><dt>Останні дані</dt><dd>о ${F.time(last)}</dd></dl>`
          : tail === 'dead'
            ? `<div class="tip__title">Немає даних<small>з ${F.time(last)}</small></div><dl><dt>Причина</dt><dd>станція, ймовірно, розрядилась</dd><dt>Заряд перед цим</dt><dd>${lp ? lp.soc.toFixed(0) : '—'}%</dd></dl>`
            : `<div class="tip__title">Немає даних<small>з ${F.time(last)}</small></div><dl><dt>Що зі світлом</dt><dd>невідомо</dd></dl>`;
      } else {
        let best = pts[0];
        for (const p of pts) if (Math.abs(p.t - t) < Math.abs(best.t - t)) best = p;
        px = x(best.t);
        if (best.soc == null) {
          ay = y(50);
          const o = D.outages.find(o => o.diedAt && best.t >= o.diedAt && best.t < o.end);
          html = `<div class="tip__title">Немає даних<small>${when(best.t)}</small></div>` + (best.gap === 'dead' && o
            ? `<dl><dt>Станція розрядилась</dt><dd>о ${F.time(o.diedAt)}</dd><dt>Знову на зв'язку</dt><dd>о ${F.time(o.end)}</dd></dl>`
            : `<dl><dt>Причина</dt><dd>додаток не працював</dd></dl><div class="tip__foot">Що було зі світлом, невідомо.</div>`);
        } else {
          dotOn = true; ay = y(best.soc);
          hover.lastChild.setAttribute('cx', px); hover.lastChild.setAttribute('cy', ay);
          const pw = best.p > 0 ? `<dd class="t-good">заряд ${best.p} Вт</dd>` : best.p < 0 ? `<dd class="t-warn">розряд ${-best.p} Вт</dd>` : '<dd>очікування</dd>';
          html = `<div class="tip__title">${Math.round(best.soc)}%<small>${when(best.t)}</small></div><dl><dt>Потужність</dt>${pw}<dt>Світло</dt>${best.grid ? '<dd class="t-good">є</dd>' : '<dd class="t-warn">немає</dd>'}</dl>`;
        }
      }
      hover.style.display = '';
      hover.lastChild.style.display = dotOn ? '' : 'none';
      hover.firstChild.setAttribute('x1', px); hover.firstChild.setAttribute('x2', px);
      MM.tip.show(svg, html, { rect: { left: r.left + px, right: r.left + px, width: 0, top: r.top + ay - 6, bottom: r.top + ay + 6, height: 12 } });
    });
    hit.addEventListener('pointerleave', () => { hover.style.display = 'none'; MM.tip.hide(); });
  }
  window.MMSoc = { render };
})();
