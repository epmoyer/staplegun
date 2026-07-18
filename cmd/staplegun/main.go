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
	"os"
	"path/filepath"
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
	vars := &varFlag{vars: staplegun.VarMap{}}
	verbose := flag.Bool("verbose", false, "print a trace of the processing steps")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Var(vars, "var", "define a template variable as name=value (may be repeated)")

	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(staplegun.VersionInfo())
		return
	}

	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "error: expected exactly two arguments: <sourceDir> <destDir>")
		flag.Usage()
		os.Exit(2)
	}

	sourceDir := flag.Arg(0)
	destDir := flag.Arg(1)

	// The destination directory is frequently a generated, git-ignored
	// directory that does not exist on a fresh checkout. Create it so the
	// build does not have to remember to.
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "staplegun ERROR: could not create destination %q: %s\n", destDir, err.Error())
		os.Exit(1)
	}

	fmt.Printf("Using: %s\n", staplegun.VersionInfo())

	if err := staplegun.MakeTemplates(sourceDir, destDir, *verbose, vars.vars); err != nil {
		fmt.Fprintf(os.Stderr, "staplegun ERROR: %s\n", err.Error())
		os.Exit(1)
	}
}

func usage() {
	out := flag.CommandLine.Output()
	prog := filepath.Base(os.Args[0])
	fmt.Fprintf(out, "%s - compose text templates by importing files and inserting blocks\n\n", staplegun.VersionInfo())
	fmt.Fprintf(out, "Usage:\n  %s [flags] <sourceDir> <destDir>\n\n", prog)
	fmt.Fprintf(out, "Flags:\n")
	flag.PrintDefaults()
	fmt.Fprintf(out, "\nExamples:\n")
	fmt.Fprintf(out, "  %s ./raw ./processed\n", prog)
	fmt.Fprintf(out, "  %s -verbose -var cacheBustingVersion=1.4.2 ./raw ./processed\n", prog)
	fmt.Fprintf(out, "  %s -var name=Acme -var year=2026 ./raw ./processed\n", prog)
}
