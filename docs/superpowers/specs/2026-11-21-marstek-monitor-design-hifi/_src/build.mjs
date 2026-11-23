// Builds the self-contained mockup pages.
//   node docs/superpowers/specs/2026-11-21-marstek-monitor-design-hifi/_src/build.mjs
// Reads _src/pages/*.html, expands <!--@macro key="value"--> includes, inlines
// tokens.css + components.css + app.js, writes the result one level up.
import { readFileSync, writeFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = dirname(fileURLToPath(import.meta.url));
const OUT = join(SRC, '..');
const read = p => readFileSync(join(SRC, p), 'utf8');
const tokens = readFileSync(join(OUT, 'tokens.css'), 'utf8');
const components = read('components.css');
const appjs = read('app.js');

const ICONS = {
  'plug': '<path d="M12 22v-5"/><path d="M9 8V2"/><path d="M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z"/>',
  'plug-off': '<path d="M12 22v-5"/><path d="M9 8V2"/><path d="M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z"/><path d="m3 3 18 18"/>',
  'zap': '<path d="M13 2 4 14h8l-1 8 9-12h-8l1-8Z"/>',
  'battery': '<rect x="2" y="7" width="17" height="10" rx="2"/><path d="M22 11v2"/><path d="M6 11v2M10 11v2"/>',
  'alert': '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
  'check-circle': '<circle cx="12" cy="12" r="9.5"/><path d="m8.5 12 2.5 2.5 4.5-5"/>',
  'check': '<path d="M20 6 9 17l-5-5"/>',
  'wifi': '<path d="M12 20h.01"/><path d="M2 8.8a15 15 0 0 1 20 0"/><path d="M5 12.9a10 10 0 0 1 14 0"/><path d="M8.5 16.4a5 5 0 0 1 7 0"/>',
  'wifi-off': '<path d="M12 20h.01"/><path d="M8.5 16.4a5 5 0 0 1 7 0"/><path d="M5 12.9a10 10 0 0 1 5.2-2.7"/><path d="M19 12.9a10 10 0 0 0-2.1-1.6"/><path d="M2 8.8a15 15 0 0 1 4.2-2.6"/><path d="M22 8.8a15 15 0 0 0-11.3-3.7"/><path d="m2 2 20 20"/>',
  'cloud': '<path d="M17.5 19H9a7 7 0 1 1 6.7-9h1.8a4.5 4.5 0 1 1 0 9Z"/>',
  'bluetooth': '<path d="m7 7 10 10-5 5V2l5 5L7 17"/>',
  'clock': '<circle cx="12" cy="12" r="9.5"/><path d="M12 7v5l3 2"/>',
  'chevron-down': '<path d="m6 9 6 6 6-6"/>',
  'chevron-right': '<path d="m9 6 6 6-6 6"/>',
  'settings': '<path d="M12.2 2h-.4a2 2 0 0 0-2 2v.2a2 2 0 0 1-1 1.7l-.4.3a2 2 0 0 1-2 0l-.2-.1a2 2 0 0 0-2.7.7l-.2.4a2 2 0 0 0 .7 2.7l.2.1a2 2 0 0 1 1 1.7v.5a2 2 0 0 1-1 1.7l-.2.1a2 2 0 0 0-.7 2.7l.2.4a2 2 0 0 0 2.7.7l.2-.1a2 2 0 0 1 2 0l.4.3a2 2 0 0 1 1 1.7v.2a2 2 0 0 0 2 2h.4a2 2 0 0 0 2-2v-.2a2 2 0 0 1 1-1.7l.4-.3a2 2 0 0 1 2 0l.2.1a2 2 0 0 0 2.7-.7l.2-.4a2 2 0 0 0-.7-2.7l-.2-.1a2 2 0 0 1-1-1.7v-.5a2 2 0 0 1 1-1.7l.2-.1a2 2 0 0 0 .7-2.7l-.2-.4a2 2 0 0 0-2.7-.7l-.2.1a2 2 0 0 1-2 0l-.4-.3a2 2 0 0 1-1-1.7V4a2 2 0 0 0-2-2Z"/><circle cx="12" cy="12" r="3"/>',
  'sliders': '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M2 14h4M10 8h4M18 16h4"/>',
  'bell': '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.9 1.9 0 0 0 3.4 0"/>',
  'database': '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14a8 3 0 0 0 16 0V5"/><path d="M4 12a8 3 0 0 0 16 0"/>',
  'info': '<circle cx="12" cy="12" r="9.5"/><path d="M12 16v-4"/><path d="M12 8h.01"/>',
  'plus': '<path d="M5 12h14"/><path d="M12 5v14"/>',
  'search': '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  'keyboard': '<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M6 9h.01M10 9h.01M14 9h.01M18 9h.01M6 13h.01M18 13h.01M10 13h4M8 16h8"/>',
  'x': '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  'calendar': '<rect x="3" y="4.5" width="18" height="17" rx="2"/><path d="M16 2.5v4M8 2.5v4M3 10h18"/>',
  'history': '<path d="M3 12a9 9 0 1 0 2.7-6.4L3 8.3"/><path d="M3 3.5v4.8h4.8"/><path d="M12 7.5V12l3 2"/>',
  'thermo': '<path d="M14 4v10.5a4 4 0 1 1-4 0V4a2 2 0 0 1 4 0Z"/>',
  'moon': '<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>',
  'external': '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  'network': '<rect x="15" y="15" width="6" height="6" rx="1"/><rect x="3" y="15" width="6" height="6" rx="1"/><rect x="9" y="3" width="6" height="6" rx="1"/><path d="M6 15v-2a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v2"/><path d="M12 12V9"/>',
  'home': '<path d="M3 10.5 12 3l9 7.5"/><path d="M5 9v11h14V9"/><path d="M10 20v-5h4v5"/>',
  'sun': '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M6.3 17.7l-1.4 1.4M19.1 4.9l-1.4 1.4"/>',
  'arrow-down': '<path d="M12 5v14"/><path d="m18 13-6 6-6-6"/>',
  'arrow-up': '<path d="M12 19V5"/><path d="m6 11 6-6 6 6"/>',
  'download': '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>',
  'loader': '<path d="M21 12a9 9 0 1 1-6.2-8.6"/>',
  'pause': '<rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/>',
  'list': '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
  'grip': '<circle cx="9" cy="6" r="1"/><circle cx="15" cy="6" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="18" r="1"/><circle cx="15" cy="18" r="1"/>',
};
const sprite = '<svg width="0" height="0" style="position:absolute" aria-hidden="true"><defs>' +
  Object.entries(ICONS).map(([k, v]) => `<symbol id="i-${k}" viewBox="0 0 24 24">${v}</symbol>`).join('') +
  '<pattern id="gap-hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><line x1="0" y1="0" x2="0" y2="6" stroke="rgb(138 148 161 / 0.22)" stroke-width="1.5"/></pattern>' +
  '<pattern id="cloud-hatch" width="5" height="5" patternUnits="userSpaceOnUse" patternTransform="rotate(-45)"><rect width="5" height="5" fill="var(--color-warn)"/><line x1="0" y1="0" x2="0" y2="5" stroke="rgb(14 17 21 / 0.45)" stroke-width="2"/></pattern>' +
  '</defs></svg>';

const icon = (name, cls = '') => `<svg class="icon ${cls}" aria-hidden="true"><use href="#i-${name}"/></svg>`;

const minidial = ({ pct = 0, tone = 'good', num = '', lg = '' }) => {
  const col = { good: 'var(--color-good)', warn: 'var(--color-warn)', info: 'var(--color-info)', bad: 'var(--color-bad)', mute: 'var(--color-text-subtle)' }[tone];
  return `<span class="minidial${lg ? ' minidial--lg' : ''}" aria-hidden="true"><svg viewBox="0 0 48 48"><circle class="track" cx="24" cy="24" r="18"/>` +
    (Number(pct) > 0 ? `<circle class="fill" cx="24" cy="24" r="18" pathLength="100" stroke-dasharray="${pct} 100" transform="rotate(-90 24 24)" style="stroke:${col}"/>` : '') +
    `</svg><b>${num}</b></span>`;
};

// The ONE gauge. Every ring in the product is this markup.
const gauge = a => {
  const pct = Number(a.pct || 0);
  const cls = ['gauge', pct <= 0 ? 'gauge--empty' : '', a.stale ? 'gauge--stale' : ''].filter(Boolean).join(' ');
  const aria = (a.aria || `${a.title}: ${a.num}${a.small || ''}. ${a.primary || ''} ${a.secondary || ''}`).replace(/<[^>]+>/g, ' ').replace(/s+/g, ' ').trim();
  return `<figure class="${cls}" data-tone="${a.tone || 'info'}"${a.tip ? ` data-tip="${a.tip}" tabindex="0"` : ''}>
  <figcaption class="gauge__title">${a.title}</figcaption>
  <div class="gauge__dial" role="img" aria-label="${aria.replace(/"/g, '&quot;')}">
    <svg viewBox="0 0 120 120" aria-hidden="true"><circle class="gauge__track" cx="60" cy="60" r="50"/><circle class="gauge__fill" cx="60" cy="60" r="50" pathLength="100" stroke-dasharray="${Math.min(pct, 100)} 100" transform="rotate(-90 60 60)"/></svg>
    <div class="gauge__value"><span class="gauge__num${a.small && String(a.num).length >= 4 ? ' gauge__num--long' : ''}${a.numcls ? ' ' + a.numcls : ''}">${a.num}${a.small ? `<small>${a.small}</small>` : ''}</span>${a.unit ? `<span class="gauge__unit${a.utone ? ` t-${a.utone}` : ''}">${a.unit}</span>` : ''}</div>
  </div>
  <div class="gauge__meta">${a.primary ? `<span class="gauge__primary${a.ptone ? ` t-${a.ptone}` : ''}">${a.primary}</span>` : ''}${a.secondary ? `<span class="gauge__secondary">${a.secondary}</span>` : ''}</div>
</figure>`;
};

const TABS = [['overview', 'Огляд', '01-overview.html'], ['outages', 'Відключення', '02-outages.html'], ['errors', 'Помилки', '03-errors.html'], ['battery', 'Батарея', '04-battery.html'], ['energy', 'Енергія', '05-energy.html']];
const LOGO = `<svg class="titlebar__logo" viewBox="0 0 16 16" aria-hidden="true"><rect x="1.5" y="4.5" width="11" height="7" rx="1.5" fill="none" stroke="currentColor"/><rect x="13.5" y="6.5" width="1.5" height="3" rx=".5" fill="currentColor"/><rect x="3" y="6" width="6" height="4" rx=".5" fill="var(--color-good)"/></svg>`;

const titlebar = a => `<div class="titlebar">${LOGO}<span>Marstek Monitor</span>${a.state ? `<span aria-hidden="true">·</span><span class="titlebar__state">${a.state}</span>` : ''}<div class="titlebar__ctrls" aria-hidden="true"><span><svg width="10" height="10" viewBox="0 0 10 10"><path d="M0 5h10" stroke="currentColor"/></svg></span><span><svg width="10" height="10" viewBox="0 0 10 10"><rect x=".5" y=".5" width="9" height="9" rx="1.5" fill="none" stroke="currentColor"/></svg></span><span><svg width="10" height="10" viewBox="0 0 10 10"><path d="M0 0l10 10M10 0 0 10" stroke="currentColor"/></svg></span></div></div>`;

const appbar = a => {
  const tabs = TABS.map(([id, name, href]) =>
    `<a class="tab" href="${href}"${a.tab === id ? ' aria-current="page"' : ''}>${name}${id === 'errors' && a.errors ? `<span class="tab__count" aria-label="${a.errors} нових">${a.errors}</span>` : ''}</a>`).join('');
  const soc = a.soc || '72';
  return `<header class="appbar" data-grid="${a.grid || 'off'}">
  <button class="station-btn" aria-haspopup="dialog" aria-expanded="${a.open ? 'true' : 'false'}" ${a.open ? '' : 'onclick="location.href=\'06-stations.html\'"'}>
    ${minidial({ pct: soc, tone: a.soctone || 'good', num: soc })}
    <span class="station-btn__text"><span class="station-btn__name">${a.name || 'Дім'}</span><span class="station-btn__model">Venus E 3.0</span></span>
    ${icon('chevron-down')}
  </button>
  <nav class="tabs" aria-label="Розділи">${tabs}</nav>
  <div class="appbar__end"><a class="iconbtn" href="07-settings.html" aria-label="Налаштування"${a.tab === 'settings' ? ' aria-current="page"' : ''}>${icon('settings', 'icon--lg')}</a></div>
</header>`;
};

const PAGES = [['index.html', 'Зміст'], ['01-overview.html', 'Огляд'], ['02-outages.html', 'Відключення'], ['03-errors.html', 'Помилки'], ['04-battery.html', 'Батарея'], ['05-energy.html', 'Енергія'], ['06-stations.html', 'Станції'], ['07-settings.html', 'Налаштування'], ['08-tray-toasts.html', 'Трей і сповіщення'], ['09-states.html', 'Стани зв\x27язку'], ['10-first-run.html', 'Перший запуск'], ['11-dialogs.html', 'Діалоги']];
const docnav = a => {
  const i = PAGES.findIndex(p => p[0] === a.page);
  const prev = PAGES[i - 1], next = PAGES[i + 1];
  return `<div class="doc-head"><h1>${a.title}</h1><a href="index.html">← усі екрани</a>${prev && i > 1 ? `<a href="${prev[0]}">‹ ${prev[1]}</a>` : ''}${next ? `<a href="${next[0]}">${next[1]} ›</a>` : ''}</div>`;
};

const head = a => `<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${a.title} — Marstek Monitor hi-fi</title>
<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap&subset=cyrillic" rel="stylesheet">
<style>
${tokens}
${components}
</style>`;

const parseAttrs = s => { const o = {}; s.replace(/([\w-]+)="([^"]*)"/g, (_, k, v) => { o[k] = v; }); return o; };
const datajs = read('data.js');
const socjs = read('soc-chart.js');
const MACROS = { soc: () => `<script>
${socjs}
</script>`, data: () => `<script>
${datajs}
</script>`, head, gauge, minidial, titlebar, appbar, docnav, icon: a => icon(a.n, a.cls || ''), sprite: () => sprite, script: () => `<script>\n${appjs}\n</script>` };

for (const f of readdirSync(join(SRC, 'pages')).filter(f => f.endsWith('.html'))) {
  let html = read(join('pages', f));
  for (let pass = 0; pass < 3; pass++)
    html = html.replace(/<!--@(\w+)((?:\s+[\w-]+="[^"]*")*)\s*-->/g, (m, name, attrs) => MACROS[name] ? MACROS[name](parseAttrs(attrs)) : m);
  writeFileSync(join(OUT, f), html);
  console.log('built', f);
}
