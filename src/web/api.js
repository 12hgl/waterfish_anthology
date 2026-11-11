/* 后端接口封装 */

const JSON_HEADERS = { 'Content-Type': 'application/json' };

async function req(path, opts = {}) {
  // X-App-Language 供后端调用 trim.file.convertPath 时使用当前界面语言
  const headers = {
    'X-App-Language': (typeof navigator !== 'undefined' && navigator.language) || 'zh-CN',
    ...(opts.headers || JSON_HEADERS),
  };
  const res = await fetch(path, { ...opts, headers });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (!res.ok) {
    const msg = (data && (data.error || data.message)) || `请求失败 (${res.status})`;
    const err = new Error(msg);
    err.status = res.status;
    err.data = data;
    throw err;
  }
  return data;
}

const enc = encodeURIComponent;

export const api = {
  state: () => req('api/state'),

  articles: () => req('api/articles'),
  article: (id) => req(`api/articles/${enc(id)}`),
  createArticle: (body) => req('api/articles', { method: 'POST', body: JSON.stringify(body) }),
  updateArticle: (id, body) => req(`api/articles/${enc(id)}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteArticle: (id) => req(`api/articles/${enc(id)}`, { method: 'DELETE' }),
  copyArticle: (id) => req(`api/articles/${enc(id)}/copy`),
  articleEditor: (id) => req(`api/articles/${enc(id)}/editor`),
  saveArticleEditor: (id, body) => req(`api/articles/${enc(id)}/editor`, { method: 'PUT', body: JSON.stringify(body) }),

  images: () => req('api/images'),
  deleteImage: (name) => req(`api/images/local?name=${enc(name)}`, { method: 'DELETE' }),
  // 批量删除：{ local: [文件名], cloud: [publicId 或序号] }
  deleteImages: (body) => req('api/images/batch', { method: 'POST', body: JSON.stringify(body) }),
  upload: (file) => {
    const fd = new FormData();
    fd.append('file', file, file.name || 'image.png');
    return req('api/uploads', { method: 'POST', body: fd, headers: {} });
  },

  seoList: () => req('api/seo'),
  seoCheck: (url) => req('api/seo', { method: 'POST', body: JSON.stringify({ url }) }),
  seoGet: (id) => req(`api/seo/${enc(id)}`),
  seoRecheck: (id) => req(`api/seo/${enc(id)}/recheck`, { method: 'POST' }),
  seoDelete: (id) => req(`api/seo/${enc(id)}`, { method: 'DELETE' }),

  settings: () => req('api/settings'),

  // 管理员：GitHub 加速源（系统级，全用户生效）
  githubProxy: () => req('api/settings/github'),
  saveGithubProxy: (body) => req('api/settings/github', { method: 'PUT', body: JSON.stringify(body) }),
  saveSettings: (body) => req('api/settings', { method: 'PUT', body: JSON.stringify(body) }),
  // 发布功能开关（每账号独立，默认开启）
  setPublishEnabled: (enabled) => req('api/settings', { method: 'PUT', body: JSON.stringify({ publishEnabled: enabled }) }),
  setStorage: (path) => req('api/settings/storage', { method: 'POST', body: JSON.stringify({ path }) }),
  browse: (path) => req('api/browse' + (path ? `?path=${enc(path)}` : '')),

  // 版本更新检测（服务端拉取仓库 latest 分支 releases.js，直连失败自动走加速线路）
  checkUpdate: () => req('api/update-check'),

  // 发布适配器（实验性功能）
  publishers: () => req('api/publishers'),
  installPublisher: (id, name, desc) => req('api/publishers', {
    method: 'POST', body: JSON.stringify({ id, name, desc }),
  }),
  savePublisherConfig: (id, config) => req(`api/publishers/${enc(id)}`, {
    method: 'PUT', body: JSON.stringify({ config }),
  }),
  removePublisher: (id) => req(`api/publishers/${enc(id)}`, { method: 'DELETE' }),

  // 外部 Markdown 文件（文件管理器「打开方式」与导入文章）
  externalCaps: () => req('api/external/caps'),
  externalOpen: (path) => req('api/external/open', { method: 'POST', body: JSON.stringify({ path }) }),
  externalSave: (path, title, content) => req('api/external/save', {
    method: 'PUT', body: JSON.stringify({ path, title, content }),
  }),
  externalGrant: (path) => req('api/external/grant', { method: 'POST', body: JSON.stringify({ path }) }),
  externalScan: (path) => req('api/external/scan', { method: 'POST', body: JSON.stringify({ path }) }),
  externalRoots: () => req('api/external/roots'),
  externalImport: (paths) => req('api/external/import', { method: 'POST', body: JSON.stringify({ paths }) }),
  externalFormats: () => req('api/external/formats'),
  // 浏览器原生预览地址（PDF 用内置阅读器渲染）
  externalRawURL: (path) => `api/external/raw?path=${enc(path)}`,
  // 加入文章库（不复制内容，正文始终读写原文件）
  externalLink: (path, title) => req('api/external/link', { method: 'POST', body: JSON.stringify({ path, title }) }),
  externalUnlink: (id) => req('api/external/unlink', { method: 'POST', body: JSON.stringify({ id }) }),
};
