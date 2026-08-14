package file

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Export writes a portable pack tarball (.tgz) to out from a pack directory or
// an already-installed pack name in storeRoot. The result round-trips with
// Install. It validates the pack (manifest + every challenge loads) before
// writing. Returns the resolved Pack.
func Export(src, storeRoot, out string) (*Pack, error) {
	srcDir, err := resolveExportDir(src, storeRoot)
	if err != nil {
		return nil, err
	}
	man, err := readManifest(srcDir)
	if err != nil {
		return nil, err
	}
	chs, err := discoverChallenges(srcDir)
	if err != nil {
		return nil, err
	}
	if len(chs) == 0 {
		return nil, fmt.Errorf("pack %q has no challenges to export", man.Metadata.Name)
	}
	if err := writeTarGz(srcDir, out); err != nil {
		return nil, err
	}
	return &Pack{
		Name:        man.Metadata.Name,
		Version:     man.Metadata.Version,
		Description: man.Metadata.Description,
		Dir:         srcDir,
		Challenges:  chs,
	}, nil
}

// resolveExportDir accepts either a pack directory (has pack.yaml) or the name
// of an installed pack (highest version wins).
func resolveExportDir(src, storeRoot string) (string, error) {
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		if _, err := os.Stat(filepath.Join(src, manifestName)); err == nil {
			return src, nil
		}
		return "", fmt.Errorf("%s is a directory but has no %s", src, manifestName)
	}
	// Treat src as an installed pack name.
	packs, err := Installed(storeRoot)
	if err != nil {
		return "", err
	}
	dir := ""
	for _, p := range packs { // ascending; last (highest) wins
		if p.Name == src {
			dir = p.Dir
		}
	}
	if dir == "" {
		return "", fmt.Errorf("no pack directory or installed pack named %q", src)
	}
	return dir, nil
}

// writeTarGz tars+gzips srcDir into out. Paths are relative to srcDir; the
// executable bit is preserved on scripts.
func writeTarGz(srcDir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}
