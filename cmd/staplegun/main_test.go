package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/epmoyer/staplegun"
)

// runCLI invokes run with the given args, capturing stdout and stderr.
func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestVarFlagSet(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{name: "simple", input: "a=b", wantKey: "a", wantValue: "b"},
		{name: "empty value", input: "a=", wantKey: "a", wantValue: ""},
		{name: "value contains equals", input: "a=b=c", wantKey: "a", wantValue: "b=c"},
		{name: "value contains spaces", input: "greeting=hello world", wantKey: "greeting", wantValue: "hello world"},
		{name: "no equals", input: "abc", wantErr: true},
		{name: "empty name", input: "=x", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &varFlag{vars: staplegun.VarMap{}}
			err := v.Set(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Set(%q) = nil error, want error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Set(%q) unexpected error: %v", tc.input, err)
			}
			if got := v.vars[tc.wantKey]; got != tc.wantValue {
				t.Fatalf("var %q = %q, want %q", tc.wantKey, got, tc.wantValue)
			}
		})
	}
}

func TestVarFlagMultiple(t *testing.T) {
	v := &varFlag{vars: staplegun.VarMap{}}
	for _, in := range []string{"name=Acme", "year=2026"} {
		if err := v.Set(in); err != nil {
			t.Fatalf("Set(%q): %v", in, err)
		}
	}
	if len(v.vars) != 2 || v.vars["name"] != "Acme" || v.vars["year"] != "2026" {
		t.Fatalf("unexpected vars: %#v", v.vars)
	}
}

func TestRunVersion(t *testing.T) {
	code, stdout, _ := runCLI("-version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, staplegun.VersionInfo()) {
		t.Fatalf("stdout = %q, want it to contain %q", stdout, staplegun.VersionInfo())
	}
}

func TestRunHelp(t *testing.T) {
	// -h triggers flag.ErrHelp, which should be a clean (exit 0) request.
	code, _, stderr := runCLI("-h")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr = %q, want it to contain usage", stderr)
	}
}

func TestRunMissingArgs(t *testing.T) {
	code, _, stderr := runCLI()
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "expected exactly two arguments") {
		t.Fatalf("stderr = %q, want argument-count error", stderr)
	}
}

func TestRunTooManyArgs(t *testing.T) {
	code, _, _ := runCLI("a", "b", "c")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRunBadVar(t *testing.T) {
	code, _, stderr := runCLI("-var", "noequalssign", "src", "dst")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "-var") {
		t.Fatalf("stderr = %q, want it to mention the -var flag", stderr)
	}
}

func TestRunBadSourceDir(t *testing.T) {
	dest := t.TempDir()
	code, _, stderr := runCLI(filepath.Join(t.TempDir(), "does-not-exist"), dest)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "staplegun ERROR") {
		t.Fatalf("stderr = %q, want a processing error", stderr)
	}
}

func TestRunUndefinedBlockInParent(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "index.html", "{{ staplegun parent }}\n{{ staplegun insert_block missing }}\n")
	code, _, stderr := runCLI(src, t.TempDir())
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "staplegun ERROR") {
		t.Fatalf("stderr = %q, want a processing error", stderr)
	}
}

func TestRunSuccess(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "index.html",
		"{{ staplegun parent }}\n"+
			"<link href=\"/app.css?v={{ staplegun var version }}\">\n"+
			"{{ staplegun import_file _partial.html }}\n")
	writeFile(t, src, "_partial.html",
		"{{ staplegun child }}\n"+
			"<p>hi {{ staplegun var name }}</p>\n")

	dest := t.TempDir()
	code, stdout, stderr := runCLI("-var", "version=9", "-var", "name=Bob", src, dest)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Using: "+staplegun.VersionInfo()) {
		t.Fatalf("stdout = %q, want it to report the version", stdout)
	}

	out := readFile(t, filepath.Join(dest, "index.html"))
	if !strings.Contains(out, "v=9") {
		t.Errorf("output missing substituted version var:\n%s", out)
	}
	if !strings.Contains(out, "hi Bob") {
		t.Errorf("output missing substituted name var from imported child:\n%s", out)
	}
	if !strings.Contains(out, "<!-- sg:file:start:_partial.html -->") {
		t.Errorf("output missing import markers:\n%s", out)
	}

	// The child document must not be written to the destination.
	if _, err := os.Stat(filepath.Join(dest, "_partial.html")); !os.IsNotExist(err) {
		t.Errorf("child document _partial.html was written to dest, but should not be")
	}
}

func TestRunCreatesMissingDestDir(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "index.html", "{{ staplegun parent }}\nhello\n")

	// A nested destination that does not exist yet (the fresh-checkout case).
	dest := filepath.Join(t.TempDir(), "generated", "processed")
	code, _, stderr := runCLI(src, dest)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dest, "index.html")); err != nil {
		t.Fatalf("expected dest dir to be created and index.html written: %v", err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}
