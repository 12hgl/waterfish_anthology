/* 编辑器撤销 / 重做引擎
 *
 * 为什么不用浏览器原生 undo：工具栏插入标题、图片等操作用的是
 * setRangeText 程序化赋值，浏览器的原生撤销栈会因此失效或错乱
 * （表现为“撤销跳不回插入前的位置”）。这里自建快照栈：
 *
 *   - 连续输入在 MERGE_WINDOW 内合并为一帧，不会每敲一个字就存一帧
 *   - 工具栏 / 图片插入等程序化修改后调用 push() 强制入栈
 *   - undo / redo 同时恢复正文与光标位置
 *   - restore() 从文章旁的 .wf.json 侧车文件恢复历史，并把栈顶
 *     锚定为当前加载的正文（.md 是权威状态）
 */

const MERGE_WINDOW = 400; // 连续输入合并窗口（ms）
const MAX_STACK = 60;     // 内存撤销栈上限（服务器侧另有 50 条封顶）

export function createHistory(textarea, { onApply, onStackChange } = {}) {
  let stack = [];
  let redoStack = [];
  let lastPush = 0;
  let suppress = false; // 应用历史状态期间不产生新快照

  const snap = () => ({
    content: textarea.value,
    selStart: textarea.selectionStart,
    selEnd: textarea.selectionEnd,
  });

  const notify = () => onStackChange && onStackChange({
    canUndo: stack.length > 1,
    canRedo: redoStack.length > 0,
  });

  stack.push(snap());

  // 用户输入：窗口内合并进栈顶，窗口外压入新帧
  const onInput = () => {
    if (suppress) return;
    const now = Date.now();
    if (now - lastPush < MERGE_WINDOW) {
      stack[stack.length - 1] = snap();
    } else {
      stack.push(snap());
      if (stack.length > MAX_STACK) stack.shift();
    }
    lastPush = now;
    redoStack = [];
    notify();
  };

  // 程序化修改后调用：强制压入一帧（与栈顶相同则忽略）
  const push = () => {
    if (suppress) return;
    const cur = snap();
    const top = stack[stack.length - 1];
    if (top && top.content === cur.content && top.selStart === cur.selStart && top.selEnd === cur.selEnd) {
      notify();
      return;
    }
    stack.push(cur);
    if (stack.length > MAX_STACK) stack.shift();
    redoStack = [];
    lastPush = Date.now();
    notify();
  };

  const apply = (entry) => {
    suppress = true;
    textarea.value = entry.content;
    const len = entry.content.length;
    textarea.setSelectionRange(
      Math.max(0, Math.min(entry.selStart, len)),
      Math.max(0, Math.min(entry.selEnd, len)),
    );
    suppress = false;
    onApply && onApply(entry);
  };

  const undo = () => {
    if (stack.length < 2) return false;
    redoStack.push(stack.pop());
    apply(stack[stack.length - 1]);
    lastPush = Date.now();
    notify();
    return true;
  };

  const redo = () => {
    if (!redoStack.length) return false;
    const entry = redoStack.pop();
    stack.push(entry);
    apply(entry);
    lastPush = Date.now();
    notify();
    return true;
  };

  // 从侧车文件恢复；current 为当前加载的正文，锚定为栈顶
  const restore = (entries, current) => {
    const arr = (Array.isArray(entries) ? entries : [])
      .filter((e) => e && typeof e.content === 'string')
      .map((e) => ({
        content: e.content,
        selStart: Number(e.selStart) || 0,
        selEnd: Number(e.selEnd) || 0,
      }));
    const cur = current || snap();
    if (!arr.length) {
      arr.push(cur);
    } else if (arr[arr.length - 1].content !== cur.content) {
      arr[arr.length - 1] = cur;
    }
    stack = arr.slice(-MAX_STACK);
    redoStack = [];
    notify();
  };

  const entries = () => stack.slice();

  textarea.addEventListener('input', onInput);

  return {
    push, undo, redo, restore, entries, notify,
    destroy: () => textarea.removeEventListener('input', onInput),
  };
}
