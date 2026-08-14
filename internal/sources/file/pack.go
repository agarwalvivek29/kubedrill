// Package file is the file/tarball PackSource: it installs challenge packs from
// a local directory or tarball into the shared pack store and resolves
// challenges out of it. It is the sole writer of ~/.kubedrill/store/<pack>/<ver>/
// and serializes installs with a per-pack flock (AD-13).
package file

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gofrs/flock"
	"sigs.k8s.io/yaml"

	"github.com/agarwalvivek29/kubedrill/internal/challenge"
)

// PackKind/APIVersion are the manifest discriminators.
const (
	PackAPIVersion = "kubedrill.dev/v1alpha1"
	PackKind       = "Pack"
	manifestName   = "pack.yaml"
)

var nameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Manifest is the pack.yaml at the root of a pack.
type Manifest struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description,omitempty"`
	} `json:"metadata"`
}

// Pack describes an installed (or inspected) pack.
type Pack struct {
	Name        string
	Version     string
	Description string
	Dir         string   // where it lives on disk
	Challenges  []string // challenge names it contains
}

// Install reads a pack from src (a directory or a .tgz/.tar.gz/.tar) and installs
// it atomically to storeRoot/<name>/<version>/, validating that every challenge
// loads first. Concurrent installs of the same pack are serialized with a flock.
func Install(src, storeRoot string) (*Pack, error) {
	srcDir, cleanup, err := materializeSource(src)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	man, err := readManifest(srcDir)
	if err != nil {
		return nil, err
	}
	chs, err := discoverChallenges(srcDir)
	if err != nil {
		return nil, err
	}
	if len(chs) == 0 {
		return nil, fmt.Errorf("pack %q contains no challenges (no subdirectory with a challenge.yaml)", man.Metadata.Name)
	}

	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}
	// Per-pack lock so concurrent installs don't tear the store (AD-13).
	lock := flock.New(filepath.Join(storeRoot, man.Metadata.Name+".lock"))
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock pack store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	packDir := filepath.Join(storeRoot, man.Metadata.Name)
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return nil, err
	}
	final := filepath.Join(packDir, man.Metadata.Version)

	// Stage into a temp dir in the same parent, then atomically swap into place.
	tmp, err := os.MkdirTemp(packDir, ".tmp-"+man.Metadata.Version+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := copyTree(srcDir, tmp); err != nil {
		return nil, fmt.Errorf("stage pack: %w", err)
	}
	// Replace any existing version atomically-ish: rename old aside, move new in.
	if _, err := os.Stat(final); err == nil {
		old := final + ".old"
		_ = os.RemoveAll(old)
		if err := os.Rename(final, old); err != nil {
			return nil, fmt.Errorf("replace existing version: %w", err)
		}
		defer os.RemoveAll(old)
	}
	if err := os.Rename(tmp, final); err != nil {
		return nil, fmt.Errorf("install pack version: %w", err)
	}

	return &Pack{
		Name:        man.Metadata.Name,
		Version:     man.Metadata.Version,
		Description: man.Metadata.Description,
		Dir:         final,
		Challenges:  chs,
	}, nil
}

// Installed returns every installed pack (highest version per name last), each
// with the challenges it contains.
func Installed(storeRoot string) ([]Pack, error) {
	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Pack
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		vers, err := os.ReadDir(filepath.Join(storeRoot, e.Name()))
		if err != nil {
			continue
		}
		for _, v := range vers {
			if !v.IsDir() || strings.HasPrefix(v.Name(), ".") {
				continue
			}
			dir := filepath.Join(storeRoot, e.Name(), v.Name())
			chs, _ := discoverChallenges(dir)
			if len(chs) == 0 {
				continue
			}
			out = append(out, Pack{Name: e.Name(), Version: v.Name(), Dir: dir, Challenges: chs})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// Resolve finds a challenge by name across installed packs, returning its
// directory and the owning pack. When multiple packs/versions provide the same
// challenge name, the lexically-highest (pack, version) wins deterministically.
func Resolve(storeRoot, challengeName string) (dir, pack string, ok bool) {
	packs, err := Installed(storeRoot)
	if err != nil {
		return "", "", false
	}
	for _, p := range packs { // Installed is sorted ascending; last match wins
		for _, c := range p.Challenges {
			if c == challengeName {
				dir, pack, ok = filepath.Join(p.Dir, c), p.Name, true
			}
		}
	}
	return dir, pack, ok
}

func readManifest(srcDir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(srcDir, manifestName))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", manifestName, err)
	}
	var m Manifest
	if err := yaml.UnmarshalStrict(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", manifestName, err)
	}
	if m.APIVersion != PackAPIVersion || m.Kind != PackKind {
		return nil, fmt.Errorf("%s: want apiVersion=%q kind=%q, got %q/%q", manifestName, PackAPIVersion, PackKind, m.APIVersion, m.Kind)
	}
	if !nameRE.MatchString(m.Metadata.Name) {
		return nil, fmt.Errorf("pack name %q must be a DNS-1123 label", m.Metadata.Name)
	}
	if m.Metadata.Version == "" {
		return nil, fmt.Errorf("pack %q: metadata.version is required", m.Metadata.Name)
	}
	return &m, nil
}

// discoverChallenges returns the names of the challenge subdirectories (those
// with a challenge.yaml) that load cleanly.
func discoverChallenges(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cdir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(cdir, "challenge.yaml")); err != nil {
			continue
		}
		if _, err := challenge.LoadDir(cdir); err != nil {
			return nil, fmt.Errorf("challenge %q does not load: %w", e.Name(), err)
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// materializeSource returns a directory for src: src itself if it's a directory,
// or an extracted temp dir if it's a tar/tgz. cleanup removes any temp dir.
func materializeSource(src string) (dir string, cleanup func(), err error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", func() {}, fmt.Errorf("read pack source: %w", err)
	}
	if fi.IsDir() {
		return src, func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "kubedrill-pack-")
	if err != nil {
		return "", func() {}, err
	}
	if err := extractTarball(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", func() {}, err
	}
	// A tarball may wrap its contents in a single top-level dir; descend if so.
	root := tmp
	if entries, _ := os.ReadDir(tmp); len(entries) == 1 && entries[0].IsDir() {
		if _, err := os.Stat(filepath.Join(tmp, entries[0].Name(), manifestName)); err == nil {
			root = filepath.Join(tmp, entries[0].Name())
		}
	}
	return root, func() { os.RemoveAll(tmp) }, nil
}

func extractTarball(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(src, ".gz") || strings.HasSuffix(src, ".tgz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gunzip: %w", err)
		}
		defer gz.Close()
		r = gz
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// Guard against path traversal.
		clean := filepath.Clean(h.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("unsafe path in tarball: %q", h.Name)
		}
		target := filepath.Join(dest, clean)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // bounded by tarball
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

// copyTree recursively copies src into dst, preserving the executable bit on
// scripts.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info.Mode()&0o111 != 0 || strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		return os.WriteFile(target, b, mode)
	})
}
