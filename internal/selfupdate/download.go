package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxExtractedSize caps what will be written out of an archive (256 MB). The
// archives are our own release artefacts, so this is a backstop against a
// corrupt zip filling the disk, not a security boundary.
const maxExtractedSize = 256 << 20

// Fetch downloads an asset, its checksum and, when published, its Sigstore
// bundle into dir, verifies the asset against the checksum and returns the
// verified archive path and the bundle path ("" when none).
//
// Verification is not optional and happens here, before the artefact reaches
// any install step, so a truncated or tampered download never touches the target.
func Fetch(ctx context.Context, client *Client, tag string, asset Asset, dir string) (archive, bundle string, err error) {
	if tag == "" {
		return "", "", errors.New("no release tag given to download")
	}

	archive = filepath.Join(dir, asset.Name)
	checksum := filepath.Join(dir, asset.ChecksumName)

	if err := client.Download(ctx, tag, asset.Name, archive); err != nil {
		return "", "", err
	}

	if err := client.Download(ctx, tag, asset.ChecksumName, checksum); err != nil {
		return "", "", err
	}

	if asset.BundleName != "" {
		bundle = filepath.Join(dir, asset.BundleName)
		if err := client.Download(ctx, tag, asset.BundleName, bundle); err != nil {
			if _, ok := errors.AsType[*AssetNotFoundError](err); !ok {
				return "", "", err
			}
			// A release without a bundle is still installable; the signature step reports it.
			bundle = ""
		}
	}

	expected, err := readChecksumFile(checksum)
	if err != nil {
		return "", "", err
	}

	actual, err := fileSHA256(archive)
	if err != nil {
		return "", "", err
	}

	if !strings.EqualFold(expected, actual) {
		return "", "", &ChecksumMismatchError{Asset: asset.Name, Expected: expected, Actual: actual}
	}

	return archive, bundle, nil
}

// readChecksumFile reads a published `.sha256` file. The pipeline writes a
// bare hash; the conventional shasum form "<hash>  <file>" is accepted too.
func readChecksumFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading the published checksum: %w", err)
	}

	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return "", fmt.Errorf("the published checksum %s is empty", filepath.Base(path))
	}

	sum := fields[0]
	if len(sum) != sha256.Size*2 {
		return "", fmt.Errorf("the published checksum %s is not a SHA-256 digest: %q", filepath.Base(path), sum)
	}

	if _, err := hex.DecodeString(sum); err != nil {
		return "", fmt.Errorf("the published checksum %s is not hexadecimal: %q", filepath.Base(path), sum)
	}

	return sum, nil
}

// fileSHA256 hashes a file, streaming.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening the download to verify it: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing the download: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExtractBinary extracts the executable from a release zip into destDir and
// returns its path. innerName is the expected filename; when it is absent and
// the archive holds exactly one file, that file is used, so a rename in the
// release pipeline does not break updating outright.
func ExtractBinary(archivePath, innerName, destDir string) (extracted string, err error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", filepath.Base(archivePath), err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing %s: %w", filepath.Base(archivePath), closeErr)
		}
	}()

	entry, err := pickBinaryEntry(reader.File, innerName, filepath.Base(archivePath))
	if err != nil {
		return "", err
	}

	// The destination is destDir plus the entry's base name only, never its
	// path, which could contain ".." and escape destDir (zip slip).
	destPath := filepath.Join(destDir, filepath.Base(entry.Name))

	if err := writeZipEntry(entry, destPath); err != nil {
		return "", err
	}

	return destPath, nil
}

// pickBinaryEntry chooses which archive entry is the executable.
func pickBinaryEntry(files []*zip.File, innerName, archiveName string) (*zip.File, error) {
	var regular []*zip.File

	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		if innerName != "" && filepath.Base(f.Name) == innerName {
			return f, nil
		}
		regular = append(regular, f)
	}

	switch len(regular) {
	case 0:
		return nil, fmt.Errorf("%s contains no files", archiveName)
	case 1:
		return regular[0], nil
	default:
		names := make([]string, 0, len(regular))
		for _, f := range regular {
			names = append(names, f.Name)
		}

		return nil, fmt.Errorf("%s does not contain %q and holds several files, so the executable is ambiguous: %s",
			archiveName, innerName, strings.Join(names, ", "))
	}
}

// writeZipEntry writes one archive entry to destPath, executable.
func writeZipEntry(entry *zip.File, destPath string) error {
	src, err := entry.Open()
	if err != nil {
		return fmt.Errorf("reading %s from the archive: %w", entry.Name, err)
	}
	defer src.Close()

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating %s: %w", destPath, err)
	}

	written, copyErr := io.Copy(dest, io.LimitReader(src, maxExtractedSize+1))
	closeErr := dest.Close()

	if copyErr != nil {
		return errors.Join(fmt.Errorf("extracting %s: %w", entry.Name, copyErr), closeErr, os.Remove(destPath))
	}
	if closeErr != nil {
		return fmt.Errorf("closing %s: %w", destPath, closeErr)
	}
	if written > maxExtractedSize {
		return errors.Join(fmt.Errorf("%s is larger than the %d byte extraction limit", entry.Name, int64(maxExtractedSize)), os.Remove(destPath))
	}

	return nil
}
