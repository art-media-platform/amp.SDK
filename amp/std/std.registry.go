package std

import (
	"reflect"
	"strings"
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// see type Registry interface
var gRegistry = NewRegistry()

func Registry() amp.Registry {
	return gRegistry
}

func NewRegistry() amp.Registry {
	reg := &registry{
		modsByAlias: make(map[string]*amp.AppModule),
		modsByID:    make(map[tag.UID]*amp.AppModule),
		attrDefs:    make(map[tag.UID]amp.AttrDef),
	}
	return reg
}

func RegisterAttr(attr tag.Name, prototype proto.Message, subTags string) tag.Name {
	return registerAttr(attr, prototype, subTags, amp.EditType_Folded, 0)
}

// RegisterAttrFolded registers a folded attr with an explicit cell fold depth;
// retainEdits > 1 turns lineage admission on for the attr (SD-edit-resolution §6.3).
func RegisterAttrFolded(attr tag.Name, prototype proto.Message, subTags string, retainEdits int32) tag.Name {
	return registerAttr(attr, prototype, subTags, amp.EditType_Folded, retainEdits)
}

// RegisterAttrTape registers a fold-exempt journal-tape attr (EditType_Tape):
// the edit axis is the attr's time axis, the cabinet holds zero rows for it,
// and serve sources from journal replay (SD-planet-storage §8.1).
func RegisterAttrTape(attr tag.Name, prototype proto.Message, subTags string) tag.Name {
	return registerAttr(attr, prototype, subTags, amp.EditType_Tape, 0)
}

func registerAttr(attr tag.Name, prototype proto.Message, subTags string, editType amp.EditType, retainEdits int32) tag.Name {
	if subTags != "" {
		attr = attr.With(subTags)
	}
	// Use-scope words lead; the stored message's exact name trails, always —
	// no elided default (ZO §4.8).
	typeOf := reflect.TypeOf(prototype)
	if typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	attr = attr.With(typeOf.Name())

	err := gRegistry.RegisterAttr(amp.AttrDef{
		Name:        attr,
		Prototype:   prototype,
		EditType:    editType,
		RetainEdits: retainEdits,
	})
	if err != nil {
		panic(err)
	}

	return attr
}

// Implements Registry
type registry struct {
	mu          sync.RWMutex
	modsByAlias map[string]*amp.AppModule
	modsByID    map[tag.UID]*amp.AppModule
	attrDefs    map[tag.UID]amp.AttrDef
}

func (reg *registry) RegisterAttr(def amp.AttrDef) error {
	attrID := def.Name.ID
	if attrID.IsNil() {
		return status.Code_BadTag.Errorf("RegisterAttr: missing Attr.ID")
	}
	if def.EditType == amp.EditType_Tape && def.RetainEdits != 0 {
		return status.Code_BadRequest.Errorf("RegisterAttr: %q: RetainEdits is meaningless on a Tape attr", def.Name.Canonic())
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	// An attr's storage policy is write-once: the fold and serve resolve it
	// process-statically, so re-registration may never change it.
	if prev, exists := reg.attrDefs[attrID]; exists {
		if prev.EditType != def.EditType || prev.RetainEdits != def.RetainEdits {
			return status.Code_BadRequest.Errorf("RegisterAttr: %q: storage policy already registered differently", def.Name.Canonic())
		}
	}
	reg.attrDefs[attrID] = def
	return nil
}

func (reg *registry) FindAttr(attrID tag.UID) (amp.AttrDef, bool) {
	reg.mu.RLock()
	def, exists := reg.attrDefs[attrID]
	reg.mu.RUnlock()
	return def, exists
}

// Implements Registry
func (reg *registry) RegisterModule(mod *amp.AppModule) error {
	modID := mod.Info.Name.ID

	reg.mu.Lock()
	defer reg.mu.Unlock()

	reg.modsByID[modID] = mod

	for _, alias := range mod.Info.Aliases {
		if alias != "" {
			reg.modsByAlias[alias] = mod
		}
	}
	// Module aliases resolve case-insensitively, so key them by the folded
	// canonic form and its leaf component.
	canonic := mod.Info.Name.Canonic()
	reg.modsByAlias[canonic] = mod
	if dot := strings.LastIndexByte(canonic, tag.CanonicSeparatorChar); dot >= 0 {
		reg.modsByAlias[canonic[dot+1:]] = mod
	}

	return nil
}

func (reg *registry) FindModule(moduleID tag.UID, moduleAlias string) *amp.AppModule {
	var mod *amp.AppModule

	reg.mu.RLock()
	{
		if moduleID.IsSet() {
			mod = reg.modsByID[moduleID]
		}
		if mod == nil && moduleAlias != "" {
			mod = reg.modsByAlias[moduleAlias]
		}
	}
	reg.mu.RUnlock()

	return mod
}

// Makes an instance of the given attribute "spec"" tag.UID
func (reg *registry) NewValue(attrID tag.UID) (proto.Message, error) {

	// Often, an attrID will be a unnamed scalar attr (which means we can get the elemDef directly.
	// This is also essential during bootstrapping when the client sends a RegisterDefs is not registered yet.
	def, exists := reg.attrDefs[attrID]
	if !exists {
		return nil, status.Code_ItemNotFound.Errorf("attr %q not found", attrID.String())
	}
	return data.NewLike(def.Prototype), nil
}
