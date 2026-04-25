package amp_test

import (
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
)

// TestPlanetInvite_IsExpired_ZeroNeverExpires — ExpiresAt of 0 means the
// issuer opted out of the bootstrap TTL.  Should never report expired.
func TestPlanetInvite_IsExpired_ZeroNeverExpires(t *testing.T) {
	invite := &amp.PlanetInvite{ExpiresAt: 0}
	if invite.IsExpired(time.Now()) {
		t.Error("ExpiresAt=0 must never be expired")
	}
	farFuture := time.Now().Add(100 * 365 * 24 * time.Hour)
	if invite.IsExpired(farFuture) {
		t.Error("ExpiresAt=0 must never be expired even decades on")
	}
}

// TestPlanetInvite_IsExpired_NilSafe — a nil invite is treated as expired so
// callers can chain unconditionally without segfault risk.
func TestPlanetInvite_IsExpired_NilSafe(t *testing.T) {
	var invite *amp.PlanetInvite
	if !invite.IsExpired(time.Now()) {
		t.Error("nil invite must be reported as expired")
	}
}

// TestPlanetInvite_IsExpired_BoundaryEquality — ExpiresAt exactly equal to
// now() is NOT expired.  Strict `>` semantics: the last second of validity
// belongs to the invite.
func TestPlanetInvite_IsExpired_BoundaryEquality(t *testing.T) {
	expiry := time.Unix(1_700_000_000, 0)
	invite := &amp.PlanetInvite{ExpiresAt: expiry.Unix()}

	if invite.IsExpired(expiry) {
		t.Error("now == ExpiresAt must NOT be reported as expired (strict >)")
	}
	if !invite.IsExpired(expiry.Add(time.Second)) {
		t.Error("now == ExpiresAt + 1s must be reported as expired")
	}
	if invite.IsExpired(expiry.Add(-time.Second)) {
		t.Error("now == ExpiresAt - 1s must NOT be reported as expired")
	}
}

// TestVaultOpts_BootstrapTTL_DefaultAndOverride — nil opts and zero field both
// fall through to DefaultBootstrapTTL; non-zero override takes precedence.
func TestVaultOpts_BootstrapTTL_DefaultAndOverride(t *testing.T) {
	defaultTTL := time.Duration(amp.DefaultBootstrapTTL) * time.Second

	if got := (*amp.VaultOpts)(nil).BootstrapTTL(); got != defaultTTL {
		t.Errorf("nil VaultOpts: got %v want %v", got, defaultTTL)
	}
	if got := (&amp.VaultOpts{}).BootstrapTTL(); got != defaultTTL {
		t.Errorf("zero VaultOpts: got %v want %v", got, defaultTTL)
	}
	if got := (&amp.VaultOpts{BootstrapTTLSecs: -5}).BootstrapTTL(); got != defaultTTL {
		t.Errorf("negative override must fall through to default: got %v want %v", got, defaultTTL)
	}

	override := int64(3600)
	want := time.Hour
	if got := (&amp.VaultOpts{BootstrapTTLSecs: override}).BootstrapTTL(); got != want {
		t.Errorf("explicit 3600s override: got %v want %v", got, want)
	}
}
