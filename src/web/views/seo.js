/* SEO 检测：站点体检 + 收录查询 */

import { api } from '../api.js';
import * as ui from '../ui.js';

const ENGINE_HINT = {
  indexed: { cls: 'tag-ok', text: '已收录' },
  not_indexed: { cls: 'tag-err', text: '未收录' },
  unknown: { cls: 'tag-warn', text: '未知' },
  error: { cls: 'tag-warn', text: '异常' },
};

export async function render(el, ctx) {
  if (ctx.params && ctx.params.id) return renderDetail(el, ctx, ctx.params.id);
  return renderList(el, ctx);
}

/* ==================== 列表 ==================== */

async function renderList(el, ctx) {
  el.innerHTML = `
    <div class="page-head">
      <div>
        <h1 class="page-title">SEO 检测</h1>
        <div class="page-sub">检测网站的SEO数据</div>
      </div>
    </div>

    <div class="card card-pad section">
      <div class="seo-form">
        <input class="input" id="url" placeholder="https://example.com" autocapitalize="off" spellcheck="false">
        <button class="btn btn-primary" id="run">${ui.icon('search')}<span>开始检测</span></button>
      </div>
      <div class="field" style="margin:12px 0 0">
        <span class="hint">检测元信息、标题层级、图片 alt、内外链与必应 / 百度 / 360 / 搜狗收录。</span>
      </div>
    </div>

    <div id="list"></div>
  `;

  const $ = (s) => el.querySelector(s);
  const input = $('#url');
  const runBtn = $('#run');

  const run = async () => {
    const url = input.value.trim();
    if (!url) { ui.toast('请输入网址', '', 'warn'); input.focus(); return; }
    runBtn.disabled = true;
    const old = runBtn.innerHTML;
    runBtn.innerHTML = `<span class="spinner"></span><span>检测中…</span>`;
    try {
      const rec = await api.seoCheck(url);
      ui.toast('检测完成', `${rec.host} · 得分 ${rec.score}`, rec.error ? 'warn' : 'ok');
      ctx.navigate('seo', { id: rec.id });
      ctx.refreshState();
    } catch (e) {
      ui.toast('检测失败', e.message, 'err', 7000);
    } finally {
      runBtn.disabled = false;
      runBtn.innerHTML = old;
    }
  };
  runBtn.onclick = run;
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') run(); });

  let records = [];
  try {
    records = await api.seoList();
  } catch (e) {
    $('#list').innerHTML = `<div class="empty">${ui.icon('alert', 56)}<h3>无法读取记录</h3><p>${ui.esc(e.message)}</p></div>`;
    return;
  }

  const list = $('#list');
  if (!records.length) {
    list.innerHTML = `<div class="empty">${ui.icon('search', 56)}<h3>还没有检测记录</h3><p>在上方输入一个网址，开始第一次检测</p></div>`;
    return;
  }

  list.innerHTML = `<div class="section-title" style="margin-bottom:14px">历史记录 · ${records.length}</div>
    <div class="record-list">${records.map(recordHTML).join('')}</div>`;

  list.querySelectorAll('[data-id]').forEach((n) => {
    n.onclick = () => ctx.navigate('seo', { id: n.dataset.id });
  });
}

function recordHTML(r) {
  const hosts = (r.engines || []).map((e) => {
    const h = ENGINE_HINT[e.status] || ENGINE_HINT.unknown;
    return `<span class="tag ${h.cls}">${ui.esc(e.name)} ${e.status === 'indexed' ? ui.fmtCount(e.count) : h.text}</span>`;
  }).join('');
  return `
    <div class="record" data-id="${ui.esc(r.id)}">
      <div class="score-ring">${ringHTML(r.error ? null : r.score)}</div>
      <div class="record-main">
        <div class="record-host">${ui.esc(r.host)}</div>
        <div class="record-url">${ui.esc(r.url)}</div>
        <div class="record-time">${ui.fmtDate(r.updated)} · ${(r.issues || []).length} 项建议${r.error ? ' · 部分检测失败' : ''}</div>
      </div>
      <div class="record-engines">${hosts}</div>
      <div class="spacer" style="width:8px"></div>
      ${ui.icon('chevronRight', 18)}
    </div>`;
}

function ringHTML(score) {
  if (score === null || score === undefined || Number.isNaN(Number(score))) {
    return `
      <svg viewBox="0 0 62 62" width="62" height="62">
        <circle cx="31" cy="31" r="26" fill="none" stroke="var(--surface-3)" stroke-width="5"/>
      </svg>
      <span class="val" style="color:var(--ink-3)">—</span>`;
  }
  const value = Math.max(0, Math.min(100, Number(score) || 0));
  const r = 26;
  const c = 2 * Math.PI * r;
  const color = value >= 80 ? 'var(--ok)' : value >= 50 ? 'var(--warn)' : 'var(--err)';
  return `
    <svg viewBox="0 0 62 62" width="62" height="62">
      <circle cx="31" cy="31" r="${r}" fill="none" stroke="var(--surface-3)" stroke-width="5"/>
      <circle cx="31" cy="31" r="${r}" fill="none" stroke="${color}" stroke-width="5" stroke-linecap="round"
        stroke-dasharray="${c}" stroke-dashoffset="${c * (1 - value / 100)}"/>
    </svg>
    <span class="val" style="color:${color}">${value}</span>`;
}

/* ==================== 详情 ==================== */

async function renderDetail(el, ctx, id) {
  el.innerHTML = `<div class="page-head"><button class="btn btn-quiet" id="back">${ui.icon('chevronLeft')}<span>返回</span></button><div class="spacer"></div></div>
    <div class="card card-pad"><div class="skeleton" style="height:200px"></div></div>`;

  el.querySelector('#back').onclick = () => ctx.navigate('seo');

  let rec;
  try {
    rec = await api.seoGet(id);
  } catch (e) {
    ui.toast('无法读取记录', e.message, 'err');
    return ctx.navigate('seo');
  }

  const d = rec.details || {};
  el.innerHTML = `
    <div class="page-head">
      <button class="btn btn-quiet" id="back">${ui.icon('chevronLeft')}<span>返回</span></button>
      <div style="flex:1;min-width:200px">
        <h1 class="page-title" style="font-size:23px">${ui.esc(rec.host)}</h1>
        <div class="page-sub"><a href="${ui.esc(rec.url)}" target="_blank" rel="noopener noreferrer">${ui.esc(rec.url)}</a> · 更新于 ${ui.fmtDate(rec.updated)}</div>
      </div>
      <button class="btn btn-ghost" id="recheck">${ui.icon('refresh')}<span>重新检测</span></button>
      <button class="btn btn-danger" id="del">${ui.icon('trash')}<span>删除记录</span></button>
    </div>

    <div class="detail-panel">
      ${rec.error ? `<div class="issue error" style="margin-bottom:18px"><span class="dot"></span><div><div class="msg">站点体检未完成</div><div class="det">${ui.esc(rec.error)}</div></div></div>` : ''}

      <div class="card card-pad section" style="display:flex;gap:26px;align-items:center;flex-wrap:wrap">
        <div class="score-ring" style="width:88px;height:88px">
          ${rec.error ? `
          <svg viewBox="0 0 62 62" width="88" height="88">
            <circle cx="31" cy="31" r="26" fill="none" stroke="var(--surface-3)" stroke-width="5"/>
          </svg>
          <span class="val" style="font-size:24px;color:var(--ink-3)">—</span>` : `
          <svg viewBox="0 0 62 62" width="88" height="88">
            <circle cx="31" cy="31" r="26" fill="none" stroke="var(--surface-3)" stroke-width="5"/>
            <circle cx="31" cy="31" r="26" fill="none" stroke="${scoreColor(rec.score)}" stroke-width="5" stroke-linecap="round"
              stroke-dasharray="${2 * Math.PI * 26}" stroke-dashoffset="${2 * Math.PI * 26 * (1 - Math.max(0, Math.min(100, rec.score)) / 100)}"/>
          </svg>
          <span class="val" style="font-size:24px;color:${scoreColor(rec.score)}">${rec.score}</span>`}
        </div>
        <div style="flex:1;min-width:220px">
          <div style="font-family:var(--serif);font-size:17px;font-weight:700">SEO 综合评分</div>
          <div class="page-sub" style="margin-top:6px">${rec.error ? '本次站点体检未完成，见上方说明' : `页面加载 ${rec.loadTime || '—'} ms · 发现 ${(rec.issues || []).length} 项待优化项`}</div>
        </div>
      </div>

      <div class="section">
        <div class="section-title">搜索引擎收录</div>
        <p class="section-desc">「未知」多因搜索引擎风控或页面结构变化，不代表未收录。</p>
        <div class="engine-grid">
          ${(rec.engines || []).map(engineHTML).join('')}
        </div>
      </div>

      ${(rec.issues || []).length ? `
      <div class="section">
        <div class="section-title">优化建议</div>
        <div class="issue-list">
          ${rec.issues.map((i) => `<div class="issue ${ui.esc(i.type || 'warning')}"><span class="dot"></span><div><div class="msg">${ui.esc(i.message)}</div>${i.detail ? `<div class="det">${ui.esc(i.detail)}</div>` : ''}</div></div>`).join('')}
        </div>
      </div>` : ''}

      <div class="section">
        <div class="section-title">页面元信息</div>
        <div class="card card-pad">
          <dl class="kv">${detailsHTML(d)}</dl>
        </div>
      </div>
    </div>
  `;

  el.querySelector('#back').onclick = () => ctx.navigate('seo');
  el.querySelector('#del').onclick = async () => {
    const ok = await ui.confirmDialog({ title: '删除这条记录？', desc: 'seo 目录下的 JSON 记录会被移除。', okText: '删除', danger: true });
    if (!ok) return;
    try { await api.seoDelete(rec.id); ui.toast('已删除', '', 'ok'); ctx.navigate('seo'); ctx.refreshState(); }
    catch (e) { ui.toast('删除失败', e.message, 'err'); }
  };
  el.querySelector('#recheck').onclick = async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    const old = btn.innerHTML;
    btn.innerHTML = `<span class="spinner"></span><span>检测中…</span>`;
    try {
      await api.seoRecheck(rec.id);
      ui.toast('已重新检测', rec.host, 'ok');
      ctx.route();
    } catch (err) {
      ui.toast('检测失败', err.message, 'err');
    } finally {
      btn.disabled = false;
      btn.innerHTML = old;
    }
  };

}

function scoreColor(s) {
  const v = Number(s) || 0;
  return v >= 80 ? 'var(--ok)' : v >= 50 ? 'var(--warn)' : 'var(--err)';
}

function engineHTML(e) {
  const h = ENGINE_HINT[e.status] || ENGINE_HINT.unknown;
  const big = e.status === 'indexed' ? ui.fmtCount(e.count) : h.text;
  return `
    <div class="engine">
      <div class="name">${ui.esc(e.name)}<span class="tag ${h.cls}">${h.text}</span></div>
      <div class="num">${big}</div>
      <div class="note">${ui.esc(e.message || '')}</div>
    </div>`;
}

function detailsHTML(d) {
  const rows = [];
  const add = (k, v) => { if (v !== undefined && v !== null && v !== '') rows.push(`<dt>${k}</dt><dd>${v}</dd>`); };

  if (d.title) add('标题', `${ui.esc(d.title.value)} <span class="tag">${d.title.length || 0} 字</span>`);
  if (d.description) add('描述', `${ui.esc(d.description.value)} <span class="tag">${d.description.length || 0} 字</span>`);
  if (d.keywords) add('关键词', `${(d.keywords.value || []).map((k) => `<span class="tag">${ui.esc(k)}</span>`).join(' ') || '—'}`);
  if (d.headings) add('标题层级', `H1 ${d.headings.h1 || 0} · H2 ${d.headings.h2 || 0} · H3 ${d.headings.h3 || 0}`);
  if (d.images) add('图片', `共 ${d.images.total || 0} 张，${d.images.withoutAlt || 0} 张缺少 alt`);
  if (d.links) add('链接', `内链 ${d.links.internal || 0} · 外链 ${d.links.external || 0} · nofollow ${d.links.nofollow || 0}`);
  if (d.canonical) add('Canonical', `<code>${ui.esc(d.canonical)}</code>`);
  if (d.robots) add('Robots', `<code>${ui.esc(d.robots)}</code>`);
  if (d.viewport) add('Viewport', `<code>${ui.esc(d.viewport)}</code>`);
  if (d.charset) add('字符集', ui.esc(d.charset));
  if (d.lang) add('语言', ui.esc(d.lang));
  if (d.ogTags?.title) add('OG 标题', ui.esc(d.ogTags.title));
  if (d.twitterTags?.card) add('Twitter Card', ui.esc(d.twitterTags.card));
  if (!rows.length) return '<dt>提示</dt><dd>本次未取到页面元信息</dd>';
  return rows.join('');
}
