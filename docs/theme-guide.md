# 水鱼文集 · 外观主题定制说明

水鱼文集的外观以**主题**为单位管理：内置主题 + 自定义主题（JS）。每套内置主题把配色、字体、字号、背景**打包**在一起，选一套即整体生效，不再逐项微调。所有外观配置按 NAS 账号独立保存，换账号登录互不影响。

---

## 一、快速开始

进入 **设置 → 外观**：

| 选项 | 说明 |
| --- | --- |
| 主题模式 | 跟随系统 / 浅色 / 深色 |
| 主题 | 点选主题卡片，立即整体生效；刷新页面后保持 |
| 自定义主题 | 上传 `.js` 文件生成新主题，见下文 |

### 内置主题

| 主题 | 配色 | 字体 | 字号 | 背景 |
| --- | --- | --- | --- | --- |
| 纸墨（默认） | 湖蓝 | 系统字体 | 100% | 纯色 |
| 琥珀 | 琥珀棕 | 系统字体 | 100% | 淡彩 |
| 松绿 | 松绿 | 衬线字体 | 105% | 淡彩 |
| 紫罗兰 | 紫罗兰 | 圆体 | 100% | 淡彩 |
| 玫红 | 玫红 | 系统字体 | 100% | 淡彩 |

主题模式（浅色 / 深色）与主题相互独立：每套主题都自带深浅两套配色，切换模式不用重选主题。

---

## 二、自定义主题（JS）

### 上传方法

1. 设置 → 外观 → 自定义主题 → 「选择 .js 主题文件」
2. 上传后立即生效，主题列表中会出现一张以**文件名**命名的新卡片（如 `my-theme.js` → 主题「my-theme」）
3. 与已有主题重名时自动加后缀：`my-theme` → `my-theme(2)`
4. 点卡片切换主题；鼠标悬停卡片右下角的 **✕** 可删除主题
5. 限制：单个文件 ≤ **64 KB**，最多 20 个自定义主题，按当前登录账号生效

### 执行时机与能力

- 脚本在内置主题**应用之后**执行。自定义主题以「纸墨」为基底，脚本可覆盖其中任意样式
- 脚本在页面主世界运行，可以：
  - 读写 `document.documentElement` 上的 CSS 变量（推荐）
  - 切换 `data-theme`（light / dark）
  - 增删页面类名
- 脚本出错时保持内置外观并弹错误提示，不会白屏
- 删除或切换走自定义主题时，脚本引入的 CSS 变量会被自动清理，不会残留

### 可用的 CSS 变量

主题相关变量（均定义在 `:root`，深色模式有对应覆盖值）：

| 变量 | 用途 |
| --- | --- |
| `--paper` | 页面纸面背景 |
| `--surface` / `--surface-2` / `--surface-3` | 卡片 / 次级 / 三级面 |
| `--line` / `--line-strong` | 边框 / 强边框 |
| `--ink` / `--ink-2` / `--ink-3` | 主文字 / 次文字 / 弱文字 |
| `--accent` | 强调色（图标、边框、高亮） |
| `--accent-ink` | 强调色文字 |
| `--accent-soft` | 强调色浅底 |
| `--accent-line` | 强调色边框 |
| `--btn-bg` / `--btn-ink` | 主按钮背景 / 文字 |
| `--r-sm` / `--r` / `--r-lg` | 圆角：小 / 默认 / 大 |
| `--serif` / `--sans` / `--mono` | 字体栈 |

### 示例

**示例 1：护眼米色 + 暖棕强调色**

```js
const root = document.documentElement;
root.style.setProperty('--paper', '#F5F0E6');
root.style.setProperty('--surface', '#FBF7EE');
root.style.setProperty('--surface-2', '#F0E9DA');
root.style.setProperty('--accent', '#8C5A2B');
root.style.setProperty('--accent-ink', '#8C5A2B');
root.style.setProperty('--accent-soft', '#F3E5D3');
root.style.setProperty('--accent-line', '#DEC9A8');
```

**示例 2：大圆角 + 更醒目的主按钮**

```js
const root = document.documentElement;
root.style.setProperty('--r-sm', '10px');
root.style.setProperty('--r', '14px');
root.style.setProperty('--r-lg', '20px');
root.style.setProperty('--btn-bg', '#1F6C9F');
root.style.setProperty('--btn-ink', '#FFFFFF');
```

**示例 3：跟随深浅色自动切换的两套配色**

```js
const mq = window.matchMedia('(prefers-color-scheme: dark)');
function apply() {
  const root = document.documentElement;
  const dark = root.dataset.theme === 'dark' ||
    (!root.dataset.theme && mq.matches);
  root.style.setProperty('--accent', dark ? '#E0A458' : '#B45309');
}
apply();
mq.addEventListener('change', apply);
// 应用内切换深浅色时也重算一次
new MutationObserver(apply).observe(document.documentElement, {
  attributes: true, attributeFilter: ['data-theme'],
});
```

**示例 4：直接引入一份外部样式（CDN 字体等）**

```js
const link = document.createElement('link');
link.rel = 'stylesheet';
link.href = 'https://fonts.googleapis.com/css2?family=Noto+Serif+SC:wght@400;600&display=swap';
document.head.appendChild(link);
document.documentElement.style.setProperty('--serif', '"Noto Serif SC", serif');
```

---

## 三、调试技巧

1. 浏览器按 `F12` 打开控制台，直接执行上面的代码可实时预览
2. 在 Elements 面板选中 `<html>`，右侧可看到全部 CSS 变量当前值
3. 改哪里不对，把 `setProperty` 那行删掉刷新即可恢复内置值

## 四、注意事项

- 自定义 JS 以**当前登录用户的权限**在页面执行，请只上传自己看得懂的代码
- 脚本体积上限 64 KB，超出部分会被截断
- 删除自定义主题后，之前选中的内置主题重新生效
- 外观配置保存在应用数据目录的 `users/<你的账号>/config.json`，卸载应用会一并清除
