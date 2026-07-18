// Command staplegun is the command-line interface to the staplegun templating
// engine. It processes a directory of source templates and writes the finished
// parent documents to a destination directory — the same work that
// staplegun.MakeTemplates does at runtime, exposed for use in build scripts.
//
// Usage:
//
//	staplegun [flags] <sourceDir> <destDir>
//
// Flags:
//
//	-var name=value   define a template variable (may be repeated)
//	-verbose          print a trace of the processing steps
//	-version          print version information and exit
//
// Example:
//
//	staplegun -var cacheBustingVersion=1.4.2 ./data/templates/raw ./data/templates/processed
//
// The destination directory is created if it does not already exist, so it is
// safe to run against a generated/git-ignored output directory on a fresh
// checkout.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/epmoyer/staplegun"
)

// varFlag collects repeated -var name=value flags into a staplegun.VarMap.
type varFlag struct {
	vars staplegun.VarMap
}

func (v *varFlag) String() string {
	if len(v.vars) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(v.vars))
	for name, value := range v.vars {
		pairs = append(pairs, name+"="+value)
	}
	return strings.Join(pairs, ",")
}

func (v *varFlag) Set(s string) error {
	name, value, found := strings.Cut(s, "=")
	if !found {
		return fmt.Errorf("expected name=value, got %q", s)
	}
	if name == "" {
		return fmt.Errorf("variable name is empty in %q", s)
	}
	v.vars[name] = value
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args and performs the template processing, writing informational
// output to stdout and errors to stderr. It returns the process exit code:
// 0 on success, 2 for usage errors, and 1 for processing errors.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("staplegun", flag.ContinueOnError)
	fs.SetOutput(stderr)

	vars := &varFlag{vars: staplegun.VarMap{}}
	verbose := fs.Bool("verbose", false, "print a trace of the processing steps")
	showVersion := fs.Bool("version", false, "print version information and exit")
	fs.Var(vars, "var", "define a template variable as name=value (may be repeated)")
	fs.Usage = func() { usage(fs, stderr) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		// flag has already reported the error and printed usage.
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, staplegun.VersionInfo())
		return 0
	}

	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "error: expected exactly two arguments: <sourceDir> <destDir>")
		fs.Usage()
		return 2
	}

	sourceDir := fs.Arg(0)
	destDir := fs.Arg(1)

	// The destination directory is frequently a generated, git-ignored
	// directory that does not exist on a fresh checkout. Create it so the
	// build does not have to remember to.
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(stderr, "staplegun ERROR: could not create destination %q: %s\n", destDir, err.Error())
		return 1
	}

	fmt.Fprintf(stdout, "Using: %s\n", staplegun.VersionInfo())

	if err := staplegun.MakeTemplates(sourceDir, destDir, *verbose, vars.vars); err != nil {
		fmt.Fprintf(stderr, "staplegun ERROR: %s\n", err.Error())
		return 1
	}
	return 0
}

func usage(fs *flag.FlagSet, out io.Writer) {
	prog := fs.Name()
	fmt.Fprintf(out, "%s - compose text templates by importing files and inserting blocks\n\n", staplegun.VersionInfo())
	fmt.Fprintf(out, "Usage:\n  %s [flags] <sourceDir> <destDir>\n\n", prog)
	fmt.Fprintf(out, "Flags:\n")
	fs.PrintDefaults()
	fmt.Fprintf(out, "\nExamples:\n")
	fmt.Fprintf(out, "  %s ./raw ./processed\n", prog)
	fmt.Fprintf(out, "  %s -verbose -var cacheBustingVersion=1.4.2 ./raw ./processed\n", prog)
	fmt.Fprintf(out, "  %s -var name=Acme -var year=2026 ./raw ./processed\n", prog)
}
