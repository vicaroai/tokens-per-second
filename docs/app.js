// tokens-per-second dashboard. Reads the benchmark data (copied into
// docs/data/latest.json by the benchmark workflow) and renders a sortable
// leaderboard + a tokens/sec bar chart. Zero dependencies.
'use strict';

// Distinct dot color per provider (deck-palette hues), so every row and bar
// is clearly attributed to its provider.
const PROVIDER_COLORS = {
  openai: '#0d9a5c',
  anthropic: '#d97757',
  gemini: '#5b6fd4',
  crusoe: '#e570b0',
  fireworks: '#e8a33d',
  deepseek: '#7b61ff',
};
const providerColor = (p) => PROVIDER_COLORS[p] || '#7a8199';
// Real provider brandmarks live in docs/logos/<provider>.svg (from the MIT
// @lobehub/icons set). An <img> with an onerror fallback to a colored dot so a
// missing/renamed file never leaves a broken image.
const LOGO_FILE = {
  openai: 'logos/openai.svg',
  anthropic: 'logos/anthropic.svg',
  gemini: 'logos/gemini.svg',
  deepseek: 'logos/deepseek.svg',
  crusoe: 'logos/crusoe.png',
  fireworks: 'logos/fireworks.png',
};
function providerMark(p) {
  const src = LOGO_FILE[p];
  if (src) {
    return `<img class="prov-logo" src="${src}" alt="" width="17" height="17" loading="lazy"
      onerror="this.outerHTML='<span class=&quot;prov-dot&quot; style=&quot;background:${providerColor(p)}&quot;></span>'" />`;
  }
  return `<span class="prov-dot" style="background:${providerColor(p)}"></span>`;
}
let RAW = null;
let sortKey = 'tokens_per_second';
let sortDir = -1; // desc
const activeProviders = new Set();
let hideNoise = false;
let query = '';

const $ = (id) => document.getElementById(id);
const fmt = (n, d = 1) => (n == null ? '—' : Number(n).toFixed(d));

async function load() {
  // Try the local copy first (docs/data), then fall back to the repo path
  // (useful when previewing from the repo root).
  const candidates = ['data/latest.json', '../benchmarks/results/latest.json'];
  for (const url of candidates) {
    try {
      const r = await fetch(url, { cache: 'no-store' });
      if (r.ok) { RAW = await r.json(); break; }
    } catch (_) { /* try next */ }
  }
  if (!RAW) {
    $('error').hidden = false;
    $('error').textContent = 'Could not load benchmark data (data/latest.json).';
    $('run-meta').textContent = '';
    return;
  }
  init();
}

function init() {
  const d = new Date(RAW.generated_at);
  const okCount = RAW.results.filter((r) => r.ok).length;
  $('run-meta').innerHTML =
    `<b>${okCount}</b> models &middot; ${RAW.iso_week} &middot; updated <b>${d.toISOString().slice(0, 10)}</b> ` +
    `&middot; median of <b>${RAW.measured_runs}</b> runs &middot; est. run cost <b>$${fmt(RAW.total_cost_usd, 2)}</b>`;

  // provider filter chips
  const providers = [...new Set(RAW.results.map((r) => r.provider))].sort();
  providers.forEach((p) => activeProviders.add(p));
  const box = $('provider-filters');
  box.innerHTML = '';
  providers.forEach((p) => {
    const b = document.createElement('button');
    b.className = 'chip on';
    b.innerHTML = `${providerMark(p)}<span>${p}</span>`;
    b.onclick = () => {
      if (activeProviders.has(p)) { activeProviders.delete(p); b.classList.remove('on'); }
      else { activeProviders.add(p); b.classList.add('on'); }
      render();
    };
    box.appendChild(b);
  });

  $('hide-failed').onchange = (e) => { hideNoise = e.target.checked; render(); };
  $('search').oninput = (e) => { query = e.target.value.trim().toLowerCase(); render(); };

  document.querySelectorAll('th.sortable').forEach((th) => {
    th.onclick = () => {
      const k = th.dataset.sort;
      if (sortKey === k) sortDir *= -1;
      else { sortKey = k; sortDir = k === 'ttft_millis' || k === 'cost_usd' ? 1 : -1; }
      document.querySelectorAll('th.sortable').forEach((x) => x.classList.remove('active'));
      th.classList.add('active');
      render();
    };
  });

  ['controls', 'chart-card', 'table-card'].forEach((id) => ($(id).hidden = false));
  render();
}

function currentRows() {
  let rows = RAW.results.filter((r) => activeProviders.has(r.provider));
  if (hideNoise) rows = rows.filter((r) => r.ok && r.reasoning_applied === 'none');
  if (query) rows = rows.filter((r) => (r.model + ' ' + r.provider).toLowerCase().includes(query));
  rows.sort((a, b) => {
    // failed rows always sink
    if (a.ok !== b.ok) return a.ok ? -1 : 1;
    const av = a[sortKey], bv = b[sortKey];
    if (typeof av === 'string') return sortDir * String(av).localeCompare(String(bv));
    return sortDir * ((av ?? 0) - (bv ?? 0));
  });
  return rows;
}

function reasoningTag(r) {
  if (!r.ok) return '<span class="tag failed">—</span>';
  if (r.reasoning_applied === 'none') return '<span class="tag off">off</span>';
  return `<span class="tag flag">${r.reasoning_applied} ⚠️</span>`;
}

function render() {
  const rows = currentRows();

  // chart: only OK rows, by tps desc (top 12 to stay readable)
  const chartRows = rows.filter((r) => r.ok).slice().sort((a, b) => b.tokens_per_second - a.tokens_per_second).slice(0, 14);
  const max = Math.max(1, ...chartRows.map((r) => r.tokens_per_second));
  $('chart').innerHTML = chartRows.map((r) => `
    <div class="bar-row">
      <span class="bar-label" title="${r.provider}/${r.model}">${providerMark(r.provider)}<span class="prov-name">${r.provider}</span> ${shortModel(r.model)}</span>
      <span class="bar-track"><span class="bar-fill" style="width:${(r.tokens_per_second / max * 100).toFixed(1)}%"></span></span>
      <span class="bar-val">${fmt(r.tokens_per_second)}</span>
    </div>`).join('');

  // table
  let rank = 0;
  $('board-body').innerHTML = rows.map((r) => {
    const rk = r.ok ? ++rank : '—';
    return `<tr>
      <td class="rank num">${rk}</td>
      <td class="provider">${providerMark(r.provider)}${r.provider}</td>
      <td class="model"><code>${shortModel(r.model)}</code></td>
      <td class="tps num">${r.ok ? fmt(r.tokens_per_second) : '<span class="tag failed">failed</span>'}</td>
      <td class="mono num hide-sm">${r.ok ? fmt(r.ttft_millis, 0) : '—'}</td>
      <td>${reasoningTag(r)}</td>
      <td class="mono num hide-sm">${r.costed ? '$' + fmt(r.cost_usd, 4) : '—'}</td>
      <td class="mono num hide-sm">${r.successful_runs}/${r.total_runs}</td>
    </tr>`;
  }).join('');
}

// Trim the noisy Fireworks "accounts/fireworks/models/" prefix for display.
function shortModel(m) {
  return m.replace(/^accounts\/fireworks\/models\//, '');
}

load();
