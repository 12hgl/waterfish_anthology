/* 与飞牛系统的对接：官方 JS SDK 装载、目录/文件授权、外部文件导入 */

import { api } from './api.js';
import * as ui from './ui.js';

/* ---------------- 官方 JS SDK ---------------- */

let sdkPromise = null;

// getSDK 懒加载内置的飞牛前端 SDK（static/vendor/trim-web-app.js）。
// 非宿主环境（如浏览器直连端口）初始化会失败，返回 null 由调用方降级。
export function getSDK() {
  if (sdkPromise) return sdkPromise;
  sdkPromise = (async () => {
    try {
      const mod = await import('./vendor/trim-web-app.js');
      const sdk = new mod.TrimApp();
      // 探一次能力，确认宿主桥可用
      if (typeof sdk.isWeb !== 'boolean') return null;
      return sdk;
    } catch {
      return null;
    }
  })();
  return sdkPromise;
}

// isStandalone 页面是否运行在独立浏览器里（没有宿主桥，授权要走跳转路由）
export function isStandalone(sdk) {
  try { return !sdk || sdk.isStandaloneWeb === true; } catch { return true; }
}

// withTimeout 给宿主调用兜底，避免弹窗未响应时界面永久卡住
function withTimeout(promise, ms, label) {
  return Promise.race([
    promise,
    new Promise((_, reject) => setTimeout(() => reject(new Error(`${label}超时，请重试`)), ms)),
  ]);
}

/* ---------------- 授权 ---------------- */

// authorizePath 请求用户授权某个文件或目录。
// 成功后系统会把目标路径的 ACL 授予应用运行用户，随后由后端登记到本用户的可访问范围。
export async function authorizePath(path, { directory = false } = {}) {
  const sdk = await getSDK();
  if (!sdk) {
    return { ok: false, reason: '当前环境没有飞牛系统授权能力，请在飞牛桌面或文件管理器中打开应用后重试' };
  }
  if (isStandalone(sdk)) {
    return { ok: false, reason: '请从飞牛桌面打开水鱼文集后再完成授权' };
  }
  try {
    const res = await withTimeout(sdk.authorizeUserFile(path), 120000, '系统授权');
    const granted = res && (res.code === 0 || res.code === undefined) && res.data !== false && res.data !== null;
    if (!granted) {
      return { ok: false, reason: (res && res.msg) || '授权未完成' };
    }
    // 登记到本用户的可访问范围（后端会再确认应用确实能访问该路径）
    await api.externalGrant(path);
    return { ok: true, directory };
  } catch (e) {
    return { ok: false, reason: e.message || '授权失败' };
  }
}

// pickDirectory 打开系统目录选择器并授权该目录。
export async function pickDirectory() {
  const sdk = await getSDK();
  if (!sdk) return { ok: false, reason: '当前环境不支持系统目录选择' };
  if (isStandalone(sdk)) return { ok: false, reason: '请从飞牛桌面打开水鱼文集后再选择文件夹' };
  try {
    const res = await withTimeout(sdk.pickUserFile({
      directory: true,
      title: '选择要导入的文件夹',
      okText: '授权并继续',
      sidebarGroup: ['myFiles', 'otherShare', 'favorites'],
    }), 300000, '目录选择');
    const paths = (res && Array.isArray(res.data)) ? res.data : [];
    if (!paths.length) return { ok: false, reason: '未选择文件夹' };
    await api.externalGrant(paths[0]);
    return { ok: true, path: paths[0] };
  } catch (e) {
    return { ok: false, reason: e.message || '选择文件夹失败' };
  }
}

// revealInManager 在系统文件管理器中定位文件（宿主不支持时静默失败）
export async function revealInManager(path) {
  const sdk = await getSDK();
  if (!sdk) return false;
  try {
    await sdk.openFileManager(path);
    return true;
  } catch {
    return false;
  }
}

/* ---------------- 导入文章 ---------------- */
function fmtSize(n) {
  if (!n) return '0 B';
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(1) + ' MB';
}

// openImportDialog 从授权目录导入 Markdown 为文集文章。
// 来源优先级：系统目录选择器（可授权任意目录）→ 已授权目录列表。
export function openImportDialog(onDone) {
  let files = [];
  let currentDir = '';

  const close = ui.modal(`
    <div class="modal-head">
      <h3>导入 Markdown 文章</h3>
      <p>从飞牛磁盘中选择文件，复制为文集文章（原文件保持不变）。</p>
    </div>
    <div class="modal-body">
      <div class="imp-bar">
        <button class="btn btn-primary btn-sm" id="impPick">${ui.icon('folder', 14)}<span>选择文件夹</span></button>
        <span class="imp-dir" id="impDir">尚未选择文件夹</span>
      </div>
      <div class="imp-list" id="impList">
        <div class="imp-empty">选择文件夹后会列出其中的 Markdown 文件（含子目录，最多 3 层）</div>
      </div>
      <p class="imp-hint" id="impHint"></p>
    </div>
    <div class="modal-foot">
      <button class="btn btn-ghost" data-act="cancel">取消</button>
      <button class="btn btn-primary" data-act="import" disabled>导入所选</button>
    </div>`, {
    wide: true,
    onMount(root, done) {
      const listEl = root.querySelector('#impList');
      const dirEl = root.querySelector('#impDir');
      const hintEl = root.querySelector('#impHint');
      const pickBtn = root.querySelector('#impPick');
      const importBtn = root.querySelector('[data-act="import"]');

      const paint = () => {
        if (!files.length) {
          listEl.innerHTML = `<div class="imp-empty">该文件夹里没有可导入的文件</div>`;
          importBtn.disabled = true;
          return;
        }
        listEl.innerHTML = files.map((f, i) => `
          <label class="imp-row">
            <input type="checkbox" data-i="${i}" checked>
            <span class="imp-name">${ui.esc(f.title || f.name)}</span>
            <span class="imp-sub">${ui.esc(f.name)} · ${fmtSize(f.size)}</span>
          </label>`).join('');
        importBtn.disabled = false;
        listEl.querySelectorAll('input[type="checkbox"]').forEach((c) => {
          c.onchange = () => { importBtn.disabled = !listEl.querySelector('input:checked'); };
        });
      };

      const scan = async (dir) => {
        currentDir = dir;
        dirEl.textContent = dir;
        dirEl.title = dir;
        hintEl.textContent = '';
        listEl.innerHTML = `<div class="imp-empty"><span class="spinner"></span> 正在扫描…</div>`;
        try {
          const r = await api.externalScan(dir);
          files = Array.isArray(r.files) ? r.files : [];
          paint();
        } catch (e) {
          files = [];
          listEl.innerHTML = `<div class="imp-empty" style="color:var(--err)">${ui.esc(e.message)}</div>`;
          importBtn.disabled = true;
        }
      };

      pickBtn.onclick = async () => {
        pickBtn.disabled = true;
        pickBtn.innerHTML = `<span class="spinner"></span><span>等待选择…</span>`;
        const picked = await pickDirectory();
        pickBtn.disabled = false;
        pickBtn.innerHTML = `${ui.icon('folder', 14)}<span>选择文件夹</span>`;
        if (!picked.ok) {
          if (picked.reason && picked.reason !== '未选择文件夹') {
            ui.toast('无法选择文件夹', picked.reason, 'warn', 7000);
          }
          await offerLocalRoots(scan);
          return;
        }
        await scan(picked.path);
      };

      importBtn.onclick = async () => {
        const chosen = [...listEl.querySelectorAll('input[type="checkbox"]')]
          .filter((c) => c.checked)
          .map((c) => files[Number(c.dataset.i)].path)
          .filter(Boolean);
        if (!chosen.length) return;
        importBtn.disabled = true;
        importBtn.innerHTML = `<span class="spinner"></span><span>导入中…</span>`;
        try {
          const r = await api.externalImport(chosen);
          const ok = (r.imported || []).length;
          const skipped = (r.skipped || []).length;
          if (ok) ui.toast(`已导入 ${ok} 篇文章`, skipped ? `${skipped} 个文件被跳过` : '', 'ok', 6000);
          else ui.toast('没有导入任何文件', skipped ? `${skipped} 个文件被跳过` : '', 'warn', 6000);
          done();
          onDone && onDone(r);
        } catch (e) {
          ui.toast('导入失败', e.message, 'err', 7000);
          importBtn.disabled = false;
          importBtn.innerHTML = '导入所选';
        }
      };

      root.querySelector('[data-act="cancel"]').onclick = done;
      root.querySelector('[data-act="cancel"]').textContent = '关闭';
      // 打开即尝试列出已授权目录，方便再次导入
      offerLocalRoots(scan);
    },
  });
  return close;
}

// offerLocalRoots 在没有系统选择器时，退回到「已授权目录」列表供选择
async function offerLocalRoots(scan) {
  let roots = [];
  try {
    const r = await api.externalRoots();
    roots = (r.roots || []).filter((x) => x.path);
  } catch { /* 忽略 */ }
  const listEl = document.querySelector('.modal .imp-list');
  if (!listEl || !roots.length) return;
  listEl.innerHTML = `
    <div class="imp-empty">也可以从已授权的目录导入：</div>
    ${roots.map((r) => `<div class="imp-root" data-path="${ui.esc(r.path)}">${ui.icon('folder', 16)}<span>${ui.esc(r.path)}</span></div>`).join('')}`;
  listEl.querySelectorAll('.imp-root').forEach((n) => {
    n.onclick = () => scan(n.dataset.path);
  });
}
