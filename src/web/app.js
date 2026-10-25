/* 水鱼文集 —— 应用主入口 */

import { api } from './api.js';
import * as ui from './ui.js';
import { setupMarkdown } from './markdown.js';
import * as articles from './views/articles.js';
import * as images from './views/images.js';
import * as seo from './views/seo.js';
import * as settings from './views/settings.js';

const VIEWS = { articles, images, seo, settings };
const TITLES = { articles: '文章库', images: '图片库', seo: 'SEO 检测', settings: '设置' };

/* ---------------- 外观 ---------------- */

// 内置主题：每套主题打包配色 / 字体 / 字号 / 背景，选一套即整体生效
// （与 settings.js 的 THEMES 表保持一致）
const THEMES = [
  { id: 'default', name: '纸墨',   color: '#1F6C9F', font: 'system', scale: 100, bg: 'plain' },
  { id: 'amber',   name: '琥珀',   color: '#B45309', font: 'system', scale: 100, bg: 'tint' },
  { id: 'forest',  name: '松绿',   color: '#2F6B4F', font: 'serif',  scale: 105, bg: 'tint' },
  { id: 'violet',  name: '紫罗兰', color: '#6D5BD0', font: 'round',  scale: 100, bg: 'tint' },
  { id: 'rose',    name: '玫红',   color: '#BE4B6B', font: 'system', scale: 100, bg: 'tint' },
];

// 字体方案（空 = 系统默认）
const FONT_STACKS = {
  system: '',
  serif: '"Newsreader", "Noto Serif SC", "Source Han Serif SC", "Songti SC", "SimSun", Georgia, serif',
  round: '"Yuanti SC", "Hiragino Maru Gothic ProN", "YouYuan", "PingFang SC", "Microsoft YaHei", sans-serif',
  mono: 'ui-monospace, "SF Mono", "JetBrains Mono", Menlo, Consolas, "Noto Sans Mono CJK SC", monospace',
};

// 当前生效的内置主题；自定义主题（theme 不在内置表）时以「纸墨」为基底，
// 具体样式由上传的 JS 脚本覆盖，保证同一主题在任何时候表现一致。
function themeOf(ap) {
  const id = ap && ap.theme;
  return THEMES.find((t) => t.id === id) || THEMES[0];
}

function hexToRgb(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || '');
  if (!m) return null;
  const n = parseInt(m[1], 16);
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

// mix：把 a 向 b 混合 t 比例（0~1），返回 #RRGGBB
function mix(a, b, t) {
  const ca = hexToRgb(a), cb = hexToRgb(b);
  if (!ca || !cb) return a;
  const f = (x, y) => Math.round(x + (y - x) * t);
  const h = (v) => v.toString(16).padStart(2, '0');
  return '#' + h(f(ca.r, cb.r)) + h(f(ca.g, cb.g)) + h(f(ca.b, cb.b));
}

function isDarkNow() {
  const t = document.documentElement.dataset.theme;
  if (t === 'dark') return true;
  if (t === 'light') return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

// applyAppearance 把外观配置应用到页面（深浅色 / 内置主题打包 / 背景）。
// 自定义主题脚本由 loadCustomTheme 单独加载执行，覆盖在内置外观之上。
function applyAppearance(ap) {
  ap = ap || {};
  const root = document.documentElement;
  const st = root.style;
  const shell = document.getElementById('shell');

  // 深浅色模式
  if (ap.mode === 'light' || ap.mode === 'dark') {
    root.dataset.theme = ap.mode;
    localStorage.setItem('wf-theme', ap.mode);
  } else {
    delete root.dataset.theme;
    localStorage.removeItem('wf-theme');
  }

  // 当前主题（内置主题整体生效：配色 / 字体 / 字号 / 背景一次到位）
  const th = themeOf(ap);
  if (th.id !== 'default') root.dataset.preset = th.id;
  else delete root.dataset.preset;

  // 字体
  document.body.style.fontFamily = FONT_STACKS[th.font] || '';

  // 字号缩放（作用于整体外壳，弹窗不受影响）
  if (shell) shell.style.zoom = th.scale && th.scale !== 100 ? th.scale + '%' : '';

  // 背景：淡彩 = 纸面向主题色混入少量色相；渐变 = 线性渐变类
  shell && shell.classList.toggle('bg-gradient', th.bg === 'gradient');
  if (th.bg === 'tint') {
    const dark = isDarkNow();
    const paper = dark ? '#0E0E0D' : '#FAFAF9';
    st.setProperty('--paper', mix(paper, th.color, dark ? 0.07 : 0.045));
  } else {
    st.removeProperty('--paper');
  }
}

// 自定义主题脚本：从服务端拉取当前用户的 theme.js 并执行。
// 脚本新增的 <html> 行内变量会被记录，移除 / 替换主题时一并清理，
// 避免朴素脚本（直接 setProperty）移除后留下残留样式。
// 出错时保持内置外观，仅提示，不阻断使用。
let themeScriptEl = null;
let themeAddedProps = [];

function clearThemeVars() {
  themeAddedProps.forEach((p) => document.documentElement.style.removeProperty(p));
  themeAddedProps = [];
}

async function loadCustomTheme() {
  if (themeScriptEl) {
    themeScriptEl.remove();
    themeScriptEl = null;
  }
  clearThemeVars();

  let js = '';
  try {
    const res = await fetch('api/theme.js', { cache: 'no-store' });
    if (res.status === 204) return;
    if (!res.ok) throw new Error(String(res.status));
    js = (await res.text()).trim();
  } catch { return; }
  if (!js) return;

  const before = new Set([...document.documentElement.style]);
  try {
    themeScriptEl = document.createElement('script');
    themeScriptEl.id = 'wf-custom-theme';
    themeScriptEl.textContent = js;
    document.body.appendChild(themeScriptEl);
    themeAddedProps = [...document.documentElement.style].filter((p) => !before.has(p));
  } catch (e) {
    ui.toast('自定义主题执行失败', String(e.message || e).slice(0, 120), 'err', 6000);
  }
}

const ctx = {
  state: null,
  params: {},

  // 离开确认：由编辑器等有未保存状态的视图注册；route() 渲染前自动清理。
  // check() 返回 false 表示阻止本次离开；hasUnsaved() 供刷新/关标签时同步查询。
  leaveGuard: null,
  hasUnsaved: () => false,

  async refreshState() {
    try {
      ctx.state = await api.state();
    } catch { /* 保持旧状态 */ }
    return ctx.state;
  },

  navigate(view, params = {}) {
    const hash = '#' + view + (params && params.id ? '/' + encodeURIComponent(params.id) : '');
    if (location.hash === hash) route();
    else location.hash = hash;
  },

  refresh() { route(); },

  // 保存外观配置（分段保存，立即应用）
  async saveAppearance(patch) {
    const next = { ...(ctx.state?.appearance || {}), ...patch };
    try {
      await api.saveSettings({ appearance: next });
      ctx.state = ctx.state || {};
      ctx.state.appearance = next;
      applyAppearance(next);
      // 主题脚本始终后置于内置外观执行，保持「自定义主题优先」语义
      await loadCustomTheme();
    } catch (e) {
      ui.toast('外观保存失败', e.message, 'err', 5000);
      throw e;
    }
  },

  // 切换深浅色（保留其余外观配置）
  setTheme(mode) {
    return ctx.saveAppearance({ mode });
  },

  route,
};

function parseHash() {
  const raw = location.hash.replace(/^#/, '');
  const [view, ...rest] = raw.split('/');
  const id = rest.length ? decodeURIComponent(rest.join('/')) : '';
  return VIEWS[view] ? { view, id } : { view: 'articles', id: '' };
}

let rendering = false;

async function route() {
  // 渲染前清理离开确认：新视图自行按需注册，避免过期状态拦截后续导航
  ctx.leaveGuard = null;
  ctx.hasUnsaved = () => false;

  const { view, id } = parseHash();
  ctx.params = id ? { id } : {};

  document.querySelectorAll('.nav-item').forEach((n) => {
    n.classList.toggle('is-active', n.dataset.view === view);
  });
  document.title = `${TITLES[view] || '水鱼文集'} · 水鱼文集`;

  if (rendering) return;
  rendering = true;
  const host = document.getElementById('view');
  host.scrollTop = 0;
  document.getElementById('content').scrollTop = 0;
  try {
    await VIEWS[view].render(host, ctx);
  } catch (e) {
    host.innerHTML = `<div class="empty">${ui.icon('alert', 56)}<h3>页面渲染出错</h3><p>${ui.esc(e.message)}</p></div>`;
  } finally {
    rendering = false;
  }
}

// 所有导航（侧边栏 / 返回 / 浏览器前进后退）都经 hashchange：先过离开确认
let lastHash = location.hash;
let revertingHash = false;

async function onHashChange() {
  if (revertingHash) { // 上次离开被取消，正在还原 hash
    revertingHash = false;
    lastHash = location.hash;
    return;
  }
  if (ctx.leaveGuard) {
    const ok = await ctx.leaveGuard();
    if (!ok) {
      revertingHash = true;
      location.hash = lastHash; // 还原地址（会再触发一次 hashchange，被上方分支消费）
      return;
    }
  }
  lastHash = location.hash;
  route();
}

// 刷新 / 关闭标签页：浏览器原生确认（此场景无法使用自定义弹窗）
window.addEventListener('beforeunload', (e) => {
  if (ctx.hasUnsaved()) {
    e.preventDefault();
    e.returnValue = '';
  }
});

function setupShell() {
  const shell = document.getElementById('shell');
  const saved = localStorage.getItem('wf-collapsed');
  const collapsed = saved === null ? window.innerWidth <= 900 : saved === '1';
  shell.classList.toggle('collapsed', collapsed);

  document.getElementById('collapseBtn').onclick = () => {
    const next = !shell.classList.contains('collapsed');
    shell.classList.toggle('collapsed', next);
    localStorage.setItem('wf-collapsed', next ? '1' : '0');
  };

  document.querySelectorAll('.nav-item').forEach((n) => {
    n.onclick = () => ctx.navigate(n.dataset.view);
  });
}

async function boot() {
  setupShell();
  setupMarkdown();

  if (!window.marked) {
    ui.toast('Markdown 渲染库未加载', '预览可能不可用，请刷新页面重试', 'warn', 6000);
  }

  await ctx.refreshState();
  // 外观：优先用账号级配置；接口不可用时退回 localStorage 里的本地选择
  if (ctx.state?.appearance) applyAppearance(ctx.state.appearance);
  else {
    const saved = localStorage.getItem('wf-theme');
    if (saved) document.documentElement.dataset.theme = saved;
  }
  await loadCustomTheme();

  // 系统深浅色变化时，重算依赖当前模式的外观（淡色背景 / 自定义强调色）
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (ctx.state?.appearance) applyAppearance(ctx.state.appearance);
  });

  window.addEventListener('hashchange', onHashChange);
  await route();
}

boot();
