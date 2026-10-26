/* 文章发布（实验性 Beta）
 * 两种方式：
 *  1) 复制发布 —— 先复制文章（富文本），再按引用顺序逐张复制本地图片
 *  2) 发布到建站系统 —— 加载仓库适配器（NotionNext 等），配置密钥后直接发布
 */

import { api } from './api.js';
import * as ui from './ui.js';
import { renderMarkdown, buildCopyHTML } from './markdown.js';

/* ---------------- 实验开关 ---------------- */

const EXP_KEY = 'wf.exp.publish';

export function expPublishEnabled() {
  try { return localStorage.getItem(EXP_KEY) === '1'; } catch { return false; }
}

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
    s.onerror = () => reject(new Error('适配器加载失败，请卸载后重新下载'));
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
    <p>密钥仅保存在本机，不会随文章上传。</p></div>
    ${configFieldsHTML(fields, meta.config, 'cfg')}
    <div class="modal-foot">
      <button class="btn btn-ghost" data-act="cancel">取消</button>
      <button class="btn btn-primary" data-act="save">保存配置</button>
    </div>
  `, {
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

/* ---------------- 复制发布 ---------------- */

async function copyImage(name, btn) {
  btn.disabled = true;
  btn.textContent = '复制中…';
  try {
    if (!(navigator.clipboard && window.isSecureContext && window.ClipboardItem)) {
      throw new Error('当前浏览器不支持图片剪贴板（需 HTTPS 或 localhost 环境），请手动保存图片后插入');
    }
    const res = await fetch('photo/' + encodeURIComponent(name));
    if (!res.ok) throw new Error(`读取图片失败 (HTTP ${res.status})`);
    const blob = await res.blob();
    const type = (blob.type || 'image/png').toLowerCase();
    await navigator.clipboard.write([new ClipboardItem({ [type]: blob })]);
    btn.textContent = '已复制';
    btn.classList.add('is-done');
  } catch (e) {
    btn.textContent = '重试';
    ui.toast('复制图片失败', e.message, 'err', 6000);
  } finally {
    btn.disabled = false;
  }
}

/* ---------------- 远程发布 ---------------- */

// 纯文本摘要（取前若干字符）
function excerptOf(markdown, max = 120) {
  const text = renderMarkdown(markdown).replace(/<[^>]+>/g, ' ')
    .replace(/&[a-z]+;/gi, ' ').replace(/\s+/g, ' ').trim();
  return text.length > max ? text.slice(0, max) + '…' : text;
}

// 把 Markdown 中的本地图片替换为公网 URL：
// 已上传过图床的直接用现成地址；图床可用且本地可读的自动上传；其余保留相对名并告警。
async function resolveImages(markdown, onProgress) {
  const imgState = await api.images();
  const cloud = imgState.cloud || [];
  const cloudByName = {};
  cloud.forEach((c) => { if (c && c.name) cloudByName[c.name] = c; });
  const cloudOn = !!imgState.cloudEnabled;

  const warnings = [];
  const cache = {}; // name -> url | null(失败)

  const re = /!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;
  let out = '';
  let last = 0;
  let m;
  while ((m = re.exec(markdown))) {
    out += markdown.slice(last, m.index);
    last = m.index + m[0].length;
    const src = m[1].trim();
    if (/^(https?:)?\/\//i.test(src) || /^data:/i.test(src)) {
      out += m[0]; // 云端图片原样保留
      continue;
    }
    const name = decodeURIComponent(src.split('/').pop());
    if (name in cache) {
      out += cache[name] ? m[0].replace(src, cache[name]) : m[0];
      continue;
    }
    if (cloudByName[name]) {
      cache[name] = cloudByName[name].url;
      out += m[0].replace(src, cache[name]);
      continue;
    }
    if (cloudOn) {
      try {
        onProgress && onProgress(`正在上传图片 ${name}…`);
        const res = await fetch('photo/' + encodeURIComponent(name));
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const blob = await res.blob();
        const file = new File([blob], name, { type: blob.type || 'image/png' });
        const up = await api.upload(file);
        if (up && up.url) {
          cache[name] = up.url;
          out += m[0].replace(src, cache[name]);
          continue;
        }
        throw new Error(up && up.error ? up.error : '上传失败');
      } catch (e) {
        cache[name] = null;
        warnings.push(`${name}（${e.message}）`);
        out += m[0];
        continue;
      }
    }
    cache[name] = null;
    warnings.push(name);
    out += m[0];
  }
  out += markdown.slice(last);
  return { markdown: out, warnings };
}

/* ---------------- 发布弹窗 ---------------- */

export async function openPublishDialog(ctx, art) {
  // 拉取适配器清单（installed 立即可用；available 供弹窗内直接下载）
  let installed = [];
  let available = [];
  let listError = '';
  try {
    const r = await api.publishers();
    installed = r.installed || [];
    available = r.available || [];
    listError = r.error || '';
  } catch (e) {
    listError = e.message;
  }

  const imgs = localImagesOf(art.content);

  ui.modal(`
    <div class="modal-head">
      <h3>${ui.icon('rocket', 18)} 发布文章</h3>
      <p>标题：${ui.esc(art.title || '未命名文章')}</p>
    </div>

    <div class="pub-tabs">
      <button class="pub-tab is-active" data-mode="copy">复制发布</button>
      <button class="pub-tab" data-mode="remote">发布到…</button>
    </div>

    <div class="pub-pane" data-pane="copy">
      <div class="pub-step">
        <div class="pub-step-head"><span class="pub-step-no">1</span>复制文章</div>
        <p class="pub-step-desc">把文章以富文本形式写入剪贴板，粘贴到 Notion 等编辑器时标题、列表、粗体原样保留。</p>
        <button class="btn btn-primary btn-sm" id="pubCopyText">${ui.icon('copy', 14)}<span>复制文章</span></button>
      </div>
      <div class="pub-step">
        <div class="pub-step-head"><span class="pub-step-no">2</span>按顺序复制图片${imgs.length ? `（共 ${imgs.length} 张）` : ''}</div>
        ${imgs.length ? `
          <div class="pub-imgs" id="pubImgs">
            ${imgs.map((n, i) => `
              <div class="pub-img-row">
                <span class="pub-img-no">${i + 1}</span>
                <span class="pub-img-name" title="${ui.esc(n)}">${ui.esc(n)}</span>
                <button class="btn btn-ghost btn-sm" data-img="${ui.esc(n)}">复制图片</button>
              </div>`).join('')}
          </div>
          <p class="pub-step-desc">图片按文中出现顺序逐张复制；在编辑器里把文章粘贴好后，依次粘贴即可。</p>
        ` : '<p class="pub-step-desc">本文没有引用本地图片，无需此步。</p>'}
      </div>
    </div>

    <div class="pub-pane" data-pane="remote" hidden>
      <div class="field">
        <label for="pubTitle">标题</label>
        <input class="input" id="pubTitle" value="${ui.esc(art.title || '')}" spellcheck="false">
      </div>
      <div class="field">
        <label for="pubTags">标签</label>
        <input class="input" id="pubTags" placeholder="多个标签用逗号分隔" spellcheck="false">
      </div>
      <div class="field">
        <label>发布到</label>
        <div class="pub-picker" id="pubPicker"></div>
        ${listError ? `<span class="hint" style="color:var(--warn)">适配器清单加载失败：${ui.esc(listError)}</span>` : ''}
      </div>
      <div class="pub-remote-actions">
        <button class="btn btn-ghost btn-sm" id="pubConfigBtn">${ui.icon('pencil', 14)}<span>适配器配置</span></button>
        <button class="btn btn-primary btn-sm" id="pubSendBtn">${ui.icon('rocket', 14)}<span>发布</span></button>
      </div>
      <div class="pub-status" id="pubStatus"></div>

      <!-- 适配器配置内联区：不放子弹窗（modal 为单例，会顶掉发布弹窗） -->
      <div class="pub-config" id="pubConfig" hidden>
        <div class="pub-config-head">
          <b id="pubConfigTitle">适配器配置</b>
          <span class="hint">密钥仅保存在本机</span>
        </div>
        <div id="pubConfigFields"></div>
        <div class="pub-config-foot">
          <button class="btn btn-primary btn-sm" id="pubConfigSave">${ui.icon('save', 14)}<span>保存配置</span></button>
        </div>
      </div>
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
      root.querySelector('#pubCopyText').onclick = async (e) => {
        const btn = e.currentTarget;
        btn.disabled = true;
        btn.textContent = '复制中…';
        try {
          const r = await api.copyArticle(art.id);
          await ui.copyRich(buildCopyHTML(r.text), r.text, '文章已复制到剪贴板');
        } catch (err) {
          ui.toast('复制失败', err.message, 'err');
        } finally {
          btn.disabled = false;
          btn.innerHTML = `${ui.icon('copy', 14)}<span>复制文章</span>`;
        }
      };
      root.querySelectorAll('[data-img]').forEach((btn) => {
        btn.onclick = () => copyImage(btn.dataset.img, btn);
      });

      /* ---------- 远程发布 ---------- */
      let selected = '';
      const picker = root.querySelector('#pubPicker');
      const status = root.querySelector('#pubStatus');

      const paintPicker = () => {
        if (!installed.length && !available.length) {
          picker.innerHTML = '<div class="pub-empty">暂无可用适配器</div>';
          return;
        }
        picker.innerHTML = [
          ...installed.map((p) => `
            <label class="pub-card ${p.missing ? 'is-missing' : ''}">
              <input type="radio" name="pubAdapter" value="${ui.esc(p.id)}" ${selected === p.id ? 'checked' : ''}>
              <span class="pub-card-body">
                <b>${ui.esc(p.name || p.id)}</b>
                <span class="pub-card-meta">v${ui.esc(p.version || '?')}${p.missing ? ' · 文件丢失，请重新下载' : ''}</span>
                ${p.desc ? `<span class="pub-card-desc">${ui.esc(p.desc)}</span>` : ''}
              </span>
            </label>`),
          ...available.filter((a) => !installed.some((p) => p.id === a.id)).map((a) => `
            <div class="pub-card is-available">
              <span class="pub-card-body">
                <b>${ui.esc(a.name || a.id)}</b>
                <span class="pub-card-meta">未安装</span>
                ${a.desc ? `<span class="pub-card-desc">${ui.esc(a.desc)}</span>` : ''}
              </span>
              <button class="btn btn-ghost btn-sm" data-install="${ui.esc(a.id)}" data-name="${ui.esc(a.name || '')}" data-desc="${ui.esc(a.desc || '')}">下载</button>
            </div>`),
        ].join('');
        picker.querySelectorAll('input[name="pubAdapter"]').forEach((r) => {
          r.onchange = () => { selected = r.value; };
        });
        picker.querySelectorAll('[data-install]').forEach((btn) => {
          btn.onclick = async () => {
            btn.disabled = true;
            btn.textContent = '下载中…';
            try {
              const p = await api.installPublisher(btn.dataset.install, btn.dataset.name, btn.dataset.desc);
              installed.push({ ...p, config: {}, missing: false });
              ui.toast('适配器已安装', p.name, 'ok');
              paintPicker();
            } catch (e) {
              btn.disabled = false;
              btn.textContent = '下载';
              ui.toast('下载失败', e.message, 'err', 6000);
            }
          };
        });
        if (!selected && installed.length && !installed[0].missing) selected = installed[0].id;
      };
      paintPicker();

      /* ---------- 适配器配置（内联展开） ---------- */
      let cfgFields = [];
      let cfgMeta = null;
      const cfgBox = root.querySelector('#pubConfig');
      const cfgFieldsBox = root.querySelector('#pubConfigFields');

      root.querySelector('#pubConfigBtn').onclick = async () => {
        if (!selected) { ui.toast('请先选择发布目标', '', 'warn'); return; }
        const meta = installed.find((p) => p.id === selected);
        if (!meta) return;
        if (!cfgBox.hidden && cfgMeta && cfgMeta.id === selected) {
          cfgBox.hidden = true; // 再次点击收起
          return;
        }
        let fields = [];
        try {
          const reg = await loadAdapter(selected);
          fields = Array.isArray(reg.configFields) ? reg.configFields : [];
        } catch (e) {
          ui.toast('加载适配器失败', e.message, 'err', 6000);
          return;
        }
        cfgFields = fields;
        cfgMeta = meta;
        root.querySelector('#pubConfigTitle').textContent = `${meta.name || selected} · 配置`;
        cfgFieldsBox.innerHTML = configFieldsHTML(fields, meta.config, 'pubcfg');
        cfgBox.hidden = false;
        cfgBox.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      };

      root.querySelector('#pubConfigSave').onclick = async (e) => {
        const btn = e.currentTarget;
        if (!cfgMeta) return;
        btn.disabled = true;
        const next = collectConfigFields(cfgFieldsBox, cfgFields, 'pubcfg');
        try {
          await api.savePublisherConfig(cfgMeta.id, next);
          cfgMeta.config = next;
          ui.toast('配置已保存', cfgMeta.name || cfgMeta.id, 'ok');
          cfgBox.hidden = true;
        } catch (err) {
          ui.toast('保存失败', err.message, 'err');
        } finally {
          btn.disabled = false;
        }
      };

      /* ---------- 发布 ---------- */
      root.querySelector('#pubSendBtn').onclick = async (e) => {
        const btn = e.currentTarget;
        if (!selected) { ui.toast('请先选择发布目标', '', 'warn'); return; }
        const meta = installed.find((p) => p.id === selected);
        if (!meta) { ui.toast('适配器未安装', '', 'warn'); return; }
        const cfg = meta.config || {};
        const missingReq = [];
        btn.disabled = true;
        status.className = 'pub-status is-busy';
        status.textContent = '正在准备…';
        try {
          const reg = await loadAdapter(selected);
          const fields = Array.isArray(reg.configFields) ? reg.configFields : [];
          fields.forEach((f) => { if (f.required && !cfg[f.key]) missingReq.push(f.label || f.key); });
          if (missingReq.length) {
            throw new Error(`请先配置：${missingReq.join('、')}`);
          }
          status.textContent = '正在处理图片…';
          const { markdown, warnings } = await resolveImages(art.content);
          const payload = {
            title: root.querySelector('#pubTitle').value.trim() || art.title || '未命名文章',
            tags: root.querySelector('#pubTags').value.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
            markdown,
            excerpt: excerptOf(art.content),
            date: new Date().toISOString(),
          };
          status.textContent = '正在发布…';
          const result = await reg.publish(payload, cfg);
          status.className = 'pub-status is-ok';
          status.innerHTML = `发布成功${result && result.url ? ` · <a href="${ui.esc(result.url)}" target="_blank" rel="noopener noreferrer">查看页面</a>` : ''}`;
          if (warnings.length) {
            ui.toast('部分图片未能转为公网链接', warnings.slice(0, 3).join('；'), 'warn', 8000);
          }
          ui.toast('发布成功', payload.title, 'ok');
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
