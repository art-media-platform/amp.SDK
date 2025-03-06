package std

import (
	"reflect"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var gRegistry = amp.NewRegistry()

func Registry() amp.Registry {
	return gRegistry
}

func RegisterApp(app *amp.App) {
	err := gRegistry.RegisterApp(app)
	if err != nil {
		panic(err)
	}
}

func RegisterAttr(attr tag.Expr, prototype tag.Value, subTags string) tag.Expr {

	typeOf := reflect.TypeOf(prototype)
	if typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}
	attrID := attr.With(typeOf.Name())
	if subTags != "" {
		attrID = attrID.With(subTags)
	}

	err := gRegistry.RegisterAttr(amp.AttrDef{
		Expr:      attrID,
		Prototype: prototype,
	})
	if err != nil {
		panic(err)
	}

	return attrID
}
