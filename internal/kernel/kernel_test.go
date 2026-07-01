package kernel

import "testing"

func TestMergeConfig(t *testing.T) {
	tests := []struct {
		name string
		base string
		frag []string
		want string
	}{
		{
			name: "fragment overrides base value (y -> m)",
			base: "CONFIG_A=y",
			frag: []string{"CONFIG_A=m"},
			want: "CONFIG_A=m\n",
		},
		{
			name: "later fragment wins",
			base: "",
			frag: []string{"CONFIG_A=y", "CONFIG_A=n"},
			want: "# CONFIG_A is not set\n",
		},
		{
			name: "is-not-set overrides y",
			base: "CONFIG_A=y",
			frag: []string{"# CONFIG_A is not set"},
			want: "# CONFIG_A is not set\n",
		},
		{
			name: "y overrides is-not-set",
			base: "# CONFIG_A is not set",
			frag: []string{"CONFIG_A=y"},
			want: "CONFIG_A=y\n",
		},
		{
			name: "=n is equivalent to is-not-set",
			base: "",
			frag: []string{"CONFIG_A=n"},
			want: "# CONFIG_A is not set\n",
		},
		{
			name: "comments and blank lines are ignored",
			base: "# a plain comment\n\n  \nCONFIG_A=y\n# another comment",
			frag: nil,
			want: "CONFIG_A=y\n",
		},
		{
			name: "disjoint symbols are merged and sorted",
			base: "CONFIG_B=y",
			frag: []string{"CONFIG_A=y", "CONFIG_C=m"},
			want: "CONFIG_A=y\nCONFIG_B=y\nCONFIG_C=m\n",
		},
		{
			name: "string and numeric values are preserved verbatim",
			base: "CONFIG_CMDLINE=\"console=ttyAMA0\"",
			frag: []string{"CONFIG_NR_CPUS=0x10"},
			want: "CONFIG_CMDLINE=\"console=ttyAMA0\"\nCONFIG_NR_CPUS=0x10\n",
		},
		{
			name: "realistic bpf fragment over defconfig",
			base: "CONFIG_BPF=n\nCONFIG_MODULES=y",
			frag: []string{"CONFIG_BPF=y\nCONFIG_DEBUG_INFO_BTF=y\n# CONFIG_MODULES is not set"},
			want: "CONFIG_BPF=y\nCONFIG_DEBUG_INFO_BTF=y\n# CONFIG_MODULES is not set\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeConfig(tt.base, tt.frag...)
			if got != tt.want {
				t.Errorf("MergeConfig() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestCacheKey(t *testing.T) {
	const (
		ref  = "v6.17"
		arch = "arm64"
		tc   = "clang-18"
	)
	base := "CONFIG_BASE=y"
	fragBPF := "CONFIG_BPF=y"
	fragBTF := "CONFIG_DEBUG_INFO_BTF=y"

	key := func(r, a, t2, cfg string) string { return CacheKey(r, a, t2, cfg) }
	resolved := MergeConfig(base, fragBPF, fragBTF)

	t.Run("deterministic: identical inputs -> identical key (F1.3)", func(t *testing.T) {
		if key(ref, arch, tc, resolved) != key(ref, arch, tc, resolved) {
			t.Fatal("cache key is not deterministic")
		}
	})

	t.Run("order-independent for non-conflicting fragments (R1.3)", func(t *testing.T) {
		ab := MergeConfig(base, fragBPF, fragBTF)
		ba := MergeConfig(base, fragBTF, fragBPF)
		if key(ref, arch, tc, ab) != key(ref, arch, tc, ba) {
			t.Errorf("listing order of non-conflicting fragments changed the key")
		}
	})

	t.Run("sensitive to ref, arch, toolchain, and config (R1.3, F1.4)", func(t *testing.T) {
		baseKey := key(ref, arch, tc, resolved)
		cases := map[string]string{
			"ref":       key("v6.16", arch, tc, resolved),
			"arch":      key(ref, "x86_64", tc, resolved),
			"toolchain": key(ref, arch, "clang-19", resolved),
			"config":    key(ref, arch, tc, MergeConfig(base, fragBPF)), // dropped a fragment
		}
		for field, k := range cases {
			if k == baseKey {
				t.Errorf("changing %s did not change the cache key", field)
			}
		}
	})

	t.Run("length-prefixing prevents field-boundary collisions", func(t *testing.T) {
		// Without length prefixes, ("a","bc") and ("ab","c") would concatenate
		// to the same bytes; they must not collide.
		if CacheKey("a", "bc", tc, resolved) == CacheKey("ab", "c", tc, resolved) {
			t.Error("field-boundary collision: distinct (ref,arch) produced the same key")
		}
	})
}
