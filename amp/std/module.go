package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
)

// NewModuleRef projects an in-process amp.AppModuleInfo into its wire / attr-value
// descriptor: the module's identity, name, one-line caption, and maturity.  Callers
// may enrich the result (Labels.About long-form, Tags glyphs) before publishing it.
func NewModuleRef(info amp.AppModuleInfo) *ModuleRef {
	return &ModuleRef{
		Module: &amp.Tag{
			UID_0: info.Name.ID[0],
			UID_1: info.Name.ID[1],
			Text:  info.Name.Canonic(),
		},
		Labels: &Labels{
			Caption: info.About,
		},
		Version: info.Version,
	}
}
