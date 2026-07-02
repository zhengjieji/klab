package exec

import (
	"reflect"
	"testing"
)

func TestWrap(t *testing.T) {
	got := wrap("klab", []string{"uname", "-r"})
	want := []string{"limactl", "shell", "klab", "--", "uname", "-r"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrap = %v, want %v", got, want)
	}
}

func TestLimaRunnerInstanceDefault(t *testing.T) {
	if (LimaRunner{}).instance() != "klab" {
		t.Error("empty Instance should default to klab")
	}
	if (LimaRunner{Instance: "other"}).instance() != "other" {
		t.Error("explicit Instance should be used")
	}
}
