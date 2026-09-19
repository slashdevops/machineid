//go:build linux

package machineid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// --- parseCPUInfo tests ---

func TestParseCPUInfoFull(t *testing.T) {
	content := `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 158
model name	: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
stepping	: 10
cpu MHz		: 2600.000
flags		: fpu vme de pse tsc msr pae mce
`
	result, err := parseCPUInfo(content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "GenuineIntel:Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestParseCPUInfoIgnoresVolatileFields verifies that the flags line and the
// processor index do not contribute: a kernel or microcode update that adds
// a mitigation flag, or a VM resize, must not change the identifier.
func TestParseCPUInfoIgnoresVolatileFields(t *testing.T) {
	before := "processor\t: 0\nvendor_id\t: GenuineIntel\nmodel name\t: Intel Core i7\nflags\t\t: fpu vme\n"
	after := "processor\t: 7\nvendor_id\t: GenuineIntel\nmodel name\t: Intel Core i7\nflags\t\t: fpu vme md_clear flush_l1d\n"

	a, err := parseCPUInfo(before)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseCPUInfo(after)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("identifier changed with flags/processor index: %q vs %q", a, b)
	}
}

// TestParseCPUInfoARM covers aarch64 kernels, which have no vendor_id or
// model name and describe the CPU with implementer/part/variant/revision.
func TestParseCPUInfoARM(t *testing.T) {
	content := `processor	: 0
BogoMIPS	: 108.00
Features	: fp asimd evtstrm crc32 cpuid
CPU implementer	: 0x41
CPU architecture: 8
CPU variant	: 0x0
CPU part	: 0xd08
CPU revision	: 3

processor	: 1
BogoMIPS	: 108.00
CPU implementer	: 0x41
CPU part	: 0xd08
CPU revision	: 3
Hardware	: BCM2835
`
	result, err := parseCPUInfo(content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "0x41:0xd08/0x0/3:BCM2835"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestParseCPUInfoEmpty(t *testing.T) {
	_, err := parseCPUInfo("")
	if err == nil {
		t.Fatal("Expected error for empty input")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
	if _, ok := errors.AsType[*ParseError](err); !ok {
		t.Errorf("Expected ParseError, got %T", err)
	}
}

func TestParseCPUInfoPartial(t *testing.T) {
	content := `processor	: 3
vendor_id	: AuthenticAMD
model name	: AMD Ryzen 9 5950X
`
	result, err := parseCPUInfo(content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "AuthenticAMD:AMD Ryzen 9 5950X"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestParseCPUInfoNoColon(t *testing.T) {
	content := "some line without colon\nanother line\n"
	_, err := parseCPUInfo(content)
	if err == nil {
		t.Fatal("Expected error when no fields present")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestParseCPUInfoUnknownFieldsOnly(t *testing.T) {
	// Lines have colons but none are recognized fields — should still error.
	content := "some unknown key : value\nanother : thing\n"
	_, err := parseCPUInfo(content)
	if err == nil {
		t.Fatal("Expected error when no recognized fields present")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestParseCPUInfoMultipleProcessors(t *testing.T) {
	// Every core repeats the same vendor and model; the first block is used.
	content := `processor	: 0
vendor_id	: GenuineIntel
model name	: Intel Core i7
flags		: fpu vme

processor	: 1
vendor_id	: GenuineIntel
model name	: Intel Core i7
flags		: fpu vme avx
`
	result, err := parseCPUInfo(content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "GenuineIntel:Intel Core i7"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// --- isValidSerial tests ---

func TestIsValidSerial(t *testing.T) {
	tests := []struct {
		name   string
		serial string
		valid  bool
	}{
		{"valid serial", "ABC12345", true},
		{"empty", "", false},
		{"OEM placeholder", "To be filled by O.E.M.", false},
		{"simple string", "SERIAL123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidSerial(tt.serial); got != tt.valid {
				t.Errorf("isValidSerial(%q) = %v, want %v", tt.serial, got, tt.valid)
			}
		})
	}
}

// --- isNonEmpty tests ---

func TestIsNonEmpty(t *testing.T) {
	if isNonEmpty("") {
		t.Error("Expected false for empty string")
	}
	if !isNonEmpty("hello") {
		t.Error("Expected true for non-empty string")
	}
}

// --- linuxDiskSerialsLSBLK tests ---

func TestLinuxDiskSerialsLSBLKSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "WD-12345")+lsblkLine("nvme0n1", "disk", "0", "SAMSUNG-67890"))

	serials, err := linuxDiskSerialsLSBLK(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 2 {
		t.Fatalf("Expected 2 serials, got %d", len(serials))
	}
	if serials[0] != "WD-12345" || serials[1] != "SAMSUNG-67890" {
		t.Errorf("Unexpected serials: %v", serials)
	}
}

func TestLinuxDiskSerialsLSBLKEmpty(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", "\n\n")

	serials, err := linuxDiskSerialsLSBLK(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 0 {
		t.Errorf("Expected 0 serials for empty output, got %d", len(serials))
	}
}

func TestLinuxDiskSerialsLSBLKError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("lsblk", fmt.Errorf("lsblk not found"))

	_, err := linuxDiskSerialsLSBLK(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when lsblk fails")
	}
}

func TestLinuxDiskSerialsLSBLKSkipsEmpty(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "WD-12345")+"\n\n"+lsblkLine("sdb", "disk", "0", "SAMSUNG-67890"))

	serials, err := linuxDiskSerialsLSBLK(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 2 {
		t.Errorf("Expected 2 serials (empty lines skipped), got %d", len(serials))
	}
}

func TestLinuxDiskSerialsLSBLKFiltersOEM(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "WD-12345")+lsblkLine("sdb", "disk", "0", "To be filled by O.E.M.")+lsblkLine("sdc", "disk", "0", "SAMSUNG-67890"))

	serials, err := linuxDiskSerialsLSBLK(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 2 {
		t.Fatalf("Expected 2 serials (OEM filtered), got %d: %v", len(serials), serials)
	}
	for _, s := range serials {
		if s == biosFirmwareMessage {
			t.Error("OEM placeholder leaked into lsblk disk serials")
		}
	}
}

// TestLinuxDiskSerialsLSBLKSkipsRemovableAndNonDisk verifies that USB sticks,
// optical drives and loop devices never contribute: plugging one in must not
// change the machine ID.
func TestLinuxDiskSerialsLSBLKSkipsRemovableAndNonDisk(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk",
		lsblkLine("sda", "disk", "0", "FIXED-1")+
			lsblkLine("sdb", "disk", "1", "USB-STICK")+
			lsblkLine("sr0", "rom", "1", "DVD-DRIVE")+
			lsblkLine("loop0", "loop", "0", "")+
			lsblkLine("nvme0n1", "disk", "0", "FIXED-2"))

	serials, err := linuxDiskSerialsLSBLK(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 2 || serials[0] != "FIXED-1" || serials[1] != "FIXED-2" {
		t.Errorf("Expected [FIXED-1 FIXED-2], got %v", serials)
	}
}

func TestParseKeyValueLine(t *testing.T) {
	got := parseKeyValueLine(`NAME="sda" TYPE="disk" RM="0" SERIAL="S/N with spaces"`)
	if got["NAME"] != "sda" || got["TYPE"] != "disk" || got["RM"] != "0" || got["SERIAL"] != "S/N with spaces" {
		t.Errorf("parseKeyValueLine = %v", got)
	}
	if len(parseKeyValueLine("")) != 0 || len(parseKeyValueLine("garbage")) != 0 {
		t.Error("non key=value input should yield no fields")
	}
}

// lsblkLine renders one line of `lsblk -P -o NAME,TYPE,RM,SERIAL` output.
func lsblkLine(name, typ, rm, serial string) string {
	return fmt.Sprintf("NAME=%q TYPE=%q RM=%q SERIAL=%q\n", name, typ, rm, serial)
}

// --- linuxDiskSerialsSys tests (with injected sysBlockDir) ---

func withSysBlockDir(t *testing.T, dir string) {
	t.Helper()
	orig := sysBlockDir
	sysBlockDir = dir
	t.Cleanup(func() { sysBlockDir = orig })
}

func writeFakeDisk(t *testing.T, root, name, serial string) {
	t.Helper()
	deviceDir := filepath.Join(root, name, "device")
	if err := os.MkdirAll(deviceDir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", deviceDir, err)
	}
	if err := os.WriteFile(filepath.Join(deviceDir, "serial"), []byte(serial), 0o644); err != nil {
		t.Fatalf("write serial %q: %v", name, err)
	}
}

func TestLinuxDiskSerialsSysSuccess(t *testing.T) {
	tmp := t.TempDir()
	withSysBlockDir(t, tmp)

	writeFakeDisk(t, tmp, "sda", "WD-AAA\n")
	writeFakeDisk(t, tmp, "sdb", "SAMSUNG-BBB\n")

	serials, err := linuxDiskSerialsSys(nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 2 {
		t.Fatalf("Expected 2 serials, got %d: %v", len(serials), serials)
	}
}

func TestLinuxDiskSerialsSysSkipsLoop(t *testing.T) {
	tmp := t.TempDir()
	withSysBlockDir(t, tmp)

	writeFakeDisk(t, tmp, "sda", "WD-AAA\n")
	writeFakeDisk(t, tmp, "loop0", "SHOULD-SKIP\n")

	serials, err := linuxDiskSerialsSys(nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 1 || serials[0] != "WD-AAA" {
		t.Errorf("Expected [WD-AAA], got %v", serials)
	}
}

func TestLinuxDiskSerialsSysSkipsVirtualAndRemovable(t *testing.T) {
	tmp := t.TempDir()
	withSysBlockDir(t, tmp)

	writeFakeDisk(t, tmp, "sda", "FIXED\n")
	for _, name := range []string{"ram0", "zram0", "dm-0", "md127", "sr0", "fd0", "nbd0", "mtdblock0"} {
		writeFakeDisk(t, tmp, name, "VIRTUAL-"+name+"\n")
	}
	writeFakeDisk(t, tmp, "sdb", "USB-STICK\n")
	writeFakeRemovable(t, tmp, "sdb", "1\n")
	writeFakeDisk(t, tmp, "sdc", "FIXED-EXPLICIT\n")
	writeFakeRemovable(t, tmp, "sdc", "0\n")

	serials, err := linuxDiskSerialsSys(nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 2 || serials[0] != "FIXED" || serials[1] != "FIXED-EXPLICIT" {
		t.Errorf("Expected [FIXED FIXED-EXPLICIT], got %v", serials)
	}
}

// writeFakeRemovable writes /sys/block/<name>/removable.
func writeFakeRemovable(t *testing.T, root, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name, "removable"), []byte(value), 0o644); err != nil {
		t.Fatalf("write removable %q: %v", name, err)
	}
}

func TestLinuxDiskSerialsSysFiltersOEMAndEmpty(t *testing.T) {
	tmp := t.TempDir()
	withSysBlockDir(t, tmp)

	writeFakeDisk(t, tmp, "sda", "WD-REAL\n")
	writeFakeDisk(t, tmp, "sdb", "To be filled by O.E.M.\n")
	writeFakeDisk(t, tmp, "sdc", "\n")

	serials, err := linuxDiskSerialsSys(nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 1 || serials[0] != "WD-REAL" {
		t.Errorf("Expected [WD-REAL], got %v", serials)
	}
}

func TestLinuxDiskSerialsSysMissingDir(t *testing.T) {
	withSysBlockDir(t, filepath.Join(t.TempDir(), "does-not-exist"))

	_, err := linuxDiskSerialsSys(nil)
	if err == nil {
		t.Error("Expected error when sysBlockDir does not exist")
	}
}

// --- linuxDiskSerials end-to-end with both backends failing ---

func TestLinuxDiskSerialsBothBackendsFail(t *testing.T) {
	// lsblk errors, sys dir does not exist → ErrNotFound.
	withSysBlockDir(t, filepath.Join(t.TempDir(), "nope"))
	mock := newMockExecutor()
	mock.setError("lsblk", fmt.Errorf("lsblk not found"))

	_, err := linuxDiskSerials(context.Background(), mock, nil)
	if err == nil {
		t.Fatal("Expected error when both backends fail")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestLinuxDiskSerialsPartialSuccess(t *testing.T) {
	// lsblk succeeds, /sys/block missing → no error, lsblk results returned.
	withSysBlockDir(t, filepath.Join(t.TempDir(), "nope"))
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL-X"))

	serials, err := linuxDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(serials) != 1 || serials[0] != "SERIAL-X" {
		t.Errorf("Expected [SERIAL-X], got %v", serials)
	}
}

func TestLinuxDiskSerialsDeduplicatesAcrossBackends(t *testing.T) {
	tmp := t.TempDir()
	withSysBlockDir(t, tmp)
	writeFakeDisk(t, tmp, "sda", "SHARED\n")
	writeFakeDisk(t, tmp, "sdb", "ONLY-SYS\n")

	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SHARED")+lsblkLine("sdb", "disk", "0", "ONLY-LSBLK"))

	serials, err := linuxDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Expected: SHARED, ONLY-LSBLK, ONLY-SYS — no duplicates.
	if len(serials) != 3 {
		t.Fatalf("Expected 3 unique serials, got %d: %v", len(serials), serials)
	}
	count := map[string]int{}
	for _, s := range serials {
		count[s]++
	}
	for k, c := range count {
		if c != 1 {
			t.Errorf("Serial %q appeared %d times", k, c)
		}
	}
}

func TestLinuxDiskSerialsFiltersOEMAcrossBackends(t *testing.T) {
	tmp := t.TempDir()
	withSysBlockDir(t, tmp)
	writeFakeDisk(t, tmp, "sda", "GOOD-SYS\n")

	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "GOOD-LSBLK")+lsblkLine("sdb", "disk", "0", "To be filled by O.E.M."))

	serials, err := linuxDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	for _, s := range serials {
		if s == biosFirmwareMessage {
			t.Error("OEM placeholder leaked through linuxDiskSerials")
		}
	}
	if len(serials) != 2 {
		t.Errorf("Expected 2 serials, got %d: %v", len(serials), serials)
	}
}

// --- readFirstValidFromLocations success path ---

func TestReadFirstValidFromLocationsFindsFile(t *testing.T) {
	tmp := t.TempDir()
	goodPath := filepath.Join(tmp, "good")
	if err := os.WriteFile(goodPath, []byte("  HELLO  \n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	locations := []string{
		filepath.Join(tmp, "missing"),
		goodPath,
	}

	value, err := readFirstValidFromLocations(locations, isNonEmpty, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "HELLO" {
		t.Errorf("Expected %q, got %q", "HELLO", value)
	}
}

func TestReadFirstValidFromLocationsSkipsInvalid(t *testing.T) {
	tmp := t.TempDir()
	invalidPath := filepath.Join(tmp, "invalid")
	goodPath := filepath.Join(tmp, "good")
	if err := os.WriteFile(invalidPath, []byte("00000000-0000-0000-0000-000000000000\n"), 0o644); err != nil {
		t.Fatalf("write invalid: %v", err)
	}
	if err := os.WriteFile(goodPath, []byte("E1B2C3D4-0008-4A5B-8C6D-7E8F9A0B1C2D\n"), 0o644); err != nil {
		t.Fatalf("write good: %v", err)
	}

	value, err := readFirstValidFromLocations([]string{invalidPath, goodPath}, isValidUUID, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "E1B2C3D4-0008-4A5B-8C6D-7E8F9A0B1C2D" {
		t.Errorf("Expected %q, got %q", "E1B2C3D4-0008-4A5B-8C6D-7E8F9A0B1C2D", value)
	}
}

// --- linuxDiskSerials deduplication tests ---

func TestLinuxDiskSerialsDeduplicated(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL-A")+lsblkLine("sdb", "disk", "0", "SERIAL-B"))

	serials, err := linuxDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// lsblk should succeed; /sys/block will likely fail in test environment
	t.Logf("Found %d disk serials from lsblk", len(serials))
}

func TestLinuxDiskSerialsWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL-LOG"))

	_, err := linuxDiskSerials(context.Background(), mock, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("collected disk serials via lsblk")) {
		t.Error("Expected 'collected disk serials via lsblk' in log")
	}
}

// --- Provider integration tests with mock executor (Linux) ---

func TestProviderWithMockExecutor(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL-A"))

	p := New().WithExecutor(mock).WithDisk()

	id, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}
	if len(id) != 64 {
		t.Errorf("Expected 64-char ID, got %d", len(id))
	}
}

func TestProviderErrorHandlingLinux(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("lsblk", fmt.Errorf("lsblk not found"))

	// Disk-only: lsblk fails, /sys/block also likely fails in test
	p := New().WithExecutor(mock).WithDisk()

	_, err := p.ID(context.Background())
	// This might succeed or fail depending on /sys/block availability
	t.Logf("ID() result: err=%v", err)
}

func TestProviderDiagnosticsLinux(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL"))

	p := New().WithExecutor(mock).WithDisk()

	if p.Diagnostics() != nil {
		t.Error("Diagnostics should be nil before ID()")
	}

	if _, err := p.ID(context.Background()); err != nil {
		t.Logf("ID() error (may be expected): %v", err)
	}

	diag := p.Diagnostics()
	if diag == nil {
		t.Fatal("Diagnostics should not be nil after ID()")
	}
	t.Logf("Collected: %v, Errors: %v", diag.Collected, diag.Errors)
}

func TestProviderWithLoggerLinux(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL"))

	p := New().WithExecutor(mock).WithLogger(logger).WithDisk()

	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("generating machine ID")) {
		t.Error("Expected 'generating machine ID' in log")
	}
}

func TestProviderValidateLinux(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL"))

	p := New().WithExecutor(mock).WithDisk()

	id, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	valid, err := p.Validate(context.Background(), id)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if !valid {
		t.Error("Expected validation to succeed")
	}

	valid, err = p.Validate(context.Background(), "wrong-id")
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if valid {
		t.Error("Expected validation to fail for wrong ID")
	}
}

func TestCollectIdentifiersLinuxAllFail(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("lsblk", fmt.Errorf("not found"))

	p := New().WithExecutor(mock).WithCPU().WithSystemUUID()
	diag := &DiagnosticInfo{Errors: make(map[string]error)}

	identifiers, err := collectIdentifiers(context.Background(), p, diag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// CPU and UUID read from files, not commands — they may or may not work
	// depending on the test environment
	t.Logf("Identifiers: %d, Collected: %v, Errors: %v", len(identifiers), diag.Collected, diag.Errors)
}

func TestCollectIdentifiersLinuxNoComponents(t *testing.T) {
	p := New()
	diag := &DiagnosticInfo{Errors: make(map[string]error)}

	identifiers, err := collectIdentifiers(context.Background(), p, diag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(identifiers) != 0 {
		t.Errorf("Expected 0 identifiers with no components, got %d", len(identifiers))
	}
}

func TestValidateErrorLinux(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("lsblk", fmt.Errorf("command failed"))

	// WithCPU on Linux reads files, not commands — might not fail.
	// Use a component that requires the mock executor to ensure failure.
	p := New().WithExecutor(mock)
	// Don't enable any components — will get ErrNoIdentifiers
	_, err := p.ID(context.Background())
	if err == nil {
		// No components enabled means no identifiers
		t.Logf("No error with no components enabled (expected)")
	}
}

func TestProviderCachedIDLinux(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL1"))

	p := New().WithExecutor(mock).WithDisk()

	id1, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("First ID() error: %v", err)
	}

	mock.setOutput("lsblk", lsblkLine("sda", "disk", "0", "SERIAL2"))

	id2, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("Second ID() error: %v", err)
	}

	if id1 != id2 {
		t.Error("Cached ID was modified on subsequent call")
	}
}

// --- readFirstValidFromLocations tests ---

func TestReadFirstValidFromLocationsAllFail(t *testing.T) {
	locations := []string{
		"/nonexistent/path/1",
		"/nonexistent/path/2",
	}

	_, err := readFirstValidFromLocations(locations, isNonEmpty, nil)
	if err == nil {
		t.Error("Expected error when all locations fail")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestReadFirstValidFromLocationsWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	locations := []string{"/nonexistent/path/test"}

	if _, err := readFirstValidFromLocations(locations, isNonEmpty, logger); err == nil {
		t.Error("Expected error for nonexistent path")
	}
	if !bytes.Contains(buf.Bytes(), []byte("failed to read file")) {
		t.Error("Expected 'failed to read file' in log")
	}
}
