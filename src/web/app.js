/* 水鱼文集 —— 应用主入口 */

import { api } from './api.js';
import * as ui from './ui.js';
import { setupMarkdown } from './markdown.js';
import { initAppearance } from './appearance.js';
import * as articles from './views/articles.js';
import * as images from './views/images.js';
import * as seo from './views/seo.js';
import * as settings from './views/settings.js';

const VIEWS = { articles, images, seo, settings };
const TITLES = { articles: '文章库', images: '图片库', seo: '网页检测', settings: '设置' };

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
// 具体样式由主题声明覆盖，保证同一主题在任何时候表现一致。
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
// 自定义主题声明由 applyCustomTheme 单独叠加，覆盖在内置外观之上。
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

  // 字体（用变量承载，方便自定义主题在同一层覆盖）
  st.setProperty('--app-font', FONT_STACKS[th.font] || 'var(--sans)');

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

/* ---------------- 自定义主题：js 文件，简单 / 复杂两种写法 ----------------
   简单：一行一条外观声明，只调主色 / 副色 / 背景 / 字体等——
     主色: #1F6C9F
     副色: #B45309
     背景: https://example.com/bg.jpg
     字体: 宋体
   复杂：文件里定义 function theme(api)，可调界面、动画、图标——
     api.setVar(变量, 值)  设置 CSS 变量
     api.css(样式文本)     注入 <style>（动画、界面微调都写在这里）
     api.icon(名称, 图形)  覆盖内置图标（SVG path，currentColor 上色）
     api.el / api.dark     根元素 / 当前是否深色
   判别方式：文件里出现 function theme( 即按复杂模式执行，否则按声明解析。 */

const SPEC_ALIASES = {
  主色: 'accent', accent: 'accent', 强调色: 'accent',
  副色: 'accent2', accent2: 'accent2', 次色: 'accent2', secondary: 'accent2',
  背景: 'bg', bg: 'bg', background: 'bg',
  字体: 'font', font: 'font',
  文字: 'ink', ink: 'ink', 文字色: 'ink', text: 'ink',
  底色: 'paper', paper: 'paper', 纸面: 'paper',
  圆角: 'radius', radius: 'radius',
};

const COLOR_RE = /^(#[0-9a-f]{3,8}|rgba?\([^()]*\)|hsla?\([^()]*\)|[a-z]{3,20})$/i;
const FONT_RE = /^[\w\u4e00-\u9fa5 "'\-,.]{1,80}$/;

// 自定义主题会碰的变量：清理时按这份清单回退，不留残影
const SPEC_VARS = [
  '--accent', '--accent-ink', '--accent-soft', '--accent-line', '--accent-2',
  '--app-font', '--theme-bg', '--ink', '--paper', '--r', '--r-lg',
];

// parseThemeSpec 解析「一行一条」的主题声明。
// 支持 "键: 值"、"键 = 值"、"键 值" 三种写法；// 与 # 开头为注释。
function parseThemeSpec(text) {
  const spec = {};
  const invalid = [];
  for (const raw of String(text || '').split(/\r?\n/)) {
    let line = raw.trim();
    if (!line || line.startsWith('//') || line.startsWith('#')) continue;
    line = line.replace(/\s+\/\/.*$/, '').trim();
    const m = /^([^:=]+?)\s*[:=]\s*(.+)$/.exec(line) || /^(\S+)\s+(.+)$/.exec(line);
    if (!m) { invalid.push(raw.trim()); continue; }
    const key = SPEC_ALIASES[m[1].trim().toLowerCase()] || SPEC_ALIASES[m[1].trim()];
    if (!key) { invalid.push(raw.trim()); continue; }
    let val = m[2].trim().replace(/^["']|["';]$/g, '').trim();
    if (!val) continue;

    if (key === 'accent' || key === 'accent2' || key === 'ink' || key === 'paper') {
      if (!COLOR_RE.test(val)) { invalid.push(raw.trim()); continue; }
    } else if (key === 'bg') {
      if (!/^https?:\/\/\S+$/i.test(val) && !COLOR_RE.test(val)) { invalid.push(raw.trim()); continue; }
    } else if (key === 'font') {
      if (!FONT_RE.test(val)) { invalid.push(raw.trim()); continue; }
    } else if (key === 'radius') {
      const n = parseInt(val, 10);
      if (!Number.isFinite(n) || n < 0 || n > 40) { invalid.push(raw.trim()); continue; }
      val = n;
    }
    spec[key] = val;
  }
  return { spec, invalid };
}

const isUrl = (v) => /^https?:\/\//i.test(v || '');

const THEME_STYLE_ID = 'wf-theme-js';

// 清理自定义主题会留下的所有痕迹：CSS 变量、注入的 <style>、覆盖的图标
function clearCustomTheme() {
  const st = document.documentElement.style;
  SPEC_VARS.forEach((p) => st.removeProperty(p));
  const style = document.getElementById(THEME_STYLE_ID);
  if (style) style.remove();
  ui.resetCustomIcons();
}

// applyThemeSpec 把解析结果落到 CSS 变量。
function applyThemeSpec(spec) {
  const st = document.documentElement.style;
  const dark = isDarkNow();
  const paperBase = dark ? '#0E0E0D' : '#FAFAF9';

  if (spec.accent) {
    const rgb = hexToRgb(spec.accent);
    st.setProperty('--accent', spec.accent);
    st.setProperty('--accent-ink', dark && rgb ? mix(spec.accent, '#FFFFFF', 0.28) : spec.accent);
    if (rgb) {
      st.setProperty('--accent-soft',
        dark ? `rgba(${rgb.r}, ${rgb.g}, ${rgb.b}, .16)` : mix(spec.accent, '#FFFFFF', 0.88));
      st.setProperty('--accent-line',
        dark ? `rgba(${rgb.r}, ${rgb.g}, ${rgb.b}, .38)` : mix(spec.accent, '#FFFFFF', 0.66));
    }
  }

  // 副色：次级强调（子菜单高亮条）+ 页面底色的淡彩
  if (spec.accent2) {
    st.setProperty('--accent-2', spec.accent2);
    if (!spec.paper) st.setProperty('--paper', mix(paperBase, spec.accent2, dark ? 0.09 : 0.055));
  }

  if (spec.paper) st.setProperty('--paper', spec.paper);
  if (spec.ink) st.setProperty('--ink', spec.ink);

  if (spec.font) {
    const stack = spec.font in FONT_STACKS
      ? (FONT_STACKS[spec.font] || 'var(--sans)')
      : `"${spec.font}", var(--sans)`;
    st.setProperty('--app-font', stack);
  }

  if (spec.bg) {
    if (isUrl(spec.bg)) st.setProperty('--theme-bg', `url("${spec.bg.replace(/["\\]/g, '')}")`);
    else st.setProperty('--paper', spec.bg); // 背景填颜色也支持
  }

  if (spec.radius != null) {
    st.setProperty('--r', `${spec.radius}px`);
    st.setProperty('--r-lg', `${spec.radius + 4}px`);
  }
}

// 复杂主题可用的 API：变量 / 样式 / 图标 / 根元素 / 深浅色。
// 只暴露这些入口，主题不直接持有全局对象引用，切换主题时按约定统一清理。
function makeThemeAPI() {
  return {
    el: document.documentElement,
    get dark() { return isDarkNow(); },
    vars: (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim(),
    setVar: (name, value) => {
      if (!/^--[\w-]+$/.test(String(name))) throw new Error(`变量名不合法：${name}`);
      document.documentElement.style.setProperty(name, String(value ?? ''));
    },
    css: (text) => {
      let node = document.getElementById(THEME_STYLE_ID);
      if (!node) {
        node = document.createElement('style');
        node.id = THEME_STYLE_ID;
        document.head.appendChild(node);
      }
      node.textContent = String(text ?? '');
    },
    icon: (name, svg) => ui.setCustomIcon(name, svg),
  };
}

// runComplexTheme 执行复杂主题：文件中需定义 function theme(api)。
function runComplexTheme(code) {
  const factory = new Function('"use strict";\n' + code + '\n;return typeof theme === "function" ? theme : null;');
  const themeFn = factory();
  if (typeof themeFn !== 'function') {
    throw new Error('复杂主题需要定义 function theme(api)');
  }
  themeFn(makeThemeAPI());
}

// applyCustomTheme 取当前选中的自定义主题并应用。
// 返回 { invalid } 供界面提示（简单模式解析不通过的行）或 { error }（复杂模式执行失败）。
function applyCustomTheme(ap) {
  const cfg = ap || {};
  const t = (cfg.themes || []).find((x) => x.name === cfg.theme);
  if (!t) return null;
  // 文件里出现 function theme( → 复杂模式；否则按一行一条的声明解析
  if (/function\s+theme\s*\(/.test(t.js || '')) {
    try {
      runComplexTheme(t.js);
      return {};
    } catch (e) {
      console.warn('复杂主题执行失败：', e);
      return { error: e.message };
    }
  }
  const parsed = parseThemeSpec(t.js);
  applyThemeSpec(parsed.spec);
  return parsed;
}

// 统一入口：先回到内置外观，再叠加自定义主题声明
function applyAppearanceAll(ap) {
  clearCustomTheme();
  applyAppearance(ap);
  return applyCustomTheme(ap);
}

/* ---------------- 图床预连接 ---------------- */

// 启用图床时提前与 Cloudinary 建连：图片实际由 res.cloudinary.com 提供，
// api.cloudinary.com 负责上传。预连接可省掉首图的 DNS + TLS 握手时间。
// 关闭图床时移除，避免给不用图床的用户白占连接。
function preconnectCloudinary(on) {
  const ids = ['wf-pre-res', 'wf-pre-api', 'wf-dns-cloud'];
  if (!on) {
    ids.forEach((id) => { const n = document.getElementById(id); if (n) n.remove(); });
    return;
  }
  if (document.getElementById('wf-pre-res')) return;
  const add = (id, rel, href, cross = false) => {
    const l = document.createElement('link');
    l.id = id;
    l.rel = rel;
    l.href = href;
    if (cross) l.crossOrigin = '';
    document.head.appendChild(l);
  };
  add('wf-pre-res', 'preconnect', 'https://res.cloudinary.com', true);
  add('wf-pre-api', 'preconnect', 'https://api.cloudinary.com', true);
  add('wf-dns-cloud', 'dns-prefetch', 'https://cloudinary.com');
}

/* ---------------- 上下文 ---------------- */

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
    // 离开外部文件视图时清掉入口带来的 ?path=，避免刷新又跳回该文件
    if (location.search) history.replaceState(null, '', location.pathname + location.hash);
    if (location.hash === hash) route();
    else location.hash = hash;
  },

  refresh() { route(); },

  // 静默同步地址栏（不触发 hashchange / 不重渲染）。
  // 编辑器保存后标题变化会换文件名与 ID，用它避免整页重挂载带来的加载闪烁。
  syncHash(hash) {
    lastHash = hash;
    history.replaceState(null, '', location.pathname + location.search + hash);
  },

  // 打开系统「打开方式」传进来的外部文件（文件关联入口带 ?path=）
  openExternal(path) {
    const host = document.getElementById('view');
    ctx.leaveGuard = null;
    ctx.hasUnsaved = () => false;
    ctx.extPath = path;
    host.scrollTop = 0;
    document.getElementById('content').scrollTop = 0;
    document.querySelectorAll('.nav-item').forEach((n) => {
      n.classList.toggle('is-active', n.dataset.view === 'articles');
    });
    document.title = '编辑文件 · 水鱼文集';
    return articles.renderExternalFile(host, ctx, path).catch((e) => {
      host.innerHTML = `<div class="empty">${ui.icon('alert', 56)}<h3>无法打开文件</h3><p>${ui.esc(e.message)}</p></div>`;
    });
  },

  // 保存外观配置（分段保存，成功后立即应用）
  async saveAppearance(patch) {
    const next = { ...(ctx.state?.appearance || {}), ...patch };
    try {
      const saved = await api.saveSettings({ appearance: next });
      const applied = saved.appearance || next;
      ctx.state = ctx.state || {};
      ctx.state.appearance = applied;
      applyAppearanceAll(applied);
      return applied;
    } catch (e) {
      ui.toast('外观保存失败', e.message, 'err', 5000);
      throw e;
    }
  },

  // 只预览不保存（设置页「保存」前的试穿）
  previewAppearance(ap) {
    return applyAppearanceAll(ap);
  },

  // 恢复到已保存的外观
  revertAppearance() {
    return applyAppearanceAll(ctx.state?.appearance || {});
  },

  // 图床开关保存后同步预连接
  syncCloudinary(on) {
    preconnectCloudinary(!!on);
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
  // 外观抽屉是全局的（侧栏用户卡片），不随路由清理
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
  if (ctx.state?.appearance) applyAppearanceAll(ctx.state.appearance);
  else {
    const saved = localStorage.getItem('wf-theme');
    if (saved) document.documentElement.dataset.theme = saved;
  }
  preconnectCloudinary(!!ctx.state?.cloudinary?.enabled);

  // 账号与外观：用户卡片内置在侧栏底部，点开是外观抽屉（全局可用）
  initAppearance(ctx);

  // 系统深浅色变化时，重算依赖当前模式的外观（淡色背景 / 自定义强调色）
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (ctx.state?.appearance) applyAppearanceAll(ctx.state.appearance);
  });

  window.addEventListener('hashchange', onHashChange);

  // 文件关联入口：飞牛在 URL 后追加 ?path=<绝对路径>
  const extPath = new URLSearchParams(location.search).get('path');
  if (extPath) {
    await ctx.openExternal(extPath);
    return;
  }
  await route();
}

boot();
