package browser

import (
	"context"
	"os/exec"
	"runtime"
)

// Open asks the operating system to open rawURL in the user's default browser.
// It returns after the launcher process starts; login polling continues while
// the browser or OS approval UI runs independently.
func Open(ctx context.Context, rawURL string) error {
	cmd := command(ctx, runtime.GOOS, rawURL)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

// command builds the platform opener without going through a shell, so the URL
// is passed as data instead of being interpreted as shell syntax.
func command(ctx context.Context, goos, rawURL string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.CommandContext(ctx, "open", rawURL)
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return exec.CommandContext(ctx, "xdg-open", rawURL)
	}
}
