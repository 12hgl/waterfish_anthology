/* 外观与账号：用户卡片内置在侧栏底部，点开是外观抽屉（全局可用）。
   主题是 .js 文件，简单（一行一条声明）或复杂（function theme(api)）两种写法。
   保存模型：改动即时保存，失败回滚到上次保存值。 */

import { api } from './api.js';
import * as ui from './ui.js';

const MAX_THEME_JS = 64 * 1024;
const MAX_CUSTOM_THEMES = 20;

// 内置主题：每套打包配色 / 字体 / 字号 / 背景（与后端内置主题一致）
const THEMES = [
  { id: 'default', name: '纸墨',   color: '#1F6C9F', font: '系统字体', scale: '100%', bg: '纯色' },
  { id: 'amber',   name: '琥珀',   color: '#B45309', font: '系统字体', scale: '100%', bg: '淡彩' },
  { id: 'forest',  name: '松绿',   color: '#2F6B4F', font: '衬线字体', scale: '105%', bg: '淡彩' },
  { id: 'violet',  name: '紫罗兰', color: '#6D5BD0', font: '圆体',     scale: '100%', bg: '淡彩' },
  { id: 'rose',    name: '玫红',   color: '#BE4B6B', font: '系统字体', scale: '100%', bg: '淡彩' },
];

const SAMPLE_SIMPLE = `// 简单主题：一行写一项，只写想改的，其余沿用内置外观
主色: #1F6C9F
副色: #B45309
背景: https://example.com/bg.jpg
字体: 宋体
`;

const SAMPLE_COMPLEX = `// 复杂主题：定义 function theme(api)，可调界面、动画、图标
function theme(api) {
  // 调变量（CSS 变量）
  api.setVar('--accent', '#7C5CFF');
  api.setVar('--r', '14px');

  // 注入样式：界面微调、动画都写在这里
  api.css(\`
    .card { transition: transform .25s ease, box-shadow .25s ease; }
    .card:hover { transform: translateY(-2px); }
    .page-title { letter-spacing: .02em; }
    .theme-card { animation: apFadeUp .3s ease both; }
    @keyframes apFadeUp { from { opacity: 0; transform: translateY(6px); } }
  \`);

  // 换图标（可选）：用 SVG path 覆盖内置图标，颜色跟随 currentColor
  api.icon('rocket', '<path d="M12 3l7 7-9 11H4v-6z"/><circle cx="14.5" cy="8.5" r="1.4" fill="currentColor" stroke="none"/>');
}
`;

const clone = (v) => JSON.parse(JSON.stringify(v ?? null));

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

function downloadSample(name, text) {
  const blob = new Blob([text], { type: 'text/javascript;charset=utf-8' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 4000);
}

function themeCard(t, active, isCustom = false) {
  const meta = isCustom
    ? fmtBytes(new Blob([t.js || '']).size) + ' · ' + (/function\s+theme\s*\(/.test(t.js || '') ? '复杂' : '简单')
    : `${t.font} · ${t.scale} · ${t.bg}`;
  return `
    <button class="theme-card ${active ? 'is-active' : ''}" data-theme="${ui.esc(isCustom ? t.name : t.id)}" title="${ui.esc(t.name)}">
      <span class="theme-preview">
        <span class="tp-bar" style="background:${isCustom ? 'var(--ink-3)' : ui.esc(t.color)}"></span>
        <span class="tp-line"></span>
        <span class="tp-line short"></span>
      </span>
      <span class="theme-name">${ui.esc(t.name)}</span>
      <span class="theme-meta">${ui.esc(meta)}</span>
      ${isCustom ? `<span class="theme-del" data-del-theme="${ui.esc(t.name)}" title="移除主题">${ui.icon('close', 12)}</span>` : ''}
    </button>`;
}

// 初始化：填充侧栏用户卡片并挂好抽屉；返回打开抽屉的函数供需要处复用
export function initAppearance(ctx) {
  const chip = document.getElementById('userChip');
  const dockHost = document.getElementById('dockHost');

  const paintUser = () => {
    const st = ctx.state || {};
    const userName = st.user?.name || '未知';
    const avatar = document.getElementById('userAvatar');
    avatar.textContent = [...userName][0] || '?';
    document.getElementById('userName').textContent = userName;
    document.getElementById('userPalette').innerHTML = ui.icon('palette', 15);
  };

  dockHost.innerHTML = `
    <aside class="ap-drawer" id="apDrawer" aria-label="外观设置">
      <div class="ap-drawer-head">
        <div class="ap-drawer-user" id="apDrawerUser"></div>
        <button class="icon-btn ap-drawer-close" id="apClose" title="关闭">${ui.icon('close', 16)}</button>
      </div>
      <div class="ap-drawer-body">
        <div class="ap-block">
          <div class="ap-label">主题模式</div>
          <div class="tabs" id="apMode">
            <button class="tab" data-mode="auto">跟随系统</button>
            <button class="tab" data-mode="light">浅色</button>
            <button class="tab" data-mode="dark">深色</button>
          </div>
        </div>

        <div style="margin-top:20px">
          <div class="ap-label">主题</div>
          <div class="theme-grid" id="apThemes"></div>
        </div>

        <div style="margin-top:20px">
          <div class="ap-label">自定义主题</div>
          <div class="ap-upload" id="apUpload"></div>
          <div class="ap-hint" style="margin-top:10px">
            <p style="margin:0 0 8px">主题是 .js 文件，两种写法：<b>简单</b>一行一条只调外观；<b>复杂</b>定义 function theme(api)，可调界面、动画、图标。</p>
            <pre class="ap-spec">主色: #1F6C9F
副色: #B45309
背景: https://example.com/bg.jpg
字体: 宋体</pre>
            <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap">
              <button class="btn btn-ghost btn-sm" id="apSampleSimple">${ui.icon('download', 13)}<span>简单示例</span></button>
              <button class="btn btn-ghost btn-sm" id="apSampleComplex">${ui.icon('download', 13)}<span>复杂示例</span></button>
            </div>
          </div>
        </div>

        <div style="margin-top:20px">
          <div class="ap-label">预览</div>
          <div class="ap-preview">
            正文示例：<b>粗体文字</b>、<span class="ap-link">链接样式</span>、<code style="font-family:var(--mono);font-size:12px;background:var(--surface-3);padding:.12em .4em;border-radius:4px">code</code>
          </div>
        </div>
      </div>
    </aside>`;

  const drawer = dockHost.querySelector('#apDrawer');
  let appearance = clone(ctx.state?.appearance || {});

  paintUser();
  paintDrawerUser();

  function paintDrawerUser() {
    const st = ctx.state || {};
    const userName = st.user?.name || '未知';
    const isAdmin = !!st.user?.isAdmin || st.user?.source === 'local';
    const avatar = [...userName][0] || '?';
    document.getElementById('apDrawerUser').innerHTML = `
      <span class="avatar">${ui.esc(avatar)}</span>
      <div>
        <b>${ui.esc(userName)}</b>
        <span>${isAdmin ? '管理员' : '成员'} · ${st.stats?.articles ?? 0} 篇文章 · ${st.stats?.localImages ?? 0} 张图片</span>
      </div>`;
  }

  // 改动即保存：成功后以服务端清洗结果为准，失败则回滚到上次保存值
  const commitAppearance = async () => {
    try {
      const applied = await ctx.saveAppearance(clone(appearance));
      appearance = clone(applied);
    } catch { appearance = clone(ctx.state?.appearance || {}); }
    paintThemes();
    paintMode();
  };

  const paintThemes = () => {
    const active = appearance.theme || 'default';
    const custom = appearance.themes || [];
    const grid = dockHost.querySelector('#apThemes');
    grid.innerHTML = [
      ...THEMES.map((t) => themeCard(t, active === t.id)),
      ...custom.map((t) => themeCard(t, active === t.name, true)),
    ].join('');

    grid.querySelectorAll('[data-theme]').forEach((n) => {
      n.onclick = (e) => {
        if (e.target.closest('[data-del-theme]')) return;
        const id = n.dataset.theme;
        if (id === (appearance.theme || 'default')) return;
        appearance.theme = id;
        commitAppearance();
      };
    });
    grid.querySelectorAll('[data-del-theme]').forEach((n) => {
      n.onclick = (e) => {
        e.stopPropagation();
        const name = n.dataset.delTheme;
        appearance.themes = (appearance.themes || []).filter((t) => t.name !== name);
        if (appearance.theme === name) appearance.theme = 'default';
        commitAppearance();
        ui.toast('已移除主题', name, 'info', 2400);
      };
    });

    paintUploadBox();
  };

  function paintUploadBox() {
    const box = dockHost.querySelector('#apUpload');
    const custom = appearance.themes || [];
    box.innerHTML = `
      <input type="file" id="apThemeFile" accept=".js,text/javascript,application/javascript">
      <label for="apThemeFile" style="cursor:pointer;display:flex;flex-direction:column;align-items:center;gap:6px;color:var(--ink-3)">
        ${ui.icon('upload', 22)}
        <span style="font-size:13px">选择 .js 主题文件</span>
      </label>
      <div class="ap-upload-meta">${custom.length
        ? `已安装 ${custom.length}/${MAX_CUSTOM_THEMES} 个自定义主题，重名自动加 (2) 后缀`
        : `上传后以文件名生成主题，单文件截断至 64 KB，最多 ${MAX_CUSTOM_THEMES} 个`}</div>`;
    const input = box.querySelector('#apThemeFile');
    input.onchange = async () => {
      const file = input.files && input.files[0];
      if (!file) return;
      if (!/\.js$/i.test(file.name)) {
        ui.toast('请选择 .js 文件', '', 'err', 4000);
        input.value = '';
        return;
      }
      const installed = appearance.themes || [];
      if (installed.length >= MAX_CUSTOM_THEMES) {
        ui.toast('自定义主题已达上限', `最多 ${MAX_CUSTOM_THEMES} 个，请先移除不再使用的主题`, 'err', 5000);
        input.value = '';
        return;
      }
      const truncated = file.size > MAX_THEME_JS;
      try {
        const text = await file.slice(0, MAX_THEME_JS).text();
        if (!text.trim()) {
          ui.toast('主题文件为空', '请选择有效的 .js 文件', 'err', 4000);
          return;
        }
        const taken = new Set([
          ...THEMES.flatMap((t) => [t.id, t.name]),
          ...installed.map((t) => t.name),
        ]);
        const stem = file.name.replace(/\.js$/i, '').trim();
        const name = uniqueThemeName(stem || '自定义主题', taken);
        appearance.themes = [...installed, { name, js: text }];
        appearance.theme = name;
        await commitAppearance();
        ui.toast(truncated ? '主题已添加并截断至 64 KB' : '主题已添加', name, 'ok', 3200);
      } finally { input.value = ''; }
    };
  }

  const paintMode = () => {
    const mode = appearance.mode || 'auto';
    dockHost.querySelectorAll('#apMode .tab').forEach((t) => {
      t.classList.toggle('is-active', mode === t.dataset.mode);
    });
  };

  dockHost.querySelectorAll('#apMode .tab').forEach((t) => {
    t.onclick = () => {
      if ((appearance.mode || 'auto') === t.dataset.mode) return;
      appearance.mode = t.dataset.mode;
      commitAppearance();
    };
  });

  dockHost.querySelector('#apSampleSimple').onclick = () => downloadSample('my-theme.js', SAMPLE_SIMPLE);
  dockHost.querySelector('#apSampleComplex').onclick = () => downloadSample('fancy-theme.js', SAMPLE_COMPLEX);

  paintThemes();
  paintMode();

  /* ---------- 抽屉开合 ---------- */
  const setDrawer = (open) => drawer.classList.toggle('open', open);
  chip.onclick = () => {
    // 打开前刷新一次账号统计（文章 / 图片数会变）
    ctx.refreshState().then(() => { appearance = clone(ctx.state?.appearance || {}); paintDrawerUser(); paintThemes(); paintMode(); }).catch(() => {});
    setDrawer(!drawer.classList.contains('open'));
  };
  dockHost.querySelector('#apClose').onclick = () => setDrawer(false);
  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    if (drawer.classList.contains('open') && !document.getElementById('modalHost').classList.contains('open')) {
      setDrawer(false);
    }
  });

  return setDrawer;
}
