// Command inkstudio is InkStudio, a desktop editor and viewer for inkpress slide packs.
package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/inkcheck/inkstudio/internal/studio"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set by scripts/package.sh.
var version = "dev"

func main() {
	fixPath()

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	var app *application.App
	var window *application.WebviewWindow

	st := studio.New(studio.Host{
		ChooseDir: func(title string, create bool) (string, error) {
			d := app.Dialog.OpenFile().
				SetTitle(title).
				CanChooseDirectories(true).
				CanChooseFiles(false).
				CanCreateDirectories(create)
			if window != nil {
				d.AttachToWindow(window)
			}
			return d.PromptForSingleSelection()
		},
		ChooseSave: func(title, path, filter, ext string) (string, error) {
			d := app.Dialog.SaveFile().
				SetMessage(title).
				SetDirectory(filepath.Dir(path)).
				SetFilename(filepath.Base(path)).
				AddFilter(filter, "*."+ext).
				CanCreateDirectories(true)
			if window != nil {
				d.AttachToWindow(window)
			}
			return d.PromptForSingleSelection()
		},
		ChooseFile: func(title, filter string, exts []string) (string, error) {
			patterns := make([]string, len(exts))
			for i, e := range exts {
				patterns[i] = "*." + e
			}
			d := app.Dialog.OpenFile().
				SetTitle(title).
				CanChooseFiles(true).
				CanChooseDirectories(false).
				AddFilter(filter, strings.Join(patterns, ";"))
			if window != nil {
				d.AttachToWindow(window)
			}
			return d.PromptForSingleSelection()
		},
		Confirm: func(title, message, ok string) bool {
			answer := make(chan bool, 1)
			d := app.Dialog.Question().SetTitle(title).SetMessage(message)
			yes := d.AddButton(ok).OnClick(func() { answer <- true })
			no := d.AddButton("Cancel").OnClick(func() { answer <- false })
			d.SetDefaultButton(yes)
			d.SetCancelButton(no)
			if window != nil {
				d.AttachToWindow(window)
			}
			d.Show()
			return <-answer
		},
		Emit: func(name string, data any) {
			app.Event.Emit(name, data)
		},
		Opened: func(info *studio.ProjectInfo) {
			if window != nil {
				window.SetTitle(info.Title + " — InkStudio")
			}
			saveState(state{LastDir: info.Dir})
		},
	})

	app = application.New(application.Options{
		Name:        "InkStudio",
		Description: "Edit and present inkpress slides (" + version + ")",
		Assets: application.AssetOptions{
			Handler:    application.BundledAssetFileServer(dist),
			Middleware: st.Middleware,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Menu.Set(appMenu(app, st))

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:              "InkStudio",
		Width:              1440,
		Height:             900,
		MinWidth:           900,
		MinHeight:          560,
		URL:                "/",
		UseApplicationMenu: true,
		BackgroundColour:   application.NewRGB(30, 30, 32),
	})

	// A folder on the command line, or the last one opened. The Finder
	// passes no folder here; it sends ApplicationOpenedWithFile instead.
	dir := loadState().LastDir
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		dir = os.Args[1]
	}
	if dir != "" {
		if _, err := st.OpenDir(dir); err != nil {
			log.Println("inkstudio:", err)
		}
	}

	// A folder or .inktheme opened from the Finder, dropped on the Dock
	// icon, or passed to `open -a InkStudio`.
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		path := e.Context().Filename()
		go func() {
			if err := st.OpenPath(path); err != nil {
				app.Dialog.Error().SetTitle("InkStudio").SetMessage(err.Error()).Show()
			}
		}()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func appMenu(app *application.App, st *studio.Studio) *application.Menu {
	report := func(err error) {
		if err != nil {
			app.Dialog.Error().SetTitle("InkStudio").SetMessage(err.Error()).Show()
		}
	}
	menu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}
	file := menu.AddSubmenu("File")
	file.Add("New Presentation…").SetAccelerator("CmdOrCtrl+Shift+N").OnClick(func(*application.Context) {
		_, err := st.PromptNew()
		report(err)
	})
	file.Add("Open…").SetAccelerator("CmdOrCtrl+O").OnClick(func(*application.Context) {
		_, err := st.PromptOpen()
		report(err)
	})
	file.AddSeparator()
	// The frontend owns the buffers, so it does the saving, and saves
	// before it exports.
	file.Add("Save").SetAccelerator("CmdOrCtrl+S").OnClick(func(*application.Context) {
		app.Event.Emit("menu:save")
	})
	file.AddSeparator()
	file.Add("Export PDF…").SetAccelerator("CmdOrCtrl+E").OnClick(func(*application.Context) {
		app.Event.Emit("menu:export", "pdf")
	})
	file.Add("Export PowerPoint…").SetAccelerator("CmdOrCtrl+Shift+E").OnClick(func(*application.Context) {
		app.Event.Emit("menu:export", "pptx")
	})
	file.Add("Export OpenDocument…").OnClick(func(*application.Context) {
		app.Event.Emit("menu:export", "odp")
	})
	file.AddSeparator()
	file.Add("Import Theme…").OnClick(func(*application.Context) {
		app.Event.Emit("menu:import-theme")
	})
	menu.AddRole(application.EditMenu)
	view := menu.AddSubmenu("View")
	view.Add("Editor").SetAccelerator("CmdOrCtrl+1").OnClick(func(*application.Context) {
		app.Event.Emit("menu:view", "editor")
	})
	view.Add("Presenter").SetAccelerator("CmdOrCtrl+2").OnClick(func(*application.Context) {
		app.Event.Emit("menu:view", "presenter")
	})
	view.AddSeparator()
	view.Add("Toggle Preview").SetAccelerator("CmdOrCtrl+P").OnClick(func(*application.Context) {
		app.Event.Emit("menu:toggle-preview")
	})
	view.Add("Toggle Inspector").SetAccelerator("CmdOrCtrl+Alt+I").OnClick(func(*application.Context) {
		app.Event.Emit("menu:toggle-inspector")
	})
	view.Add("Toggle Notes").SetAccelerator("CmdOrCtrl+Shift+O").OnClick(func(*application.Context) {
		app.Event.Emit("menu:toggle-notes")
	})
	menu.AddRole(application.WindowMenu)
	return menu
}

// fixPath adds the usual install locations to PATH. An app started from the
// Finder gets a minimal PATH, and inkpress runs inkline from it to draw
// diagrams.
func fixPath() {
	dirs := filepath.SplitList(os.Getenv("PATH"))
	have := map[string]bool{}
	for _, d := range dirs {
		have[d] = true
	}
	extra := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		extra = append(extra, filepath.Join(home, "go", "bin"), filepath.Join(home, ".local", "bin"))
	}
	if gp := os.Getenv("GOPATH"); gp != "" {
		extra = append(extra, filepath.Join(gp, "bin"))
	}
	for _, d := range extra {
		if !have[d] {
			dirs = append(dirs, d)
			have[d] = true
		}
	}
	os.Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator)))
}

type state struct {
	LastDir string `json:"lastDir"`
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "inkstudio", "state.json")
}

func loadState() state {
	var s state
	if p := statePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			json.Unmarshal(b, &s)
		}
	}
	return s
}

func saveState(s state) {
	p := statePath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(p, b, 0o644)
}
