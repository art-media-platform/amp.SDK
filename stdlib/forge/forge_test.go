package forge_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/forge"
)

//"github.com/alecthomas/repr"

func TestINI(t *testing.T) {

	ini, err := forge.ParseINI("forge.test.ini", nil)
	if err != nil {
		t.Fatalf("Failed to parse INI: %v", err)
	}

	// Print the parsed INI for debugging
	t.Fatalf("Parsed INI: %v", ini)

}
