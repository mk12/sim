package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func usage() {
	fmt.Printf(`
Usage: %s [command]

Manages program symlinks in $XDG_BIN_HOME.

Commands:
  path
    Print the install path
  list [-pl] [--path] [--link]
    List programs (alias: ls)
  install [-fk] [--force] [--keep-extension]
    Install programs (alias: i)
  remove
    Remove programs (alias: rm)
  prune
    Remove broken symlinks
  doctor
    Check for issues
  help
    Show this help message
`,
		os.Args[0])
}

func main() {
	log.SetFlags(0)
	var cmd string
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if cmd == "" || cmd == "help" || cmd == "-h" || cmd == "--help" {
		usage()
		return
	}
	args := parseArguments(os.Args[2:])
	switch cmd {
	case "path":
		cmdPath(args)
	case "ls", "list":
		cmdList(args)
	case "i", "install":
		cmdInstall(args)
	case "rm", "remove":
		cmdRemove(args)
	case "prune":
		cmdPrune(args)
	case "doctor":
		cmdDoctor(args)
	default:
		log.Fatalf("%s: unrecognized command", cmd)
	}
}

func cmdPath(args arguments) {
	args.validate(noPositional)
	fmt.Println(bin())
}

func cmdList(args arguments) {
	showPath := args.bool("-p", "--path")
	showLink := args.bool("-l", "--link")
	args.validate(noPositional)
	dir := bin()
	for _, file := range files(dir) {
		if file.IsDir() {
			continue
		}
		path := filepath.Join(dir, file.Name())
		if showPath {
			fmt.Print(path)
		} else {
			fmt.Print(file.Name())
		}
		if showLink && file.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				log.Fatalf("list: %s: %s", file.Name(), err)
			}
			if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
				fmt.Printf(" %s %s %s", brightBlack("->"), red(target), brightBlack("(broken)"))
			} else if err != nil {
				log.Fatalf("list: %s: %s", file.Name(), err)
			} else {
				fmt.Printf(" %s %s", brightBlack("->"), blue(target))
			}
		}
		fmt.Println()
	}
}

func cmdInstall(args arguments) {
	force := args.bool("-f", "--force")
	keepExt := args.bool("-k", "--keep-extension")
	args.validate(atLeastOnePositional)
	dir := bin()
	fmt.Printf("Using directory %s\n", dir)
	success := true
	for _, arg := range args.pos {
		fail := func(msg interface{}) {
			log.Printf("%s: %s", arg, msg)
			success = false
		}
		var name, target string
		if info, err := os.Stat(arg); err != nil {
			fail(err)
			continue
		} else if info.IsDir() {
			fail("is a directory")
			continue
		} else if !isExecutable(info.Mode()) {
			fail("not an executable")
			continue
		} else if abs, err := filepath.Abs(arg); err != nil {
			fail(err)
			continue
		} else {
			target = abs
			name = info.Name()
			if !keepExt {
				name = strings.TrimSuffix(name, filepath.Ext(name))
			}
		}
		fmt.Printf("Symlinking %s %s %s\n", name, brightBlack("->"), blue(target))
		path := filepath.Join(dir, name)
		if force {
			os.Remove(path)
		}
		if err := os.Symlink(target, path); errors.Is(err, os.ErrExist) {
			existing, err := os.Readlink(path)
			if err != nil {
				fail(err)
				continue
			}
			if target != existing {
				fail("already installed (overwrite with --force)")
				continue
			}
		} else if err != nil {
			fail(err)
			continue
		}
	}
	if !success {
		os.Exit(1)
	}
}

func cmdRemove(args arguments) {
	fwd := make(map[string]string)
	bwd := make(map[string]string)
	args.validate(atLeastOnePositional)
	dir := bin()
	for _, file := range files(dir) {
		if file.IsDir() || file.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(dir, file.Name())
		target, err := os.Readlink(path)
		if err != nil {
			log.Fatalf("remove: %s: %s", file.Name(), err)
		}
		fwd[file.Name()] = target
		bwd[target] = file.Name()
	}
	fmt.Printf("Using directory %s\n", dir)
	find := func(arg string) (string, string, bool) {
		if target, ok := fwd[arg]; ok {
			return arg, target, true
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			if name, ok := bwd[abs]; ok {
				return name, abs, true
			}
		}
		return "", "", false
	}
	success := true
	for _, arg := range args.pos {
		if name, target, ok := find(arg); ok {
			path := filepath.Join(dir, name)
			fmt.Printf("Removing %s %s\n", name, brightBlack(fmt.Sprintf("(%s)", target)))
			if err := os.Remove(path); err != nil {
				log.Printf("remove: %s: %s", arg, err)
			}
		} else {
			log.Printf("%s: no such program", arg)
			success = false
		}
	}
	if !success {
		os.Exit(1)
	}
}

func cmdPrune(args arguments) {
	args.validate(noPositional)
	dir := bin()
	for _, file := range files(dir) {
		if file.IsDir() {
			continue
		}
		path := filepath.Join(dir, file.Name())
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("Removing %s\n", file.Name())
			if err := os.Remove(path); err != nil {
				log.Printf("prune: %s: %s", file.Name(), err)
			}
		} else if err != nil {
			log.Fatalf("prune: %s: %s", file.Name(), err)
		}
	}
}

func cmdDoctor(args arguments) {
	args.validate(noPositional)
	dir := bin()
	success := true
	for _, file := range files(dir) {
		path := filepath.Join(dir, file.Name())
		fail := func(msg string) {
			fmt.Printf("%s: %s\n", msg, path)
			success = false
		}
		if file.IsDir() {
			fail("Unexpected directory")
			continue
		}
		if file.Type().IsRegular() && strings.HasPrefix(file.Name(), ".") {
			continue
		}
		if file.Type()&os.ModeSymlink == 0 {
			fail("Not a symlink")
			continue
		}
		if info, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			fail("Broken symlink")
			continue
		} else if err != nil {
			fail(err.Error())
			continue
		} else if !isExecutable(info.Mode()) {
			fail("Not an executable")
			continue
		}
	}
	if !success {
		os.Exit(1)
	}
}

type arguments struct {
	pos   []string
	flags map[string]struct{}
}

func parseArguments(raw []string) arguments {
	args := arguments{
		flags: make(map[string]struct{}),
	}
	for _, arg := range raw {
		if strings.HasPrefix(arg, "-") {
			args.flags[arg] = struct{}{}
		} else {
			args.pos = append(args.pos, arg)
		}
	}
	return args
}

type validateOpts int

const (
	noPositional validateOpts = iota
	atLeastOnePositional
)

func (a *arguments) validate(opts validateOpts) {
	success := true
	fail := func(format string, args ...interface{}) {
		log.Printf(format, args...)
		success = false
	}
	for flag := range a.flags {
		fail("%s: unrecognized flag", flag)
	}
	switch opts {
	case noPositional:
		if len(a.pos) > 0 {
			fail("%s: unexpected argument", a.pos[0])
		}
	case atLeastOnePositional:
		if len(a.pos) == 0 {
			fail("expected at least one argument")
		}
	}
	if !success {
		os.Exit(1)
	}
}

func (a *arguments) bool(short, long string) bool {
	if _, ok := a.flags[short]; ok {
		delete(a.flags, short)
		return true
	}
	if _, ok := a.flags[long]; ok {
		delete(a.flags, long)
		return true
	}
	return false
}

func bin() string {
	var rel string
	if dir, ok := os.LookupEnv("XDG_BIN_HOME"); ok {
		rel = dir
	} else {
		rel = filepath.Join(os.Getenv("HOME"), ".local", "bin")
	}
	abs, err := filepath.Abs(rel)
	if err != nil {
		log.Fatalf("getting absolute path to %s: %s", rel, err)
	}
	return abs
}

func files(dir string) []fs.DirEntry {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("reading %s: %s", dir, err)
	}
	return files
}

func isExecutable(mode fs.FileMode) bool {
	return mode&0o111 != 0
}

var noColor = func() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}()

func red(s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\x1b[31m%s\x1b[0m", s)
}

func blue(s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\x1b[34m%s\x1b[0m", s)
}

func brightBlack(s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\x1b[90m%s\x1b[0m", s)
}
