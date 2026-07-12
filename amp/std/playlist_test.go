package std

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// playlist_test.go proves source selection over the AD-playlists §5 example
// bag: two blob tiers, a direct URL, and a provider scheme — the three pin
// kinds as peers.

func sourcesBag() *amp.Tags {
	blob320 := &amp.Tag{ContentTypeRaw: "audio/mpeg", I: 320, Units: amp.Units_BitrateKbps}
	blob320.SetID(tag.UID{0xA, 0x1}) // any non-nil UID reads as a blob asset key
	blob96 := &amp.Tag{ContentTypeRaw: "audio/mpeg", I: 96, Units: amp.Units_BitrateKbps}
	blob96.SetID(tag.UID{0xA, 0x2})
	url320 := &amp.Tag{URI: "https://cdn.example/x.mp3", ContentTypeRaw: "audio/mpeg", I: 320, Units: amp.Units_BitrateKbps}
	spot := &amp.Tag{URI: "spotify:track:4uLU6hMCjMI75M1A2tKUQC", ContentTypeRaw: "audio/x-spotify"}
	return amp.NewTags(nil, blob320, blob96, url320, spot)
}

func TestSelectBestSource(t *testing.T) {
	bag := sourcesBag()

	tests := []struct {
		name     string
		want     SourceCriteria
		wantKbps int64
		wantBlob bool
		wantURI  string
	}{
		{"unconstrained: highest bitrate, blob beats URL", SourceCriteria{}, 320, true, ""},
		{"offline: blobs only", SourceCriteria{Offline: true}, 320, true, ""},
		{"cap 128: highest within cap", SourceCriteria{MaxKbps: 128}, 96, true, ""},
		{"cap 64: nothing fits, lowest overshoot", SourceCriteria{MaxKbps: 64}, 96, true, ""},
		{"codec family prefix", SourceCriteria{ContentType: "audio/"}, 320, true, ""},
		{"provider scheme only", SourceCriteria{ContentType: "audio/x-spotify"}, 0, false, "spotify:track:4uLU6hMCjMI75M1A2tKUQC"},
	}
	for _, tt := range tests {
		got := SelectBestSource(bag, tt.want)
		if got == nil {
			t.Errorf("%s: no source selected", tt.name)
			continue
		}
		if kbps := sourceKbps(got); kbps != tt.wantKbps {
			t.Errorf("%s: kbps = %d, want %d", tt.name, kbps, tt.wantKbps)
		}
		if SourceIsBlob(got) != tt.wantBlob {
			t.Errorf("%s: blob = %v, want %v", tt.name, SourceIsBlob(got), tt.wantBlob)
		}
		if tt.wantURI != "" && got.URI != tt.wantURI {
			t.Errorf("%s: URI = %q, want %q", tt.name, got.URI, tt.wantURI)
		}
	}

	if src := SelectBestSource(bag, SourceCriteria{ContentType: "video/"}); src != nil {
		t.Errorf("no codec match: got %v, want nil", src)
	}
	if src := SelectBestSource(nil, SourceCriteria{}); src != nil {
		t.Errorf("nil bag: got %v, want nil", src)
	}
}

func TestSourceKindHelpers(t *testing.T) {
	blob := &amp.Tag{}
	blob.SetID(tag.UID{0xA, 0x3})
	if !SourceIsBlob(blob) || SourceScheme(blob) != "" {
		t.Error("blob leaf misclassified")
	}
	url := &amp.Tag{URI: "HTTPS://x/y.mp3"}
	if SourceIsBlob(url) || SourceScheme(url) != "https" {
		t.Error("URL leaf misclassified")
	}
	spot := &amp.Tag{URI: "spotify:track:abc"}
	if SourceScheme(spot) != "spotify" {
		t.Error("provider leaf misclassified")
	}
}
