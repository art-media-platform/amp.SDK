package std

import (
	"reflect"
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// see type Registry
var gRegistry = NewRegistry()

func Registry() amp.Registry {
	return gRegistry
}

func NewRegistry() amp.Registry {
	reg := &registry{
		appsByInvoke: make(map[string]*amp.App),
		appsByTag:    make(map[tag.ID]*amp.App),
		elemDefs:     make(map[tag.ID]amp.AttrDef),
		attrDefs:     make(map[tag.ID]amp.AttrDef),
	}
	return reg
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

// Implements Registry
type registry struct {
	mu           sync.RWMutex
	appsByInvoke map[string]*amp.App
	appsByTag    map[tag.ID]*amp.App
	elemDefs     map[tag.ID]amp.AttrDef
	attrDefs     map[tag.ID]amp.AttrDef
}

func (reg *registry) RegisterAttr(def amp.AttrDef) error {

	if def.ID.IsNil() || def.ID == amp.HeadCellID {
		return amp.ErrCode_BadTag.Errorf("RegisterAttr: missing Attr.ID")
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if _, exist := reg.elemDefs[def.ID]; !exist {
		reg.attrDefs[def.ID] = def
	}
	return nil
}

func (reg *registry) Import(other amp.Registry) error {
	src := other.(*registry)

	src.mu.Lock()
	defer src.mu.Unlock()

	{
		reg.mu.Lock()
		for _, def := range src.elemDefs {
			reg.elemDefs[def.ID] = def
		}
		for _, def := range src.attrDefs {
			reg.attrDefs[def.ID] = def
		}
		reg.mu.Unlock()
	}

	for _, app := range src.appsByTag {
		if err := reg.RegisterApp(app); err != nil { //fix me
			return err
		}
	}
	return nil
}

// Implements Registry
func (reg *registry) RegisterApp(app *amp.App) error {
	appTag := app.AppSpec.ID

	reg.mu.Lock()
	defer reg.mu.Unlock()

	reg.appsByTag[appTag] = app

	for _, invok := range app.Invocations {
		if invok != "" {
			reg.appsByInvoke[invok] = app
		}
	}

	// invoke by full app ID
	reg.appsByInvoke[app.AppSpec.Canonic] = app

	// invoke by first component of app ID
	_, leafName := app.AppSpec.LeafTags(1)
	reg.appsByInvoke[leafName] = app

	return nil
}

// Implements Registry
func (reg *registry) GetAppByTag(appTag tag.ID) (*amp.App, error) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	app := reg.appsByTag[appTag]
	if app == nil {
		return nil, amp.ErrCode_AppNotFound.Errorf("app not found: %s", appTag)
	} else {
		return app, nil
	}
}

// type interface Registry
func (reg *registry) GetAppForInvocation(invocation string) (*amp.App, error) { // TODO refactor to use tag,ID
	if invocation == "" {
		return nil, amp.ErrCode_AppNotFound.Errorf("missing app invocation")
	}

	reg.mu.RLock()
	defer reg.mu.RUnlock()

	app := reg.appsByInvoke[invocation]
	if app == nil {
		return nil, amp.ErrCode_AppNotFound.Errorf("app not found: %q", invocation)
	}
	return app, nil
}

// Makes an instance of the given attribute "spec"" tag.ID
func (reg *registry) MakeValue(attrExprID tag.ID) (tag.Value, error) {

	// Often, an attrID will be a unnamed scalar attr (which means we can get the elemDef directly.
	// This is also essential during bootstrapping when the client sends a RegisterDefs is not registered yet.
	def, exists := reg.elemDefs[attrExprID]
	if !exists {
		def, exists = reg.attrDefs[attrExprID]
		if !exists {
			return nil, amp.ErrCode_AttrNotFound.Errorf("MakeValue: attr %s not found", attrExprID.String())
		}
	}
	return def.Prototype.New(), nil
}

/*


TODO func MakeSchemaForType(valTyp reflect.Type) (*AttrSchema, error) {
	numFields := valTyp.NumField()

	schema := &AttrSchema{
		CellDataModel: valTyp.Name(),
		SchemaName:    "on-demand-reflect",
		Attrs:         make([]*tag.Expr, 0, numFields),
	}

	for i := 0; i < numFields; i++ {

		// Importantly, TagExprID is always set to the field index + 1, so we know what field to inspect when given an TagExprID.
		field := valTyp.Field(i)
		if !field.IsExported() {
			continue
		}

		attr := &tag.Expr{
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
			return nil, ErrCode_ExportErr.Errorf("unsupported type '%s.%s (%v)", schema.CellDataModel, attr.TypedName, attrKind)
		}

		schema.Attrs = append(schema.Attrs, attr)
	}
	return schema, nil
}

// ReadCell loads a cell with the given URI having the inferred schema (built from its fields using reflection).
// The URI is scoped into the user's home space and AppID.
func ReadCell(ctx AppContext, subKey string, schema *AttrSchema, dstStruct any) error {

	dst := reflect.Indirect(reflect.ValueOf(dstStruct))
	switch dst.Kind() {
	case reflect.Pointer:
		dst = dst.Elem()
	case reflect.Struct:
	default:
		return ErrCode_ExportErr.Errorf("expected struct, got %v", dst.Kind())
	}

	var keyBuf [128]byte
	cellKey := append(append(keyBuf[:0], []byte(ctx.StateScope())...), []byte(subKey)...)

	msgs := make([]*Msg, 0, len(schema.Attrs))
	err := ctx.LoginInfo().HomePlanet().ReadCell(cellKey, schema, func(msg *Msg) {
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

// WriteCell is the write analog of ReadCell.
func WriteCell(ctx AppContext, subKey string, schema *AttrSchema, srcStruct any) error {

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
		msg.Op = MsgOp_UpsertCell
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
