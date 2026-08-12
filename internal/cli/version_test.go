package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command errored: %v", err)
	}
	if got := out.String(); !strings.HasPrefix(got, "kubedrill ") {
		t.Fatalf("version output %q does not start with %q", got, "kubedrill ")
	}
}

func TestRootExecuteReturnsZeroOnHelp(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--help errored: %v", err)
	}
}
