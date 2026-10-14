package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// VaultAttestationDigest is the one digest site of a member's vault
// attestation (AOM SD-security-sync.md §5.8): the builtin hash kit over the
// membership planet, the member, the vault and the founded planet under
// SigningDomain_MemberVault.  The signer passes it to Enclave.SignRaw; the
// relay verifies the signature over it with the member's recorded key.
func VaultAttestationDigest(planetID, memberID, vaultID, foundedID tag.UID) ([]byte, error) {
	if planetID.IsNil() || memberID.IsNil() || vaultID.IsNil() || foundedID.IsNil() {
		return nil, status.Code_BadRequest.Error("amp: a vault attestation names a planet, a member, a vault and a founded planet")
	}
	return safe.SigningDigest(safe.HashKitID_Blake2s_256, safe.SigningDomain_MemberVault,
		planetID.AppendTo(nil),
		memberID.AppendTo(nil),
		vaultID.AppendTo(nil),
		foundedID.AppendTo(nil),
	)
}

// PlanetID is the planet the attestation claims membership on.
func (att *MemberVaultAttestation) PlanetID() tag.UID {
	return tag.UID{att.GetPlanetID_0(), att.GetPlanetID_1()}
}

// MemberID is the signing member.
func (att *MemberVaultAttestation) MemberID() tag.UID {
	return tag.UID{att.GetMemberID_0(), att.GetMemberID_1()}
}

// VaultID is the attested vault.
func (att *MemberVaultAttestation) VaultID() tag.UID {
	return tag.UID{att.GetVaultID_0(), att.GetVaultID_1()}
}

// FoundedID is the planet the device founded that the claim admits.
func (att *MemberVaultAttestation) FoundedID() tag.UID {
	return tag.UID{att.GetFoundedID_0(), att.GetFoundedID_1()}
}

// Digest is VaultAttestationDigest over the attestation's own fields.
func (att *MemberVaultAttestation) Digest() ([]byte, error) {
	return VaultAttestationDigest(att.PlanetID(), att.MemberID(), att.VaultID(), att.FoundedID())
}
