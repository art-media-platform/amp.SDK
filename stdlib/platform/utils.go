package platform

import (
	"github.com/pkg/browser"
)

// LaunchURL() pushes an OS-level event to open the given URL using the user's default / primary browser.
//
// For future-proofing, use this instead of browser.OpenURL().
func LaunchURL(url string) error {
	return browser.OpenURL(url)
}
