/* 设置：账号 / 外观 / 管理员设置（图床 + 存储，仅管理员） / 关于 */

import { api } from '../api.js';
import * as ui from '../ui.js';

const TUTORIAL_URL = 'https://blog.waterfish.ren/article/4023794e-eb27-8082-a989-db0ca64a70c9';
const MAX_THEME_JS = 64 * 1024;

// 内置主题：每套打包配色 / 字体 / 字号 / 背景（与 app.js 的 THEMES 表一致）
const THEMES = [
  { id: 'default', name: '纸墨',   color: '#1F6C9F', font: '系统字体', scale: '100%', bg: '纯色' },
  { id: 'amber',   name: '琥珀',   color: '#B45309', font: '系统字体', scale: '100%', bg: '淡彩' },
  { id: 'forest',  name: '松绿',   color: '#2F6B4F', font: '衬线字体', scale: '105%', bg: '淡彩' },
  { id: 'violet',  name: '紫罗兰', color: '#6D5BD0', font: '圆体',     scale: '100%', bg: '淡彩' },
  { id: 'rose',    name: '玫红',   color: '#BE4B6B', font: '系统字体', scale: '100%', bg: '淡彩' },
];

function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(1) + ' MB';
}

// 主题重名时加 (2)、(3)… 后缀（与后端去重规则一致）
function uniqueThemeName(name, taken) {
  let n = name;
  for (let i = 2; taken.has(n); i++) n = `${name}(${i})`;
  return n;
}

export async function render(el, ctx) {
  const st = ctx.state || await ctx.refreshState();
  const isAdmin = !!st.user?.isAdmin || st.user?.source === 'local';
  const ap = st.appearance || {};

  el.innerHTML = `
    <div class="page-head">
      <div>
        <h1 class="page-title">设置</h1>
      </div>
    </div>

    <div class="subnav" id="subnav">
      <button class="subnav-item is-active" data-tab="account">账号</button>
      <button class="subnav-item" data-tab="appearance">外观</button>
      ${isAdmin ? '<button class="subnav-item" data-tab="admin">管理员设置</button>' : ''}
      <button class="subnav-item" data-tab="updates">更新与实验</button>
      <button class="subnav-item" data-tab="about">关于</button>
    </div>

    <!-- 账号 -->
    <div class="card card-pad section" data-panel="account">
      <dl class="kv">
        <dt>登录账号</dt>
        <dd style="display:flex;align-items:center;gap:8px">
          <b>${ui.esc(st.user?.name || '未知')}</b>
          ${isAdmin ? '<span class="tag tag-ok">管理员</span>' : '<span class="tag">成员</span>'}
          ${st.user?.source === 'local' ? '<span class="tag tag-warn">局域网直连</span>' : ''}
        </dd>
        <dt>内容</dt>
        <dd>${st.stats?.articles ?? 0} 篇文章 · ${st.stats?.localImages ?? 0} 张本地图片 · ${st.stats?.seoRecords ?? 0} 条 SEO 记录</dd>
      </dl>
    </div>

    <!-- 外观 -->
    <div data-panel="appearance" hidden>
      <div class="card card-pad section">
        <div class="ap-block">
          <div class="ap-label">主题模式</div>
          <div class="tabs" id="apMode">
            <button class="tab" data-mode="auto">跟随系统</button>
            <button class="tab" data-mode="light">浅色</button>
            <button class="tab" data-mode="dark">深色</button>
          </div>
        </div>

        <div style="margin-top:22px">
          <div class="ap-label">主题</div>
          <div class="theme-grid" id="apThemes"></div>
        </div>

        <div style="margin-top:22px">
          <div class="ap-label">自定义主题</div>
          <div class="ap-upload" id="apUpload"></div>
        </div>

        <div style="margin-top:18px">
          <div class="ap-label">预览</div>
          <div class="ap-preview">
            正文示例：<b>粗体文字</b>、<span class="ap-link">链接样式</span>、<code style="font-family:var(--mono);font-size:12px;background:var(--surface-3);padding:.12em .4em;border-radius:4px">code</code>
          </div>
        </div>
      </div>
    </div>

    <!-- 管理员设置：图床 + 存储路径（仅管理员可见） -->
    ${isAdmin ? `
    <div data-panel="admin" hidden>
      <div class="card card-pad section">
        <div class="section-title">图床 · Cloudinary</div>

        <div class="field">
          <label>启用图床</label>
          <label style="display:flex;align-items:center;gap:10px;cursor:pointer;font-weight:400">
            <input type="checkbox" id="cloudOn" ${st.cloudinary.enabled ? 'checked' : ''} style="width:16px;height:16px;accent-color:var(--accent)">
            <span id="cloudStateText">${st.cloudinary.enabled ? '已启用' : '未启用'}</span>
          </label>
        </div>

        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:0 18px">
          <div class="field">
            <label for="cloudName">Cloud name</label>
            <input class="input" id="cloudName" placeholder="如 dxxxxxx" value="${ui.esc(st.cloudinary.cloudName || '')}" spellcheck="false">
          </div>
          <div class="field">
            <label for="preset">Upload preset</label>
            <input class="input" id="preset" placeholder="如 waterfish_unsigned" value="${ui.esc(st.cloudinary.uploadPreset || '')}" spellcheck="false">
          </div>
        </div>

        <div class="field" style="margin-bottom:18px">
          <a class="tut-link" href="${TUTORIAL_URL}" target="_blank" rel="noopener noreferrer">${ui.icon('external', 14)}<span>获取教程：Cloudinary 上传凭证怎么拿</span></a>
        </div>

        <button class="btn btn-primary" id="saveCloud">${ui.icon('save')}<span>保存图床设置</span></button>
      </div>

      <div class="card card-pad section">
        <div class="section-title">本地存储路径</div>

        <dl class="kv" style="margin-bottom:16px">
          <dt>当前路径</dt>
          <dd><code style="font-family:var(--mono);font-size:12.5px">${ui.esc(st.storageRoot)}</code></dd>
          ${st.storageBase ? `<dt>共享存储路径</dt>
          <dd><code style="font-family:var(--mono);font-size:12.5px">${ui.esc(st.storageBase)}</code></dd>` : ''}
          <dt>应用数据目录</dt>
          <dd><code style="font-family:var(--mono);font-size:12.5px">${ui.esc(st.defaultStorageRoot)}</code></dd>
        </dl>

        <div class="field">
          <label>可见目录</label>
          <div class="dirlist" style="max-height:190px">
            ${(st.roots || []).map((r) => `
              <div class="dir-row" style="cursor:default">
                ${ui.icon('folder', 16)}
                <span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;font-family:var(--mono);font-size:12px">${ui.esc(r.path)}</span>
                <span class="tag ${r.writable ? 'tag-ok' : 'tag-warn'}">${r.writable ? '可写' : '只读'}</span>
                <span class="tag">${ui.esc(r.label)}</span>
                <span class="tag">${ui.fmtSize(r.free)} 可用</span>
              </div>`).join('') || '<div class="dir-row" style="cursor:default;color:var(--ink-3)">暂无可见目录</div>'}
          </div>
          <span class="hint">新增授权目录后，重启应用生效。</span>
        </div>

        <div style="display:flex;gap:10px;flex-wrap:wrap">
          <button class="btn btn-primary" id="changeStorage">${ui.icon('drive')}<span>更改存储路径</span></button>
          <button class="btn btn-ghost" id="resetStorage">${ui.icon('refresh')}<span>恢复为应用数据目录</span></button>
          <button class="btn btn-ghost" id="reload">${ui.icon('refresh')}<span>刷新页面数据</span></button>
        </div>
      </div>
    </div>` : ''}

    <!-- 更新与实验 -->
    <div class="card card-pad section" data-panel="updates" hidden>
      <div class="section-title">检测更新</div>
      <p class="section-desc">当前版本 v${ui.esc(st.version)}</p>
      <div class="section-title" style="margin-top:24px">实验性功能</div>
      <p class="section-desc">暂无开放的实验功能</p>
    </div>

    <!-- 关于 -->
    <div class="card card-pad section" data-panel="about" hidden>
      <div class="section-title">关于</div>
      <dl class="kv">
        <dt>应用名称</dt><dd>水鱼文集</dd>
        <dt>当前版本</dt><dd>v${ui.esc(st.version)}</dd>
      </dl>
    </div>
  `;

  const $ = (s) => el.querySelector(s);
  const cur = () => ctx.state?.appearance || {};

  /* ---------- 主题卡片 + 自定义主题上传区（选择 / 上传 / 删除后原地更新） ---------- */
  const paintThemes = () => {
    const cfg = cur();
    const active = cfg.theme || 'default';
    const custom = cfg.themes || [];

    const grid = $('#apThemes');
    grid.innerHTML = [
      ...THEMES.map((t) => themeCard(t, active === t.id)),
      ...custom.map((t) => themeCard(t, active === t.name, true)),
    ].join('');

    grid.querySelectorAll('[data-theme]').forEach((n) => {
      n.onclick = async (e) => {
        // 自定义主题卡片上的删除按钮不触发选中
        if (e.target.closest('[data-del-theme]')) return;
        const id = n.dataset.theme;
        if (id === cur().theme) return;
        try {
          await ctx.saveAppearance({ theme: id });
          paintThemes();
        } catch { /* 错误提示已由 saveAppearance 给出 */ }
      };
    });
    grid.querySelectorAll('[data-del-theme]').forEach((n) => {
      n.onclick = async (e) => {
        e.stopPropagation();
        const name = n.dataset.delTheme;
        const themes = (cur().themes || []).filter((t) => t.name !== name);
        const theme = cur().theme === name ? 'default' : cur().theme;
        try {
          await ctx.saveAppearance({ theme, themes });
          paintThemes();
          paintUploadBox();
          ui.toast('已删除主题', name, 'ok', 2500);
        } catch { /* 错误提示已由 saveAppearance 给出 */ }
      };
    });

    paintUploadBox();
  };

  function themeCard(t, active, isCustom = false) {
    const meta = isCustom ? fmtBytes((t.js || '').length) + ' · JS 主题' : `${t.font} · ${t.scale} · ${t.bg}`;
    return `
      <button class="theme-card ${active ? 'is-active' : ''}" data-theme="${ui.esc(isCustom ? t.name : t.id)}" title="${ui.esc(t.name)}">
        <span class="theme-preview">
          <span class="tp-bar" style="background:${isCustom ? 'var(--ink-3)' : ui.esc(t.color)}"></span>
          <span class="tp-line"></span>
          <span class="tp-line short"></span>
        </span>
        <span class="theme-name">${ui.esc(t.name)}</span>
        <span class="theme-meta">${ui.esc(meta)}</span>
        ${isCustom ? `<span class="theme-del" data-del-theme="${ui.esc(t.name)}" title="删除主题">${ui.icon('close', 12)}</span>` : ''}
      </button>`;
  }

  const paintUploadBox = () => {
    const box = $('#apUpload');
    if (!box) return;
    const custom = cur().themes || [];
    box.innerHTML = `
      <input type="file" id="apThemeFile" accept=".js,text/javascript,application/javascript">
      <label for="apThemeFile" style="cursor:pointer;display:flex;flex-direction:column;align-items:center;gap:6px;color:var(--ink-3)">
        ${ui.icon('upload', 22)}
        <span style="font-size:13px">选择 .js 主题文件</span>
      </label>
      <div class="ap-upload-meta">${custom.length
        ? `已安装 ${custom.length} 个自定义主题，上传后以文件名命名，重名自动加 (2) 后缀`
        : '上传后以文件名生成主题，上限 64 KB'}</div>`;
    bindUploadBox();
  };

  function bindUploadBox() {
    const input = $('#apThemeFile');
    input.onchange = async () => {
      const file = input.files && input.files[0];
      if (!file) return;
      if (!/\.js$/i.test(file.name)) {
        ui.toast('请选择 .js 文件', '', 'err', 4000);
        input.value = '';
        return;
      }
      if (file.size > MAX_THEME_JS) {
        ui.toast('主题文件过大', '上限 64 KB，请精简后重试', 'err', 5000);
        input.value = '';
        return;
      }
      try {
        const text = await file.text();
        if (!text.trim()) {
          ui.toast('主题文件为空', '请选择有效的 .js 文件', 'err', 4000);
          return;
        }
        const taken = new Set([...THEMES.map((t) => t.name), ...(cur().themes || []).map((t) => t.name)]);
        const stem = file.name.replace(/\.js$/i, '').trim();
        const name = uniqueThemeName(stem || '自定义主题', taken);
        const themes = [...(cur().themes || []), { name, js: text }];
        await ctx.saveAppearance({ theme: name, themes });
        paintThemes();
        ui.toast('主题已添加', name, 'ok', 3000);
      } catch { /* 错误提示已由 saveAppearance 给出 */ }
      finally { input.value = ''; }
    };
  }
  paintThemes();

  /* ---------- 子菜单切换 ---------- */
  const tabs = [...el.querySelectorAll('.subnav-item')];
  const panels = [...el.querySelectorAll('[data-panel]')];
  tabs.forEach((t) => {
    t.onclick = () => {
      tabs.forEach((x) => x.classList.toggle('is-active', x === t));
      panels.forEach((p) => { p.hidden = p.dataset.panel !== t.dataset.tab; });
    };
  });

  /* ---------- 图床 ---------- */
  $('#cloudOn').onchange = (e) => { $('#cloudStateText').textContent = e.target.checked ? '已启用' : '未启用'; };
  $('#saveCloud').onclick = async () => {
    const body = {
      cloudinary: {
        enabled: $('#cloudOn').checked,
        cloudName: $('#cloudName').value.trim(),
        uploadPreset: $('#preset').value.trim(),
      },
    };
    try {
      await api.saveSettings(body);
      ui.toast('已保存', '图床设置已更新', 'ok');
      ctx.refreshState();
    } catch (e) { ui.toast('保存失败', e.message, 'err', 6000); }
  };

  /* ---------- 外观：主题模式 ---------- */
  const modeTabs = $('#apMode');
  modeTabs.querySelectorAll('.tab').forEach((t) => {
    t.classList.toggle('is-active', (ap.mode || 'auto') === t.dataset.mode);
    t.onclick = async () => {
      modeTabs.querySelectorAll('.tab').forEach((x) => x.classList.toggle('is-active', x === t));
      await ctx.setTheme(t.dataset.mode);
    };
  });

  /* ---------- 存储路径 ---------- */
  const changeBtn = $('#changeStorage');
  changeBtn && (changeBtn.onclick = () => openBrowser(ctx));
  const resetBtn = $('#resetStorage');
  resetBtn && (resetBtn.onclick = async () => {
    const ok = await ui.confirmDialog({
      title: '恢复到应用数据目录？',
      desc: '当前目录下的文章、图片与 SEO 记录会被移动到应用数据目录，原目录内本应用的文件将被清空。',
      okText: '移动并切换',
    });
    if (!ok) return;
    try {
      const r = await api.setStorage('');
      ui.toast('已切换存储路径', r.storageRoot, 'ok', 6000);
      ctx.refresh();
    } catch (e) { ui.toast('切换失败', e.message, 'err', 7000); }
  });
  const reloadBtn = $('#reload');
  reloadBtn && (reloadBtn.onclick = () => ctx.refresh());
}

/* ---------------- 目录浏览器 ---------------- */

function openBrowser(ctx) {
  let current = '';

  const close = ui.modal(`
    <div class="modal-head">
      <h3>选择存储文件夹</h3>
      <p>只能选择已授权范围内的目录。</p>
    </div>
    <div class="modal-body">
      <div class="dirlist" id="dirlist" style="max-height:340px"></div>
    </div>
    <div class="modal-foot">
      <button class="btn btn-ghost" data-act="cancel">取消</button>
      <button class="btn btn-primary" data-act="pick" disabled>选择此文件夹</button>
    </div>`, {
    wide: true,
    onMount(root) {
      const listEl = root.querySelector('#dirlist');
      const pickBtn = root.querySelector('[data-act="pick"]');

      const load = async (path) => {
        listEl.innerHTML = `<div class="dir-row" style="cursor:default"><span class="spinner"></span><span>读取中…</span></div>`;
        let data;
        try {
          data = await api.browse(path);
        } catch (e) {
          listEl.innerHTML = `<div class="dir-row" style="cursor:default;color:var(--err)">${ui.esc(e.message)}</div>`;
          return;
        }
        current = data.path;
        pickBtn.disabled = !data.path || !data.writable;
        const rows = [];
        if (data.path) {
          rows.push(`<div class="dir-path">${ui.esc(data.path)}</div>`);
          if (data.parent) rows.push(`<div class="dir-row" data-go="${ui.esc(data.parent)}">${ui.icon('chevronLeft', 16)}<span>返回上一级</span></div>`);
        }
        for (const d of data.dirs) {
          rows.push(`<div class="dir-row" data-go="${ui.esc(d.path)}">${ui.icon('folder', 16)}<span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis">${ui.esc(d.name)}</span>${ui.icon('chevronRight', 14)}</div>`);
        }
        if (!data.dirs.length && data.path) {
          rows.push(`<div class="dir-row" style="cursor:default;color:var(--ink-3)">${ui.icon('folder', 16)}<span>该目录下没有子文件夹</span></div>`);
        }
        listEl.innerHTML = rows.join('');
        listEl.querySelectorAll('[data-go]').forEach((n) => { n.onclick = () => load(n.dataset.go); });
      };

      pickBtn.onclick = async () => {
        if (!current) return;
        const ok = await ui.confirmDialog({
          title: '切换到该路径？',
          desc: `文章、图片与 SEO 记录将移动到 ${current} 下你的专属文件夹。`,
          okText: '移动并切换',
        });
        if (!ok) return;
        pickBtn.disabled = true;
        pickBtn.innerHTML = `<span class="spinner"></span><span>搬迁中…</span>`;
        try {
          const r = await api.setStorage(current);
          ui.toast('已切换存储路径', r.storageRoot, 'ok', 6000);
          close();
          ctx.refresh();
        } catch (e) {
          ui.toast('切换失败', e.message, 'err', 8000);
          pickBtn.disabled = false;
          pickBtn.innerHTML = '选择此文件夹';
        }
      };

      root.querySelector('[data-act="cancel"]').onclick = () => close();
      load('');
    },
  });
}
