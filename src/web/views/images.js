/* 图片库：本地图片 + 图床图片
 * - 点击任意图片直接进入灯箱预览（←/→ 切换，Esc 关闭）
 * - 「批量管理」进入多选：全选 / 复制链接 / 复制 Markdown / 批量删除
 */

import { api } from '../api.js';
import * as ui from '../ui.js';

const local = { tab: 'local', selectMode: false };
const sel = new Set(); // key = "local:文件名" 或 "cloud:publicId|url"

export async function render(el, ctx) {
  local.selectMode = false;
  sel.clear();

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
      <button class="btn btn-ghost btn-sm" id="selectToggle">${ui.icon('check', 14)}<span id="selectLabel">批量管理</span></button>
    </div>
    <div class="selbar" id="selbar" hidden>
      <span class="selbar-count" id="selCount">已选 0 张</span>
      <div class="spacer" style="flex:1"></div>
      <button class="btn btn-ghost btn-sm" id="selAll">全选</button>
      <button class="btn btn-ghost btn-sm" id="copyLinks">复制链接</button>
      <button class="btn btn-ghost btn-sm" id="copyMd">复制 Markdown</button>
      <button class="btn btn-danger btn-sm" id="selDel">${ui.icon('trash', 14)}<span>删除</span></button>
      <button class="btn btn-quiet btn-sm" id="selCancel">取消</button>
    </div>
    <div id="grid" class="image-grid"></div>
  `;

  const $ = (s) => el.querySelector(s);
  const grid = $('#grid');
  const sub = el.querySelector('.page-sub');
  const tip = $('#tip');
  const selbar = $('#selbar');

  let data;
  try {
    data = await api.images();
  } catch (e) {
    grid.innerHTML = `<div class="empty" style="grid-column:1/-1">${ui.icon('image', 56)}<h3>无法读取图片</h3><p>${ui.esc(e.message)}</p></div>`;
    return;
  }

  const cloudOn = !!data.cloudEnabled;
  sub.textContent = `本地 ${data.local.length} 张 · 图床 ${data.cloud.length} 张`;
  tip.textContent = cloudOn ? '新上传的图片进入 Cloudinary' : '新上传的图片保存到 photo 目录';

  // 统一成一种结构，供网格、灯箱、批量操作共用
  const itemsOf = (kind) => (kind === 'local'
    ? data.local.map((l) => ({
      kind: 'local', key: `local:${l.name}`, name: l.name,
      src: l.url, big: l.url, size: l.size, time: l.updated,
    }))
    : data.cloud.map((c, i) => ({
      kind: 'cloud', key: `cloud:${c.publicId || c.url || i}`, id: c.publicId || c.url || String(i),
      name: c.name, src: c.url.replace('/upload/', '/upload/w_400,q_auto,f_auto/'), big: c.url,
      size: c.bytes, time: c.created,
    })));

  const absolute = (item) => (item.kind === 'local'
    ? new URL(item.src, document.baseURI).href
    : item.big);

  const markdownOf = (item) => (item.kind === 'local'
    ? `![${item.name}](${item.src})`
    : `![${item.name}](${item.big})`);

  $('#tabs').querySelectorAll('.tab').forEach((t) => {
    t.classList.toggle('is-active', t.dataset.tab === local.tab);
    t.onclick = () => {
      local.tab = t.dataset.tab;
      $('#tabs').querySelectorAll('.tab').forEach((x) => x.classList.toggle('is-active', x.dataset.tab === local.tab));
      paint();
    };
  });

  const paintSel = () => {
    selbar.hidden = !local.selectMode;
    $('#selectLabel').textContent = local.selectMode ? '退出批量' : '批量管理';
    $('#selectToggle').classList.toggle('on', local.selectMode);
    $('#selCount').textContent = `已选 ${sel.size} 张`;
    $('#copyLinks').disabled = !sel.size;
    $('#copyMd').disabled = !sel.size;
    $('#selDel').disabled = !sel.size;
    const cur = itemsOf(local.tab);
    $('#selAll').textContent = cur.length && cur.every((i) => sel.has(i.key)) ? '取消全选' : '全选';
  };

  const paint = () => {
    const items = itemsOf(local.tab);
    if (!items.length) {
      grid.innerHTML = local.tab === 'local'
        ? emptyBox('还没有本地图片', '上传后保存到 photo 目录，或到设置里启用图床')
        : emptyBox('还没有图床图片', cloudOn ? '在编辑器或本页上传图片' : '先在设置里启用 Cloudinary');
      paintSel();
      return;
    }
    grid.innerHTML = items.map((item, i) => cellHTML(item, i)).join('');

    grid.querySelectorAll('.image-cell').forEach((cell) => {
      const idx = Number(cell.dataset.idx);
      const item = items[idx];
      cell.onclick = () => {
        if (local.selectMode) toggle(item, cell);
        else openViewer(items, idx);
      };
      const check = cell.querySelector('[data-check]');
      check.onclick = (e) => { e.stopPropagation(); toggle(item, cell); };
    });
    paintSel();
  };

  const toggle = (item, cell) => {
    if (!local.selectMode) local.selectMode = true;
    if (sel.has(item.key)) sel.delete(item.key);
    else sel.add(item.key);
    cell.classList.toggle('is-selected', sel.has(item.key));
    paintSel();
  };

  /* ---------- 灯箱 ---------- */

  function openViewer(items, start) {
    let idx = start;
    const host = document.createElement('div');
    host.className = 'img-viewer';
    host.innerHTML = `
      <div class="img-viewer-stage">
        <img class="img-viewer-img" alt="">
        <button class="img-viewer-nav prev" title="上一张（←）">${ui.icon('chevronLeft', 22)}</button>
        <button class="img-viewer-nav next" title="下一张（→）">${ui.icon('chevronRight', 22)}</button>
        <button class="img-viewer-close" title="关闭（Esc）">${ui.icon('close', 18)}</button>
      </div>
      <div class="img-viewer-bar">
        <div class="img-viewer-meta">
          <b></b><span class="dim"></span>
        </div>
        <div class="img-viewer-ops">
          <button class="btn btn-ghost btn-sm" data-v="copy">${ui.icon('link', 14)}<span>复制链接</span></button>
          <button class="btn btn-ghost btn-sm" data-v="md">${ui.icon('copy', 14)}<span>复制 Markdown</span></button>
          <button class="btn btn-ghost btn-sm" data-v="open">${ui.icon('external', 14)}<span>新窗口打开</span></button>
          <button class="btn btn-danger btn-sm" data-v="del">${ui.icon('trash', 14)}<span>删除</span></button>
        </div>
      </div>`;
    document.body.appendChild(host);
    document.body.style.overflow = 'hidden';

    const imgEl = host.querySelector('.img-viewer-img');
    const nameEl = host.querySelector('.img-viewer-meta b');
    const dimEl = host.querySelector('.img-viewer-meta .dim');
    const delBtn = host.querySelector('[data-v="del"]');

    const show = () => {
      const item = items[idx];
      imgEl.src = item.big;
      imgEl.alt = item.name;
      nameEl.textContent = item.name;
      const kind = item.kind === 'local' ? '本地' : '图床';
      dimEl.textContent = [kind, ui.fmtSize(item.size), ui.fmtDate(item.time)].filter(Boolean).join(' · ');
      host.querySelector('.prev').hidden = items.length < 2;
      host.querySelector('.next').hidden = items.length < 2;
      delBtn.querySelector('span').textContent = item.kind === 'local' ? '删除' : '移除记录';
    };
    show();

    const close = () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = '';
      host.remove();
    };
    const step = (d) => {
      if (items.length < 2) return;
      idx = (idx + d + items.length) % items.length;
      show();
    };
    const onKey = (e) => {
      if (e.key === 'Escape') close();
      else if (e.key === 'ArrowLeft') step(-1);
      else if (e.key === 'ArrowRight') step(1);
    };
    document.addEventListener('keydown', onKey);
    host.querySelector('.prev').onclick = (e) => { e.stopPropagation(); step(-1); };
    host.querySelector('.next').onclick = (e) => { e.stopPropagation(); step(1); };
    host.querySelector('.img-viewer-close').onclick = close;
    host.onclick = (e) => { if (e.target === host || e.target.classList.contains('img-viewer-stage')) close(); };

    host.querySelector('[data-v="copy"]').onclick = () => ui.copy(absolute(items[idx]), '图片直链已复制');
    host.querySelector('[data-v="md"]').onclick = () => ui.copy(markdownOf(items[idx]), 'Markdown 已复制');
    host.querySelector('[data-v="open"]').onclick = () => window.open(absolute(items[idx]), '_blank', 'noopener');
    delBtn.onclick = async () => {
      const item = items[idx];
      const removed = await removeItems([item]);
      if (!removed) return;
      if (items.length <= 1) return close();
      items.splice(idx, 1);
      if (idx >= items.length) idx = items.length - 1;
      show();
    };
  }

  /* ---------- 删除（单张 / 批量共用） ---------- */

  async function removeItems(items) {
    const localNames = items.filter((i) => i.kind === 'local').map((i) => i.name);
    const cloudIDs = items.filter((i) => i.kind === 'cloud').map((i) => i.id);
    const onlyCloud = localNames.length === 0;
    const ok = await ui.confirmDialog({
      title: items.length > 1 ? `删除选中的 ${items.length} 张图片？` : (onlyCloud ? '移除这条图床记录？' : '删除这张图片？'),
      desc: onlyCloud
        ? '只移除水鱼文集里的图床记录，Cloudinary 上的原图不会被删除。'
        : `photo 目录下的 ${localNames.length} 个文件将被永久删除。若文章仍引用它，图片会失效。`,
      okText: '删除', danger: true,
    });
    if (!ok) return false;

    let res;
    try {
      res = await api.deleteImages({ local: localNames, cloud: cloudIDs });
    } catch (e) {
      ui.toast('删除失败', e.message, 'err', 6000);
      return false;
    }
    const names = new Set(localNames);
    const ids = new Set(cloudIDs);
    data.local = data.local.filter((x) => !names.has(x.name));
    data.cloud = data.cloud.filter((c, i) => !ids.has(c.publicId || c.url || String(i)));
    items.forEach((i) => sel.delete(i.key));
    sub.textContent = `本地 ${data.local.length} 张 · 图床 ${data.cloud.length} 张`;
    paint();
    ctx.refreshState();
    if ((res.failed || []).length) {
      ui.toast('部分图片未删除', res.failed.slice(0, 3).join('、'), 'warn', 6000);
    } else {
      ui.toast('已删除', `${res.deletedLocal || 0} 张本地 · ${res.deletedCloud || 0} 条图床记录`, 'ok');
    }
    return true;
  }

  paint();

  /* ---------- 顶部与批量操作 ---------- */

  $('#refresh').onclick = () => ctx.refresh();
  $('#selectToggle').onclick = () => {
    local.selectMode = !local.selectMode;
    if (!local.selectMode) sel.clear();
    paint();
  };
  $('#selCancel').onclick = () => { local.selectMode = false; sel.clear(); paint(); };
  $('#selAll').onclick = () => {
    const items = itemsOf(local.tab);
    if (items.every((i) => sel.has(i.key))) sel.clear();
    else items.forEach((i) => sel.add(i.key));
    paint();
  };
  $('#copyLinks').onclick = () => {
    const picked = itemsOf(local.tab).filter((i) => sel.has(i.key));
    if (!picked.length) return;
    ui.copy(picked.map(absolute).join('\n'), `已复制 ${picked.length} 条链接`);
  };
  $('#copyMd').onclick = () => {
    const picked = itemsOf(local.tab).filter((i) => sel.has(i.key));
    if (!picked.length) return;
    ui.copy(picked.map(markdownOf).join('\n'), `已复制 ${picked.length} 条 Markdown`);
  };
  $('#selDel').onclick = () => {
    const picked = itemsOf(local.tab).filter((i) => sel.has(i.key));
    if (picked.length) removeItems(picked);
  };

  $('#uploadBtn').onclick = () => $('#pick').click();
  $('#pick').onchange = async (e) => {
    const files = [...e.target.files];
    e.target.value = '';
    if (!files.length) return;
    const btn = $('#uploadBtn');
    btn.disabled = true;
    const old = btn.innerHTML;
    let ok = 0, dup = 0;
    for (let i = 0; i < files.length; i++) {
      btn.innerHTML = `<span class="spinner"></span><span>上传中 ${i + 1}/${files.length}</span>`;
      try { const r = await api.upload(files[i]); ok++; if (r.duplicate) dup++; } catch (err) {
        ui.toast('上传失败', `${files[i].name}：${err.message}`, 'err', 7000);
      }
    }
    btn.disabled = false;
    btn.innerHTML = old;
    if (ok) ui.toast(`已上传 ${ok} 张`, '', 'ok');
    if (dup) ui.toast(`${dup} 张图片已存在`, '已复用已有图片', 'ok');
    local.tab = data.cloudEnabled ? 'cloud' : 'local';
    ctx.refresh();
  };
}

function cellHTML(item, idx) {
  const badge = item.kind === 'local' ? '本地' : '图床';
  return `
    <div class="image-cell ${item.kind === 'cloud' ? 'is-cloud' : ''}" data-idx="${idx}" data-kind="${item.kind}">
      <span class="image-badge ${item.kind === 'cloud' ? 'cloud' : ''}">${badge}</span>
      <span class="image-check" data-check title="选择"><span class="image-check-box">${ui.icon('check', 11)}</span></span>
      <img src="${ui.esc(item.src)}" alt="${ui.esc(item.name)}" loading="lazy" decoding="async">
      <div class="veil">
        <div class="name" title="${ui.esc(item.name)}">${ui.esc(item.name)}</div>
        <div class="row"><button type="button" data-preview>${ui.icon('eye', 12)}<span>预览</span></button></div>
      </div>
    </div>`;
}

function emptyBox(title, desc) {
  return `<div class="empty" style="grid-column:1/-1">${ui.icon('image', 56)}<h3>${ui.esc(title)}</h3><p>${ui.esc(desc)}</p></div>`;
}
