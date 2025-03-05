package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func Registry() amp.Registry {
	if gRegistry == nil {
		gRegistry = amp.NewRegistry()
		AddBuiltins(gRegistry)
	}

	return gRegistry
}

func AddBuiltins(reg amp.Registry) {

	// these aren't' reallt in
	prototypes := []tag.Value{
		&amp.Err{},
		&amp.Tag{},
		&amp.Login{},
		&amp.LoginChallenge{},
		&amp.LoginResponse{},
		&amp.LoginCheckpoint{},
		&amp.PinRequest{},
	}

	for _, pi := range prototypes {
		gRegistry.RegisterPrototype(SessionAttr, pi, "")
	}

}

var gRegistry amp.Registry
