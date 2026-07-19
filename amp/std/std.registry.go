package std

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

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
	reg := &registry{}
	reg.snap.Store(&registrySnap{
		modsByAlias: make(map[string]*amp.AppModule),
		modsByID:    make(map[tag.UID]*amp.AppModule),
		attrDefs:    make(map[tag.UID]amp.AttrDef),
	})
	return reg
}

func RegisterAttr(attr tag.Name, prototype proto.Message, subTags string) tag.Name {
	return registerAttr(attr, prototype, subTags, amp.EditFlow_Fold, 0)
}

// RegisterAttrFolded registers a folded attr with an explicit cell fold depth;
// retainEdits > 1 turns lineage admission on for the attr (SD-edit-resolution §6.3).
func RegisterAttrFolded(attr tag.Name, prototype proto.Message, subTags string, retainEdits int32) tag.Name {
	return registerAttr(attr, prototype, subTags, amp.EditFlow_Fold, retainEdits)
}

// RegisterAttrTape registers a fold-exempt journal-tape attr (EditFlow_Tape):
// the edit axis is the attr's time axis, the cabinet holds zero rows for it,
// and serve sources from journal replay (SD-planet-storage §8.1).
func RegisterAttrTape(attr tag.Name, prototype proto.Message, subTags string) tag.Name {
	return registerAttr(attr, prototype, subTags, amp.EditFlow_Tape, 0)
}

func registerAttr(attr tag.Name, prototype proto.Message, subTags string, editFlow amp.EditFlow, retainEdits int32) tag.Name {
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
		EditFlow:    editFlow,
		RetainEdits: retainEdits,
	})
	if err != nil {
		panic(err)
	}

	return attr
}

// registrySnap is one immutable copy-on-write registry state: readers take it
// whole with one atomic load and never see a mutation.
type registrySnap struct {
	modsByAlias map[string]*amp.AppModule
	modsByID    map[tag.UID]*amp.AppModule
	attrDefs    map[tag.UID]amp.AttrDef
}

// clone deep-copies the snapshot's maps for a registrar's insert-then-publish.
func (snap *registrySnap) clone() *registrySnap {
	next := &registrySnap{
		modsByAlias: make(map[string]*amp.AppModule, len(snap.modsByAlias)+1),
		modsByID:    make(map[tag.UID]*amp.AppModule, len(snap.modsByID)+1),
		attrDefs:    make(map[tag.UID]amp.AttrDef, len(snap.attrDefs)+1),
	}
	for alias, mod := range snap.modsByAlias {
		next.modsByAlias[alias] = mod
	}
	for modID, mod := range snap.modsByID {
		next.modsByID[modID] = mod
	}
	for attrID, def := range snap.attrDefs {
		next.attrDefs[attrID] = def
	}
	return next
}

// Implements Registry.  Registrars (init-time, rare) serialize on writeMu and
// publish a cloned snapshot; FindAttr / FindModule / NewValue take one atomic
// load with no lock — no reader/writer contention on the merge hot path.
type registry struct {
	snap    atomic.Pointer[registrySnap]
	writeMu sync.Mutex
}

func (reg *registry) RegisterAttr(def amp.AttrDef) error {
	attrID := def.Name.ID
	if attrID.IsNil() {
		return status.Code_BadTag.Errorf("RegisterAttr: missing Attr.ID")
	}
	if def.EditFlow == amp.EditFlow_Tape && def.RetainEdits != 0 {
		return status.Code_BadRequest.Errorf("RegisterAttr: %q: RetainEdits is meaningless on a Tape attr", def.Name.Canonic())
	}

	reg.writeMu.Lock()
	defer reg.writeMu.Unlock()

	// An attr's storage policy is write-once: the fold and serve resolve it
	// process-statically, so re-registration may never change it.  An
	// IDENTICAL policy no-ops before any clone — the common redundant init
	// publishes nothing.
	snap := reg.snap.Load()
	if prev, exists := snap.attrDefs[attrID]; exists {
		if prev.EditFlow != def.EditFlow || prev.RetainEdits != def.RetainEdits {
			return status.Code_BadRequest.Errorf("RegisterAttr: %q: storage policy already registered differently", def.Name.Canonic())
		}
		return nil
	}
	next := snap.clone()
	next.attrDefs[attrID] = def
	reg.snap.Store(next)
	return nil
}

func (reg *registry) FindAttr(attrID tag.UID) (amp.AttrDef, bool) {
	snap := reg.snap.Load()
	def, exists := snap.attrDefs[attrID]
	return def, exists
}

// Implements Registry
func (reg *registry) RegisterModule(mod *amp.AppModule) error {
	modID := mod.Info.Name.ID

	reg.writeMu.Lock()
	defer reg.writeMu.Unlock()

	next := reg.snap.Load().clone()
	next.modsByID[modID] = mod

	for _, alias := range mod.Info.Aliases {
		if alias != "" {
			next.modsByAlias[alias] = mod
		}
	}
	// Module aliases resolve case-insensitively, so key them by the folded
	// canonic form and its leaf component.
	canonic := mod.Info.Name.Canonic()
	next.modsByAlias[canonic] = mod
	if dot := strings.LastIndexByte(canonic, tag.CanonicSeparatorChar); dot >= 0 {
		next.modsByAlias[canonic[dot+1:]] = mod
	}

	reg.snap.Store(next)
	return nil
}

func (reg *registry) FindModule(moduleID tag.UID, moduleAlias string) *amp.AppModule {
	snap := reg.snap.Load()

	var mod *amp.AppModule
	if moduleID.IsSet() {
		mod = snap.modsByID[moduleID]
	}
	if mod == nil && moduleAlias != "" {
		mod = snap.modsByAlias[moduleAlias]
	}
	return mod
}

// Makes an instance of the given attribute "spec"" tag.UID
func (reg *registry) NewValue(attrID tag.UID) (proto.Message, error) {

	// Often, an attrID will be a unnamed scalar attr (which means we can get the elemDef directly.
	// This is also essential during bootstrapping when the client sends a RegisterDefs is not registered yet.
	def, exists := reg.snap.Load().attrDefs[attrID]
	if !exists {
		return nil, status.Code_ItemNotFound.Errorf("attr %q not found", attrID.String())
	}
	return data.NewLike(def.Prototype), nil
}
