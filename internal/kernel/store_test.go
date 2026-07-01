package kernel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeBuilder stands in for the real `make`-based builder: it records calls and
// writes a stub image, so the Store's cache logic is testable with no compile.
type fakeBuilder struct{ calls int }

func (b *fakeBuilder) Build(_ context.Context, _ Spec, _, outDir string) (Artifact, error) {
	b.calls++
	img := filepath.Join(outDir, "Image")
	if err := os.WriteFile(img, []byte("stub-kernel"), 0o644); err != nil {
		return Artifact{}, err
	}
	return Artifact{Image: img, Vmlinux: filepath.Join(outDir, "vmlinux")}, nil
}

type errBuilder struct{}

func (errBuilder) Build(context.Context, Spec, string, string) (Artifact, error) {
	return Artifact{}, errors.New("build failed")
}

func TestStoreCacheHitMiss(t *testing.T) {
	ctx := context.Background()
	b := &fakeBuilder{}
	s := &Store{Root: t.TempDir(), Toolchain: "clang-18", Builder: b}
	spec := Spec{Name: "bpf-arm64", Ref: "v6.17", Arch: "arm64"}
	cfg := MergeConfig("CONFIG_BASE=y", "CONFIG_BPF=y")

	a1, hit1, err := s.Get(ctx, spec, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if hit1 {
		t.Error("first Get should be a cache miss")
	}
	if b.calls != 1 {
		t.Fatalf("builder calls = %d, want 1", b.calls)
	}
	if a1.Hash == "" || a1.Image == "" || a1.Config == "" {
		t.Errorf("artifact not fully populated: %+v", a1)
	}

	// Identical inputs -> cache hit, no rebuild (F1.3).
	a2, hit2, err := s.Get(ctx, spec, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !hit2 {
		t.Error("second Get with identical inputs should be a cache hit")
	}
	if b.calls != 1 {
		t.Errorf("builder ran again on a hit: calls = %d, want 1", b.calls)
	}
	if a2.Hash != a1.Hash || a2.Image != a1.Image {
		t.Errorf("hit artifact differs from miss: %+v vs %+v", a2, a1)
	}

	// Changed config -> cache miss, rebuild (F1.4).
	a3, hit3, err := s.Get(ctx, spec, MergeConfig("CONFIG_BASE=y", "CONFIG_BPF=y", "CONFIG_XDP_SOCKETS=y"))
	if err != nil {
		t.Fatal(err)
	}
	if hit3 {
		t.Error("changed config should be a cache miss")
	}
	if b.calls != 2 {
		t.Errorf("builder calls = %d, want 2", b.calls)
	}
	if a3.Hash == a1.Hash {
		t.Error("changed config should yield a different cache key")
	}
}

func TestStoreKeySensitivity(t *testing.T) {
	ctx := context.Background()
	cfg := "CONFIG_BPF=y\n"
	base := Spec{Ref: "v6.17", Arch: "arm64"}
	for _, c := range []Spec{
		{Ref: "v6.16", Arch: "arm64"},  // different ref
		{Ref: "v6.17", Arch: "x86_64"}, // different arch
	} {
		b := &fakeBuilder{}
		s := &Store{Root: t.TempDir(), Toolchain: "clang-18", Builder: b}
		a0, _, err := s.Get(ctx, base, cfg)
		if err != nil {
			t.Fatal(err)
		}
		a1, hit, err := s.Get(ctx, c, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if hit {
			t.Errorf("%+v should miss against the base entry", c)
		}
		if a1.Hash == a0.Hash {
			t.Errorf("%+v produced the same cache key as base", c)
		}
	}
}

func TestStoreBuilderErrorLeavesNoHit(t *testing.T) {
	ctx := context.Background()
	spec := Spec{Ref: "v6.17", Arch: "arm64"}
	cfg := "CONFIG_X=y\n"
	s := &Store{Root: t.TempDir(), Toolchain: "clang-18", Builder: errBuilder{}}

	if _, _, err := s.Get(ctx, spec, cfg); err == nil {
		t.Fatal("expected the builder error to propagate")
	}

	// A prior failed build must not register as a cache hit.
	b := &fakeBuilder{}
	s.Builder = b
	_, hit, err := s.Get(ctx, spec, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("a prior failed build must not count as a cache hit")
	}
	if b.calls != 1 {
		t.Errorf("builder calls = %d, want 1 (rebuild after failure)", b.calls)
	}
}
