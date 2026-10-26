/* NotionNext 发布适配器
 *
 * 把文章发布为 Notion 数据库页面（NotionNext 等建站系统的数据源）。
 * 本文件由水鱼文集「设置 → 更新与实验 → 实验性功能」按需下载，
 * 不内置在应用内，可独立更新。应用通过 <script> 加载后读取
 * window.WFPublishers.notionnext 并调用其 publish()。
 *
 * 适配器契约：
 *   id / name / desc / version      基本信息（展示用）
 *   configFields                    配置项 [{ key, label, type, required, hint }]
 *   publish(payload, config)        Promise<{ url }>；失败时 throw Error（message 直接展示给用户）
 *
 * payload = {
 *   title:    string,            文章标题
 *   tags:     string[],          标签
 *   markdown: string,            正文（图片已是可公开访问的绝对 URL）
 *   excerpt:  string,            摘要
 *   date:     string,            ISO 时间
 * }
 */
(function () {
  'use strict';

  var API = 'https://api.notion.com/v1';
  var NOTION_VERSION = '2022-06-28';

  // Notion 代码块语言白名单（其余按纯文本处理）
  var LANG_MAP = {
    js: 'javascript', javascript: 'javascript', jsx: 'javascript',
    ts: 'typescript', typescript: 'typescript', tsx: 'typescript',
    py: 'python', python: 'python', rb: 'ruby', ruby: 'ruby',
    go: 'go', rust: 'rust', rs: 'rust', java: 'java', kt: 'kotlin',
    c: 'c', cpp: 'c++', 'c++': 'c++', cs: 'c#', 'c#': 'c#',
    php: 'php', swift: 'swift', sh: 'shell', bash: 'shell', zsh: 'shell', shell: 'shell',
    sql: 'sql', json: 'json', yaml: 'yaml', yml: 'yaml', toml: 'toml',
    html: 'html', xml: 'xml', css: 'css', scss: 'scss', less: 'less',
    md: 'markdown', markdown: 'markdown', diff: 'diff', dockerfile: 'dockerfile',
    makefile: 'makefile', nginx: 'nginx', ini: 'ini', text: 'plain text', txt: 'plain text',
  };

  /* ---------- 行内 Markdown → Notion rich_text ---------- */

  function pushText(out, content, ann) {
    if (!content) return;
    // Notion 单个 rich_text 上限 2000 字符，分段兜底
    for (var i = 0; i < content.length; i += 1900) {
      var item = { type: 'text', text: { content: content.slice(i, i + 1900) } };
      if (ann) item.annotations = ann;
      out.push(item);
    }
  }

  function inlineRichText(text) {
    var out = [];
    var pattern = /(\*\*|__)(.+?)\1|(\*|_)(.+?)\3|~~(.+?)~~|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)/g;
    var last = 0, m;
    while ((m = pattern.exec(text))) {
      pushText(out, text.slice(last, m.index));
      if (m[2] !== undefined) pushText(out, m[2], { bold: true });
      else if (m[4] !== undefined) pushText(out, m[4], { italic: true });
      else if (m[5] !== undefined) pushText(out, m[5], { strikethrough: true });
      else if (m[6] !== undefined) pushText(out, m[6], { code: true });
      else if (m[7] !== undefined) out.push({ type: 'text', text: { content: m[7], link: { url: m[8] } } });
      last = m.index + m[0].length;
    }
    pushText(out, text.slice(last));
    if (!out.length) out.push({ type: 'text', text: { content: '' } });
    return out;
  }

  /* ---------- Markdown → Notion 块 ---------- */

  function mdToBlocks(md) {
    var text = String(md || '').replace(/^\uFEFF/, '').replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, '');
    var lines = text.split(/\r?\n/);
    var blocks = [], para = [], tableBuf = [];
    var i = 0;

    function flushPara() {
      if (!para.length) return;
      var content = para.join('\n').trim();
      if (content) blocks.push({ object: 'block', type: 'paragraph', paragraph: { rich_text: inlineRichText(content) } });
      para.length = 0;
    }
    function flushTable() {
      if (!tableBuf.length) return;
      blocks.push({ object: 'block', type: 'code', code: { rich_text: inlineRichText(tableBuf.join('\n')), language: 'plain text' } });
      tableBuf.length = 0;
    }

    while (i < lines.length) {
      var line = lines[i];
      var t = line.trim();

      var fence = t.match(/^```(\w*)/);
      if (fence) {
        flushPara(); flushTable();
        var lang = LANG_MAP[(fence[1] || '').toLowerCase()] || 'plain text';
        var code = [];
        i++;
        while (i < lines.length && !/^```/.test(lines[i].trim())) { code.push(lines[i]); i++; }
        i++;
        var content = code.join('\n');
        if (content) blocks.push({ object: 'block', type: 'code', code: { rich_text: inlineRichText(content), language: lang } });
        continue;
      }

      if (/^\|/.test(t)) { // 表格按原文保留为代码块，结构不错乱
        flushPara();
        tableBuf.push(t.replace(/^\||\|$/g, '').split('|').map(function (c) { return c.trim(); }).join(' | '));
        i++;
        continue;
      }
      flushTable();

      if (!t) { flushPara(); i++; continue; }

      if (/^(-{3,}|\*{3,}|_{3,})$/.test(t)) {
        flushPara();
        blocks.push({ object: 'block', type: 'divider', divider: {} });
        i++;
        continue;
      }

      var h = t.match(/^(#{1,6})\s+(.*)$/);
      if (h) {
        flushPara();
        var lv = Math.min(h[1].length, 3);
        var rt = inlineRichText(h[2]);
        var blk = { object: 'block', type: 'heading_' + lv };
        blk['heading_' + lv] = { rich_text: rt };
        blocks.push(blk);
        i++;
        continue;
      }

      if (/^>\s?/.test(t)) {
        flushPara();
        var q = [];
        while (i < lines.length && /^>\s?/.test(lines[i].trim())) { q.push(lines[i].trim().replace(/^>\s?/, '')); i++; }
        blocks.push({ object: 'block', type: 'quote', quote: { rich_text: inlineRichText(q.join('\n')) } });
        continue;
      }

      var todo = t.match(/^[-*+]\s+\[([ xX])\]\s+(.*)$/);
      if (todo) {
        flushPara();
        blocks.push({ object: 'block', type: 'to_do', to_do: { rich_text: inlineRichText(todo[2]), checked: todo[1].toLowerCase() === 'x' } });
        i++;
        continue;
      }

      var ul = t.match(/^[-*+]\s+(.*)$/);
      if (ul) {
        flushPara();
        blocks.push({ object: 'block', type: 'bulleted_list_item', bulleted_list_item: { rich_text: inlineRichText(ul[1]) } });
        i++;
        continue;
      }

      var ol = t.match(/^\d+[.)]\s+(.*)$/);
      if (ol) {
        flushPara();
        blocks.push({ object: 'block', type: 'numbered_list_item', numbered_list_item: { rich_text: inlineRichText(ol[1]) } });
        i++;
        continue;
      }

      var img = t.match(/^!\[([^\]]*)\]\(([^)\s]+)\)$/);
      if (img) {
        flushPara();
        blocks.push({ object: 'block', type: 'image', image: { type: 'external', external: { url: img[2] } } });
        i++;
        continue;
      }

      para.push(t);
      i++;
    }
    flushPara(); flushTable();
    return blocks;
  }

  /* ---------- Notion API ---------- */

  function notion(token, method, path, body) {
    return fetch(API + path, {
      method: method,
      headers: {
        'Authorization': 'Bearer ' + token,
        'Notion-Version': NOTION_VERSION,
        'Content-Type': 'application/json',
      },
      body: body ? JSON.stringify(body) : undefined,
    }).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (data) {
        if (!res.ok) {
          var err = new Error(data.message || ('Notion API 错误 (' + res.status + ')'));
          err.code = data.code || '';
          throw err;
        }
        return data;
      });
    });
  }

  function friendlyErr(e) {
    if (/failed to fetch|networkerror|load failed/i.test(e.message || '')) {
      return new Error('无法连接 Notion API，请检查网络后重试');
    }
    switch (e.code) {
      case 'unauthorized':
        return new Error('Notion 令牌无效或已过期，请重新填写集成令牌');
      case 'object_not_found':
        return new Error('数据库不存在或未共享给集成：请确认数据库 ID 正确，并在 Notion 中打开数据库「… → 连接」选择该集成');
      case 'validation_error':
        return new Error(e.message);
      default:
        return e;
    }
  }

  /* ---------- 发布 ---------- */

  function publish(payload, config) {
    var token = (config.token || '').trim();
    var dbId = (config.databaseId || '').trim();
    var statusValue = (config.statusValue || '').trim() || 'Published';
    if (!token) return Promise.reject(new Error('请先填写 Notion 集成令牌'));
    if (!dbId) return Promise.reject(new Error('请先填写数据库 ID'));

    // 1. 读取数据库结构：只写入实际存在的属性，避免「属性不存在」报错
    return notion(token, 'GET', '/databases/' + encodeURIComponent(dbId))
      .catch(function (e) { throw friendlyErr(e); })
      .then(function (schema) {
        var props = schema.properties || {};
        var properties = {};

        var titleKey = Object.keys(props).filter(function (k) { return props[k].type === 'title'; })[0];
        if (titleKey) {
          properties[titleKey] = { title: [{ type: 'text', text: { content: payload.title || '未命名文章' } }] };
        }
        if (props.tags && props.tags.type === 'multi_select' && payload.tags && payload.tags.length) {
          properties.tags = { multi_select: payload.tags.map(function (t) { return { name: t }; }) };
        }
        if (props.status && props.status.type === 'select') {
          properties.status = { select: { name: statusValue } };
        }
        if (props.date && props.date.type === 'date') {
          properties.date = { date: { start: String(payload.date || new Date().toISOString()).slice(0, 10) } };
        }
        if (props.summary && props.summary.type === 'rich_text' && payload.excerpt) {
          properties.summary = { rich_text: [{ type: 'text', text: { content: String(payload.excerpt).slice(0, 1900) } }] };
        }

        // 2. 正文分块（Notion 单次请求最多 100 个块）
        var blocks = mdToBlocks(payload.markdown);
        var chunks = [];
        for (var i = 0; i < blocks.length; i += 90) chunks.push(blocks.slice(i, i + 90));
        if (!chunks.length) chunks.push([]);

        // 3. 建页（首批块随页面创建）
        return notion(token, 'POST', '/pages', {
          parent: { database_id: dbId },
          properties: properties,
          children: chunks[0],
        }).catch(function (e) { throw friendlyErr(e); })
          .then(function (page) {
            // 4. 其余块追加
            var chain = Promise.resolve();
            chunks.slice(1).forEach(function (chunk, idx) {
              chain = chain.then(function () {
                return notion(token, 'PATCH', '/blocks/' + page.id + '/children', { children: chunk })
                  .catch(function (e) {
                    throw new Error('页面已创建，但第 ' + (idx + 2) + ' 段内容追加失败：' + e.message + '。页面地址：' + page.url);
                  });
              });
            });
            return chain.then(function () { return { url: page.url }; });
          });
      });
  }

  /* ---------- 契约导出 ---------- */

  window.WFPublishers = window.WFPublishers || {};
  window.WFPublishers.notionnext = {
    id: 'notionnext',
    name: 'NotionNext',
    desc: '发布到 NotionNext 等基于 Notion 数据库的博客系统（标题 / 标签 / 状态 / 正文）',
    version: '1.0.0',
    configFields: [
      { key: 'token', label: 'Notion 集成令牌', type: 'password', required: true, hint: 'notion.so/my-integrations 创建内部集成并复制令牌；再在 Notion 数据库「… → 连接」中共享给该集成' },
      { key: 'databaseId', label: '数据库 ID', type: 'text', required: true, hint: 'NotionNext 读取的数据库：网页链接中数据库名后的 32 位 ID' },
      { key: 'statusValue', label: '发布状态', type: 'text', required: false, hint: '数据库 status 属性的值，默认 Published' },
    ],
    publish: publish,
  };
})();
