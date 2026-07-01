package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Builder turns resolved inputs into a built kernel artifact, writing its
// outputs under outDir. The real implementation runs `make LLVM=1 ARCH=<arch>`
// inside the host VM; keeping it an interface lets the Store's cache logic be
// unit-tested with a fake builder — no kernel tree, no compile.
type Builder interface {
	Build(ctx context.Context, spec Spec, resolvedConfig, outDir string) (Artifact, error)
}

// Store is a content-addressed cache of built kernels rooted at Root. A build is
// keyed by CacheKey(ref, arch, toolchain, resolved config): identical inputs
// resolve to the same directory and are reused (cache hit, F1.3); any change to
// the inputs resolves to a new directory and triggers a rebuild (F1.4).
type Store struct {
	Root      string  // cache root directory
	Toolchain string  // toolchain identity, mixed into the cache key
	Builder   Builder // how to build on a miss
}

// artifactMeta is written last, once a build succeeds; its presence is what
// distinguishes a complete cache entry from a partial/failed one.
const artifactMeta = "artifact.json"

// Get returns the artifact for (spec, resolvedConfig), building it on a cache
// miss and reusing it on a hit. The bool reports whether it was a cache hit.
func (s *Store) Get(ctx context.Context, spec Spec, resolvedConfig string) (Artifact, bool, error) {
	key := CacheKey(spec.Ref, spec.Arch, s.Toolchain, resolvedConfig)
	dir := filepath.Join(s.Root, key)
	meta := filepath.Join(dir, artifactMeta)

	switch data, err := os.ReadFile(meta); {
	case err == nil:
		var a Artifact
		if err := json.Unmarshal(data, &a); err != nil {
			return Artifact{}, false, fmt.Errorf("kernel: corrupt cache metadata %s: %w", meta, err)
		}
		return a, true, nil
	case !os.IsNotExist(err):
		return Artifact{}, false, fmt.Errorf("kernel: reading cache %s: %w", meta, err)
	}

	// Cache miss: materialize the resolved config, build, then write the marker.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Artifact{}, false, fmt.Errorf("kernel: creating cache dir: %w", err)
	}
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte(resolvedConfig), 0o644); err != nil {
		return Artifact{}, false, fmt.Errorf("kernel: writing resolved config: %w", err)
	}

	a, err := s.Builder.Build(ctx, spec, resolvedConfig, dir)
	if err != nil {
		return Artifact{}, false, err
	}
	a.Hash = key
	a.Config = configPath

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return Artifact{}, false, fmt.Errorf("kernel: encoding artifact metadata: %w", err)
	}
	if err := os.WriteFile(meta, data, 0o644); err != nil {
		return Artifact{}, false, fmt.Errorf("kernel: writing artifact metadata: %w", err)
	}
	return a, false, nil
}
