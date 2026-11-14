/* WordPress 发布适配器
 *
 * 通过 WordPress REST API（/wp-json/wp/v2）把文章发布为站点文章：
 *   1. 用「用户名 + 应用密码」做 Basic 认证（不要用登录密码）；
 *   2. 标签按名称查找，缺的自动创建；
 *   3. 正文从 Markdown 转成 HTML 后提交，带标题、摘要、标签。
 *
 * 适配器契约（见 publishers/README.md）：
 *   id / name / desc / version / configFields / publish(payload, config)
 *   payload = { title, tags[], markdown, excerpt, date }
 *   成功返回 { url }，失败 throw Error（message 直接展示给用户）
 *
 * 本文件不内置在应用里，由「设置 → 更新与实验 → 实验性功能」按需下载，可独立更新。
 */
(function () {
  'use strict';

  /* ---------------- Markdown → HTML（够用即可，不追求完整规范） ---------------- */

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function inline(s) {
    return s
      .replace(/!\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g, (m, alt, src) =>
        '<img src="' + esc(src) + '" alt="' + esc(alt) + '">')
      .replace(/\[([^\]]+)\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g, (m, text, href) =>
        '<a href="' + esc(href) + '">' + text + '</a>')
      .replace(/`([^`]+)`/g, (m, code) => '<code>' + code + '</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>')
      .replace(/~~([^~]+)~~/g, '<del>$1</del>');
  }

  function mdToHtml(md) {
    var lines = String(md || '').replace(/\r\n/g, '\n').split('\n');
    var out = [];
    var i = 0;
    var inCode = false;
    var codeBuf = [];
    var listType = '';
    var para = [];

    function flushPara() {
      if (para.length) {
        out.push('<p>' + inline(para.join(' ')) + '</p>');
        para = [];
      }
    }
    function closeList() {
      if (listType) {
        out.push(listType === 'ul' ? '</ul>' : '</ol>');
        listType = '';
      }
    }

    while (i < lines.length) {
      var line = lines[i];

      if (/^```/.test(line)) {
        if (inCode) {
          out.push('<pre><code>' + esc(codeBuf.join('\n')) + '</code></pre>');
          codeBuf = [];
          inCode = false;
        } else {
          flushPara(); closeList();
          inCode = true;
        }
        i++;
        continue;
      }
      if (inCode) { codeBuf.push(line); i++; continue; }

      if (/^\s*$/.test(line)) { flushPara(); closeList(); i++; continue; }

      var h = line.match(/^(#{1,6})\s+(.*)$/);
      if (h) {
        flushPara(); closeList();
        var lv = h[1].length;
        out.push('<h' + lv + '>' + inline(h[2].trim()) + '</h' + lv + '>');
        i++;
        continue;
      }

      if (/^\s*(---|\*\*\*)\s*$/.test(line)) {
        flushPara(); closeList();
        out.push('<hr>');
        i++;
        continue;
      }

      var q = line.match(/^>\s?(.*)$/);
      if (q) {
        flushPara(); closeList();
        var buf = [];
        while (i < lines.length && /^>\s?/.test(lines[i])) {
          buf.push(lines[i].replace(/^>\s?/, ''));
          i++;
        }
        out.push('<blockquote><p>' + inline(buf.join(' ')) + '</p></blockquote>');
        continue;
      }

      var ul = line.match(/^\s*[-*+]\s+(.*)$/);
      var ol = line.match(/^\s*\d+\.\s+(.*)$/);
      if (ul || ol) {
        flushPara();
        var want = ul ? 'ul' : 'ol';
        if (listType !== want) { closeList(); out.push('<' + want + '>'); listType = want; }
        out.push('<li>' + inline((ul ? ul[1] : ol[1]).trim()) + '</li>');
        i++;
        continue;
      }

      if (/^\s*\|.*\|\s*$/.test(line) && i + 1 < lines.length && /^\s*\|[\s:|-]+\|\s*$/.test(lines[i + 1])) {
        flushPara(); closeList();
        var rows = [];
        while (i < lines.length && /^\s*\|.*\|\s*$/.test(lines[i])) {
          rows.push(lines[i].trim());
          i++;
        }
        var cells = function (row) {
          return row.replace(/^\||\|$/g, '').split('|').map(function (c) { return c.trim(); });
        };
        var html = ['<table><thead><tr>'];
        cells(rows[0]).forEach(function (c) { html.push('<th>' + inline(c) + '</th>'); });
        html.push('</tr></thead><tbody>');
        rows.slice(2).forEach(function (r) {
          html.push('<tr>');
          cells(r).forEach(function (c) { html.push('<td>' + inline(c) + '</td>'); });
          html.push('</tr>');
        });
        html.push('</tbody></table>');
        out.push(html.join(''));
        continue;
      }

      para.push(line.trim());
      i++;
    }

    if (inCode && codeBuf.length) out.push('<pre><code>' + esc(codeBuf.join('\n')) + '</code></pre>');
    flushPara();
    closeList();
    return out.join('\n');
  }

  /* ---------------- 接口调用 ---------------- */

  function trimBase(url) {
    var u = String(url || '').trim().replace(/\/+$/, '');
    if (!/^https?:\/\//i.test(u)) throw new Error('站点地址需以 http:// 或 https:// 开头');
    return u;
  }

  function authHeader(cfg) {
    var raw = (cfg.username || '') + ':' + (cfg.appPassword || '');
    return 'Basic ' + btoa(unescape(encodeURIComponent(raw)));
  }

  function req(cfg, path, opts) {
    opts = opts || {};
    var base = trimBase(cfg.siteUrl);
    var headers = { 'Content-Type': 'application/json', Authorization: authHeader(cfg) };
    return fetch(base + '/wp-json/wp/v2' + path, {
      method: opts.method || 'GET',
      headers: headers,
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    }).then(function (res) {
      return res.text().then(function (text) {
        var data = null;
        try { data = text ? JSON.parse(text) : null; } catch (e) { /* 非 JSON */ }
        if (!res.ok) {
          var msg = (data && data.message) ? data.message : ('HTTP ' + res.status);
          if (res.status === 401) msg = '认证失败：请检查用户名与应用密码（不是登录密码）';
          if (res.status === 403) msg = '无权发布：该账号需要「编辑文章」及以上权限';
          if (res.status === 404) msg = '接口不存在：请确认站点地址正确，且未禁用 REST API';
          throw new Error(msg);
        }
        return data;
      });
    }).catch(function (e) {
      if (e && e.name === 'TypeError') {
        throw new Error('无法连接站点：请检查地址与网络，或站点是否允许跨域访问（REST API 被安全插件拦截时也会这样）');
      }
      throw e;
    });
  }

  // 标签按名称查询，缺的创建；返回 id 数组
  function ensureTags(cfg, names) {
    var ids = [];
    var chain = Promise.resolve();
    names.forEach(function (name) {
      chain = chain.then(function () {
        return req(cfg, '/tags?search=' + encodeURIComponent(name) + '&per_page=100')
          .then(function (list) {
            var hit = (list || []).filter(function (t) { return t.name === name; })[0];
            if (hit) { ids.push(hit.id); return null; }
            return req(cfg, '/tags', { method: 'POST', body: { name: name } })
              .then(function (t) { if (t && t.id) ids.push(t.id); })
              .catch(function () { /* 创建标签失败不阻断发布 */ });
          })
          .catch(function () { /* 查询失败不阻断发布 */ });
      });
    });
    return chain.then(function () { return ids; });
  }

  /* ---------------- 注册 ---------------- */

  window.WFPublishers = window.WFPublishers || {};

  window.WFPublishers.wordpress = {
    id: 'wordpress',
    name: 'WordPress',
    desc: '通过 WordPress REST API 发布文章（标题 / 标签 / 正文 / 摘要），支持草稿或直接发布',
    version: '1.0.0',

    configFields: [
      {
        key: 'siteUrl', label: '站点地址', type: 'text', required: true,
        hint: '如 https://blog.example.com（不要带 /wp-admin）',
      },
      {
        key: 'username', label: '用户名', type: 'text', required: true,
        hint: '有「编辑文章」权限的账号，建议单独建一个发布账号',
      },
      {
        key: 'appPassword', label: '应用密码', type: 'password', required: true,
        hint: 'WordPress 后台 → 用户 → 个人资料 → 应用程序密码，生成后形如 xxxx xxxx xxxx xxxx',
      },
      {
        key: 'status', label: '发布状态', type: 'text', required: false,
        hint: 'publish 直接发布，draft 存为草稿，默认 publish',
      },
      {
        key: 'category', label: '分类 ID', type: 'text', required: false,
        hint: '可选。留空则用站点默认分类',
      },
    ],

    publish: function (payload, config) {
      var cfg = config || {};
      if (!cfg.siteUrl) throw new Error('请先在设置里填写站点地址');
      if (!cfg.username || !cfg.appPassword) throw new Error('请先填写用户名与应用密码');

      var status = (cfg.status || 'publish').trim();
      if (status !== 'publish' && status !== 'draft' && status !== 'pending' && status !== 'private') {
        status = 'publish';
      }
      var tags = (payload.tags || []).map(function (t) { return String(t).trim(); }).filter(Boolean).slice(0, 10);

      var body = {
        title: payload.title || '未命名文章',
        content: mdToHtml(payload.markdown || ''),
        status: status,
      };
      if (payload.excerpt) body.excerpt = payload.excerpt;
      if (payload.date) {
        var d = new Date(payload.date);
        if (!isNaN(d.getTime())) body.date = d.toISOString();
      }
      var catId = parseInt(String(cfg.category || '').trim(), 10);
      if (!isNaN(catId) && catId > 0) body.categories = [catId];

      return ensureTags(cfg, tags).then(function (ids) {
        if (ids.length) body.tags = ids;
        return req(cfg, '/posts', { method: 'POST', body: body });
      }).then(function (post) {
        if (!post || !post.id) throw new Error('发布结果异常：站点未返回文章 ID');
        return { url: post.link || (trimBase(cfg.siteUrl) + '/?p=' + post.id) };
      });
    },
  };
})();
