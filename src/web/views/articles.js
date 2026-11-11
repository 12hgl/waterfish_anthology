/* 文章库 + Markdown 编辑器 */

import { api } from '../api.js';
import * as ui from '../ui.js';
import { renderMarkdown, buildCopyHTML } from '../markdown.js';
import { createHistory } from '../editor.js';
import { openPublishDialog } from '../publish.js';
import { openImportDialog, authorizePath, revealInManager } from '../external.js';

const local = { list: [], keyword: '' };

export async function render(el, ctx) {
  if (ctx.params && ctx.params.id) return renderEditor(el, ctx, ctx.params.id);
  return renderList(el, ctx);
}

/* ==================== 列表 ==================== */

async function renderList(el, ctx) {
  el.innerHTML = `
    <div class="page-head">
      <div>
        <h1 class="page-title">文章库</h1>
        <div class="page-sub">正在加载…</div>
      </div>
      <div class="spacer"></div>
      <button class="btn btn-ghost" id="importArticle" title="从磁盘导入文档">${ui.icon('download')}<span>导入</span></button>
      <button class="btn btn-primary" id="newArticle">${ui.icon('plus')}<span>新建文章</span></button>
    </div>
    <div class="article-grid" id="grid">${'<div class="skeleton" style="height:168px"></div>'.repeat(6)}</div>
  `;

  el.querySelector('#newArticle').onclick = () => ctx.navigate('articles', { id: 'new' });
  el.querySelector('#importArticle').onclick = () => openImportDialog(() => ctx.refresh());

  let list = [];
  try {
    list = await api.articles();
  } catch (e) {
    el.querySelector('#grid').innerHTML = emptyHTML('无法读取文章', e.message);
    return;
  }
  local.list = list;

  const sub = el.querySelector('.page-sub');
  const total = list.reduce((s, a) => s + (a.size || 0), 0);
  sub.textContent = `${list.length} 篇文章 · 共 ${ui.fmtSize(total)}`;

  const grid = el.querySelector('#grid');
  const toolbar = document.createElement('div');
  toolbar.className = 'toolbar';
  toolbar.innerHTML = `
    <div class="search">${ui.icon('search')}<input class="input" id="q" placeholder="搜索标题或正文…" value="${ui.esc(local.keyword)}"></div>
    <div class="spacer" style="flex:1"></div>
    <button class="btn btn-ghost btn-sm" id="refresh">${ui.icon('refresh', 14)}<span>刷新</span></button>
  `;
  el.insertBefore(toolbar, grid);

  const paint = () => {
    const kw = local.keyword.trim().toLowerCase();
    const rows = kw
      ? local.list.filter((a) => a.title.toLowerCase().includes(kw) || (a.excerpt || '').toLowerCase().includes(kw))
      : local.list;

    if (!local.list.length) {
      grid.innerHTML = emptyHTML('还没有文章', '点击右上角「新建文章」，用 Markdown 写下第一篇');
      return;
    }
    if (!rows.length) {
      grid.innerHTML = emptyHTML('没有匹配的文章', '换个关键词试试');
      return;
    }
    grid.innerHTML = rows.map(cardHTML).join('');
    grid.querySelectorAll('[data-open]').forEach((n) => {
      n.onclick = () => ctx.navigate('articles', { id: n.dataset.open });
    });
    grid.querySelectorAll('[data-copy]').forEach((n) => {
      n.onclick = (e) => { e.stopPropagation(); doCopy(n.dataset.copy, ctx); };
    });
    grid.querySelectorAll('[data-del]').forEach((n) => {
      n.onclick = async (e) => {
        e.stopPropagation();
        const meta = local.list.find((x) => x.id === n.dataset.del);
        const ext = !!(meta && meta.external);
        const ok = await ui.confirmDialog({
          title: ext ? '从文章库移除？' : '删除这篇文章？',
          desc: ext
            ? '只移除文章库里的条目，磁盘上的原文件不会被删除。'
            : '对应 .md 文件将从 passage 目录中移除，且无法恢复。',
          okText: ext ? '移除' : '删除', danger: true,
        });
        if (!ok) return;
        try {
          await api.deleteArticle(n.dataset.del);
          ui.toast(ext ? '已从文章库移除' : '已删除', ext ? '原文件保持不变' : '', 'ok');
          ctx.refresh();
        } catch (err) { ui.toast('删除失败', err.message, 'err'); }
      };
    });
  };

  paint();
  toolbar.querySelector('#q').addEventListener('input', ui.debounce((e) => {
    local.keyword = e.target.value;
    paint();
  }, 160));
  toolbar.querySelector('#refresh').onclick = () => ctx.refresh();
}

function cardHTML(a) {
  const badge = a.external
    ? `<span class="card-badge ${a.missing ? 'is-missing' : ''}" title="${ui.esc(a.path || '')}">${ui.icon('file', 11)}${ui.esc(a.missing ? '文件已移动' : (a.format || '外部文件'))}</span>`
    : '';
  return `
    <article class="article-card ${a.external ? 'is-external' : ''}" data-open="${ui.esc(a.id)}">
      <h3>${badge}${ui.esc(a.title)}</h3>
      <p>${ui.esc(a.excerpt || '（空白文章）')}</p>
      <div class="article-meta">
        <span title="${ui.esc(ui.fmtDate(a.updated))}">${ui.esc(ui.fmtDateShort(a.updated))}</span>
        <div class="spacer"></div>
        <span>${ui.fmtSize(a.size)}</span>
        <div class="article-actions">
          <button class="icon-btn" title="复制文章" data-copy="${ui.esc(a.id)}">${ui.icon('copy', 15)}</button>
          <button class="icon-btn danger" title="删除" data-del="${ui.esc(a.id)}">${ui.icon('trash', 15)}</button>
        </div>
      </div>
    </article>`;
}

function emptyHTML(title, desc) {
  return `<div class="empty" style="grid-column:1/-1">
    ${ui.icon('book', 56)}
    <h3>${ui.esc(title)}</h3><p>${ui.esc(desc || '')}</p>
  </div>`;
}

async function doCopy(id, ctx) {
  try {
    const r = await api.copyArticle(id);
    // 富文本复制：text/html + text/plain 同时入剪贴板。
    // Notion 等编辑器优先按 HTML 解析，粗体/标题/列表原样保留，不会错乱换行；
    // 纯文本环境拿到的是 Markdown 源码。
    const html = buildCopyHTML(r.text);
    await ui.copyRich(html, r.text, '文章已复制到剪贴板');
    if (r.localImages && r.localImages.length) {
      ui.toast(
        `有 ${r.localImages.length} 张本地图片`,
        `已复制文件名：${r.localImages.slice(0, 3).join('、')}${r.localImages.length > 3 ? ' 等' : ''}`,
        'warn', 6000,
      );
    }
  } catch (e) {
    ui.toast('复制失败', e.message, 'err');
  }
}

/* ==================== 编辑器 ==================== */

/* ==================== 外部文件（文件管理器「打开方式」） ==================== */

// renderExternalFile 打开系统传来的 .md 文件：先取内容，未授权则引导授权后再进编辑器。
export async function renderExternalFile(el, ctx, path) {
  el.innerHTML = `
    <div class="empty">
      ${ui.icon('file', 56)}
      <h3>正在打开文件…</h3>
      <p>${ui.esc(path)}</p>
    </div>`;
  let file;
  try {
    file = await api.externalOpen(path);
  } catch (e) {
    const needGrant = !!(e.status === 403 && e.data && e.data.needGrant);
    renderGrantGate(el, ctx, path, needGrant, e.message);
    return;
  }
  // 纯文本可直接编辑写回；含版式的文档（docx/pdf/epub…）只读预览 + 另存为
  if (file.mode === 'readonly') return renderReadonlyPreview(el, ctx, file);
  return renderEditor(el, ctx, 'external', { path: file.path, file });
}

// renderReadonlyPreview 富文本 / 电子书 / PDF 的只读预览：
// 正文解析成 Markdown 后渲染，PDF 直接用浏览器原生阅读器显示原文件（版式不走样）。
function renderReadonlyPreview(el, ctx, file) {
  const isPDF = file.format === 'pdf' && file.rawUrl;
  el.innerHTML = `
    <div class="editor">
      <div class="editor-head">
        <button class="btn btn-quiet" id="back" title="返回列表">${ui.icon('chevronLeft')}<span>返回</span></button>
        <input class="editor-title" id="roTitle" value="${ui.esc(file.title)}" readonly>
        <span class="editor-status">${ui.esc(file.formatLabel)} · 只读 · ${ui.fmtSize(file.size)}</span>
        <button class="btn btn-ghost btn-sm" id="roLink" ${file.linked ? 'disabled' : ''}>
          ${ui.icon('book', 14)}<span id="roLinkLabel">${file.linked ? '已在文章库' : '加入文章库'}</span></button>
        <button class="btn btn-primary btn-sm" id="roSaveAs">${ui.icon('download', 14)}<span>另存为文集文章</span></button>
      </div>
      <div class="ext-bar">
        ${ui.icon('file', 15)}
        <span class="ext-path" title="${ui.esc(file.displayPath || file.path)}">${ui.esc(file.displayPath || file.path)}</span>
        <span class="ext-warn">${ui.icon('shield', 13)}只读预览，不会改动原文件</span>
        <button class="btn btn-quiet btn-xs" id="roReveal" title="在文件管理器中显示">${ui.icon('folder', 13)}<span>定位</span></button>
      </div>
      ${(file.warnings || []).length ? `<div class="ro-warn">${file.warnings.map((w) => `<p>${ui.icon('alert', 13)}<span>${ui.esc(w)}</span></p>`).join('')}</div>` : ''}
      ${isPDF
        ? `<div class="ro-pdfwrap"><iframe class="ro-pdf" src="${ui.esc(file.rawUrl)}" title="${ui.esc(file.title)}"></iframe></div>`
        : `<div class="ro-body"><div class="markdown">${renderMarkdown(file.content || '')}</div></div>`}
      <div class="ro-foot">
        <p>「另存为文集文章」会把解析出的正文复制成一篇新文章，之后可在水鱼文集里编辑；原文件保持不变。</p>
      </div>
    </div>`;

  el.querySelector('#back').onclick = () => ctx.navigate('articles');
  const revealBtn = el.querySelector('#roReveal');
  revealBtn.onclick = async () => {
    const ok = await revealInManager(file.path);
    if (!ok) ui.toast('无法定位文件', '请在飞牛文件管理器中手动查找', 'warn');
  };

  const linkBtn = el.querySelector('#roLink');
  linkBtn.onclick = async () => {
    linkBtn.disabled = true;
    try {
      await api.externalLink(file.path, el.querySelector('#roTitle').value.trim() || file.title);
      el.querySelector('#roLinkLabel').textContent = '已在文章库';
      ui.toast('已加入文章库', '之后可从文章库直接打开这个文件', 'ok', 4000);
    } catch (e) {
      linkBtn.disabled = false;
      ui.toast('加入失败', e.message, 'err', 6000);
    }
  };

  el.querySelector('#roSaveAs').onclick = async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    btn.innerHTML = `<span class="spinner"></span><span>保存中…</span>`;
    try {
      const art = await api.createArticle({
        title: el.querySelector('#roTitle').value.trim() || file.title,
        content: file.content || '',
      });
      ui.toast('已另存为文集文章', art.title || '', 'ok', 4000);
      ctx.navigate('articles', { id: art.id });
    } catch (err) {
      btn.disabled = false;
      btn.innerHTML = `${ui.icon('download', 14)}<span>另存为文集文章</span>`;
      ui.toast('保存失败', err.message, 'err', 6000);
    }
  };
}

// renderGrantGate 授权引导：系统只把用户明确同意的路径授予应用。
function renderGrantGate(el, ctx, path, needGrant, message) {
  const dir = path.replace(/\/[^/]+\/?$/, '') || '/';
  el.innerHTML = `
    <div class="ext-gate">
      <div class="ext-gate-ico">${ui.icon('shield', 40)}</div>
      <h2>${needGrant ? '需要授权访问' : '无法打开该文件'}</h2>
      <p class="ext-gate-desc">${needGrant
        ? '水鱼文集以独立的应用用户运行，读取和写回磁盘文件前需要你确认授权。系统会弹出选择窗口，授权范围是该文件所在的文件夹。'
        : '文件可能已被移动、删除，或该格式无法解析（.doc / .pages 等闭源格式请先另存为 .docx 或 PDF）。'}</p>
      <div class="ext-gate-path" title="${ui.esc(path)}">${ui.esc(path)}</div>
      ${message && message !== '需要授权访问该文件所在位置' ? `<p class="ext-gate-err">${ui.esc(message)}</p>` : ''}
      <div class="ext-gate-actions">
        ${needGrant ? `<button class="btn btn-primary" id="extGrant">${ui.icon('shield', 15)}<span>授权访问</span></button>` : ''}
        <button class="btn btn-ghost" id="extBack">返回文章库</button>
      </div>
      ${needGrant ? `<ul class="ext-gate-tips">
        <li>授权只授予「${ui.esc(dir)}」，不会扩大到你其他目录；</li>
        <li>可随时在「应用中心 → 水鱼文集 → 设置 → 授权目录」中移除；</li>
        <li>编辑保存后直接写回原文件，不会另存副本。</li>
      </ul>` : ''}
    </div>`;

  const grantBtn = el.querySelector('#extGrant');
  grantBtn && (grantBtn.onclick = async () => {
    grantBtn.disabled = true;
    grantBtn.innerHTML = `<span class="spinner"></span><span>等待系统确认…</span>`;
    const r = await authorizePath(dir, { directory: true });
    if (r.ok) {
      ui.toast('已授权', '正在打开文件…', 'ok', 3000);
      ctx.openExternal(path);
      return;
    }
    grantBtn.disabled = false;
    grantBtn.innerHTML = `${ui.icon('shield', 15)}<span>重试授权</span>`;
    const errEl = el.querySelector('.ext-gate-err');
    if (errEl) errEl.textContent = r.reason || '授权未完成';
    else {
      const p = document.createElement('p');
      p.className = 'ext-gate-err';
      p.textContent = r.reason || '授权未完成';
      el.querySelector('.ext-gate-actions').before(p);
    }
  });
  el.querySelector('#extBack').onclick = () => ctx.navigate('articles');
}

async function renderEditor(el, ctx, id, ext) {
  const isExt = !!(ext && ext.path);
  let isNew = id === 'new';
  // 发布功能默认开启（设置里可按账号关闭）
  const publishOn = !isExt && ctx.state?.publishEnabled !== false;
  let art = { id: '', title: '', content: '', created: '', updated: '' };
  let dirty = false;
  let autoSave = false;  // 每篇文章独立的自动保存开关（持久化在文章旁的 .wf.json）
  let persistedHistory = []; // 从侧车文件读到的修改记录

  if (isExt) {
    // 外部文件：正文与标题来自磁盘，保存写回原文件；不建立侧车、不改文件名
    art = {
      id: '', title: ext.file.title, content: ext.file.content,
      created: ext.file.modTime, updated: ext.file.modTime,
    };
  } else if (!isNew) {
    try {
      art = await api.article(id);
    } catch (e) {
      ui.toast('无法打开文章', e.message, 'err');
      return ctx.navigate('articles');
    }
    // 外链文章：正文在磁盘原文件里，转到外部文件编辑器打开
    if (art.external && art.path) {
      return ctx.openExternal(art.path);
    }
    try {
      const estate = await api.articleEditor(id);
      autoSave = !!estate.autoSave;
      persistedHistory = Array.isArray(estate.history) ? estate.history : [];
    } catch { /* 无侧车文件时保持关闭 */ }
  }

  el.innerHTML = `
    <div class="editor">
      <div class="editor-head">
        <button class="btn btn-quiet" id="back" title="返回列表">${ui.icon('chevronLeft')}<span>返回</span></button>
        <input class="editor-title" id="title" placeholder="文章标题…" value="${ui.esc(art.title)}">
        <span class="editor-status" id="status">${isExt
          ? `外部文件 · ${ui.fmtSize(ext.file.size)}`
          : (isNew ? '尚未保存' : `更新于 ${ui.fmtDate(art.updated)}`)}</span>
        <button class="btn btn-ghost btn-sm" id="autoSaveBtn" title="自动保存">${ui.icon('check', 13)}<span id="autoSaveLabel">自动保存</span></button>
        ${isExt ? '' : `<button class="btn btn-ghost btn-sm" id="copyBtn" title="复制文章（兼容 Notion 粘贴）">${ui.icon('copy', 14)}<span>复制</span></button>`}
        <button class="btn btn-ghost btn-sm" id="viewBtn" title="切换编辑 / 分栏 / 预览">${ui.icon('columns', 14)}<span id="viewLabel">分栏</span></button>
        ${isNew || isExt ? '' : `<button class="btn btn-danger btn-sm" id="delBtn">${ui.icon('trash', 14)}<span>删除</span></button>`}
        <div class="head-actions">
          ${publishOn ? `<button class="btn btn-ghost btn-sm" id="publishBtn" title="发布到平台或复制发布">${ui.icon('rocket', 14)}<span>发布</span></button>` : ''}
          <button class="btn btn-primary btn-sm" id="saveBtn">${ui.icon('save', 14)}<span>保存</span></button>
        </div>
      </div>
      ${isExt ? `
      <div class="ext-bar">
        ${ui.icon('file', 15)}
        <span class="ext-path" title="${ui.esc(ext.file.displayPath || ext.path)}">${ui.esc(ext.file.displayPath || ext.path)}</span>
        ${ext.file.writable ? '' : `<span class="ext-warn">${ui.icon('alert', 13)}只读：应用对该文件没有写入权限</span>`}
        <button class="btn btn-quiet btn-xs" id="extReveal" title="在文件管理器中显示">${ui.icon('folder', 13)}<span>定位</span></button>
        <button class="btn btn-quiet btn-xs" id="extLink" title="加入文章库：只记录路径，正文仍读写原文件" ${ext.file.linked ? 'disabled' : ''}>${ui.icon('book', 13)}<span>${ext.file.linked ? '已在文章库' : '加入文章库'}</span></button>
        <button class="btn btn-quiet btn-xs" id="extToLib" title="把内容复制成一篇新的文集文章">${ui.icon('download', 13)}<span>复制一份</span></button>
      </div>` : ''}

      <div class="md-toolbar">
        <button class="md-btn" data-md="undo" id="undoBtn" title="撤销 (Ctrl+Z)">${ui.icon('undo')}</button>
        <button class="md-btn" data-md="redo" id="redoBtn" title="重做 (Ctrl+Shift+Z)">${ui.icon('redo')}</button>
        <span class="sep"></span>
        <button class="md-btn" data-md="h1" title="一级标题">H₁</button>
        <button class="md-btn" data-md="h2" title="二级标题">H₂</button>
        <button class="md-btn" data-md="h3" title="三级标题">H₃</button>
        <button class="md-btn" data-md="h4" title="四级标题">H₄</button>
        <span class="sep"></span>
        <button class="md-btn" data-md="bold" title="加粗">${ui.icon('bold')}</button>
        <button class="md-btn" data-md="italic" title="斜体">${ui.icon('italic')}</button>
        <button class="md-btn" data-md="strike" title="删除线">${ui.icon('strike')}</button>
        <button class="md-btn" data-md="code" title="行内代码">${ui.icon('code')}</button>
        <button class="md-btn" data-md="codeblock" title="代码块">${ui.icon('codeblock')}</button>
        <span class="sep"></span>
        <button class="md-btn" data-md="quote" title="引用">${ui.icon('quote')}</button>
        <button class="md-btn" data-md="ul" title="无序列表">${ui.icon('list')}</button>
        <button class="md-btn" data-md="ol" title="有序列表">${ui.icon('ordered')}</button>
        <button class="md-btn" data-md="task" title="待办">${ui.icon('task')}</button>
        <span class="sep"></span>
        <button class="md-btn" data-md="link" title="链接">${ui.icon('link')}</button>
        <button class="md-btn" data-md="image" title="上传图片">${ui.icon('image')}</button>
        <button class="md-btn" data-md="rule" title="分隔线">${ui.icon('rule')}</button>
        <button class="md-btn" data-md="table" title="表格">${ui.icon('table')}</button>
      </div>

      <div class="editor-body" id="body">
        <div class="pane pane-edit">
          <textarea class="md-input" id="md" spellcheck="false"
            placeholder="用 Markdown 写作，可拖入或粘贴图片"></textarea>
        </div>
        <div class="pane pane-preview">
          <div class="preview-flag">实时预览</div>
          <div class="markdown" id="preview"></div>
        </div>
      </div>
      <input type="file" id="filePick" accept="image/*" multiple hidden>
    </div>
  `;

  const $ = (s) => el.querySelector(s);
  const titleEl = $('#title');
  const mdEl = $('#md');
  const previewEl = $('#preview');
  const statusEl = $('#status');
  const bodyEl = $('#body');

  mdEl.value = art.content || '';
  const paint = () => { previewEl.innerHTML = renderMarkdown(mdEl.value); };
  paint();

  /* ---------- 撤销 / 重做 ---------- */

  let lastPersisted = ''; // 上次持久化的栈顶内容，避免重复请求

  const undoBtn = $('#undoBtn');
  const redoBtn = $('#redoBtn');
  const hist = createHistory(mdEl, {
    onApply: () => { markDirty(); paint(); scheduleAutoSave(); },
    onStackChange: ({ canUndo, canRedo }) => {
      undoBtn.disabled = !canUndo;
      redoBtn.disabled = !canRedo;
      persistEditorSoon();
    },
  });

  /* ---------- 自动保存与状态持久化 ---------- */

  const nowTime = () => {
    const d = new Date();
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
  };
  const autoSaveBtn = $('#autoSaveBtn');
  const autoSaveLabel = $('#autoSaveLabel');
  const paintAutoSave = () => {
    autoSaveBtn.classList.toggle('on', autoSave);
    autoSaveLabel.textContent = autoSave ? '自动保存 · 开' : '自动保存 · 关';
    autoSaveBtn.title = autoSave
      ? '自动保存已开启，点击关闭'
      : '自动保存已关闭，点击开启';
  };
  paintAutoSave();

  const persistEditor = () => {
    if (isExt || !art.id) return; // 外部文件不写侧车；新文章首次保存后再建立侧车文件
    const entries = hist.entries();
    const top = entries.length ? entries[entries.length - 1].content : '';
    if (top === lastPersisted) return;
    const hasEdits = entries.length > 1;
    if (!autoSave && !hasEdits) return; // 没开自动保存也没编辑过，不落盘
    lastPersisted = top;
    return api.saveArticleEditor(art.id, { autoSave, history: entries }).catch(() => {});
  };
  // 强制刷新一次侧车状态（保存后文件名可能已变）
  const flushEditorState = async () => {
    if (!art.id) return;
    lastPersisted = '';
    await persistEditor();
  };
  const persistEditorSoon = ui.debounce(persistEditor, 1000);

  // 恢复侧车文件里的修改记录（栈顶锚定为当前加载的正文）；
  // 放在 persistEditorSoon 声明之后，避免恢复时触发持久化早于其定义
  hist.restore(persistedHistory);
  lastPersisted = (hist.entries().slice(-1)[0] || {}).content || '';

  let saveChain = Promise.resolve(); // 保存串行化，避免自动/手动保存并发
  const scheduleAutoSave = ui.debounce(() => {
    if (autoSave && dirty && !saving) enqueueSave(true);
  }, 1500);
  let saving = false;
  const enqueueSave = (auto) => {
    saveChain = saveChain.then(() => save({ auto })).catch(() => {});
    return saveChain;
  };

  autoSaveBtn.onclick = () => {
    autoSave = !autoSave;
    paintAutoSave();
    persistEditor(); // 开关状态立即落盘
    if (autoSave) {
      if (dirty) enqueueSave(true);
      ui.toast('自动保存已开启', '', 'ok', 2600);
    } else {
      ui.toast('自动保存已关闭', '', '', 2600);
    }
  };

  const markDirty = () => {
    if (dirty) return;
    dirty = true;
    if (autoSave) {
      statusEl.textContent = '正在自动保存…';
      statusEl.style.color = 'var(--ink-3)';
    } else {
      statusEl.textContent = '有未保存的修改';
      statusEl.style.color = 'var(--warn)';
    }
  };
  mdEl.addEventListener('input', () => { markDirty(); ui.debounce(paint, 180)(); scheduleAutoSave(); });
  titleEl.addEventListener('input', () => { markDirty(); scheduleAutoSave(); });

  // 撤销 / 重做快捷键（接管被程序化插入破坏的原生 undo）
  mdEl.addEventListener('keydown', (e) => {
    if (!(e.ctrlKey || e.metaKey)) return;
    const k = e.key.toLowerCase();
    if (k === 'z' && !e.shiftKey) { e.preventDefault(); hist.undo(); }
    else if ((k === 'z' && e.shiftKey) || k === 'y') { e.preventDefault(); hist.redo(); }
  });

  // 编辑 / 分栏 / 预览
  const modes = ['split', 'edit', 'preview'];
  const labels = { split: '分栏', edit: '编辑', preview: '预览' };
  let mode = 'split';
  const applyMode = () => {
    bodyEl.classList.toggle('single', mode !== 'split');
    bodyEl.classList.toggle('preview-only', mode === 'preview');
    $('#viewLabel').textContent = labels[mode];
  };
  $('#viewBtn').onclick = () => {
    mode = modes[(modes.indexOf(mode) + 1) % modes.length];
    applyMode();
  };

  // 工具栏
  const wrap = (before, after = before, placeholder = '') => {
    const s = mdEl.selectionStart, e = mdEl.selectionEnd;
    const sel = mdEl.value.slice(s, e) || placeholder;
    const text = before + sel + after;
    mdEl.setRangeText(text, s, e, 'end');
    if (!mdEl.value.slice(s, e)) mdEl.setSelectionRange(s + before.length, s + before.length + sel.length);
    mdEl.focus();
    hist.push(); // 程序化插入：强制入栈，保证撤销精准
    markDirty();
    scheduleAutoSave();
    ui.debounce(paint, 60)();
  };
  const linePrefix = (prefix) => {
    const s = mdEl.selectionStart;
    const ls = mdEl.value.lastIndexOf('\n', s - 1) + 1;
    mdEl.setRangeText(prefix, ls, ls, 'end');
    mdEl.focus();
    hist.push();
    markDirty();
    scheduleAutoSave();
    ui.debounce(paint, 60)();
  };
  const actions = {
    undo: () => hist.undo(),
    redo: () => hist.redo(),
    h1: () => linePrefix('# '),
    h2: () => linePrefix('## '),
    h3: () => linePrefix('### '),
    h4: () => linePrefix('#### '),
    bold: () => wrap('**', '**', '粗体'),
    italic: () => wrap('*', '*', '斜体'),
    strike: () => wrap('~~', '~~', '删除线'),
    code: () => wrap('`', '`', 'code'),
    codeblock: () => wrap('\n```\n', '\n```\n', '代码'),
    quote: () => linePrefix('> '),
    ul: () => linePrefix('- '),
    ol: () => linePrefix('1. '),
    task: () => linePrefix('- [ ] '),
    rule: () => wrap('\n---\n', '', ''),
    table: () => wrap('\n| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |\n', '', ''),
    link: () => wrap('[', '](https://)', '链接文字'),
    image: () => $('#filePick').click(),
    imageUrl: () => wrap('![', '](https://)', '图片说明'),
  };
  el.querySelectorAll('[data-md]').forEach((b) => {
    b.onclick = () => (actions[b.dataset.md] || (() => {}))();
  });

  // 插入文本
  const insertAtCursor = (text) => {
    const s = mdEl.selectionStart, e = mdEl.selectionEnd;
    mdEl.setRangeText(text, s, e, 'end');
    hist.push();
    markDirty();
    scheduleAutoSave();
    paint();
  };

  // 上传
  const uploadFiles = async (files) => {
    const imgs = [...files].filter((f) => f.type.startsWith('image/'));
    if (!imgs.length) return;
    statusEl.textContent = `正在上传 ${imgs.length} 张图片…`;
    statusEl.style.color = 'var(--ink-3)';
    let done = 0;
    for (const f of imgs) {
      try {
        const r = await api.upload(f);
        const alt = (f.name || 'image').replace(/\.[^.]+$/, '');
        insertAtCursor(`\n![${alt}](${r.markdown})\n`);
        done++;
        if (r.duplicate) ui.toast('图片已存在', `复用 ${r.name}`, 'ok');
        else if (r.provider === 'cloud') ui.toast('已上传到图床', r.name, 'ok');
        else ui.toast('已保存到本地', `photo/${r.name}`, 'ok');
      } catch (e) {
        ui.toast('上传失败', `${f.name}：${e.message}`, 'err', 7000);
      }
    }
    statusEl.textContent = `已插入 ${done} 张图片`;
    statusEl.style.color = 'var(--ink-3)';
    ctx.refreshState();
  };

  $('#filePick').onchange = (e) => { uploadFiles(e.target.files); e.target.value = ''; };
  mdEl.addEventListener('paste', (e) => {
    const files = [...(e.clipboardData?.items || [])]
      .filter((i) => i.kind === 'file' && i.type.startsWith('image/'))
      .map((i) => i.getAsFile())
      .filter(Boolean);
    if (files.length) { e.preventDefault(); uploadFiles(files); }
  });
  mdEl.addEventListener('dragover', (e) => { e.preventDefault(); mdEl.classList.add('dragover'); });
  mdEl.addEventListener('dragleave', () => mdEl.classList.remove('dragover'));
  mdEl.addEventListener('drop', (e) => {
    e.preventDefault();
    mdEl.classList.remove('dragover');
    if (e.dataTransfer?.files?.length) uploadFiles(e.dataTransfer.files);
  });

  // 滚动同步
  let syncing = false;
  mdEl.addEventListener('scroll', () => {
    if (syncing || mode !== 'split') return;
    syncing = true;
    const pane = el.querySelector('.pane-preview');
    const r = mdEl.scrollTop / Math.max(1, mdEl.scrollHeight - mdEl.clientHeight);
    pane.scrollTop = r * Math.max(0, pane.scrollHeight - pane.clientHeight);
    setTimeout(() => { syncing = false; }, 40);
  });

  // 保存
  const save = async (opts = {}) => {
    const auto = !!opts.auto;
    const title = titleEl.value.trim() || '未命名文章';
    const content = mdEl.value;
    const prevID = art.id;
    saving = true;
    $('#saveBtn').disabled = true;
    statusEl.textContent = auto ? '正在自动保存…' : '正在保存…';
    statusEl.style.color = 'var(--ink-3)';
    try {
      let saved;
      if (isExt) {
        saved = await api.externalSave(ext.path, title, content);
        art.title = saved.title;
        art.content = saved.content;
        art.updated = saved.modTime;
      } else {
        saved = isNew
          ? await api.createArticle({ title, content })
          : await api.updateArticle(prevID, { title, content });
        isNew = false;
        art = saved;
        // 标题变化会换文件名与 ID。这里只静默替换地址，不重挂编辑器：
        // 否则开启自动保存后每改一次标题都会整页重渲染（表现为加载动画反复闪现）。
        if (saved.id !== prevID) {
          await flushEditorState();
          ctx.params = { id: saved.id };
          ctx.syncHash('#articles/' + encodeURIComponent(saved.id));
          ensureDeleteBtn();
        }
      }
      dirty = false;
      statusEl.textContent = auto
        ? `已自动保存 · ${nowTime()}`
        : (isExt ? `已写回原文件 · ${nowTime()}` : `已保存 · ${ui.fmtDate(saved.updated)}`);
      statusEl.style.color = 'var(--ok)';
      ctx.refreshState();
      if (!auto) ui.toast(isExt ? '已写回原文件' : '已保存', isExt ? saved.name : saved.title, 'ok', 2000);
      return true;
    } catch (e) {
      statusEl.textContent = auto ? '自动保存失败' : '保存失败';
      statusEl.style.color = 'var(--err)';
      if (auto) ui.toast('自动保存失败', e.message, 'err');
      else ui.toast('保存失败', e.message, 'err');
      return false;
    } finally {
      saving = false;
      $('#saveBtn').disabled = false;
    }
  };

  $('#saveBtn').onclick = () => save();
  $('#copyBtn') && ($('#copyBtn').onclick = async () => {
    if (isNew || dirty) {
      const ok = await save();
      if (!ok) return;
    }
    await doCopy(art.id, ctx);
  });

  // 外部文件：在文件管理器中定位 / 复制一份到文章库（不打断当前编辑）
  $('#extReveal') && ($('#extReveal').onclick = async () => {
    const ok = await revealInManager(ext.path);
    if (!ok) ui.toast('无法打开文件管理器', '请在飞牛桌面中操作', 'warn', 5000);
  });
  $('#extLink') && ($('#extLink').onclick = async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    if (dirty) {
      const ok = await save();
      if (!ok) { btn.disabled = false; return; }
    }
    try {
      await api.externalLink(ext.path, titleEl.value.trim());
      const label = btn.querySelector('span');
      if (label) label.textContent = '已在文章库';
      ui.toast('已加入文章库', '条目只记录路径，不复制内容、不占额外空间', 'ok', 4500);
    } catch (err) {
      btn.disabled = false;
      ui.toast('加入失败', err.message, 'err', 6000);
    }
  });
  $('#extToLib') && ($('#extToLib').onclick = async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    try {
      if (dirty) {
        const ok = await save();
        if (!ok) { btn.disabled = false; return; }
      }
      const r = await api.externalImport([ext.path]);
      const one = (r.imported || [])[0];
      if (one) {
        ui.toast('已导入到文章库', one.title, 'ok', 4000);
        ctx.navigate('articles', { id: one.id });
      } else {
        ui.toast('导入失败', (r.skipped || []).join('、') || '', 'err', 6000);
      }
    } catch (err) {
      ui.toast('导入失败', err.message, 'err', 6000);
    } finally {
      btn.disabled = false;
    }
  });
  $('#publishBtn') && ($('#publishBtn').onclick = async () => {
    if (isNew || dirty) {
      const ok = await save();
      if (!ok) return;
    }
    await openPublishDialog(ctx, {
      id: art.id,
      title: titleEl.value.trim() || art.title,
      content: mdEl.value,
    });
  });
  const onDelete = async () => {
    const ok = await ui.confirmDialog({
      title: '删除这篇文章？',
      desc: '对应 .md 文件将被移除，且无法恢复。',
      okText: '删除', danger: true,
    });
    if (!ok) return;
    try {
      await api.deleteArticle(art.id);
      dirty = false;
      ui.toast('已删除', '', 'ok');
      ctx.navigate('articles'); // 直接回列表，避免按已删除的 ID 重渲染出 404
    } catch (e) { ui.toast('删除失败', e.message, 'err'); }
  };
  const bindDelete = () => { const b = $('#delBtn'); if (b) b.onclick = onDelete; };
  bindDelete();
  // 新文章首次保存后不再重挂编辑器，删除按钮改为就地补上
  const ensureDeleteBtn = () => {
    if ($('#delBtn')) return;
    const actions = el.querySelector('.head-actions');
    if (!actions) return;
    actions.insertAdjacentHTML('beforebegin',
      `<button class="btn btn-danger btn-sm" id="delBtn">${ui.icon('trash', 14)}<span>删除</span></button>`);
    bindDelete();
  };
  $('#back').onclick = () => ctx.navigate('articles'); // 未保存提示由路由层统一处理

  // 离开确认：侧边栏 / 返回 / 浏览器后退都经 hashchange 触发这里
  ctx.leaveGuard = async () => {
    if (!dirty) return true;
    const choice = await ui.unsavedDialog(() => save());
    return !!choice; // 'save' / 'leave' 放行；null（取消或保存失败）拦截
  };
  ctx.hasUnsaved = () => dirty;

  bindShortcut(() => save());

  mdEl.focus();
}

// 编辑器只保留一份快捷键监听，避免多次渲染后重复触发
let shortcutHandler = null;

function bindShortcut(onSave) {
  if (shortcutHandler) document.removeEventListener('keydown', shortcutHandler);
  shortcutHandler = (e) => {
    if (!document.getElementById('md')) {
      document.removeEventListener('keydown', shortcutHandler);
      shortcutHandler = null;
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      onSave();
    }
  };
  document.addEventListener('keydown', shortcutHandler);
}
