package registry

import (
	"github.com/art-media-platform/amp.SDK/amp"
)

func Global() amp.Registry {
	if gRegistry == nil {
		gRegistry = amp.NewRegistry()
	}
	amp.RegisterBuiltinTypes(gRegistry)
	return gRegistry
}

var gRegistry amp.Registry
