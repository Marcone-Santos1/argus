// Package notify shows desktop notifications: Notification Center on macOS,
// notify-send (freedesktop) on Linux.
package notify

import (
	"os/exec"
	"runtime"
)

// script receives the title and body as arguments: none of the text becomes code.
const script = `on run argv
display notification (item 2 of argv) with title (item 1 of argv) sound name "Glass"
end run`

// Command builds the notification command for the current OS (nil when there
// is no way to notify).
func Command(title, body string) *exec.Cmd {
	return commandFor(runtime.GOOS, title, body)
}

func commandFor(goos, title, body string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("/usr/bin/osascript", "-e", script, title, body)
	default:
		bin, err := exec.LookPath("notify-send")
		if err != nil {
			return nil
		}
		return exec.Command(bin, "--app-name=argus", "--", title, body)
	}
}

// Send shows the notification without waiting (one process per notification).
func Send(title, body string) {
	cmd := Command(title, body)
	if cmd == nil {
		return
	}
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}
