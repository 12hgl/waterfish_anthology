package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// 编辑器状态侧车文件：保存在文章 .md 旁边（<文章ID>.wf.json），
// 记录每篇文章的自动保存开关与撤销栈（修改记录）。
// 仅应用自用，不出现在文章列表（列表只读 .md）。

// EditorState 单篇文章的编辑器状态。
type EditorState struct {
	AutoSave bool           `json:"autoSave"`
	History  []HistoryEntry `json:"history,omitempty"`
}

// HistoryEntry 撤销栈中的一帧（一次修改记录）。
type HistoryEntry struct {
	Content  string `json:"content"`
	SelStart int    `json:"selStart"`
	SelEnd   int    `json:"selEnd"`
}

// 持久化上限：防止侧车文件随修改记录无限增长。
const (
	maxHistoryEntries   = 50         // 最多保留的修改记录条数
	maxHistoryStateSize = 512 * 1024 // 侧车序列化后的体积上限，超出从头裁剪
)

// articleBaseName 把文章 ID 清洗为不含扩展名的安全文件名基名。
func articleBaseName(id string) (string, error) {
	name, err := safeName(id)
	if err != nil {
		return "", err
	}
	name = strings.TrimSuffix(name, ".md")
	if name == "" {
		return "", errors.New("文章 ID 不能为空")
	}
	return name, nil
}

// editorStatePath 侧车文件路径：与文章 .md 同目录。
func (u *UserApp) editorStatePath(id string) (string, error) {
	base, err := articleBaseName(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(u.PassageDir(), base+".wf.json"), nil
}

func (u *UserApp) loadEditorState(id string) EditorState {
	st := EditorState{}
	p, err := u.editorStatePath(id)
	if err != nil {
		return st
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func (u *UserApp) saveEditorState(id string, st EditorState) error {
	st = trimHistory(st)
	p, err := u.editorStatePath(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// deleteEditorState 删除文章时连带清理侧车文件。
func (u *UserApp) deleteEditorState(id string) {
	if p, err := u.editorStatePath(id); err == nil {
		_ = os.Remove(p)
	}
}

// renameEditorState 标题变化导致文章改名时，侧车文件随之迁移。
func (u *UserApp) renameEditorState(oldID, newID string) {
	if oldID == "" || newID == "" || oldID == newID {
		return
	}
	op, err1 := u.editorStatePath(oldID)
	np, err2 := u.editorStatePath(newID)
	if err1 != nil || err2 != nil || op == np {
		return
	}
	if _, err := os.Stat(op); err == nil {
		_ = os.Rename(op, np)
	}
}

// trimHistory 按条数、体积双重裁剪修改记录（保留最近的）。
func trimHistory(st EditorState) EditorState {
	if len(st.History) > maxHistoryEntries {
		st.History = st.History[len(st.History)-maxHistoryEntries:]
	}
	for len(st.History) > 1 {
		data, err := json.Marshal(st)
		if err == nil && len(data) <= maxHistoryStateSize {
			break
		}
		st.History = st.History[1:]
	}
	return st
}
