//go:build darwin

package studio

import (
	"os"
	"runtime"
	"testing"

	"github.com/inkcheck/inkstudio/platform/webkit"
)

func init() { runtime.LockOSThread() }

// TestMain runs the tests beside the main run loop, which WebKit needs, as
// in the app.
func TestMain(m *testing.M) {
	go func() { os.Exit(m.Run()) }()
	webkit.RunMainLoop()
}
