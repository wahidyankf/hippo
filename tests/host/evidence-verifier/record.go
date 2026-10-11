package main

import "time"

const (
	tmuxTab           = "Tab"
	tmuxUp            = "Up"
	tmuxEnter         = "Enter"
	tmuxDown          = "Down"
	outcomePassed     = "passed"
	inputTab          = "tab"
	inputEnter        = "enter"
	tmuxInterrupt     = "C-c"
	hostTmux          = "tmux"
	hostNeovim        = "neovim"
	hostWezterm       = "wezterm"
	inputShiftTab     = "shift-tab"
	inputDown         = "down"
	inputResize       = "resize"
	pendingResizeID   = "resize-pending-refresh"
	terminalInputMode = "terminal-input"
	tmuxBackTab       = "BTab"
)

type evidenceDocument struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Host          string              `json:"host"`
	RecordedAt    time.Time           `json:"recordedAt"`
	Command       []string            `json:"command"`
	Candidate     recordedCandidate   `json:"candidate"`
	Fixtures      map[string][]string `json:"fixtures"`
	Steps         []recordedStep      `json:"steps"`
	SourceRoot    string              `json:"sourceRoot,omitempty"`
	ConfigRoots   []string            `json:"configRoots,omitempty"`
	Target        hostTarget          `json:"target,omitzero"`
}

type hostTarget struct {
	Socket       string `json:"socket,omitempty"`
	Pane         string `json:"pane"`
	EditorServer string `json:"editorServer,omitempty"`
	OwnerToken   string `json:"ownerToken"`
}

type recordedCandidate struct {
	Revision      string            `json:"revision"`
	DirtyPaths    map[string]string `json:"dirtyPaths"`
	ConfigPaths   map[string]string `json:"configPaths"`
	BinarySHA256  string            `json:"binarySHA256"`
	BinaryVersion string            `json:"binaryVersion"`
	BinaryCommit  string            `json:"binaryCommit"`
}

type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type viewState struct {
	View             string `json:"view"`
	Section          string `json:"section"`
	SelectedID       string `json:"selectedID"`
	Scroll           int    `json:"scroll"`
	RefreshCount     int    `json:"refreshCount"`
	SampleAgeSeconds int    `json:"sampleAgeSeconds"`
}

type recordedStep struct {
	ID               string    `json:"id"`
	Input            string    `json:"input"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	Expected         string    `json:"expected"`
	Actual           string    `json:"actual"`
	Outcome          string    `json:"outcome"`
	Capture          artifact  `json:"capture"`
	Before           viewState `json:"before"`
	After            viewState `json:"after"`
	RefreshPending   bool      `json:"refreshPending"`
	ExpectedExit     int       `json:"expectedExit"`
	ActualExit       int       `json:"actualExit"`
	ModesBefore      string    `json:"modesBefore"`
	ModesAfter       string    `json:"modesAfter"`
	ForegroundBefore int       `json:"foregroundBefore"`
	ForegroundAfter  int       `json:"foregroundAfter"`
	HostUsable       bool      `json:"hostUsable"`
	PaneBefore       string    `json:"paneBefore"`
	PaneAfter        string    `json:"paneAfter"`
	PaneReturn       string    `json:"paneReturn"`
	EditorModeBefore string    `json:"editorModeBefore"`
	EditorModeAfter  string    `json:"editorModeAfter"`
	SectionBefore    string    `json:"sectionBefore"`
	SectionAfter     string    `json:"sectionAfter"`
	OperatorEvent    artifact  `json:"operatorEvent"`
	BeforeCapture    artifact  `json:"beforeCapture,omitzero"`
	InputReceipt     artifact  `json:"inputReceipt,omitzero"`
}

type plannedStep struct {
	id, input string
	width     int
	height    int
	exit      int
}
