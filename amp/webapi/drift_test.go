package webapi_test

// Wire-freeze drift guard.  testdata/*.json are the shared golden fixtures for
// the /api/v1/* JSON contract — the SAME files are asserted by the TypeScript
// SDK (amp-web/src/drift.test.ts), so a silent field rename, json-tag edit, or
// omitempty/omitzero regression on either side fails that side's CI.
//
// Per fixture entry: decode into the Go wire struct with DisallowUnknownFields
// (catches removed/renamed Go fields), re-marshal, and require the two JSON
// trees to be deeply equal as maps (catches dropped populated keys AND
// newly-emitted zero-value keys, order-insensitive).  Fixture entry names are
// "{Shape}" or "{Shape}.{variant}" — the prefix picks the Go type.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/webapi"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
)

// nonShapeFiles hold enum-name goldens and the operator manifest, not struct
// shapes; each has its own test below.
var nonShapeFiles = map[string]bool{
	"access.json":           true,
	"cryptokits.json":       true,
	"operator-go-only.json": true,
}

// shapeCtors maps a fixture entry's shape prefix to its Go wire struct.
var shapeCtors = map[string]func() any{
	"LoginRequest":            func() any { return &webapi.LoginRequest{} },
	"ChallengeResponse":       func() any { return &webapi.ChallengeResponse{} },
	"LoginResponse":           func() any { return &webapi.LoginResponse{} },
	"AmpMember":               func() any { return &webapi.AmpMember{} },
	"SessionResponse":         func() any { return &webapi.SessionResponse{} },
	"EmailCredential":         func() any { return &webapi.EmailCredential{} },
	"EmailIssueResponse":      func() any { return &webapi.EmailIssueResponse{} },
	"ErrorResponse":           func() any { return &webapi.ErrorResponse{} },
	"WithdrawNote":            func() any { return &webapi.WithdrawNote{} },
	"TxOp":                    func() any { return &webapi.TxOp{} },
	"TxRequest":               func() any { return &webapi.TxRequest{} },
	"TxOpResult":              func() any { return &webapi.TxOpResult{} },
	"TxResponse":              func() any { return &webapi.TxResponse{} },
	"Item":                    func() any { return &webapi.Item{} },
	"ListResponse":            func() any { return &webapi.ListResponse{} },
	"SubscribeFrame":          func() any { return &webapi.SubscribeFrame{} },
	"EditEntry":               func() any { return &webapi.EditEntry{} },
	"EditChainResponse":       func() any { return &webapi.EditChainResponse{} },
	"InviteIssueRequest":      func() any { return &webapi.InviteIssueRequest{} },
	"InviteIssueResponse":     func() any { return &webapi.InviteIssueResponse{} },
	"InviteAcceptRequest":     func() any { return &webapi.InviteAcceptRequest{} },
	"InviteAcceptResponse":    func() any { return &webapi.InviteAcceptResponse{} },
	"InviteRevokeRequest":     func() any { return &webapi.InviteRevokeRequest{} },
	"InviteListResponse":      func() any { return &webapi.InviteListResponse{} },
	"GovernanceGrantRequest":  func() any { return &webapi.GovernanceGrantRequest{} },
	"GovernanceGrantResponse": func() any { return &webapi.GovernanceGrantResponse{} },
	"VaultEndpoint":           func() any { return &webapi.VaultEndpoint{} },
	"ResolveRequest":          func() any { return &webapi.ResolveRequest{} },
	"ResolveResponse":         func() any { return &webapi.ResolveResponse{} },
	"SearchRequest":           func() any { return &webapi.SearchRequest{} },
	"SearchResponse":          func() any { return &webapi.SearchResponse{} },
	"FederationPeersResponse": func() any { return &webapi.FederationPeersResponse{} },
	"MediaResolveRequest":     func() any { return &webapi.MediaResolveRequest{} },
	"Tag":                     func() any { return &amp.Tag{} },

	// Operator tier (Go-only by design — see testdata/operator-go-only.json;
	// TestOperatorGoOnlyManifest pins these registrations to that manifest).
	"PlanetCreateRequest":   func() any { return &webapi.PlanetCreateRequest{} },
	"PlanetCreateResponse":  func() any { return &webapi.PlanetCreateResponse{} },
	"BrandSetRequest":       func() any { return &webapi.BrandSetRequest{} },
	"BrandSetResponse":      func() any { return &webapi.BrandSetResponse{} },
	"ForumsReserveRequest":  func() any { return &webapi.ForumsReserveRequest{} },
	"ForumsReserveResponse": func() any { return &webapi.ForumsReserveResponse{} },
}

func TestWireFixturesRoundTrip(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures under testdata/")
	}

	shapesSeen := map[string]bool{}
	for _, file := range files {
		if nonShapeFiles[filepath.Base(file)] {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		entries := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("%s: %v", file, err)
		}

		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			shape, _, _ := strings.Cut(name, ".")
			ctor := shapeCtors[shape]
			if ctor == nil {
				t.Errorf("%s: fixture %q has no Go shape registered — add it to shapeCtors", file, name)
				continue
			}
			shapesSeen[shape] = true
			roundTrip(t, fmt.Sprintf("%s: %s", filepath.Base(file), name), entries[name], ctor())
		}
	}

	// Reverse coverage: a registered shape with no fixture means a fixture
	// entry was deleted (or renamed) without updating this registry.
	for shape := range shapeCtors {
		if !shapesSeen[shape] {
			t.Errorf("shape %s registered but no fixture entry exercises it", shape)
		}
	}
}

// roundTrip decodes the fixture into target rejecting unknown fields, then
// re-marshals and requires deep JSON-tree equality with the fixture.
func roundTrip(t *testing.T, label string, fixture json.RawMessage, target any) {
	dec := json.NewDecoder(bytes.NewReader(fixture))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		t.Errorf("%s: decode into %T: %v", label, target, err)
		return
	}

	remarshaled, err := json.Marshal(target)
	if err != nil {
		t.Errorf("%s: re-marshal %T: %v", label, target, err)
		return
	}

	var want, got map[string]any
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Errorf("%s: fixture is not a JSON object: %v", label, err)
		return
	}
	if err := json.Unmarshal(remarshaled, &got); err != nil {
		t.Errorf("%s: re-marshaled %T is not a JSON object: %v", label, target, err)
		return
	}
	if reflect.DeepEqual(want, got) {
		return
	}

	// Actionable diff: per-key missing / extra / changed.
	for key, wantVal := range want {
		gotVal, ok := got[key]
		switch {
		case !ok:
			t.Errorf("%s: key %q dropped by round trip (omitempty/omitzero or json-tag drift?)", label, key)
		case !reflect.DeepEqual(wantVal, gotVal):
			t.Errorf("%s: key %q changed by round trip:\n  fixture:    %v\n  remarshal:  %v", label, key, wantVal, gotVal)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("%s: round trip emitted key %q absent from the fixture (lost omitempty/omitzero, or fixture is missing a required field)", label, key)
		}
	}
}

// TestOperatorGoOnlyManifest pins the operator-tier manifest: every listed
// verb's request/response shape must be registered in shapeCtors (and so
// fixture-guarded by TestWireFixturesRoundTrip's reverse coverage).  The TS
// side asserts the inverse against the same file — that the browser client
// has no binding for these verbs (NodeAdminModule=true verbs bind only in the
// Node-only admin module) — so an operator verb cannot silently gain a
// browser method or silently fall out of the drift guard.
func TestOperatorGoOnlyManifest(t *testing.T) {
	manifest := struct {
		Comment string
		Verbs   []struct {
			Verb            string
			Endpoint        string
			Request         string
			Response        string
			NodeAdminModule bool
		}
	}{}
	loadJSON(t, filepath.Join("testdata", "operator-go-only.json"), &manifest)

	if len(manifest.Verbs) == 0 {
		t.Fatal("operator manifest lists no verbs")
	}
	for _, verb := range manifest.Verbs {
		if verb.Verb == "" || !strings.HasPrefix(verb.Endpoint, "/api/v1/admin/") {
			t.Errorf("manifest verb %q: endpoint %q is not an /api/v1/admin/ surface", verb.Verb, verb.Endpoint)
		}
		for _, shape := range []string{verb.Request, verb.Response} {
			if shapeCtors[shape] == nil {
				t.Errorf("manifest verb %q: shape %s not registered in shapeCtors — operator shape outside the drift guard", verb.Verb, shape)
			}
		}
	}
}

// TestAccessGolden pins the enum NAMES that ride the JSON wire.  The TS side
// asserts the same file against its string-literal unions.
func TestAccessGolden(t *testing.T) {
	golden := struct {
		AccessLevels    []string
		WithdrawReasons []string
		InviteStatuses  []string
		TrustStates     []string
	}{}
	loadJSON(t, filepath.Join("testdata", "access.json"), &golden)

	// Every golden mirrors its full wire vocabulary — set-equal, so adding an
	// enum value Go-side forces the golden (and the TS union) to move.
	// AccessLevels is the full amp.Access set: the grant endpoints (invite
	// issue, governance grant) parse any Access enum name, and NotAllowed is
	// meaningful as an explicit-deny governance grant.
	assertNameSetEqual(t, "Access", golden.AccessLevels, amp.Access_name)
	assertNameSetEqual(t, "WithdrawReason", golden.WithdrawReasons, amp.WithdrawReason_name, "UnspecifiedReason")
	assertNameSetEqual(t, "InviteStatus", golden.InviteStatuses, amp.InviteStatus_name)
	assertNameSetEqual(t, "TrustState", golden.TrustStates, amp.TrustState_name)
}

// assertNameSetEqual requires golden to equal the proto enum's name set,
// minus any excluded names (invalid-on-the-wire defaults).
func assertNameSetEqual(t *testing.T, enum string, golden []string, names map[int32]string, exclude ...string) {
	excluded := map[string]bool{}
	for _, name := range exclude {
		excluded[name] = true
	}
	want := map[string]bool{}
	for _, name := range names {
		if !excluded[name] {
			want[name] = true
		}
	}
	for _, name := range golden {
		if !want[name] {
			t.Errorf("%s golden %q is not a wire-valid Go enum name", enum, name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("%s enum name %q missing from the golden (added Go-side without updating the contract?)", enum, name)
	}
}

// TestCryptoKitsGolden pins the TS CryptoKitID small-int table to the Go
// safe.Crypto tag.UID registry.  The TS side asserts the same file against
// its CryptoKitID const table; the GoUID column is authoritative here.
func TestCryptoKitsGolden(t *testing.T) {
	golden := struct {
		Kits []struct {
			Name    string
			TsKitID int
			GoUID   string
		}
	}{}
	loadJSON(t, filepath.Join("testdata", "cryptokits.json"), &golden)

	goKits := map[string]string{
		"Poly25519": safe.Crypto.Poly25519.ID.String(),
		"P256":      safe.Crypto.P256.ID.String(),
		"Secp256k1": safe.Crypto.Secp256k1.ID.String(),
	}

	for _, kit := range golden.Kits {
		if kit.Name == "Unspecified" {
			if kit.GoUID != "" {
				t.Errorf("Unspecified kit must carry no Go UID, got %q", kit.GoUID)
			}
			continue
		}
		want, ok := goKits[kit.Name]
		if !ok {
			t.Errorf("kit golden %q has no safe.Crypto counterpart", kit.Name)
			continue
		}
		if kit.GoUID != want {
			t.Errorf("kit %s: golden GoUID %q != safe.Crypto UID %q", kit.Name, kit.GoUID, want)
		}
		delete(goKits, kit.Name)
	}
	for name := range goKits {
		t.Errorf("safe.Crypto kit %s missing from the golden", name)
	}
}

func loadJSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
