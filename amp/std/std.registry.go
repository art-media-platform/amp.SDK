package std

import (
	"reflect"
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
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

func RegisterAttr(attr tag.Name, prototype amp.Value, subTags string) tag.Name {
	typeOf := reflect.TypeOf(prototype)
	if typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}
	name := typeOf.Name()
	attrID := attr.With(name)
	if subTags != "" {
		attrID = attrID.With(subTags)
	}

	err := gRegistry.RegisterAttr(amp.AttrDef{
		Name:      attrID,
		Prototype: prototype,
	})
	if err != nil {
		panic(err)
	}

	return attrID
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
		return amp.ErrCode_BadTag.Errorf("RegisterAttr: missing Attr.ID")
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.attrDefs[attrID] = def
	return nil
}

func (reg *registry) Import(other amp.Registry) error {
	src := other.(*registry)

	src.mu.Lock()
	defer src.mu.Unlock()

	{
		reg.mu.Lock()
		for _, def := range src.attrDefs {
			reg.attrDefs[def.Name.ID] = def
		}
		reg.mu.Unlock()
	}

	for _, mod := range src.modsByID {
		if err := reg.RegisterModule(mod); err != nil {
			return err
		}
	}
	return nil
}

// Implements Registry
func (reg *registry) RegisterModule(mod *amp.AppModule) error {
	modID := mod.Info.Tag.ID

	reg.mu.Lock()
	defer reg.mu.Unlock()

	reg.modsByID[modID] = mod

	for _, alias := range mod.Info.Aliases {
		if alias != "" {
			reg.modsByAlias[alias] = mod
		}
	}
	reg.modsByAlias[mod.Info.Tag.Canonic] = mod

	// invoke by first component of mod ID
	_, leafName := mod.Info.Tag.LeafTags(1)
	reg.modsByAlias[leafName] = mod

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
func (reg *registry) MakeValue(attrID tag.UID) (amp.Value, error) {

	// Often, an attrID will be a unnamed scalar attr (which means we can get the elemDef directly.
	// This is also essential during bootstrapping when the client sends a RegisterDefs is not registered yet.
	def, exists := reg.attrDefs[attrID]
	if !exists {
		return nil, amp.ErrCode_AttrNotFound.Errorf("attr %q not found", attrID.String())
	}
	return def.Prototype.New(), nil
}

/*


TODO func MakeSchemaForType(valTyp reflect.Type) (*AttrSchema, error) {
	numFields := valTyp.NumField()

	schema := &AttrSchema{
		ItemDataModel: valTyp.Name(),
		SchemaName:    "on-demand-reflect",
		Attrs:         make([]*tag.Name, 0, numFields),
	}

	for i := 0; i < numFields; i++ {

		// Importantly, TagExprID is always set to the field index + 1, so we know what field to inspect when given an TagExprID.
		field := valTyp.Field(i)
		if !field.IsExported() {
			continue
		}

		attr := &tag.Name{
			TypedName: field.Name,
			TagExprID:  int32(i + 1),
		}

		attrType := field.Type
		attrKind := attrType.Kind()
		switch attrKind {
		case reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64:
			attr.ValTypeID = int32(ValType_int)
		case reflect.String:
			attr.ValTypeID = int32(ValType_string)
		case reflect.Slice:
			elementType := attrType.Elem().Kind()
			switch elementType {
			case reflect.Uint8, reflect.Int8:
				attr.ValTypeID = int32(ValType_bytes)
			}
		}

		if attr.ValTypeID == 0 {
			return nil, ErrCode_ExportErr.Errorf("unsupported type '%s.%s (%v)", schema.ItemDataModel, attr.TypedName, attrKind)
		}

		schema.Attrs = append(schema.Attrs, attr)
	}
	return schema, nil
}

// ReadItem loads a item with the given URI having the inferred schema (built from its fields using reflection).
// The URI is scoped into the user's home space and AppID.
func ReadItem(ctx AppContext, subKey string, schema *AttrSchema, dstStruct any) error {

	dst := reflect.Indirect(reflect.ValueOf(dstStruct))
	switch dst.Kind() {
	case reflect.Pointer:
		dst = dst.Elem()
	case reflect.Struct:
	default:
		return ErrCode_ExportErr.Errorf("expected struct, got %v", dst.Kind())
	}

	var keyBuf [128]byte
	itemKey := append(append(keyBuf[:0], []byte(ctx.StateScope())...), []byte(subKey)...)

	msgs := make([]*Msg, 0, len(schema.Attrs))
	err := ctx.LoginInfo().HomePlanet().ReadItem(itemKey, schema, func(msg *Msg) {
		switch msg.Op {
		case MsgOp_PushAttr:
			msgs = append(msgs, msg)
		}
	})
	if err != nil {
		return err
	}

	numFields := dst.NumField()
	valType := dst.Type()

	for fi := 0; fi < numFields; fi++ {
		field := valType.Field(fi)
		for _, ai := range schema.Attrs {
			if ai.TypedName == field.Name {
				for _, msg := range msgs {
					if msg.TagExprID == ai.TagExprID {
						msg.LoadVal(dst.Field(fi).Addr().Interface())
						goto nextField
					}
				}
			}
		}
	nextField:
	}
	return err
}

// WriteItem is the write analog of ReadItem.
func WriteItem(ctx AppContext, subKey string, schema *AttrSchema, srcStruct any) error {

	src := reflect.Indirect(reflect.ValueOf(srcStruct))
	switch src.Kind() {
	case reflect.Pointer:
		src = src.Elem()
	case reflect.Struct:
	default:
		return ErrCode_ExportErr.Errorf("expected struct, got %v", src.Kind())
	}

	{
		tx := NewMsgBatch()
		msg := tx.AddMsg()
		msg.Op = MsgOp_UpsertItem
		msg.ValType = ValType_SchemaID.Ord()
		msg.ValInt = int64(schema.SchemaID)
		msg.ValBuf = append(append(msg.ValBuf[:0], []byte(ctx.StateScope())...), []byte(subKey)...)

		numFields := src.NumField()
		valType := src.Type()

		for _, attr := range schema.Attrs {
			msg := tx.AddMsg()
			msg.Op = MsgOp_PushAttr
			msg.TagExprID = attr.TagExprID
			for i := 0; i < numFields; i++ {
				if valType.Field(i).Name == attr.TypedName {
					msg.setVal(src.Field(i).Interface())
					break
				}
			}
			if msg.ValType == ValType_nil.Ord() {
				panic("missing field")
			}
		}

		msg = tx.AddMsg()
		msg.Op = MsgOp_Commit

		if err := ctx.LoginInfo().HomePlanet().PushTx(tx); err != nil {
			return err
		}
	}

	return nil
}
*/
