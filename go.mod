module github.com/inkcheck/inkstudio

go 1.27.3

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/chromedp/chromedp v0.22.1
	github.com/fsnotify/fsnotify v1.10.1
	github.com/inkcheck/inkpress v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/chromedp/cdproto v0.159.2 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/dlclark/regexp2/v2 v2.12.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260820222146-c27c302e5fc3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// Until inkpress publishes its library packages.
replace github.com/inkcheck/inkpress => ../inkpress
