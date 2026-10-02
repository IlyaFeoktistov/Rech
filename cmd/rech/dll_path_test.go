package main

import (
	"os"
	"strings"
	"testing"
)

func TestWithDLLSearchPathPrependsLibraryDirs(t *testing.T) {
	sep := string(os.PathListSeparator)
	got := withDLLSearchPath([]string{"Path=C:" + sep + "D:", "X=1"}, []string{"L1", "L2"})
	if got[0] != "Path=L1"+sep+"L2"+sep+"C:"+sep+"D:" || got[1] != "X=1" {
		t.Fatalf("получено %v", got)
	}
	got = withDLLSearchPath([]string{"X=1"}, []string{"L1"})
	if !strings.HasPrefix(got[len(got)-1], "PATH=L1") {
		t.Fatalf("ожидался новый PATH, получено %v", got)
	}
}
