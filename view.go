package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	purple    = lipgloss.Color("#8A2BE2")
	blue      = lipgloss.Color("#00BFFF")
	white     = lipgloss.Color("#FFFFFF")
	gray      = lipgloss.Color("#808080")
	black     = lipgloss.Color("#121212")
	green     = lipgloss.Color("#00FF00")
	yellow    = lipgloss.Color("#FFD700")
	orange    = lipgloss.Color("#FF8C00")
	red       = lipgloss.Color("#FF4500")
	cyan      = lipgloss.Color("#00CED1")
	magenta   = lipgloss.Color("#FF00FF")

	// Styles
	pathStyle      = lipgloss.NewStyle().Foreground(gray).Italic(true)
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(white).Background(purple).Padding(0, 1)
	tabStyle       = lipgloss.NewStyle().Padding(0, 2).Foreground(gray).Background(black)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 2).Foreground(white).Background(purple).Bold(true)
	
	selectedStyle  = lipgloss.NewStyle().Bold(true).Foreground(black).Background(blue)
	normalStyle    = lipgloss.NewStyle().Foreground(white)
	
	borderStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(purple)
	helpStyle      = lipgloss.NewStyle().Foreground(gray).Padding(0, 1)
	statusStyle    = lipgloss.NewStyle().Foreground(white).Background(purple).Padding(0, 1)
	errorStyle     = lipgloss.NewStyle().Foreground(white).Background(red).Padding(0, 1)
	
	menuHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(purple).MarginBottom(1).Underline(true)
	menuItemStyle   = lipgloss.NewStyle().PaddingLeft(2)
	menuActiveStyle = lipgloss.NewStyle().Foreground(black).Background(blue).Bold(true).PaddingLeft(1)
	
	checkStyle     = lipgloss.NewStyle().Foreground(green).Bold(true)

	// File type colors
	dirColor    = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	exeColor    = lipgloss.NewStyle().Foreground(green)
	archiveColor = lipgloss.NewStyle().Foreground(red)
	configColor  = lipgloss.NewStyle().Foreground(orange)
	codeColor    = lipgloss.NewStyle().Foreground(cyan)
	mediaColor   = lipgloss.NewStyle().Foreground(magenta)
	docColor     = lipgloss.NewStyle().Foreground(white)
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 { return "Initializing..." }

	var mainContent string
	switch m.state {
	case StateList: mainContent = m.listView()
	case StateMenu: mainContent = m.menuView()
	case StateEditor: mainContent = m.editorView()
	case StateInputForm: mainContent = m.inputView()
	case StatePrompt: mainContent = m.promptView()
	case StateHelp: mainContent = m.helpPopup()
	case StateDetail: mainContent = m.detailPopup()
	case StateProgress: mainContent = m.progressView()
	case StatePermissions: mainContent = m.permissionsView()
	}

	header := m.headerView()
	tabs := m.tabsView()
	status := m.statusView()
	footer := m.footerView()

	// Fixed height adjustment to remove gaps
	// 1 (header) + 1 (tabs) + 1 (status) + 1 (footer) + 2 (borders) = 6
	fixedHeights := 6
	bodyHeight := m.height - fixedHeights
	if bodyHeight < 1 { bodyHeight = 1 }

	body := lipgloss.NewStyle().
		Height(bodyHeight).
		MaxHeight(bodyHeight).
		Width(m.width - 4).
		Render(mainContent)

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		tabs,
		body,
		status,
		footer,
	)

	return borderStyle.Width(m.width - 2).Height(m.height - 2).Render(content)
}

func (m Model) headerView() string {
	title := titleStyle.Render(" NEO FM ")
	t := m.tabs[m.activeTab]
	path := pathStyle.Render(" " + t.Path)
	return lipgloss.JoinHorizontal(lipgloss.Center, title, path)
}

func (m Model) tabsView() string {
	var tabs []string
	for i, t := range m.tabs {
		name := filepath.Base(t.Path)
		if t.Path == "This PC" { name = "💻 This PC" } else if name == "." || name == "/" || name == "" { name = t.Path }
		style := tabStyle
		if i == m.activeTab { style = activeTabStyle }
		tabs = append(tabs, style.Render(fmt.Sprintf("%d: %s", i+1, name)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) statusView() string {
	if m.errorMsg != "" { return errorStyle.Width(m.width - 4).Render(" ERROR: " + m.errorMsg) }
	t := m.tabs[m.activeTab]
	info := fmt.Sprintf(" Tab %d/%d • %d items • Selected: %d • Clip: %d ", m.activeTab+1, len(m.tabs), len(t.Files), len(t.Selected), len(m.clipboard))
	if m.isAdmin { info += " • [ADMIN] " }
	return statusStyle.Width(m.width - 4).Render(info)
}

func (m Model) footerView() string {
	return helpStyle.Render(" F1: Help • Space: Select • F10: MENU • Tab: Next Tab • Ctrl+N: New • Ctrl+W: Close ")
}

func (m Model) listView() string {
	var b strings.Builder
	t := m.tabs[m.activeTab]

	if len(t.Files) == 0 { return "\n   (Empty directory)" }

	nameWidth := m.width - 35
	if nameWidth < 20 { nameWidth = 20 }

	for i := t.StartIndex; i < len(t.Files); i++ {
		// Calculate if we reached the end of visible area
		if b.Len() > 0 && strings.Count(b.String(), "\n") >= m.height-10 { break }

		file := t.Files[i]
		icon, style := getFileIconAndStyle(file, t.Path == "This PC")
		
		name := file.Name
		if len(name) > nameWidth { name = name[:nameWidth-3] + "..." }

		selMarker := "  "
		if t.Selected[file.Name] { selMarker = checkStyle.Render("✓ ") }

		if i == t.Cursor {
			// Entire row is blue with black text
			rowText := fmt.Sprintf(" %s%s %-*s %10s ", selMarker, icon+" ", nameWidth, name, formatSize(file.Size))
			b.WriteString(selectedStyle.Width(m.width - 6).Render(rowText) + "\n")
		} else {
			namePart := style.Render(fmt.Sprintf("%-*s", nameWidth, name))
			row := fmt.Sprintf(" %s%s %s %10s ", selMarker, icon+" ", namePart, formatSize(file.Size))
			b.WriteString(row + "\n")
		}
	}
	return b.String()
}

func getFileIconAndStyle(file FileItem, isPC bool) (string, lipgloss.Style) {
	if isPC { return "💽", dirColor }
	if file.IsDir { return "📂", dirColor }
	ext := strings.ToLower(filepath.Ext(file.Name))
	switch ext {
	case ".exe", ".bat", ".cmd", ".sh", ".ps1": return "⚡", exeColor
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".enc": return "📦", archiveColor
	case ".json", ".toml", ".yaml", ".yml", ".ini", ".conf", ".cfg": return "⚙️", configColor
	case ".go", ".rs", ".py", ".js", ".ts", ".cpp", ".c", ".h", ".cs", ".php", ".html", ".css", ".md": return "📜", codeColor
	case ".jpg", ".jpeg", ".png", ".gif", ".mp4", ".mp3", ".wav", ".flac": return "🖼️", mediaColor
	case ".pdf", ".docx", ".doc", ".txt": return "📄", docColor
	default: return "📄", normalStyle
	}
}

func (m Model) menuView() string {
	menuGroups := []struct { Title string; Items []string; Icons []string }{
		{"FILE OPS", []string{"New File", "New Folder", "Rename", "Details"}, []string{"📄", "📂", "✏️", "ℹ️"}},
		{"CLIPBOARD", []string{"Copy", "Paste", "Select All", "Deselect All"}, []string{"📋", "📥", "✅", "❌"}},
		{"TOOLS", []string{"Zip", "Unzip", "Encrypt", "Decrypt"}, []string{"📦", "📂", "🔒", "🔓"}},
		{"SYSTEM", []string{"Terminal", "Permissions", "Restart Admin", "Exit Menu"}, []string{"💻", "🛡️", "⚡", "🚪"}},
	}
	var columns []string
	globalIdx := 0
	for _, group := range menuGroups {
		var g strings.Builder
		g.WriteString(menuHeaderStyle.Render(group.Title) + "\n")
		for i, item := range group.Items {
			icon := group.Icons[i]
			if globalIdx == m.menuIndex { g.WriteString(menuActiveStyle.Width(22).Render("> "+icon+" "+item) + "\n") } else { g.WriteString(menuItemStyle.Render(icon+" "+item) + "\n") }
			globalIdx++
		}
		columns = append(columns, lipgloss.NewStyle().MarginRight(4).Render(g.String()))
	}
	menuGrid := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(blue).Padding(1, 2).Render(menuGrid))
}

func (m Model) permissionsView() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(blue).Render(" FILE PERMISSIONS (SPACE TO TOGGLE) ") + "\n\n")
	for i, p := range m.perms {
		check := "[ ]"; if p.IsSet { check = checkStyle.Render("[x]") }
		line := fmt.Sprintf(" %s %s ", check, p.Label)
		if i == m.permCursor { b.WriteString(lipgloss.NewStyle().Background(purple).Foreground(white).Render(" > "+line) + "\n") } else { b.WriteString("   " + line + "\n") }
	}
	b.WriteString("\n" + helpStyle.Render("ENTER: Apply • ESC: Cancel"))
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(blue).Padding(1, 2).Render(b.String()))
}

func (m Model) progressView() string {
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, fmt.Sprintf("%s\n\n%s\n\nWorking, please wait...", m.procMsg, m.progress.ViewAs(m.procPercent)))
}

func formatSize(size int64) string {
	if size == 0 { return "-" }
	if size < 1024 { return fmt.Sprintf("%d B", size) }
	if size < 1024*1024 { return fmt.Sprintf("%.1f KB", float64(size)/1024) }
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func (m Model) editorView() string {
	m.textarea.SetWidth(m.width - 10)
	m.textarea.SetHeight(m.height - 12)
	header := lipgloss.NewStyle().Background(purple).Foreground(white).Padding(0, 1).Bold(true).Render(" ✎ EDITING: " + filepath.Base(m.currentFile))
	
	// We call .View() directly to ensure the cursor is rendered by the component
	return fmt.Sprintf("%s\n\n%s\n\n%s", header, m.textarea.View(), helpStyle.Render(" Ctrl+S: Save • Esc: Cancel • Shift+Up/Down: Top/End "))
}

func (m Model) inputView() string {
	m.textinput.Width = m.width - 20
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, fmt.Sprintf("ENTER VALUE:\n\n%s\n\n%s", m.textinput.View(), helpStyle.Render(" Enter: Confirm • Esc: Cancel ")))
}

func (m Model) promptView() string {
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, "Are you sure you want to delete selected/current?\n\n(Y/N)")
}

func (m Model) helpPopup() string {
	helpText := ` NEO FM HELP
	
 Navigation:
 • n: Create new file | m: Create new folder
 • Space: Select multiple | Tab: Switch tabs
 • Ctrl+C: Copy | Ctrl+V: Paste
 • F10: TOOLS MENU (Advanced functions)
	
 Editor:
 • Shift + Up/Down: Top/Bottom of file
 • Shift + Left/Right: Start/End of line
 • Highlighting: Enabled
	
 Press any key to return...`
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, helpText)
}

func (m Model) detailPopup() string {
	return lipgloss.Place(m.width-4, m.height-10, lipgloss.Center, lipgloss.Center, fmt.Sprintf("FILE DETAILS\n\n%s\n\nPress any key to return...", m.detailInfo))
}
