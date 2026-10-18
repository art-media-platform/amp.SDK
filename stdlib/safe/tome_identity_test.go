package safe_test

// A store without TomeStoreIdentity coordinates by its own value, which must
// be comparable as held — a comparable type can hold an uncomparable value in
// an interface field, and that value cannot key the coordination map.

import (
	"context"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
)

// valueTomeStore is a comparable struct type; its tag field may hold an
// uncomparable dynamic value.
type valueTomeStore struct {
	tag any
}

func (valueTomeStore) Load(context.Context) (*safe.SealedTome, error) {
	return nil, nil
}

func (valueTomeStore) Save(context.Context, *safe.SealedTome) error {
	return nil
}

func TestOpen_RefusesAnUncomparableStoreValue(t *testing.T) {
	ctx := context.Background()
	guard := safe.NewFileGuard([]byte("pass"), []byte(closeTestAAD))
	defer guard.Close()
	store := valueTomeStore{tag: []byte("uncomparable")}
	opens := map[string]func() error{
		"OpenEnclave": func() error {
			enc, err := safe.OpenEnclave(ctx, store, guard, []byte(closeTestAAD))
			if err == nil {
				enc.Close(ctx)
			}
			return err
		},
		"OpenEpochKeyStore": func() error {
			eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte(closeTestAAD))
			if err == nil {
				eks.Close(ctx)
			}
			return err
		},
	}
	for name, open := range opens {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("%s panicked on an uncomparable store value: %v", name, recovered)
				}
			}()
			if err := open(); err == nil {
				t.Fatalf("%s accepted an uncomparable store value", name)
			}
		})
	}

	comparableStore := valueTomeStore{tag: "comparable"}
	enc, err := safe.OpenEnclave(ctx, comparableStore, guard, []byte(closeTestAAD))
	if err != nil {
		t.Fatalf("a comparable store value: %v", err)
	}
	closeHandle(t, enc)
}
