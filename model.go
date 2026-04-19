package main

import (
	"os"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
)

type AppState int

const (
	StateList AppState = iota
	StateEditor
	StateInputForm
	StateHelp
	StatePrompt
	StateDetail
	StateMenu
	StateProgress
	StatePermissions
)

type InputPurpose int

const (
	PurposeNewFile InputPurpose = iota
	PurposeNewFolder
	PurposeDelete
	PurposeEncrypt
	PurposeDecrypt
	PurposeRename
	PurposeSearch
)

type FileItem struct {
	Name  string
	IsDir bool
	Size  int64
	Mode  os.FileMode
}

type Tab struct {
	Path       string
	Files      []FileItem
	Cursor     int
	StartIndex int
	Selected   map[string]bool
}

type PermissionBit struct {
	Label string
	Value os.FileMode
	IsSet bool
}

type Model struct {
	state          AppState
	tabs           []Tab
	activeTab      int
	width          int
	height         int
	textarea       textarea.Model
	textinput      textinput.Model
	progress       progress.Model
	inputPurpose   InputPurpose
	clipboard      []string
	errorMsg       string
	detailInfo     string
	currentFile    string
	menuIndex      int
	menuColumn     int
	
	procMsg     string
	procPercent float64
	isAdmin     bool

	perms      []PermissionBit
	permCursor int
}

func initialModel() Model {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	ta := textarea.New()
	ta.Focus()
	ta.SetHeight(10)
	ta.SetWidth(30)

	prog := progress.New(progress.WithDefaultGradient())

	cwd, _ := os.Getwd()
	firstTab := Tab{
		Path:     cwd,
		Selected: make(map[string]bool),
	}

	return Model{
		state:     StateList,
		tabs:      []Tab{firstTab},
		activeTab: 0,
		textinput: ti,
		textarea:  ta,
		progress:  prog,
		clipboard: []string{},
		isAdmin:   checkAdmin(),
	}
}
