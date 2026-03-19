package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open opens the given URL in the user's default browser.
func Open(urlStr string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "linux":
		cmd = exec.Command("xdg-open", urlStr)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	default:
		return fmt.Errorf("unsupported platform %s, open manually: %s", runtime.GOOS, urlStr)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
