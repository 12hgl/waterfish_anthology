/* 图片库：本地图片 + 图床图片 */

import { api } from '../api.js';
import * as ui from '../ui.js';

const local = { tab: 'local' };

export async function render(el, ctx) {
  el.innerHTML = `
    <div class="page-head">
      <div>
        <h1 class="page-title">图片库</h1>
        <div class="page-sub">正在加载…</div>
      </div>
      <div class="spacer"></div>
      <button class="btn btn-ghost" id="refresh">${ui.icon('refresh')}<span>刷新</span></button>
      <button class="btn btn-primary" id="uploadBtn">${ui.icon('upload')}<span>上传图片</span></button>
      <input type="file" id="pick" accept="image/*" multiple hidden>
    </div>
    <div class="toolbar">
      <div class="tabs" id="tabs">
        <button class="tab is-active" data-tab="local">本地图片</button>
        <button class="tab" data-tab="cloud">图床图片</button>
      </div>
      <div class="spacer" style="flex:1"></div>
      <span class="page-sub" id="tip" style="margin:0"></span>
    </div>
    <div id="grid" class="image-grid"></div>
  `;

  const $ = (s) => el.querySelector(s);
  const grid = $('#grid');
  const sub = el.querySelector('.page-sub');
  const tip = $('#tip');

  // 每次进入图片库都重新拉取，保证编辑器里新上传的图片立即可见
  let data;
  try {
    data = await api.images();
  } catch (e) {
    grid.innerHTML = `<div class="empty" style="grid-column:1/-1">${ui.icon('image', 56)}<h3>无法读取图片</h3><p>${ui.esc(e.message)}</p></div>`;
    return;
  }

  sub.textContent = `本地 ${data.local.length} 张 · 图床 ${data.cloud.length} 张`;
  tip.textContent = data.cloudEnabled
    ? '新上传的图片进入 Cloudinary'
    : '新上传的图片保存到 photo 目录';

  $('#tabs').querySelectorAll('.tab').forEach((t) => {
    t.classList.toggle('is-active', t.dataset.tab === local.tab);
    t.onclick = () => {
      local.tab = t.dataset.tab;
      $('#tabs').querySelectorAll('.tab').forEach((x) => x.classList.toggle('is-active', x.dataset.tab === local.tab));
      paint();
    };
  });

  const paint = () => {
    if (local.tab === 'local') {
      if (!data.local.length) return void (grid.innerHTML = emptyBox('还没有本地图片', '保存到 photo 目录'));
      grid.innerHTML = data.local.map(localCell).join('');
      grid.querySelectorAll('[data-name]').forEach((cell) => {
        cell.querySelector('[data-copy-url]').onclick = () =>
          ui.copy(new URL(`photo/${cell.dataset.name}`, document.baseURI).href, '图片直链已复制');
        cell.querySelector('[data-copy-name]').onclick = () => ui.copy(cell.dataset.name, '文件名已复制');
        cell.querySelector('[data-del]').onclick = async () => {
          const ok = await ui.confirmDialog({
            title: '删除这张图片？',
            desc: `photo/${cell.dataset.name} 将被永久删除。若文章仍引用它，图片会失效。`,
            okText: '删除', danger: true,
          });
          if (!ok) return;
          try {
            await api.deleteImage(cell.dataset.name);
            data.local = data.local.filter((x) => x.name !== cell.dataset.name);
            sub.textContent = `本地 ${data.local.length} 张 · 图床 ${data.cloud.length} 张`;
            ui.toast('已删除', cell.dataset.name, 'ok');
            paint();
            ctx.refreshState();
          } catch (e) { ui.toast('删除失败', e.message, 'err'); }
        };
      });
      return;
    }

    if (!data.cloud.length) {
      grid.innerHTML = emptyBox('还没有图床图片', data.cloudEnabled ? '在编辑器里上传图片' : '先在设置里启用 Cloudinary');
      return;
    }
    grid.innerHTML = data.cloud.map((c) => cloudCell(c, data.local)).join('');
    grid.querySelectorAll('[data-url]').forEach((cell) => {
      cell.querySelector('[data-copy-url]').onclick = () => ui.copy(cell.dataset.url, '图床链接已复制');
      cell.querySelector('[data-copy-md]').onclick = () => ui.copy(`![${cell.dataset.name}](${cell.dataset.url})`, 'Markdown 已复制');
    });
  };

  paint();

  $('#refresh').onclick = () => ctx.refresh();
  $('#uploadBtn').onclick = () => $('#pick').click();
  $('#pick').onchange = async (e) => {
    const files = [...e.target.files];
    e.target.value = '';
    if (!files.length) return;
    const btn = $('#uploadBtn');
    btn.disabled = true;
    const old = btn.innerHTML;
    btn.innerHTML = `<span class="spinner"></span><span>上传中…</span>`;
    let ok = 0, dup = 0;
    for (const f of files) {
      try { const r = await api.upload(f); ok++; if (r.duplicate) dup++; } catch (err) { ui.toast('上传失败', `${f.name}：${err.message}`, 'err', 7000); }
    }
    btn.disabled = false;
    btn.innerHTML = old;
    if (ok) ui.toast(`已上传 ${ok} 张`, '', 'ok');
    if (dup) ui.toast(`${dup} 张图片已存在`, '已复用已有图片', 'ok');
    local.tab = data.cloudEnabled ? 'cloud' : 'local';
    ctx.refresh();
  };
}

function localCell(img) {
  return `
    <div class="image-cell" data-name="${ui.esc(img.name)}">
      <span class="image-badge">本地</span>
      <img src="${ui.esc(img.url)}" alt="${ui.esc(img.name)}" loading="lazy">
      <div class="veil">
        <div class="name" title="${ui.esc(img.name)}">${ui.esc(img.name)} · ${ui.fmtSize(img.size)}</div>
        <div class="row">
          <button data-copy-url>复制链接</button>
          <button data-copy-name>复制名</button>
          <button data-del>删除</button>
        </div>
      </div>
    </div>`;
}

function cloudCell(c, localList) {
  const thumb = c.url.replace('/upload/', '/upload/w_400,q_auto,f_auto/');
  const hasLocal = localList.some((l) => l.name === c.name);
  return `
    <div class="image-cell" data-url="${ui.esc(c.url)}" data-name="${ui.esc(c.name)}">
      <span class="image-badge cloud">图床</span>
      <img src="${ui.esc(thumb)}" alt="${ui.esc(c.name)}" loading="lazy"
           onerror="this.src='${ui.esc(c.url)}'">
      <div class="veil">
        <div class="name" title="${ui.esc(c.name)}">${ui.esc(c.name)}${hasLocal ? ' · 本地同名' : ''}</div>
        <div class="row">
          <button data-copy-url>复制链接</button>
          <button data-copy-md>复制 MD</button>
        </div>
      </div>
    </div>`;
}

function emptyBox(title, desc) {
  return `<div class="empty" style="grid-column:1/-1">${ui.icon('image', 56)}<h3>${ui.esc(title)}</h3><p>${ui.esc(desc)}</p></div>`;
}
