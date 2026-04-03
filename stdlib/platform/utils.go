package platform

import (
	"io"
	"os"
	"os/exec"
)

// Stdout is the io.Writer to which executed commands write standard output.
var Stdout io.Writer = os.Stdout

// Stderr is the io.Writer to which executed commands write standard error.
var Stderr io.Writer = os.Stderr

// LaunchURL() pushes an OS-level event to open the given URL using the user's default / primary browser.
//
// For future-proofing, use this instead of browser.OpenURL().
func LaunchURL(url string) error {
	return runCmd("open", url)
}

func runCmd(prog string, args ...string) error {
	cmd := exec.Command(prog, args...)
	cmd.Stdout = Stdout
	cmd.Stderr = Stderr
	return cmd.Run()
}
