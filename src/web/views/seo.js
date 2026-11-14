/* 网页检测：本地检查文章的网页质量（SEO + AI Agent 就绪度）
   全程离线：只分析文章自身的 Markdown，不访问网络、不上传内容。 */

import { api } from '../api.js';
import * as ui from '../ui.js';

export async function render(el, ctx) {
  el.innerHTML = `
    <div class="page-head">
      <div>
        <h1 class="page-title">网页检测</h1>
        <div class="page-sub">本地检查文章的网页质量：SEO 与 AI Agent 就绪度，全程离线</div>
      </div>
    </div>

    <div class="card card-pad section">
      <div class="seo-form">
        <select class="input" id="pickArt" style="flex:1">
          <option value="">载入文章中…</option>
        </select>
        <button class="btn btn-primary" id="run">${ui.icon('search')}<span>开始检测</span></button>
      </div>
      <div class="field" style="margin:12px 0 0">
        <span class="hint">检查内容：SEO（标题、摘要、标题层级、图片 alt、链接、字数）与 Agent 就绪（语义结构、锚文本、内容文本化、代码块标注等）。检查在本地完成，不依赖在线访问。</span>
      </div>
    </div>

    <div id="result"></div>
  `;

  const $ = (s) => el.querySelector(s);
  const pick = $('#pickArt');
  const runBtn = $('#run');
  const resultBox = $('#result');

  let articles = [];
  try {
    articles = (await api.articles()) || [];
  } catch (e) {
    pick.innerHTML = `<option value="">文章列表读取失败：${ui.esc(e.message)}</option>`;
    return;
  }
  const usable = articles.filter((a) => !a.missing);
  pick.innerHTML = usable.length
    ? ['<option value="">选择一篇文章…</option>',
        ...usable.map((a) => `<option value="${ui.esc(a.id)}">${ui.esc(a.title || '未命名文章')}</option>`)].join('')
    : '<option value="">文章库是空的，先写一篇文章再来检测</option>';

  const run = async () => {
    const id = pick.value;
    if (!id) { ui.toast('请先选择一篇文章', '', 'warn'); pick.focus(); return; }
    runBtn.disabled = true;
    const old = runBtn.innerHTML;
    runBtn.innerHTML = `<span class="spinner"></span><span>检测中…</span>`;
    try {
      const art = await api.article(id);
      const rep = checkArticle(art);
      paintResult(resultBox, art, rep);
    } catch (e) {
      ui.toast('检测失败', e.message, 'err', 6000);
    } finally {
      runBtn.disabled = false;
      runBtn.innerHTML = old;
    }
  };
  runBtn.onclick = run;
  pick.addEventListener('keydown', (e) => { if (e.key === 'Enter') run(); });
}

/* ==================== 本地检查引擎 ==================== */

// scanMarkdown 逐行扫描（跳过代码围栏内的内容），收集结构信息
function scanMarkdown(md) {
  const s = {
    h1: 0, h2: 0, h3: 0,
    images: [],            // { alt }
    links: [],             // { text }
    fences: 0, fencesLang: 0,
    lists: 0, tables: 0,
    textChars: 0,          // 正文字符数（剔除代码与标记）
    firstPara: '',
  };
  let inFence = false;
  let paraBuf = [];
  const flushPara = () => {
    if (!s.firstPara && paraBuf.length) {
      s.firstPara = paraBuf.join(' ').trim();
    }
    paraBuf = [];
  };
  for (const raw of String(md || '').split(/\r?\n/)) {
    const line = raw.trim();
    if (/^(```|~~~)/.test(line)) {
      inFence = !inFence;
      s.fences++;
      if (inFence && /^(```|~~~)\s*\S/.test(line)) s.fencesLang++;
      flushPara();
      continue;
    }
    if (inFence) continue;
    if (!line) { flushPara(); continue; }

    const hm = /^(#{1,6})\s+/.exec(line);
    if (hm) {
      const lv = hm[1].length;
      if (lv === 1) s.h1++;
      else if (lv === 2) s.h2++;
      else s.h3++;
      flushPara();
      continue;
    }
    if (/^([-*+]|\d+[.)])\s+/.test(line)) { s.lists++; continue; }
    if (/^\|.*\|/.test(line) && line.split('|').length >= 3) { s.tables++; continue; }

    // 去掉内联标记与行内代码，统计正文
    const text = line
      .replace(/`[^`]*`/g, ' ')
      .replace(/!\[([^\]]*)\]\([^)]*\)/g, (m, alt) => { s.images.push({ alt: alt.trim() }); return ' '; })
      .replace(/\[([^\]]*)\]\([^)]*\)/g, (m, txt) => { s.links.push({ text: txt.trim() }); return txt || ' '; })
      .replace(/[*_>#]+/g, ' ')
      .trim();
    s.textChars += [...text.replace(/\s+/g, '')].length;
    if (text) paraBuf.push(text);
  }
  flushPara();
  return s;
}

const isNakedURL = (t) => /^(https?:\/\/|www\.)\S+$/i.test(t);
const BAD_ANCHORS = new Set(['点击这里', '点这里', '这里', '链接', '点此', '查看详情', 'here', 'link', 'click here']);
const cjkLen = (s) => [...String(s || '').replace(/\s+/g, '')].length;

// checkArticle 返回 { score, groups, meta }
function checkArticle(art) {
  const meta = art || {};
  const md = meta.content || '';
  const s = scanMarkdown(md);
  const issues = [];

  const add = (group, type, msg, det) => issues.push({ group, type, msg, det });
  const titleLen = cjkLen(meta.title);

  /* ---------- SEO ---------- */
  if (titleLen === 0) add('seo', 'error', '缺少标题', '文章没有标题，网页 title 与搜索结果都无法展示。');
  else if (titleLen < 6 || titleLen > 40) add('seo', 'warn', '标题长度不理想', `当前 ${titleLen} 字，建议 6～40 字：太短信息量不足，太长会在搜索结果里被截断。`);
  else add('seo', 'ok', '标题长度合适', `${titleLen} 字。`);

  if (s.h1 === 0) add('seo', 'ok', '正文无多余一级标题', '发布平台通常已把文章标题渲染为 H1，正文不必重复。');
  else if (s.h1 === 1) add('seo', 'warn', '正文含 1 个一级标题', '文章标题已是 H1，正文里的 `# ` 建议降为 `## `，避免页面出现两个 H1。');
  else add('seo', 'error', `正文含 ${s.h1} 个一级标题`, '多个 H1 会稀释页面主题，请把它们降为二级或三级标题。');

  if (s.h2 + s.h3 > 0) add('seo', 'ok', '标题层级清晰', `H2 ${s.h2} 个 · H3 ${s.h3} 个。`);
  else add('seo', 'warn', '缺少分级标题', '长文建议用 H2 / H3 分节，搜索引擎按标题理解内容结构。');

  const excerptLen = cjkLen(meta.excerpt);
  if (excerptLen >= 20) add('seo', 'ok', '已有摘要', `${excerptLen} 字，可作为网页 description。`);
  else add('seo', 'warn', '缺少摘要', '写一段 20 字以上的摘要，发布时可用作 meta description。');

  if (s.textChars >= 300) add('seo', 'ok', '正文篇幅足够', `约 ${s.textChars} 字。`);
  else if (s.textChars >= 100) add('seo', 'warn', '正文偏短', `约 ${s.textChars} 字，300 字以上的内容更容易被收录。`);
  else add('seo', 'error', '内容太少', `正文仅约 ${s.textChars} 字，几乎无法参与搜索排名。`);

  if (s.images.length === 0) {
    add('seo', 'info', '没有图片', '图文并茂的内容在搜索结果里更容易获得好位置（可选）。');
  } else {
    const missing = s.images.filter((i) => !i.alt).length;
    if (missing === 0) add('seo', 'ok', '图片都有 alt 描述', `共 ${s.images.length} 张图片。`);
    else if (missing === s.images.length) add('seo', 'error', '所有图片都缺少 alt', `${s.images.length} 张图片均无替代文本，用 ![描述](图片名) 的写法补上，可同时利于 SEO 与 AI 理解。`);
    else add('seo', 'warn', '部分图片缺少 alt', `${missing}/${s.images.length} 张缺 alt，补上描述即可。`);
  }

  const naked = s.links.filter((l) => isNakedURL(l.text));
  if (s.links.length === 0) add('seo', 'info', '没有链接', '适当引用权威来源有助于可信度（可选）。');
  else if (naked.length === 0) add('seo', 'ok', '链接锚文本规范', `共 ${s.links.length} 个链接。`);
  else add('seo', 'warn', '存在裸链接', `${naked.length} 个链接直接拿网址当锚文本，建议换成描述性文字。`);

  /* ---------- AI Agent 就绪（isitagentready） ---------- */
  if (s.h2 + s.h3 > 0) add('agent', 'ok', '语义结构可被机器理解', '有分级标题，AI 可按小节抓取要点。');
  else add('agent', 'warn', '语义结构缺失', '纯段落长文对 AI 不友好，建议按主题加小节标题。');

  const bad = s.links.filter((l) => BAD_ANCHORS.has(l.text.toLowerCase()) && l.text);
  if (bad.length) add('agent', 'warn', '锚文本语义弱', `“${bad[0].text}”这类词没有信息量，AI 无法从锚文本判断目标内容。`);
  else add('agent', 'ok', '锚文本有语义', '链接文字能表达目标内容。');

  if (s.images.length > 0 && s.textChars < 200) add('agent', 'warn', '内容偏图片化', `文字仅约 ${s.textChars} 字、图片 ${s.images.length} 张：AI 爬虫读不到图片内容，请补充文字说明。`);
  else add('agent', 'ok', '内容以文本为主', '文本内容可直接被 AI 抓取与引用。');

  if (s.lists + s.tables > 0) add('agent', 'ok', '包含结构化元素', `列表 ${s.lists} 行 · 表格 ${s.tables} 行，便于机器提取要点。`);
  else add('agent', 'info', '没有列表 / 表格', '步骤或对比类内容改用列表、表格，AI 提取效果更好。');

  const bare = s.fences - s.fencesLang;
  if (s.fences === 0) add('agent', 'info', '没有代码块', '如涉及代码，建议用围栏代码块并标注语言。');
  else if (bare === 0) add('agent', 'ok', '代码块都标注了语言', `共 ${s.fences} 个代码块。`);
  else add('agent', 'warn', '部分代码块未标语言', `${bare}/${s.fences} 个代码块缺语言标注（如 \`\`\`js），标注后便于机器解析。`);

  add('agent', 'info', '发布平台侧提醒', 'AI 爬虫能否抓到页面取决于发布平台：确认 robots.txt 允许 GPTBot / ClaudeBot 等，有条件的话提供 llms.txt。此项需在博客平台检查，应用无法代查。');

  /* ---------- 汇总 ---------- */
  const scored = issues.filter((i) => i.type === 'ok' || i.type === 'warn' || i.type === 'error');
  const got = scored.reduce((n, i) => n + (i.type === 'ok' ? 1 : i.type === 'warn' ? 0.5 : 0), 0);
  const score = Math.round((got / Math.max(1, scored.length)) * 100);

  return {
    score,
    groups: {
      seo: issues.filter((i) => i.group === 'seo'),
      agent: issues.filter((i) => i.group === 'agent'),
    },
    meta: s,
  };
}

/* ==================== 结果渲染 ==================== */

function issueHTML(i) {
  const cls = i.type === 'error' ? 'error' : i.type === 'info' ? 'info' : '';
  return `<div class="issue ${cls}"><span class="dot"></span><div><div class="msg">${ui.esc(i.msg)}</div>${i.det ? `<div class="det">${ui.esc(i.det)}</div>` : ''}</div></div>`;
}

function paintResult(box, art, rep) {
  const s = rep.meta;
  const ring = (v) => {
    const r = 26, c = 2 * Math.PI * r;
    const color = v >= 80 ? 'var(--ok)' : v >= 50 ? 'var(--warn)' : 'var(--err)';
    return `
      <svg viewBox="0 0 62 62" width="88" height="88">
        <circle cx="31" cy="31" r="${r}" fill="none" stroke="var(--surface-3)" stroke-width="5"/>
        <circle cx="31" cy="31" r="${r}" fill="none" stroke="${color}" stroke-width="5" stroke-linecap="round"
          stroke-dasharray="${c}" stroke-dashoffset="${c * (1 - v / 100)}"/>
      </svg>
      <span class="val" style="font-size:24px;color:${color}">${v}</span>`;
  };

  box.innerHTML = `
    <div class="detail-panel">
      <div class="card card-pad section" style="display:flex;gap:26px;align-items:center;flex-wrap:wrap">
        <div class="score-ring" style="width:88px;height:88px">${ring(rep.score)}</div>
        <div style="flex:1;min-width:220px">
          <div style="font-family:var(--serif);font-size:17px;font-weight:700">${ui.esc(art.title || '未命名文章')}</div>
          <div class="page-sub" style="margin-top:6px">约 ${s.textChars} 字 · 图片 ${s.images.length} · 链接 ${s.links.length} · H1 ${s.h1} / H2 ${s.h2} / H3 ${s.h3}</div>
          <div class="page-sub" style="margin-top:2px">本地检查 · 未访问网络</div>
        </div>
      </div>

      <div class="section">
        <div class="section-title">SEO 检查</div>
        <div class="issue-list">${rep.groups.seo.map(issueHTML).join('')}</div>
      </div>

      <div class="section">
        <div class="section-title">AI Agent 就绪</div>
        <p class="section-desc">参照 isitagentready 的思路检查机器可读性：结构化、语义锚文本、内容文本化等。</p>
        <div class="issue-list">${rep.groups.agent.map(issueHTML).join('')}</div>
      </div>
    </div>`;

  box.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}
