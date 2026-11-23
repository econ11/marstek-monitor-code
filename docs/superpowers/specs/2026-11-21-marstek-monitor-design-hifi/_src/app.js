/* Marstek Monitor mockups — shared behaviour.
   1. Tooltip engine: one portalled .tip, edge-aware (shift + flip), never leaves the
      visible area of the app window (or the viewport, whichever is smaller).
   2. Segmented controls with a sliding thumb.
   3. Switches.
   This is reference behaviour for the frontend; any chart library used in the app
   must reproduce section 1 exactly (see README → Tooltip). */
(function () {
  'use strict';
  const EDGE = 8;        // px kept free between tooltip and the edge of the bounds
  const GAP = 10;        // px between anchor and tooltip
  const OPEN_DELAY = 120, WARM_MS = 300;

  const tip = document.createElement('div');
  tip.className = 'tip';
  tip.setAttribute('role', 'tooltip');
  tip.id = 'mm-tip';
  tip.innerHTML = '<div class="tip__body"></div><span class="tip__arrow"></span>';
  document.addEventListener('DOMContentLoaded', () => document.body.appendChild(tip));
  const body = () => tip.firstElementChild, arrow = () => tip.lastElementChild;

  let openTimer = 0, lastClose = 0, current = null;

  function boundsFor(el) {
    const vw = document.documentElement.clientWidth, vh = document.documentElement.clientHeight;
    let b = { left: 0, top: 0, right: vw, bottom: vh };
    const win = el && el.closest && el.closest('.win');
    if (win) {
      const r = win.getBoundingClientRect();
      b = { left: Math.max(b.left, r.left), top: Math.max(b.top, r.top + 32), right: Math.min(b.right, r.right), bottom: Math.min(b.bottom, r.bottom) };
    }
    return { left: b.left + EDGE, top: b.top + EDGE, right: b.right - EDGE, bottom: b.bottom - EDGE };
  }

  /* rect: anchor rect in viewport coords. Point anchors (crosshair) pass a 0-size rect. */
  function place(rect, ctx, prefer) {
    const b = boundsFor(ctx);
    tip.style.maxWidth = Math.min(280, b.right - b.left) + 'px';
    const w = tip.offsetWidth, h = tip.offsetHeight;
    const cx = rect.left + rect.width / 2;
    let side = prefer || 'top';
    let y = side === 'top' ? rect.top - GAP - h : rect.bottom + GAP;
    if (side === 'top' && y < b.top) { side = 'bottom'; y = rect.bottom + GAP; }
    if (side === 'bottom' && y + h > b.bottom) {
      if (rect.top - GAP - h >= b.top) { side = 'top'; y = rect.top - GAP - h; }
      else y = Math.max(b.top, b.bottom - h);   // no room either way: pin inside, cover the anchor
    }
    let x = Math.round(cx - w / 2);
    x = Math.min(Math.max(x, b.left), b.right - w);
    tip.dataset.side = side;
    tip.style.transform = '';
    tip.style.left = x + 'px';
    tip.style.top = Math.round(y) + 'px';
    const ax = Math.min(Math.max(cx - x - 5, 10), w - 20);
    arrow().style.left = ax + 'px';
  }

  function show(anchor, html, opts = {}) {
    body().innerHTML = html;
    current = anchor;
    const rect = opts.rect || anchor.getBoundingClientRect();
    place(rect, anchor, opts.side);
    tip.classList.add('is-open');
    if (anchor.setAttribute) anchor.setAttribute('aria-describedby', 'mm-tip');
  }
  function hide() {
    clearTimeout(openTimer);
    if (!tip.classList.contains('is-open')) return;
    tip.classList.remove('is-open');
    if (current && current.removeAttribute) current.removeAttribute('aria-describedby');
    current = null; lastClose = Date.now();
  }
  function contentFor(el) {
    const ref = el.getAttribute('data-tip');
    if (ref && ref.startsWith('#')) { const t = document.querySelector(ref); return t ? t.innerHTML : ''; }
    return ref || '';
  }
  function scheduleShow(el) {
    clearTimeout(openTimer);
    const warm = Date.now() - lastClose < WARM_MS || tip.classList.contains('is-open');
    openTimer = setTimeout(() => show(el, contentFor(el), { side: el.dataset.tipSide }), warm ? 0 : OPEN_DELAY);
  }

  document.addEventListener('pointerover', e => {
    const el = e.target.closest && e.target.closest('[data-tip]');
    if (el && el !== current) scheduleShow(el);
  });
  document.addEventListener('pointerout', e => {
    const el = e.target.closest && e.target.closest('[data-tip]');
    if (el && !el.contains(e.relatedTarget)) hide();
  });
  document.addEventListener('focusin', e => { const el = e.target.closest('[data-tip]'); if (el) scheduleShow(el); });
  document.addEventListener('focusout', e => { if (e.target.closest('[data-tip]')) hide(); });
  document.addEventListener('keydown', e => { if (e.key === 'Escape') hide(); });
  document.addEventListener('scroll', hide, true);
  window.addEventListener('resize', hide);

  /* ---------- Segmented controls ---------- */
  function syncSeg(seg) {
    const on = seg.querySelector('[aria-pressed="true"]');
    let thumb = seg.querySelector('.seg__thumb');
    if (!thumb) { thumb = document.createElement('span'); thumb.className = 'seg__thumb'; seg.prepend(thumb); }
    if (!on) { thumb.style.width = '0'; return; }
    thumb.style.width = on.offsetWidth + 'px';
    thumb.style.transform = `translateX(${on.offsetLeft}px)`;
  }
  document.addEventListener('click', e => {
    const item = e.target.closest('.seg__item');
    if (item) {
      const seg = item.closest('.seg');
      seg.querySelectorAll('.seg__item').forEach(b => b.setAttribute('aria-pressed', String(b === item)));
      syncSeg(seg);
      seg.dispatchEvent(new CustomEvent('seg-change', { bubbles: true, detail: item.dataset.value }));
    }
    const sw = e.target.closest('.switch');
    if (sw) {
      const on = sw.getAttribute('aria-checked') !== 'true';
      sw.setAttribute('aria-checked', String(on));
      const row = sw.closest('[data-dependent]');
      if (row) row.classList.toggle('is-off', !on);
    }
  });
  const syncAll = () => document.querySelectorAll('.seg').forEach(syncSeg);
  document.addEventListener('DOMContentLoaded', syncAll);
  if (document.fonts) document.fonts.ready.then(syncAll);

  /* ---------- Formatting helpers (uk) ---------- */
  const pad = n => String(n).padStart(2, '0');
  const dur = h => { const m = Math.round(h * 60); return `${Math.floor(m / 60)} год ${pad(m % 60)} хв`; };

  window.MM = { tip: { show, hide, place }, pad, dur };
})();
