package selfupdate

import (
	"errors"
	"testing"
)

// TestAssetNamesAreAContract pins every published filename. If the release
// pipeline renames an asset this test fails, which is the point.
func TestAssetNamesAreAContract(t *testing.T) {
	tests := []struct {
		goos, goarch string
		name, sum    string
		bundle       string
		inner        string
		kind         Kind
	}{
		{"linux", "amd64", "machineid-linux-amd64.zip", "machineid-linux-amd64.sha256", "machineid-linux-amd64.sigstore.json", "machineid", KindZip},
		{"linux", "arm64", "machineid-linux-arm64.zip", "machineid-linux-arm64.sha256", "machineid-linux-arm64.sigstore.json", "machineid", KindZip},
		{"darwin", "arm64", "machineid-darwin-universal.pkg", "machineid-darwin-universal.sha256", "", "", KindPkg},
		{"darwin", "amd64", "machineid-darwin-universal.pkg", "machineid-darwin-universal.sha256", "", "", KindPkg},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			a, err := AssetFor(tt.goos, tt.goarch)
			if err != nil {
				t.Fatal(err)
			}
			if a.Name != tt.name || a.ChecksumName != tt.sum || a.BundleName != tt.bundle || a.InnerName != tt.inner || a.Kind != tt.kind {
				t.Errorf("AssetFor = %+v", a)
			}
		})
	}

	z := DarwinZipAsset()
	if z.Name != "machineid-darwin-universal.zip" || z.ChecksumName != "machineid-darwin-universal.zip.sha256" || z.InnerName != "machineid" || z.Kind != KindZip {
		t.Errorf("DarwinZipAsset = %+v", z)
	}
}

func TestAssetForUnsupported(t *testing.T) {
	for _, p := range [][2]string{{"windows", "amd64"}, {"linux", "386"}, {"freebsd", "amd64"}} {
		_, err := AssetFor(p[0], p[1])
		if _, ok := errors.AsType[*UnsupportedPlatformError](err); !ok {
			t.Errorf("AssetFor(%s/%s) error = %v, want UnsupportedPlatformError", p[0], p[1], err)
		}
		if RemedyOf(err) == "" {
			t.Errorf("UnsupportedPlatformError for %s/%s should carry a remedy", p[0], p[1])
		}
	}
}

func TestKindString(t *testing.T) {
	if KindZip.String() != "zip" || KindPkg.String() != "pkg" {
		t.Error("Kind.String")
	}
}
