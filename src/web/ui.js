/* 通用 UI 工具：图标、提示、弹窗、格式化 */

export const ICONS = {
  plus: '<path d="M12 5v14M5 12h14"/>',
  pencil: '<path d="M4 20h4l10-10-4-4L4 16z"/><path d="m14 6 4 4"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5h10"/>',
  trash: '<path d="M4 7h16"/><path d="M9 7V5h6v2"/><path d="M6 7l1 12h10l1-12"/><path d="M10 11v5M14 11v5"/>',
  search: '<circle cx="11" cy="11" r="6"/><path d="m16 16 4 4"/>',
  chevronLeft: '<path d="M14.5 6 9 12l5.5 6"/>',
  chevronRight: '<path d="M9.5 6 15 12l-5.5 6"/>',
  folder: '<path d="M3.5 6.5A2 2 0 0 1 5.5 4.5h4l2 2.5h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"/>',
  image: '<rect x="3.5" y="5" width="17" height="14" rx="2.5"/><circle cx="9" cy="10" r="1.6"/><path d="m4 17 5-4.5 4 3.5 3-2.5 4 3.5"/>',
  upload: '<path d="M12 16V4"/><path d="m7.5 8.5 4.5-4.5 4.5 4.5"/><path d="M5 20h14"/>',
  link: '<path d="M10 13.5a3.5 3.5 0 0 0 5 0l3-3a3.5 3.5 0 1 0-5-5l-1 1"/><path d="M14 10.5a3.5 3.5 0 0 0-5 0l-3 3a3.5 3.5 0 1 0 5 5l1-1"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7"/>',
  close: '<path d="M6 6l12 12M18 6 6 18"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 6.3"/><path d="M20 5v6h-6"/>',
  external: '<path d="M14 4h6v6"/><path d="M20 4 11 13"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
  alert: '<path d="M12 4 2.5 20h19z"/><path d="M12 10v4M12 17.2v.1"/>',
  info: '<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5.5M12 7.8v.1"/>',
  bold: '<path d="M7 5h6a3.5 3.5 0 0 1 0 7H7z"/><path d="M7 12h7a3.5 3.5 0 0 1 0 7H7z"/>',
  italic: '<path d="M15 5h-5M14 19H9M14.5 5 9.5 19"/>',
  heading: '<path d="M6 5v14M18 5v14M6 12h12"/>',
  quote: '<path d="M7 7c-2 0-3 1.4-3 3.4S5 14 7 14c0 2-1 3-3 3.5"/><path d="M16 7c-2 0-3 1.4-3 3.4S14 14 16 14c0 2-1 3-3 3.5"/>',
  code: '<path d="m9 8-4 4 4 4M15 8l4 4-4 4"/>',
  codeblock: '<rect x="3.5" y="3.5" width="17" height="17" rx="2.5"/><path d="M10 9.5 8 12l2 2.5"/><path d="M14 9.5l2 2.5-2 2.5"/>',
  list: '<path d="M8 7h11M8 12h11M8 17h11"/><circle cx="4.5" cy="7" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="17" r="1"/>',
  ordered: '<path d="M9 7h10M9 12h10M9 17h10"/><path d="M4 6h1.2v4M4 10h2.4M4 15.4c0-.7 2-.7 2 .2 0 .6-2 1.2-2 2.4h2.4"/>',
  rule: '<path d="M4 12h16"/>',
  strike: '<path d="M5 12h14"/><path d="M8.5 7.5C8.5 6 10 5 12 5s3.5 1 4.5 2.5"/><path d="M15.5 16.5c0 1.5-1.5 2.5-3.5 2.5s-3.5-1-4.5-2.5"/>',
  task: '<rect x="4" y="4" width="16" height="16" rx="4"/><path d="m8.5 12 2.5 2.5 4.5-5"/>',
  table: '<rect x="3.5" y="5" width="17" height="14" rx="2"/><path d="M3.5 10h17M9.5 10v9M15 10v9"/>',
  eye: '<path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12 18 17.5 12 17.5 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="2.6"/>',
  save: '<path d="M5 5h11l3 3v11H5z"/><path d="M9 5v5h6V5"/><path d="M8 19v-5h8v5"/>',
  columns: '<rect x="3.5" y="5" width="17" height="14" rx="2"/><path d="M12 5v14"/>',
  cloud: '<path d="M7 18h10a4 4 0 0 0 .6-7.95A5.5 5.5 0 0 0 7 9.2 4.4 4.4 0 0 0 7 18z"/>',
  drive: '<path d="M4 13.5h16"/><path d="M5.5 13.5 8 5h8l2.5 8.5"/><path d="M5.5 13.5V19h13v-5.5"/>',
  rocket: '<path d="M12 3c3.5 1 6 4 6 8 0 2-1 3.5-2 4.5H8C7 14.5 6 13 6 11c0-4 2.5-7 6-8z"/><circle cx="12" cy="10" r="1.6"/><path d="M8.5 15.5 6 21l3.5-1.8M15.5 15.5 18 21l-3.5-1.8"/>',
  book: '<path d="M4 5.5A1.5 1.5 0 0 1 5.5 4H11v16H5.5A1.5 1.5 0 0 1 4 18.5z"/><path d="M20 5.5A1.5 1.5 0 0 0 18.5 4H13v16h5.5A1.5 1.5 0 0 0 20 18.5z"/>',
  clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>',
  undo: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5 5.5 5.5 0 0 1-5.5 5.5H11"/>',
  redo: '<path d="m15 14 5-5-5-5"/><path d="M20 9H9.5A5.5 5.5 0 0 0 4 14.5 5.5 5.5 0 0 0 9.5 20H13"/>',
  download: '<path d="M12 4v12"/><path d="m7.5 11.5 4.5 4.5 4.5-4.5"/><path d="M5 20h14"/>',
  palette: '<path d="M12 3a9 9 0 1 0 0 18 2.2 2.2 0 0 0 2.2-2.2c0-1.2-1-2.2-2.2-2.2H11a2 2 0 0 1 0-4h4.5A4.5 4.5 0 0 0 20 8.2C19.5 5.3 16 3 12 3z"/><circle cx="8" cy="10" r="1.2" fill="currentColor" stroke="none"/><circle cx="12" cy="7.5" r="1.2" fill="currentColor" stroke="none"/><circle cx="16" cy="10" r="1.2" fill="currentColor" stroke="none"/>',
  type: '<path d="M5 6.5V5h14v1.5"/><path d="M12 5v14"/><path d="M9 19h6"/>',
};

export function icon(name, size = 16) {
  const d = ICONS[name] || '';
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="${size}" height="${size}">${d}</svg>`;
}

export function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

/* ---------------- 提示 ---------------- */

export function toast(title, desc = '', kind = 'ok', ms = 3600) {
  const host = document.getElementById('toasts');
  const node = document.createElement('div');
  node.className = `toast ${kind}`;
  node.innerHTML = `<span class="bar"></span><div><div class="title">${esc(title)}</div>${desc ? `<div class="desc">${esc(desc)}</div>` : ''}</div>`;
  host.appendChild(node);
  const kill = () => {
    node.classList.add('out');
    setTimeout(() => node.remove(), 240);
  };
  setTimeout(kill, ms);
  node.addEventListener('click', kill);
}

/* ---------------- 弹窗 ---------------- */

export function modal(html, { wide = false, onMount, onClose } = {}) {
  const host = document.getElementById('modalHost');
  host.innerHTML = `<div class="modal ${wide ? 'wide' : ''}">${html}</div>`;
  host.classList.add('open');
  const close = () => {
    host.classList.remove('open');
    host.innerHTML = '';
    onClose && onClose();
  };
  host.onclick = (e) => { if (e.target === host) close(); };
  document.addEventListener('keydown', function onKey(e) {
    if (e.key === 'Escape' && host.classList.contains('open')) {
      close();
      document.removeEventListener('keydown', onKey);
    }
  });
  if (onMount) onMount(host.querySelector('.modal'), close);
  return close;
}

export function confirmDialog({ title, desc, okText = '确认', danger = false }) {
  return new Promise((resolve) => {
    let done = false;
    const finish = (v) => { if (!done) { done = true; resolve(v); } };
    const close = modal(`
      <div class="modal-head"><h3>${esc(title)}</h3>${desc ? `<p>${esc(desc)}</p>` : ''}</div>
      <div class="modal-foot">
        <button class="btn btn-ghost" data-act="cancel">取消</button>
        <button class="btn ${danger ? 'btn-primary' : 'btn-primary'}" data-act="ok" ${danger ? 'style="background:var(--err)"' : ''}>${esc(okText)}</button>
      </div>`, {
      onMount(root, closeFn) {
        root.querySelector('[data-act="cancel"]').onclick = () => { finish(false); closeFn(); };
        root.querySelector('[data-act="ok"]').onclick = () => { finish(true); closeFn(); };
      },
    });
  });
}

/* ---------------- 未保存离开确认 ---------------- */

// 有未保存修改时拦截离开：保存并离开 / 直接离开 / 取消。
// onSave 返回 Promise<boolean>（保存成功才放行）；点遮罩或 Esc 视为取消。
export function unsavedDialog(onSave) {
  return new Promise((resolve) => {
    let done = false;
    const finish = (v) => { if (!done) { done = true; resolve(v); } };
    const close = modal(`
      <div class="modal-head"><h3>有未保存的修改</h3><p>离开将丢失未保存的内容，建议先保存。</p></div>
      <div class="modal-foot">
        <button class="btn btn-ghost" data-act="cancel">取消</button>
        <button class="btn btn-ghost" data-act="leave">直接离开</button>
        <button class="btn btn-primary" data-act="save">保存并离开</button>
      </div>`, {
      onClose: () => finish(null),
      onMount(root, closeFn) {
        root.querySelector('[data-act="cancel"]').onclick = () => { finish(null); closeFn(); };
        root.querySelector('[data-act="leave"]').onclick = () => { finish('leave'); closeFn(); };
        root.querySelector('[data-act="save"]').onclick = async (e) => {
          const btn = e.currentTarget;
          btn.disabled = true;
          btn.textContent = '保存中…';
          const ok = await onSave();
          if (ok) { finish('save'); closeFn(); }
          else { btn.disabled = false; btn.textContent = '保存并离开'; }
        };
      },
    });
  });
}

/* ---------------- 剪贴板（兼容非 HTTPS 环境） ---------------- */

export async function copy(text, label = '已复制') {
  let ok = false;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      ok = true;
    }
  } catch { ok = false; }

  if (!ok) {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;left:-9999px;top:0;opacity:0';
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    try { ok = document.execCommand('copy'); } catch { ok = false; }
    ta.remove();
  }
  if (ok) toast(label, '', 'ok', 2200);
  else toast('复制失败', '当前环境不支持自动复制，请手动选择内容', 'err');
  return ok;
}

// 富文本剪贴板：同时写入 text/html 与 text/plain。
// 粘贴进 Notion 等编辑器时优先按 HTML 解析——粗体、标题、列表原样保留，
// 不会出现 Markdown 分词导致的错乱换行；纯文本环境则退化为 Markdown 源码。
export async function copyRich(html, text, label = '已复制') {
  // 1) 现代剪贴板 API（需安全上下文）
  try {
    if (navigator.clipboard && window.isSecureContext && window.ClipboardItem) {
      const item = new ClipboardItem({
        'text/html': new Blob([html], { type: 'text/html' }),
        'text/plain': new Blob([text], { type: 'text/plain' }),
      });
      await navigator.clipboard.write([item]);
      toast(label, '', 'ok', 2200);
      return true;
    }
  } catch { /* 落到 execCommand 兼容路径 */ }

  // 2) 兼容路径（http 直连常见）：隐藏容器选中后 execCommand，浏览器会自动带上 text/html
  try {
    const box = document.createElement('div');
    box.contentEditable = 'true';
    box.innerHTML = html;
    box.style.cssText = 'position:fixed;left:-9999px;top:0;opacity:0;white-space:pre-wrap;max-width:1px;overflow:hidden';
    document.body.appendChild(box);
    const range = document.createRange();
    range.selectNodeContents(box);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { ok = false; }
    sel.removeAllRanges();
    box.remove();
    if (ok) {
      toast(label, '', 'ok', 2200);
      return true;
    }
  } catch { /* 忽略 */ }

  // 3) 全部失败：退化为纯 Markdown 文本
  return copy(text, label);
}

/* ---------------- 格式化 ---------------- */

export function fmtDate(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fmtDateShort(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n) => String(n).padStart(2, '0');
  const now = new Date();
  const sameYear = d.getFullYear() === now.getFullYear();
  const md = `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return sameYear ? md : `${d.getFullYear()}-${md}`;
}

export function fmtSize(bytes) {
  if (!bytes && bytes !== 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let n = bytes, i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`;
}

export function fmtCount(n) {
  const v = Number(n);
  if (!Number.isFinite(v)) return '—';
  if (v >= 100000000) return `${(v / 100000000).toFixed(1)} 亿`;
  if (v >= 10000) return `${(v / 10000).toFixed(1)} 万`;
  return v.toLocaleString('zh-CN');
}

export function debounce(fn, ms = 260) {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}
