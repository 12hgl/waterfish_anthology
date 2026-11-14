/* 设置：发布设置 / 管理员设置 / 关于与更新
   账号与外观内置在侧栏（appearance.js），保存模型：改动即时保存。 */

import { api } from '../api.js';
import * as ui from '../ui.js';
import { openPublisherConfigDialog } from '../publish.js';

const TUTORIAL_URL = 'https://blog.waterfish.ren/article/4023794e-eb27-8082-a989-db0ca64a70c9';

export async function render(el, ctx) {
  const st = ctx.state || await ctx.refreshState();
  const isAdmin = !!st.user?.isAdmin || st.user?.source === 'local';
  const userName = st.user?.name || '未知';

  el.innerHTML = `
    <div class="page-head">
      <div>
        <h1 class="page-title">设置</h1>
      </div>
    </div>

    <div class="subnav" id="subnav">
      <button class="subnav-item is-active" data-tab="publish">发布设置</button>
      ${isAdmin ? '<button class="subnav-item" data-tab="admin">管理员设置</button>' : ''}
      <button class="subnav-item" data-tab="about">关于与更新</button>
    </div>

    <!-- 发布设置（所有账号可用；平台安装 / 卸载在管理员设置） -->
    <div data-panel="publish">
      <div class="card card-pad section">
        <div class="section-title">发布文章（Beta）</div>
        <div class="exp-row">
          <div class="exp-info">
            <b>启用发布</b>
            <p>开启后文章编辑页会显示「发布」按钮：可复制发布（有图床时图片自动转成直链），也可统一发布到已安装的平台。开关、密钥都按账号独立保存。</p>
          </div>
          <label class="switch" title="开启 / 关闭发布功能">
            <input type="checkbox" id="pubToggle" ${st.publishEnabled !== false ? 'checked' : ''}>
            <span class="switch-track"></span>
          </label>
        </div>
      </div>

      <div class="card card-pad section">
        <div class="section-title">平台密钥</div>
        <p class="section-desc">各发布平台的密钥在这里配置，仅保存在你自己的账号下。</p>
        <div class="pub-admin-list" id="pubKeys"></div>
      </div>
    </div>

    <!-- 管理员设置：平台安装 + 图床 + 存储路径 + GitHub 加速（仅管理员可见） -->
    ${isAdmin ? `
    <div data-panel="admin" hidden>
      <div class="card card-pad section">
        <div class="section-title">已安装平台</div>
        <p class="section-desc">装一次全体账号可用。</p>
        <div class="pub-admin-list" id="pubList"></div>
      </div>

      <div class="card card-pad section">
        <div class="section-title">下载安装（仅管理员）</div>
        <p class="section-desc">适配器代码托管在项目仓库，按需下载，不内置在安装包里。</p>
        <div class="pub-admin-list" id="pubAvailable"></div>
        <span class="hint" id="pubListError"></span>
      </div>

      <div class="card card-pad section">
        <div class="section-title">图床 · Cloudinary</div>

        <div class="field">
          <label>启用图床</label>
          <label style="display:flex;align-items:center;gap:10px;cursor:pointer;font-weight:400">
            <input type="checkbox" id="cloudOn" ${(st.cloudinary?.enabled) ? 'checked' : ''} style="width:16px;height:16px;accent-color:var(--accent)">
            <span id="cloudStateText">${st.cloudinary?.enabled ? '已启用' : '未启用'}</span>
          </label>
        </div>

        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:0 18px">
          <div class="field">
            <label for="cloudName">Cloud name</label>
            <input class="input" id="cloudName" placeholder="如 dxxxxxx" value="${ui.esc(st.cloudinary?.cloudName || '')}" spellcheck="false">
          </div>
          <div class="field">
            <label for="preset">Upload preset</label>
            <input class="input" id="preset" placeholder="如 waterfish_unsigned" value="${ui.esc(st.cloudinary?.uploadPreset || '')}" spellcheck="false">
          </div>
        </div>

        <div class="field" style="margin-bottom:0">
          <a class="tut-link" href="${TUTORIAL_URL}" target="_blank" rel="noopener noreferrer">${ui.icon('external', 14)}<span>获取教程：Cloudinary 上传凭证怎么拿</span></a>
        </div>
      </div>

      <div class="card card-pad section">
        <div class="section-title">本地存储路径</div>
        <p class="section-desc">切换路径会立即搬迁数据。</p>

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

      <div class="card card-pad section">
        <div class="section-title">GitHub 加速源</div>
        <p class="section-desc">「检查更新」与「下载发布适配器」需要访问 GitHub。默认直连；国内网络不通时可在此开启加速线路，仅对应用的这两类请求生效。</p>

        <div class="field">
          <label>启用加速</label>
          <label class="switch" title="开启 / 关闭 GitHub 加速">
            <input type="checkbox" id="ghProxyToggle">
            <span class="switch-track"></span>
          </label>
        </div>

        <div id="ghProxyBody" hidden>
          <div class="field">
            <label>加速线路</label>
            <div class="pub-picker" id="ghPresets"></div>
            <span class="hint">线路地址来自预设或你自己添加的条目；格式为「前缀 + 原始地址」。</span>
          </div>

          <div class="field">
            <label for="ghCustom">添加自定义线路</label>
            <div style="display:flex;gap:8px">
              <input class="input" id="ghCustom" placeholder="https://你的加速域名/" spellcheck="false" autocomplete="off">
              <button class="btn btn-ghost" id="ghAdd" style="flex:none">${ui.icon('plus', 14)}<span>添加</span></button>
            </div>
            <span class="hint">只接受 http(s) 前缀型代理，最多 8 条。</span>
          </div>

          <div id="ghCustomList" class="gh-list"></div>
        </div>

        <div style="display:flex;gap:10px;flex-wrap:wrap;margin-top:6px">
          <button class="btn btn-ghost" id="testGhProxy">${ui.icon('refresh')}<span>测试线路</span></button>
          <span class="hint" id="ghStatus"></span>
        </div>
      </div>
    </div>` : ''}

    <!-- 关于与更新 -->
    <div data-panel="about" hidden>
      <div class="card card-pad section">
        <div class="section-title">关于</div>
        <dl class="kv">
          <dt>应用名称</dt><dd>水鱼文集</dd>
          <dt>当前版本</dt><dd>v${ui.esc(st.version)}</dd>
          <dt>登录账号</dt><dd>${ui.esc(userName)}${isAdmin ? ' · 管理员' : ' · 成员'}${st.user?.source === 'local' ? ' · 局域网直连' : ''}</dd>
        </dl>
      </div>

      <div class="card card-pad section">
        <div class="section-title">检测更新</div>
        <p class="section-desc">当前版本 v${ui.esc(st.version)}</p>
        <div class="update-bar">
          <button class="btn btn-primary btn-sm" id="checkUpdateBtn">${ui.icon('refresh', 14)}<span>检查更新</span></button>
        </div>
        <div class="update-result" id="updateResult" hidden></div>
      </div>
    </div>

    <div style="height:20px" aria-hidden="true"></div>
  `;

  const $ = (s) => el.querySelector(s);

  /* ---------- 子菜单 ---------- */
  const tabs = [...el.querySelectorAll('.subnav-item')];
  const panels = [...el.querySelectorAll('[data-panel]')];
  tabs.forEach((t) => {
    t.onclick = () => {
      tabs.forEach((x) => x.classList.toggle('is-active', x === t));
      panels.forEach((p) => { p.hidden = p.dataset.panel !== t.dataset.tab; });
    };
  });

  /* ---------- 发布设置（即时保存） ---------- */
  const pubToggle = $('#pubToggle');
  pubToggle.onchange = async () => {
    const want = pubToggle.checked;
    pubToggle.disabled = true;
    try {
      await api.setPublishEnabled(want);
      if (ctx.state) ctx.state.publishEnabled = want;
      ui.toast(want ? '发布已开启' : '发布已关闭', '', 'ok', 2000);
    } catch (e) {
      pubToggle.checked = !want;
      ui.toast('保存失败', e.message, 'err', 5000);
    } finally {
      pubToggle.disabled = false;
    }
  };

  /* ---------- 平台密钥（所有账号可用，仅存自己的账号下） ---------- */
  const pubKeys = $('#pubKeys');
  const paintPubKeys = async () => {
    if (!pubKeys) return;
    pubKeys.innerHTML = '<div class="pub-admin-loading">加载中…</div>';
    let installed = [];
    try {
      installed = (await api.publishers()).installed || [];
    } catch (e) {
      pubKeys.innerHTML = `<div class="hint" style="color:var(--err)">加载失败：${ui.esc(e.message)}</div>`;
      return;
    }
    if (!installed.length) {
      pubKeys.innerHTML = '<div class="pub-admin-empty">还没有已安装的发布平台（由管理员在「管理员设置」中安装）</div>';
      return;
    }
    pubKeys.innerHTML = installed.map((p) => `
      <div class="pub-admin-row">
        <div class="pub-admin-info">
          <b>${ui.esc(p.name || p.id)}</b>
          <span class="pub-admin-meta">v${ui.esc(p.version || '?')}${p.config && Object.keys(p.config).length ? ' · 已配置' : ''}</span>
          ${p.desc ? `<span class="pub-admin-desc">${ui.esc(p.desc)}</span>` : ''}
        </div>
        <div class="pub-admin-ops">
          <button class="btn btn-ghost btn-sm" data-key-config="${ui.esc(p.id)}" ${p.missing ? 'disabled' : ''}>配置密钥</button>
        </div>
      </div>`).join('');
    pubKeys.querySelectorAll('[data-key-config]').forEach((btn) => {
      btn.onclick = () => {
        const meta = installed.find((p) => p.id === btn.dataset.keyConfig);
        if (meta) openPublisherConfigDialog(meta, paintPubKeys);
      };
    });
  };
  paintPubKeys();

  /* ---------- 管理员：平台安装 / 卸载 ---------- */
  const pubList = $('#pubList');
  const pubAvailable = $('#pubAvailable');
  const pubListError = $('#pubListError');

  const paintPublish = async () => {
    if (!pubList) return;
    pubList.innerHTML = '<div class="pub-admin-loading">加载中…</div>';
    let r;
    try {
      r = await api.publishers();
    } catch (e) {
      pubList.innerHTML = `<div class="hint" style="color:var(--err)">加载失败：${ui.esc(e.message)}</div>`;
      return;
    }
    const installed = r.installed || [];
    const available = (r.available || []).filter((a) => !installed.some((p) => p.id === a.id));

    pubList.innerHTML = installed.length
      ? installed.map((p) => `
        <div class="pub-admin-row">
          <div class="pub-admin-info">
            <b>${ui.esc(p.name || p.id)}</b>
            <span class="pub-admin-meta">v${ui.esc(p.version || '?')}${p.missing ? ' · 文件丢失，请重新安装' : ''}</span>
            ${p.desc ? `<span class="pub-admin-desc">${ui.esc(p.desc)}</span>` : ''}
          </div>
          <div class="pub-admin-ops">
            <button class="btn btn-ghost btn-sm" data-pub-config="${ui.esc(p.id)}" ${p.missing ? 'disabled' : ''}>配置</button>
            <button class="btn btn-danger btn-sm" data-pub-del="${ui.esc(p.id)}">卸载</button>
          </div>
        </div>`).join('')
      : `<div class="pub-admin-empty">还没有安装平台，可在下方按需下载</div>`;

    if (pubAvailable) {
      pubAvailable.innerHTML = available.length
        ? available.map((a) => `
          <div class="pub-admin-row is-available">
            <div class="pub-admin-info">
              <b>${ui.esc(a.name || a.id)}</b>
              <span class="pub-admin-meta">未安装${a.version ? ` · v${ui.esc(a.version)}` : ''}</span>
              ${a.desc ? `<span class="pub-admin-desc">${ui.esc(a.desc)}</span>` : ''}
            </div>
            <div class="pub-admin-ops">
              <button class="btn btn-ghost btn-sm" data-pub-install="${ui.esc(a.id)}"
                data-name="${ui.esc(a.name || '')}" data-desc="${ui.esc(a.desc || '')}">下载安装</button>
            </div>
          </div>`).join('')
        : '<div class="pub-admin-empty">仓库清单里没有可安装的平台</div>';
      pubListError.textContent = r.error || '';
      pubAvailable.querySelectorAll('[data-pub-install]').forEach((btn) => {
        btn.onclick = async () => {
          btn.disabled = true;
          btn.textContent = '下载中…';
          try {
            const p = await api.installPublisher(btn.dataset.pubInstall, btn.dataset.name, btn.dataset.desc);
            ui.toast('平台已安装', p.name || p.id, 'ok');
            paintPublish();
          paintPubKeys();
            paintPubKeys();
          } catch (e) {
            btn.disabled = false;
            btn.textContent = '下载安装';
            ui.toast('安装失败', e.message, 'err', 6000);
          }
        };
      });
    }

    pubList.querySelectorAll('[data-pub-config]').forEach((btn) => {
      btn.onclick = () => {
        const meta = installed.find((p) => p.id === btn.dataset.pubConfig);
        if (meta) openPublisherConfigDialog(meta, paintPublish);
      };
    });
    pubList.querySelectorAll('[data-pub-del]').forEach((btn) => {
      btn.onclick = async () => {
        const meta = installed.find((p) => p.id === btn.dataset.pubDel);
        const ok = await ui.confirmDialog({
          title: `卸载「${meta ? (meta.name || meta.id) : btn.dataset.pubDel}」？`,
          desc: '适配器文件与所有账号保存的该平台密钥都会被移除，且不可恢复。',
          okText: '卸载', danger: true,
        });
        if (!ok) return;
        try {
          await api.removePublisher(btn.dataset.pubDel);
          ui.toast('已卸载', '', 'ok');
          paintPublish();
          paintPubKeys();
        } catch (e) { ui.toast('卸载失败', e.message, 'err'); }
      };
    });
  };
  if (isAdmin) paintPublish();

  /* ---------- 管理员：GitHub 加速源（即时保存） ---------- */
  const ghToggle = $('#ghProxyToggle');
  const ghBody = $('#ghProxyBody');
  let gh = null;
  let paintGh = () => {};

  if (ghToggle && ghBody) {
    const presetsEl = $('#ghPresets');
    const customEl = $('#ghCustomList');
    const statusEl = $('#ghStatus');

    const commitGh = async () => {
      try {
        const r = await api.saveGithubProxy({ enabled: gh.enabled, active: gh.active, custom: gh.custom });
        gh = { ...gh, enabled: !!r.enabled, active: r.active || '', custom: r.custom || [] };
        paintGh();
        ui.toast('GitHub 加速设置已保存', '', 'ok', 1800);
      } catch (e) {
        ui.toast('GitHub 加速设置保存失败', e.message, 'err', 5000);
      }
    };

    const paintPresets = () => {
      const opts = [
        ...(gh.presets || []).map((p) => ({ url: p.url, name: p.name, desc: p.desc || '' })),
        ...(gh.custom || []).filter((u) => !(gh.presets || []).some((p) => p.url === u))
          .map((u) => ({ url: u, name: u.replace(/^https?:\/\//, '').replace(/\/$/, ''), desc: '自定义' })),
      ];
      presetsEl.innerHTML = opts.length
        ? opts.map((o) => `
          <label class="gh-opt ${gh.active === o.url ? 'is-active' : ''}" data-url="${ui.esc(o.url)}">
            <input type="radio" name="ghPreset" value="${ui.esc(o.url)}" ${gh.active === o.url ? 'checked' : ''}>
            <span class="gh-opt-name">${ui.esc(o.name)}</span>
            <span class="gh-opt-desc">${ui.esc(o.desc)}</span>
          </label>`).join('')
        : '<p class="hint">暂无可用线路，可添加自定义地址</p>';
      presetsEl.querySelectorAll('.gh-opt').forEach((n) => {
        n.onclick = () => { gh.active = n.dataset.url; paintPresets(); commitGh(); };
      });
    };

    const paintCustom = () => {
      customEl.innerHTML = (gh.custom || []).length
        ? gh.custom.map((u) => `
          <div class="gh-row">
            <span class="gh-url">${ui.esc(u)}</span>
            <button class="icon-btn danger" data-del="${ui.esc(u)}" title="删除">${ui.icon('trash', 15)}</button>
          </div>`).join('')
        : '<p class="hint" style="margin:0">还没有添加自定义线路</p>';
      customEl.querySelectorAll('[data-del]').forEach((n) => {
        n.onclick = () => {
          gh.custom = gh.custom.filter((x) => x !== n.dataset.del);
          if (gh.active === n.dataset.del) gh.active = '';
          paintCustom();
          paintPresets();
          commitGh();
        };
      });
    };

    paintGh = () => {
      ghToggle.checked = !!gh.enabled;
      ghBody.hidden = !gh.enabled;
      paintPresets();
      paintCustom();
    };

    try {
      const r = await api.githubProxy();
      gh = { enabled: !!r.enabled, active: r.active || '', custom: r.custom || [], presets: r.presets || [] };
      paintGh();
    } catch (e) {
      statusEl.textContent = '加载失败：' + e.message;
    }

    ghToggle.onchange = () => {
      gh.enabled = ghToggle.checked;
      ghBody.hidden = !gh.enabled;
      if (gh.enabled && !gh.active) {
        gh.active = (gh.presets[0] && gh.presets[0].url) || '';
        paintPresets();
      }
      commitGh();
    };

    $('#ghAdd').onclick = () => {
      const input = $('#ghCustom');
      let v = input.value.trim();
      if (!v) return;
      if (!/^https?:\/\//i.test(v)) {
        ui.toast('地址格式不对', '需以 http:// 或 https:// 开头', 'warn', 4500);
        return;
      }
      if (!v.endsWith('/')) v += '/';
      if (gh.custom.includes(v)) { ui.toast('已存在该线路', '', 'info'); return; }
      if (gh.custom.length >= 8) { ui.toast('最多 8 条自定义线路', '', 'warn'); return; }
      gh.custom.push(v);
      gh.active = v;
      input.value = '';
      paintCustom();
      paintPresets();
      commitGh();
    };

    $('#testGhProxy').onclick = async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      statusEl.textContent = '正在测试…';
      try {
        const r = await api.checkUpdate();
        if (r.error) statusEl.textContent = '测试失败：' + r.error;
        else statusEl.textContent = `连通正常 · 仓库最新 v${r.latest}`;
      } catch (err) {
        statusEl.textContent = '测试失败：' + err.message;
      } finally {
        btn.disabled = false;
      }
    };
  }

  /* ---------- 图床（输入防抖后即时保存） ---------- */
  const cloudOn = $('#cloudOn');
  if (cloudOn) {
    const commitCloudinary = ui.debounce(async (next) => {
      try {
        await api.saveSettings({ cloudinary: { ...next } });
        if (ctx.state) ctx.state.cloudinary = { ...(ctx.state.cloudinary || {}), ...next };
        ctx.syncCloudinary && ctx.syncCloudinary(next.enabled);
        ui.toast('图床设置已保存', '', 'ok', 1800);
      } catch (e) {
        ui.toast('图床设置保存失败', e.message, 'err', 5000);
      }
    }, 600);
    const snapshot = () => ({
      enabled: cloudOn.checked,
      cloudName: $('#cloudName').value.trim(),
      uploadPreset: $('#preset').value.trim(),
    });
    cloudOn.onchange = () => {
      $('#cloudStateText').textContent = cloudOn.checked ? '已启用' : '未启用';
      commitCloudinary(snapshot());
    };
    $('#cloudName').oninput = () => commitCloudinary(snapshot());
    $('#preset').oninput = () => commitCloudinary(snapshot());
  }

  /* ---------- 存储路径（立即生效） ---------- */
  const changeBtn = $('#changeStorage');
  changeBtn && (changeBtn.onclick = () => openBrowser(ctx));
  const resetBtn = $('#resetStorage');
  resetBtn && (resetBtn.onclick = async () => {
    const ok = await ui.confirmDialog({
      title: '恢复到应用数据目录？',
      desc: '当前目录下的文章、图片与检测记录会被移动到应用数据目录，原目录内本应用的文件将被清空。',
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

  /* ---------- 检测更新 ---------- */
  const paintUpdate = (r) => {
    const box = $('#updateResult');
    box.hidden = false;
    if (r.error) {
      box.innerHTML = `<div class="update-card is-err">${ui.icon('alert', 18)}<div class="update-body"><b>检查失败</b><p>${ui.esc(r.error)}</p></div></div>`;
      return;
    }
    if (!r.hasUpdate) {
      const sameVer = r.latest && r.latest === r.current;
      box.innerHTML = `<div class="update-card is-ok">${ui.icon('check', 18)}<div class="update-body"><b>已是最新版本</b><p>当前 v${ui.esc(r.current)}${sameVer ? ` · 仓库最新 v${ui.esc(r.latest)}` : ''}</p></div></div>`;
      return;
    }
    const link = (label, url, icon) => url
      ? `<a class="btn btn-ghost btn-sm" href="${ui.esc(url)}" target="_blank" rel="noopener noreferrer">${ui.icon(icon, 14)}<span>${label}</span></a>`
      : '';
    const acc = (u) => 'https://gh-proxy.com/' + u;
    box.innerHTML = `
      <div class="update-card is-new">
        <div class="update-head">${ui.icon('rocket', 18)}
          <div class="update-body">
            <b>发现新版本 v${ui.esc(r.latest)}</b>
            <p>当前 v${ui.esc(r.current)}${r.date ? ` · 发布于 ${ui.esc(r.date)}` : ''}</p>
          </div>
        </div>
        ${r.notes ? `<pre class="update-notes">${ui.esc(r.notes)}</pre>` : ''}
        <div class="update-actions">
          ${link('x86 安装包', r.x86, 'download')}
          ${link('x86 加速下载', acc(r.x86), 'download')}
          ${link('ARM 安装包', r.arm, 'download')}
          ${link('ARM 加速下载', acc(r.arm), 'download')}
          ${link('Release 页面', r.release, 'external')}
        </div>
      </div>`;
  };

  $('#checkUpdateBtn').onclick = async (e) => {
    const btn = e.currentTarget;
    const label = btn.querySelector('span');
    btn.disabled = true;
    label.textContent = '检查中…';
    try {
      paintUpdate(await api.checkUpdate());
    } catch (err) {
      paintUpdate({ error: err.message });
    } finally {
      btn.disabled = false;
      label.textContent = '检查更新';
    }
  };
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
          desc: `文章、图片与检测记录将移动到 ${current} 下你的专属文件夹。`,
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
