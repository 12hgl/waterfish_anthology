/* 后端接口封装 */

const JSON_HEADERS = { 'Content-Type': 'application/json' };

async function req(path, opts = {}) {
  const res = await fetch(path, { headers: JSON_HEADERS, ...opts });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (!res.ok) {
    const msg = (data && (data.error || data.message)) || `请求失败 (${res.status})`;
    throw new Error(msg);
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
  saveSettings: (body) => req('api/settings', { method: 'PUT', body: JSON.stringify(body) }),
  setStorage: (path) => req('api/settings/storage', { method: 'POST', body: JSON.stringify({ path }) }),
  browse: (path) => req('api/browse' + (path ? `?path=${enc(path)}` : '')),
};
