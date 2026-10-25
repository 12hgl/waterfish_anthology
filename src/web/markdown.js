/* Markdown 渲染 + 输出净化 */

import { esc } from './ui.js';

export function setupMarkdown() {
  if (window.marked && window.marked.setOptions) {
    window.marked.setOptions({ gfm: true, breaks: false, headerIds: false, mangle: false });
  }
}

const BAD_TAGS = ['script', 'style', 'iframe', 'object', 'embed', 'link', 'meta', 'form', 'input', 'button', 'textarea', 'base'];

function sanitize(html) {
  const tpl = document.createElement('template');
  tpl.innerHTML = html;
  tpl.content.querySelectorAll(BAD_TAGS.join(',')).forEach((n) => n.remove());
  tpl.content.querySelectorAll('*').forEach((n) => {
    [...n.attributes].forEach((a) => {
      const name = a.name.toLowerCase();
      const val = String(a.value || '').trim();
      if (name.startsWith('on')) { n.removeAttribute(a.name); return; }
      if ((name === 'href' || name === 'src' || name === 'xlink:href') && /^(javascript|data:text\/html|vbscript):/i.test(val)) {
        n.removeAttribute(a.name);
      }
    });
    if (n.tagName === 'A') { n.setAttribute('target', '_blank'); n.setAttribute('rel', 'noopener noreferrer'); }
    if (n.tagName === 'IMG') { n.setAttribute('loading', 'lazy'); }
  });
  return tpl.innerHTML;
}

// 把图片 / 链接目标中的相对路径做 URL 编码：
// marked 不解析含空格的链接目标（会原样吐出文本），历史文章里的
// `![图](photo/名字 带空格.png)` 需要先编码才能正常渲染。
function encodeRelativeTargets(source) {
  return source.replace(/(!?\[[^\]]*\]\()([^)\n]+)(\))/g, (m, pre, target, post) => {
    const t = target.trim();
    if (!t || /^(?:[a-z][a-z0-9+.-]*:|\/\/|#|mailto:|tel:)/i.test(t) || t.startsWith('<')) return m;
    const titled = t.match(/^(.+?)\s+("(?:[^"\\]|\\.)*")$/);
    if (titled) return pre + encodeURI(titled[1]) + ' ' + titled[2] + post;
    return pre + encodeURI(t) + post;
  });
}

export function renderMarkdown(md) {
  const source = String(md || '');
  let html;
  if (window.marked && window.marked.parse) {
    try { html = window.marked.parse(encodeRelativeTargets(source)); } catch { html = `<p>${esc(source)}</p>`; }
  } else {
    html = `<pre>${esc(source)}</pre>`;
  }
  return sanitize(html);
}
