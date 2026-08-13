package plugin

import "testing"

func TestInfoHashFromMagnet(t *testing.T) {
	m := "magnet:?xt=urn:btih:ABCDEF0123456789ABCDEF0123456789ABCDEF01&dn=test"
	h := InfoHashFromMagnet(m)
	if h != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("got %q", h)
	}
}

func TestMagnetFromHash(t *testing.T) {
	m := MagnetFromHash("ABCDEF0123456789ABCDEF0123456789ABCDEF01", "hi")
	if InfoHashFromMagnet(m) == "" {
		t.Fatalf("empty hash from %q", m)
	}
}
