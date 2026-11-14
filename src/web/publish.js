/* 文章发布
 * 1) 复制发布 —— 复制文章（富文本）。有图床时把本地图片自动换成图床直链，
 *    粘贴到任意编辑器都能直接显示图片；没有图床则保留逐张复制图片的能力。
 * 2) 统一发布 —— 勾选多个平台一次发布；平台与密钥在「设置」里配置，
 *    发布弹窗只设置封面、标题、标签这类每篇文章不同的内容。
 */

import { api } from './api.js';
import * as ui from './ui.js';
import { renderMarkdown, buildCopyHTML } from './markdown.js';

/* ---------------- 文章中的本地图片 ---------------- */

// 提取文章引用的本地图片名（按出现顺序去重；云端图片无需复制）
export function localImagesOf(markdown) {
  const re = /!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;
  const out = [];
  let m;
  while ((m = re.exec(markdown))) {
    const src = m[1].trim();
    if (/^(https?:)?\/\//i.test(src) || /^data:/i.test(src)) continue;
    const name = decodeURIComponent(src.split('/').pop());
    if (name && !out.includes(name)) out.push(name);
  }
  return out;
}

// 把 Markdown 里的本地图片引用替换为图床直链（未命中链接的保持原样）
function applyCloudLinks(markdown, links) {
  return markdown.replace(/(!\[[^\]]*\]\()([^)\s]+)((?:\s+"[^"]*")?\))/g, (m, pre, src, post) => {
    if (/^(https?:)?\/\//i.test(src) || /^data:/i.test(src)) return m;
    const name = decodeURIComponent(src.split('/').pop());
    return links[name] ? pre + links[name] + post : m;
  });
}

/* ---------------- 适配器加载 ---------------- */

// 经服务端本地文件加载适配器（安装时已落盘，发布不依赖外网）
function loadAdapter(id) {
  return new Promise((resolve, reject) => {
    const old = document.getElementById('wf-publisher-script');
    if (old) old.remove();
    const s = document.createElement('script');
    s.id = 'wf-publisher-script';
    s.src = `api/publishers/${encodeURIComponent(id)}/source`;
    s.onload = () => {
      const reg = window.WFPublishers && window.WFPublishers[id];
      if (reg && typeof reg.publish === 'function') resolve(reg);
      else reject(new Error('适配器未正确注册'));
    };
    s.onerror = () => reject(new Error('适配器加载失败，请让管理员重新安装'));
    document.head.appendChild(s);
  });
}

// 渲染适配器配置字段（发布弹窗内联区与设置页弹窗共用）
export function configFieldsHTML(fields, cfg, prefix) {
  if (!fields.length) return '<p class="section-desc">该适配器无需配置。</p>';
  return fields.map((f) => `
    <div class="field">
      <label for="${prefix}-${ui.esc(f.key)}">${ui.esc(f.label || f.key)}${f.required ? ' *' : ''}</label>
      <input class="input" id="${prefix}-${ui.esc(f.key)}" type="${f.type === 'password' ? 'password' : 'text'}"
        value="${ui.esc((cfg || {})[f.key] || '')}" spellcheck="false" autocomplete="off">
      ${f.hint ? `<span class="hint">${ui.esc(f.hint)}</span>` : ''}
    </div>`).join('');
}

// 收集配置字段值（空值不提交，保留原有密钥）
export function collectConfigFields(root, fields, prefix) {
  const next = {};
  fields.forEach((f) => {
    const el = root.querySelector(`#${prefix}-${f.key}`);
    const v = el ? el.value.trim() : '';
    if (v) next[f.key] = v;
  });
  return next;
}

// 打开适配器配置弹窗（设置页用；发布弹窗内改为内联区，避免 modal 单例互相顶掉）
export async function openPublisherConfigDialog(meta, onSaved) {
  let fields = [];
  try {
    const reg = await loadAdapter(meta.id);
    fields = Array.isArray(reg.configFields) ? reg.configFields : [];
  } catch (e) {
    ui.toast('加载适配器失败', e.message, 'err', 6000);
    return;
  }
  ui.modal(`
    <div class="modal-head"><h3>${ui.esc(meta.name || meta.id)} · 配置</h3>
    <p>密钥保存在你自己的账号下，不会随文章上传，也不会被其他账号看到。</p></div>
    <div class="modal-body">
      <div class="field-stack">${configFieldsHTML(fields, meta.config, 'cfg')}</div>
    </div>
    <div class="modal-foot">
      <button class="btn btn-ghost" data-act="cancel">取消</button>
      <button class="btn btn-primary" data-act="save">保存配置</button>
    </div>
  `, {
    wide: true,
    onMount(mroot, mclose) {
      mroot.querySelector('[data-act="cancel"]').onclick = mclose;
      mroot.querySelector('[data-act="save"]').onclick = async (e) => {
        const btn = e.currentTarget;
        btn.disabled = true;
        const next = collectConfigFields(mroot, fields, 'cfg');
        try {
          await api.savePublisherConfig(meta.id, next);
          meta.config = next;
          onSaved && onSaved(next);
          ui.toast('配置已保存', '', 'ok');
          mclose();
        } catch (err) {
          btn.disabled = false;
          ui.toast('保存失败', err.message, 'err');
        }
      };
    },
  });
}

/* ---------------- 图片转公网链接 ---------------- */

async function copyImageBlob(name, btn) {
  btn.disabled = true;
  try {
    if (!(navigator.clipboard && window.isSecureContext && window.ClipboardItem)) {
      throw new Error('当前环境不支持图片剪贴板（需 HTTPS 或 localhost），请手动保存图片后插入');
    }
    const res = await fetch('photo/' + encodeURIComponent(name));
    if (!res.ok) throw new Error(`读取图片失败 (HTTP ${res.status})`);
    const blob = await res.blob();
    const type = (blob.type || 'image/png').toLowerCase();
    await navigator.clipboard.write([new ClipboardItem({ [type]: blob })]);
    btn.innerHTML = `${ui.icon('check', 14)}<span>已复制</span>`;
    btn.classList.add('is-done');
    return true;
  } catch (e) {
    ui.toast('复制图片失败', e.message, 'err', 6000);
    return false;
  } finally {
    btn.disabled = false;
  }
}

// 把文章里的本地图片转成公网 URL：
// 已上传过图床的直接复用；图床可用且本地可读的自动上传；其余保留相对名并告警。
async function resolveImages(markdown, onProgress) {
  const state = await api.images().catch(() => ({ cloud: [], cloudEnabled: false }));
  const links = {};
  (state.cloud || []).forEach((c) => { if (c && c.name) links[c.name] = c.url; });
  const cloudOn = !!state.cloudEnabled;

  const warnings = [];
  const names = localImagesOf(markdown);
  for (const name of names) {
    if (links[name]) continue;
    if (!cloudOn) { warnings.push(name); continue; }
    try {
      onProgress && onProgress(`正在上传图片 ${name}…`);
      const res = await fetch('photo/' + encodeURIComponent(name));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const blob = await res.blob();
      const up = await api.upload(new File([blob], name, { type: blob.type || 'image/png' }));
      if (up && up.url) links[name] = up.url;
      else throw new Error((up && up.error) || '上传失败');
    } catch (e) {
      warnings.push(`${name}（${e.message}）`);
    }
  }
  return { markdown: applyCloudLinks(markdown, links), links, warnings, cloudOn };
}

/* ---------------- 远程发布 ---------------- */

// 纯文本摘要（取前若干字符）
function excerptOf(markdown, max = 120) {
  const text = renderMarkdown(markdown).replace(/<[^>]+>/g, ' ')
    .replace(/&[a-z]+;/gi, ' ').replace(/\s+/g, ' ').trim();
  return text.length > max ? text.slice(0, max) + '…' : text;
}

/* ---------------- 发布弹窗 ---------------- */

export async function openPublishDialog(ctx, art) {
  let installed = [];
  let isAdmin = false;
  let listError = '';
  try {
    const r = await api.publishers();
    installed = r.installed || [];
    isAdmin = !!r.isAdmin;
    listError = r.error || '';
  } catch (e) {
    listError = e.message;
  }

  // 图床链接表：用于直接在复制发布里给出图片直链
  const imgState = await api.images().catch(() => ({ cloud: [], cloudEnabled: false }));
  const cloudLinks = {};
  (imgState.cloud || []).forEach((c) => { if (c && c.name) cloudLinks[c.name] = c.url; });
  const cloudOn = !!imgState.cloudEnabled;

  const imgNames = localImagesOf(art.content);
  const linked = imgNames.filter((n) => cloudLinks[n]);
  const unlinked = imgNames.filter((n) => !cloudLinks[n]);

  const usable = installed.filter((p) => !p.missing);

  ui.modal(`
    <div class="modal-head">
      <h3>${ui.icon('rocket', 18)} 发布文章</h3>
      <p>标题：${ui.esc(art.title || '未命名文章')}</p>
    </div>

    <div class="pub-tabs">
      <button class="pub-tab is-active" data-mode="copy">复制发布</button>
      <button class="pub-tab" data-mode="remote">统一发布</button>
    </div>

    <div class="pub-pane" data-pane="copy">
      <div class="pub-step">
        <div class="pub-step-head"><span class="pub-step-no">1</span>复制文章</div>
        <p class="pub-step-desc">以富文本写入剪贴板，粘贴到 Notion、公众号等编辑器时标题、列表、粗体原样保留。${cloudOn
          ? '文中本地图片会自动上传并替换为图床直链，粘贴后图片直接可见。'
          : '当前未启用图床，本地图片会以文件名占位，可在下一步逐张复制图片。'}</p>
        <div class="pub-actions-row">
          <button class="btn btn-primary btn-sm" id="pubCopyText">${ui.icon('copy', 14)}<span>复制文章</span></button>
          ${imgNames.length ? `<button class="btn btn-ghost btn-sm" id="pubCopyLinks">${ui.icon('link', 14)}<span>复制全部图片链接</span></button>` : ''}
        </div>
        ${imgNames.length && !cloudOn ? '<p class="pub-step-desc" style="margin-top:10px">逐张复制图片需要 HTTPS 或本机访问；否则请手动插入图片。</p>' : ''}
      </div>
      <div class="pub-step">
        <div class="pub-step-head"><span class="pub-step-no">2</span>图片${imgNames.length ? `（${linked.length} 张已有直链 / 共 ${imgNames.length} 张）` : ''}</div>
        ${imgNames.length ? `
          <div class="pub-imgs" id="pubImgs">
            ${imgNames.map((n, i) => `
              <div class="pub-img-row" data-name="${ui.esc(n)}">
                <span class="pub-img-no">${i + 1}</span>
                <span class="pub-img-name" title="${ui.esc(n)}">${ui.esc(n)}</span>
                ${cloudLinks[n]
                  ? `<span class="pub-img-tag">图床</span><button class="btn btn-ghost btn-sm" data-link="${ui.esc(cloudLinks[n])}">${ui.icon('link', 13)}<span>复制链接</span></button>`
                  : `<span class="pub-img-tag is-local">本地</span><button class="btn btn-ghost btn-sm" data-blob="${ui.esc(n)}">${ui.icon('copy', 13)}<span>复制图片</span></button>`}
              </div>`).join('')}
          </div>
        ` : '<p class="pub-step-desc">本文没有引用本地图片，无需此步。</p>'}
      </div>
    </div>

    <div class="pub-pane" data-pane="remote" hidden>
      <div class="field">
        <label for="pubTitle">标题</label>
        <input class="input" id="pubTitle" value="${ui.esc(art.title || '')}" spellcheck="false">
      </div>
      <div class="field">
        <label for="pubCover">封面</label>
        <input class="input" id="pubCover" placeholder="图片直链，或本地图片名（有图床时自动转直链）" spellcheck="false">
      </div>
      <div class="field">
        <label for="pubTags">标签</label>
        <input class="input" id="pubTags" placeholder="多个标签用逗号分隔" spellcheck="false">
      </div>
      <div class="field">
        <label>发布到（可多选，一次统一发布）</label>
        <div class="pub-picker" id="pubPicker"></div>
        ${listError ? `<span class="hint" style="color:var(--warn)">适配器清单加载失败：${ui.esc(listError)}</span>` : ''}
      </div>
      <div class="pub-remote-actions">
        <button class="btn btn-primary btn-sm" id="pubSendBtn">${ui.icon('rocket', 14)}<span>统一发布</span></button>
      </div>
      <div class="pub-results" id="pubResults" hidden></div>
      <div class="pub-status" id="pubStatus"></div>
    </div>

    <div class="modal-foot">
      <button class="btn btn-ghost" data-act="close">关闭</button>
    </div>
  `, {
    wide: true,
    onMount(root, close) {
      root.querySelector('[data-act="close"]').onclick = close;

      /* ---------- tab 切换 ---------- */
      const tabs = [...root.querySelectorAll('.pub-tab')];
      tabs.forEach((t) => {
        t.onclick = () => {
          tabs.forEach((x) => x.classList.toggle('is-active', x === t));
          root.querySelectorAll('.pub-pane').forEach((p) => {
            p.hidden = p.dataset.pane !== t.dataset.mode;
          });
        };
      });

      /* ---------- 复制发布 ---------- */
      const copyBtn = root.querySelector('#pubCopyText');

      copyBtn.onclick = async () => {
        const old = copyBtn.innerHTML;
        copyBtn.disabled = true;
        copyBtn.innerHTML = '<span class="spinner"></span><span>处理中…</span>';
        try {
          const r = await api.copyArticle(art.id);
          // 有图床时把本地图片补传并替换成直链（粘贴出去就能看到图）
          const text = cloudOn
            ? (await resolveImages(r.text, (m) => ui.toast('正在准备图片', m, 'info', 1600))).markdown
            : applyCloudLinks(r.text, cloudLinks);
          await ui.copyRich(buildCopyHTML(text), text, '文章已复制到剪贴板');
        } catch (err) {
          ui.toast('复制失败', err.message, 'err');
        } finally {
          copyBtn.disabled = false;
          copyBtn.innerHTML = old;
        }
      };

      const copyLinksBtn = root.querySelector('#pubCopyLinks');
      copyLinksBtn && (copyLinksBtn.onclick = async () => {
        copyLinksBtn.disabled = true;
        try {
          const fresh = await api.images().catch(() => ({ cloud: [] }));
          const links = {};
          (fresh.cloud || []).forEach((c) => { if (c && c.name) links[c.name] = c.url; });
          const urls = imgNames.map((n) => links[n]).filter(Boolean);
          if (!urls.length) {
            ui.toast('还没有图片直链', '启用图床后发布会自动上传', 'warn', 5000);
            return;
          }
          ui.copy(urls.join('\n'), `已复制 ${urls.length} 条图片链接`);
        } finally {
          copyLinksBtn.disabled = false;
        }
      });

      root.querySelectorAll('[data-link]').forEach((btn) => {
        btn.onclick = () => ui.copy(btn.dataset.link, '图片链接已复制');
      });
      root.querySelectorAll('[data-blob]').forEach((btn) => {
        btn.onclick = () => copyImageBlob(btn.dataset.blob, btn);
      });

      /* ---------- 统一发布 ---------- */
      const picker = root.querySelector('#pubPicker');
      const status = root.querySelector('#pubStatus');
      const results = root.querySelector('#pubResults');
      const selected = new Set(usable.map((p) => p.id)); // 默认全选已安装平台

      const paintPicker = () => {
        if (!installed.length) {
          picker.innerHTML = `<div class="pub-empty">还没有安装发布适配器。${isAdmin
            ? '前往「设置 → 管理员设置」下载安装。'
            : '请联系管理员在「设置 → 管理员设置」中安装。'}</div>`;
          return;
        }
        picker.innerHTML = installed.map((p) => `
          <label class="pub-card ${p.missing ? 'is-missing' : ''}">
            <input type="checkbox" name="pubAdapter" value="${ui.esc(p.id)}"
              ${selected.has(p.id) ? 'checked' : ''} ${p.missing ? 'disabled' : ''}>
            <span class="pub-card-body">
              <b>${ui.esc(p.name || p.id)}</b>
              <span class="pub-card-meta">v${ui.esc(p.version || '?')}${p.missing ? ' · 文件丢失，请管理员重新安装' : ''}</span>
              ${p.desc ? `<span class="pub-card-desc">${ui.esc(p.desc)}</span>` : ''}
            </span>
          </label>`).join('');
        picker.querySelectorAll('input[name="pubAdapter"]').forEach((box) => {
          box.onchange = () => {
            if (box.checked) selected.add(box.value);
            else selected.delete(box.value);
          };
        });
      };
      paintPicker();

      /* ---------- 统一发布 ---------- */
      root.querySelector('#pubSendBtn').onclick = async (e) => {
        const btn = e.currentTarget;
        const ids = [...selected];
        if (!ids.length) { ui.toast('请先勾选发布平台', '', 'warn'); return; }
        btn.disabled = true;
        results.hidden = false;
        results.innerHTML = ids.map((id) => {
          const meta = usable.find((p) => p.id === id) || { id };
          return `<div class="pub-result" data-res="${ui.esc(id)}">
            <span class="pub-result-dot"></span>
            <b>${ui.esc(meta.name || id)}</b>
            <span class="pub-result-msg">等待中…</span>
          </div>`;
        }).join('');
        status.className = 'pub-status is-busy';
        status.textContent = '正在准备图片…';
        const setRow = (id, cls, msg) => {
          const row = results.querySelector(`[data-res="${CSS.escape(id)}"]`);
          if (!row) return;
          row.className = `pub-result ${cls}`;
          row.querySelector('.pub-result-msg').innerHTML = msg;
        };
        try {
          // 密钥在设置里配置，发布前刷新一次，避免用到弹窗打开时的旧配置
          try {
            const fresh = await api.publishers();
            (fresh.installed || []).forEach((p) => {
              const m = usable.find((x) => x.id === p.id);
              if (m) m.config = p.config || {};
            });
          } catch { /* 刷新失败就用现有配置 */ }
          const { markdown, links, warnings } = await resolveImages(art.content, (m) => { status.textContent = m; });
          // 封面：填了本地图片名且有图床直链时自动转直链
          const coverRaw = root.querySelector('#pubCover').value.trim();
          const cover = coverRaw && links[coverRaw] ? links[coverRaw] : coverRaw;
          const payload = {
            title: root.querySelector('#pubTitle').value.trim() || art.title || '未命名文章',
            cover: cover || '',
            tags: root.querySelector('#pubTags').value.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
            markdown,
            excerpt: excerptOf(art.content),
            date: new Date().toISOString(),
          };
          let okCount = 0;
          for (const id of ids) {
            const meta = usable.find((p) => p.id === id);
            setRow(id, 'is-busy', '发布中…');
            status.textContent = `正在发布到 ${meta ? (meta.name || id) : id}…`;
            try {
              const reg = await loadAdapter(id);
              const fields = Array.isArray(reg.configFields) ? reg.configFields : [];
              const cfg = (meta && meta.config) || {};
              const missingReq = fields.filter((f) => f.required && !cfg[f.key]).map((f) => f.label || f.key);
              if (missingReq.length) throw new Error(`请先在「设置 → 发布设置 → 平台密钥」配置：${missingReq.join('、')}`);
              const r = await reg.publish(payload, cfg);
              okCount++;
              setRow(id, 'is-ok', `成功${r && r.url
                ? ` · <a href="${ui.esc(r.url)}" target="_blank" rel="noopener noreferrer">查看页面</a>` : ''}`);
            } catch (err) {
              setRow(id, 'is-err', ui.esc(err.message));
            }
          }
          status.className = okCount ? 'pub-status is-ok' : 'pub-status is-err';
          status.textContent = `已发布到 ${okCount}/${ids.length} 个平台`;
          ui.toast(okCount ? '发布完成' : '发布失败', okCount ? `${okCount}/${ids.length} 个平台成功` : '请查看下方各平台结果', okCount ? 'ok' : 'err', 5000);
          if (warnings.length) {
            ui.toast('部分图片未能转为公网链接', warnings.slice(0, 3).join('；'), 'warn', 8000);
          }
        } catch (err) {
          status.className = 'pub-status is-err';
          status.textContent = `发布失败：${err.message}`;
          ui.toast('发布失败', err.message, 'err', 8000);
        } finally {
          btn.disabled = false;
        }
      };
    },
  });
}
