package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type tickMsg time.Time
type progressMsg float64
type progressDoneMsg string

type filesMsg struct {
	tabIndex int
	files    []FileItem
}

func (m Model) Init() tea.Cmd {
	return m.refreshFiles()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 10
		m.textarea.SetWidth(msg.Width - 10)
		m.textarea.SetHeight(m.height - 12)
		return m, nil

	case filesMsg:
		if msg.tabIndex < len(m.tabs) {
			m.tabs[msg.tabIndex].Files = msg.files
			if m.tabs[msg.tabIndex].Cursor >= len(msg.files) {
				m.tabs[msg.tabIndex].Cursor = 0
			}
		}
		return m, nil

	case progressMsg:
		var cmd tea.Cmd
		m.procPercent = float64(msg)
		if m.procPercent >= 1.0 {
			return m, nil
		}
		newProg, cmd := m.progress.Update(msg)
		m.progress = newProg.(progress.Model)
		return m, cmd

	case progressDoneMsg:
		m.state = StateList
		if string(msg) != "" {
			m.errorMsg = string(msg)
		}
		return m, m.refreshFiles()

	case tea.KeyMsg:
		m.errorMsg = ""
		
		switch msg.String() {
		case "ctrl+n":
			m.tabs = append(m.tabs, Tab{Path: "This PC", Selected: make(map[string]bool)})
			m.activeTab = len(m.tabs) - 1
			return m, m.refreshFiles()
		case "ctrl+w":
			if len(m.tabs) > 1 {
				m.tabs = append(m.tabs[:m.activeTab], m.tabs[m.activeTab+1:]...)
				if m.activeTab >= len(m.tabs) {
					m.activeTab = len(m.tabs) - 1
				}
			} else {
				return m, tea.Quit
			}
			return m, m.refreshFiles()
		case "tab":
			m.activeTab = (m.activeTab + 1) % len(m.tabs)
			return m, m.refreshFiles()
		case "f10":
			if m.state == StateMenu {
				m.state = StateList
			} else {
				m.state = StateMenu
				m.menuIndex = 0
				m.menuColumn = 0
			}
			return m, nil
		}

		switch m.state {
		case StateList:
			return m.updateList(msg)
		case StateMenu:
			return m.updateMenu(msg)
		case StateEditor:
			return m.updateEditor(msg)
		case StateInputForm:
			return m.updateInput(msg)
		case StatePrompt:
			return m.updatePrompt(msg)
		case StatePermissions:
			return m.updatePermissions(msg)
		case StateHelp, StateDetail:
			if msg.String() == "esc" || msg.String() == "q" || msg.String() == "enter" {
				m.state = StateList
			}
			return m, nil
		case StateProgress:
			return m, nil
		}
	}

	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.activeTab]

	switch msg.String() {
	case "n":
		m.state = StateInputForm; m.inputPurpose = PurposeNewFile; m.textinput.SetValue(""); m.textinput.Focus()
		return m, nil
	case "m":
		m.state = StateInputForm; m.inputPurpose = PurposeNewFolder; m.textinput.SetValue(""); m.textinput.Focus()
		return m, nil
	case "ctrl+c":
		m.clipboard = []string{}
		for name := range t.Selected { m.clipboard = append(m.clipboard, filepath.Join(t.Path, name)) }
		if len(m.clipboard) == 0 && len(t.Files) > 0 { m.clipboard = append(m.clipboard, filepath.Join(t.Path, t.Files[t.Cursor].Name)) }
		return m, nil
	case "ctrl+v":
		if len(m.clipboard) > 0 { return m.startOperation("Pasting...", m.doPaste) }
		return m, nil
	case " ":
		if len(t.Files) > 0 {
			name := t.Files[t.Cursor].Name
			if t.Selected[name] { delete(t.Selected, name) } else { t.Selected[name] = true }
		}
	case "up", "k":
		if t.Cursor > 0 { t.Cursor--; if t.Cursor < t.StartIndex { t.StartIndex = t.Cursor } }
	case "down", "j":
		if t.Cursor < len(t.Files)-1 {
			t.Cursor++
			visibleHeight := m.height - 10
			if t.Cursor >= t.StartIndex+visibleHeight { t.StartIndex = t.Cursor - visibleHeight + 1 }
		}
	case "left", "h":
		if t.Path != "This PC" {
			parent := filepath.Dir(t.Path)
			if parent == t.Path || parent == "." || parent == "/" || (runtimeGOOS() == "windows" && len(parent) <= 3) {
				if runtimeGOOS() == "windows" && len(parent) <= 3 && parent != t.Path { t.Path = parent } else { t.Path = "This PC" }
			} else { t.Path = parent }
			t.Cursor = 0; t.StartIndex = 0; t.Selected = make(map[string]bool)
			return m, m.refreshFiles()
		}
	case "right", "l", "enter":
		if len(t.Files) > 0 {
			selected := t.Files[t.Cursor]
			var newPath string
			if t.Path == "This PC" { newPath = selected.Name } else { newPath = filepath.Join(t.Path, selected.Name) }
			if selected.IsDir {
				t.Path = newPath; t.Cursor = 0; t.StartIndex = 0; t.Selected = make(map[string]bool)
				return m, m.refreshFiles()
			} else {
				content, err := os.ReadFile(newPath)
				if err != nil { m.errorMsg = err.Error(); return m, nil }
				m.textarea.SetValue(string(content))
				m.state = StateEditor; m.currentFile = newPath
			}
		}
	case "f1": m.state = StateHelp
	case "f8", "delete": if len(t.Files) > 0 { m.state = StatePrompt; m.inputPurpose = PurposeDelete }
	case "q": return m, tea.Quit
	}
	return m, nil
}

func runtimeGOOS() string {
	if os.PathSeparator == '\\' { return "windows" }
	return "linux"
}

func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	const rows = 4
	switch msg.String() {
	case "up", "k": if m.menuIndex%rows > 0 { m.menuIndex-- }
	case "down", "j": if m.menuIndex%rows < rows-1 { m.menuIndex++ }
	case "left", "h": if m.menuIndex >= rows { m.menuIndex -= rows }
	case "right", "l": if m.menuIndex < 12 { m.menuIndex += rows }
	case "enter":
		t := &m.tabs[m.activeTab]
		switch m.menuIndex {
		case 0: m.state = StateInputForm; m.inputPurpose = PurposeNewFile; m.textinput.SetValue(""); m.textinput.Focus()
		case 1: m.state = StateInputForm; m.inputPurpose = PurposeNewFolder; m.textinput.SetValue(""); m.textinput.Focus()
		case 2: m.state = StateInputForm; m.inputPurpose = PurposeRename; m.textinput.SetValue(t.Files[t.Cursor].Name); m.textinput.Focus()
		case 3: 
			selected := t.Files[t.Cursor]; m.state = StateDetail
			m.detailInfo = fmt.Sprintf("Name: %s\nSize: %d bytes\nMode: %v\nDir: %v", selected.Name, selected.Size, selected.Mode, selected.IsDir)
		case 4: 
			m.clipboard = []string{}
			for name := range t.Selected { m.clipboard = append(m.clipboard, filepath.Join(t.Path, name)) }
			if len(m.clipboard) == 0 && len(t.Files) > 0 { m.clipboard = append(m.clipboard, filepath.Join(t.Path, t.Files[t.Cursor].Name)) }
			m.state = StateList
		case 5: return m.startOperation("Pasting...", m.doPaste)
		case 6: for _, f := range t.Files { t.Selected[f.Name] = true }; m.state = StateList
		case 7: t.Selected = make(map[string]bool); m.state = StateList
		case 8: return m.startOperation("Zipping...", m.doZip)
		case 9: return m.startOperation("Unzipping...", m.doUnzip)
		case 10: m.state = StateInputForm; m.inputPurpose = PurposeEncrypt; m.textinput.SetValue(""); m.textinput.Focus()
		case 11: m.state = StateInputForm; m.inputPurpose = PurposeDecrypt; m.textinput.SetValue(""); m.textinput.Focus()
		case 12: openTerminal(t.Path); m.state = StateList
		case 13: m.initPermissions(); m.state = StatePermissions
		case 14: elevateAdmin(); return m, tea.Quit
		case 15: m.state = StateList
		}
		if m.state == StateMenu { m.state = StateList }
	case "esc": m.state = StateList
	}
	return m, nil
}

func (m *Model) initPermissions() {
	t := m.tabs[m.activeTab]
	if len(t.Files) == 0 { return }
	file := t.Files[t.Cursor]
	m.perms = []PermissionBit{
		{"Owner Read", 0400, file.Mode&0400 != 0},
		{"Owner Write", 0200, file.Mode&0200 != 0},
		{"Owner Exec", 0100, file.Mode&0100 != 0},
		{"Group Read", 0040, file.Mode&0040 != 0},
		{"Group Write", 0020, file.Mode&0020 != 0},
		{"Group Exec", 0010, file.Mode&0010 != 0},
		{"Other Read", 0004, file.Mode&0004 != 0},
		{"Other Write", 0002, file.Mode&0002 != 0},
		{"Other Exec", 0001, file.Mode&0001 != 0},
	}
	m.permCursor = 0
}

func (m Model) updatePermissions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k": if m.permCursor > 0 { m.permCursor-- }
	case "down", "j": if m.permCursor < len(m.perms)-1 { m.permCursor++ }
	case " ": m.perms[m.permCursor].IsSet = !m.perms[m.permCursor].IsSet
	case "enter":
		var newMode os.FileMode
		for _, p := range m.perms { if p.IsSet { newMode |= p.Value } }
		t := m.tabs[m.activeTab]
		if err := os.Chmod(filepath.Join(t.Path, t.Files[t.Cursor].Name), newMode); err != nil { m.errorMsg = err.Error() }
		m.state = StateList
		return m, m.refreshFiles()
	case "esc": m.state = StateList
	}
	return m, nil
}

func (m Model) startOperation(msg string, fn func()) (tea.Model, tea.Cmd) {
	m.state = StateProgress
	m.procMsg = msg
	m.procPercent = 0
	return m, func() tea.Msg {
		fn()
		return progressDoneMsg("")
	}
}

func (m Model) doZip() {
	t := m.tabs[m.activeTab]
	if len(t.Selected) > 0 {
		for name := range t.Selected { zipFiles(filepath.Join(t.Path, name), filepath.Join(t.Path, name+".zip")) }
	} else if len(t.Files) > 0 {
		src := filepath.Join(t.Path, t.Files[t.Cursor].Name)
		zipFiles(src, src+".zip")
	}
}

func (m Model) doUnzip() {
	t := m.tabs[m.activeTab]
	if len(t.Selected) > 0 {
		for name := range t.Selected { if strings.HasSuffix(name, ".zip") { unzipArchive(filepath.Join(t.Path, name), t.Path) } }
	} else if len(t.Files) > 0 {
		name := t.Files[t.Cursor].Name
		if strings.HasSuffix(name, ".zip") { unzipArchive(filepath.Join(t.Path, name), t.Path) }
	}
}

func (m Model) doPaste() {
	t := m.tabs[m.activeTab]
	for _, src := range m.clipboard { copyFile(src, filepath.Join(t.Path, filepath.Base(src))) }
}

func (m Model) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc": m.state = StateList
	case "ctrl+s":
		os.WriteFile(m.currentFile, []byte(m.textarea.Value()), 0644)
		m.state = StateList
		return m, m.refreshFiles()
	
	// Shift-navigation keybindings
	case "shift+up":
		m.textarea.SetCursor(0)
	case "shift+down":
		m.textarea.SetCursor(len(m.textarea.Value()))
	case "shift+left":
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyHome})
		return m, cmd
	case "shift+right":
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
		return m, cmd

	default:
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := m.textinput.Value()
		t := m.tabs[m.activeTab]
		switch m.inputPurpose {
		case PurposeNewFile: os.WriteFile(filepath.Join(t.Path, val), []byte(""), 0644)
		case PurposeNewFolder: os.MkdirAll(filepath.Join(t.Path, val), 0755)
		case PurposeEncrypt: encryptFile(filepath.Join(t.Path, t.Files[t.Cursor].Name), val)
		case PurposeDecrypt: decryptFile(filepath.Join(t.Path, t.Files[t.Cursor].Name), val)
		case PurposeRename: os.Rename(filepath.Join(t.Path, t.Files[t.Cursor].Name), filepath.Join(t.Path, val))
		}
		m.state = StateList
		return m, m.refreshFiles()
	case "esc": m.state = StateList
	default:
		var cmd tea.Cmd
		m.textinput, cmd = m.textinput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if strings.ToLower(msg.String()) == "y" {
		t := m.tabs[m.activeTab]
		if len(t.Selected) > 0 {
			for name := range t.Selected { os.RemoveAll(filepath.Join(t.Path, name)) }
		} else {
			os.RemoveAll(filepath.Join(t.Path, t.Files[t.Cursor].Name))
		}
		m.state = StateList
		return m, m.refreshFiles()
	} else if msg.String() == "esc" || strings.ToLower(msg.String()) == "n" {
		m.state = StateList
	}
	return m, nil
}

func (m Model) refreshFiles() tea.Cmd {
	return func() tea.Msg {
		idx := m.activeTab
		if idx >= len(m.tabs) { return nil }
		files, _ := listFiles(m.tabs[idx].Path)
		return filesMsg{tabIndex: idx, files: files}
	}
}
