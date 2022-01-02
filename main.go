// Copyright 2022 Mitchell Kember. Subject to the MIT License.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func usage() {
	fmt.Printf("Usage: %s [COMMAND] [OPTION ...]", os.Args[0])
	fmt.Print(`

Manage programs in $XDG_BIN_HOME

Commands:
  help        Print help message
  path        Show install path
  ls, list    List programs
  i, install  Install programs
  rm, remove  Remove programs
  prune       Remove broken symlinks
  doctor      Check for issues

Options:
  list
    -p, --path      Print full paths
    -t, --target    Print symlink targets

  install
    PROGRAM ...     Paths to programs
    -f, --force     Overwrite existing programs
    -c, --copy      Copy instead of symlinking
    -m, --move      Move instead of symlinking
    -d, --drop-ext  Drop file extension

  remove
    PROGRAM ...     Program names/paths or symlink target paths
    -q, --quiet     Ignore arguments that match nothing
    -t, --target    Only match symlink target paths
    -a, --all       Remove all programs except this one
    -s, --self      Remove this program itself
`)
}

func main() {
	opts := parseOptions(os.Args[1:])
	if len(opts.args) == 0 || opts.args[0] == "help" || opts.bool('h', "help") {
		usage()
		return
	}
	cmd := command{name: opts.shift()}
	cmd.dispatch(opts)
	if cmd.failed {
		os.Exit(1)
	}
}

type command struct {
	name   string
	failed bool
	binDir string
}

func (c *command) dispatch(opts options) {
	switch c.name {
	case "path":
		c.path(opts)
	case "ls", "list":
		c.list(opts)
	case "i", "install":
		c.install(opts)
	case "rm", "remove":
		c.remove(opts)
	case "prune":
		c.prune(opts)
	case "doctor":
		c.doctor(opts)
	default:
		c.fatal("%s: unrecognized command", c.name)
	}
}

func (c *command) path(opts options) {
	c.validate(opts, noArgs)
	fmt.Println(c.bin())
}

func (c *command) list(opts options) {
	showPath := opts.bool('p', "path")
	showTarget := opts.bool('t', "target")
	c.validate(opts, noArgs)
	for _, file := range c.files() {
		if skip(file) {
			continue
		}
		path := filepath.Join(c.bin(), file.Name())
		if showPath {
			fmt.Print(path)
		} else {
			fmt.Print(file.Name())
		}
		if !(isSymlink(file.Type()) && showTarget) {
			fmt.Println()
			continue
		}
		relOrAbsTarget, err := os.Readlink(path)
		if err != nil {
			fmt.Println()
			c.fatal("%s: %s", file.Name(), err)
		}
		absTarget := ensureAbs(c.bin(), relOrAbsTarget)
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			fmt.Printf(" %s %s %s\n", brightBlack("->"), red(absTarget), brightBlack("(broken)"))
		} else if err != nil {
			fmt.Println()
			c.fatal("%s: %s", file.Name(), err)
		} else {
			fmt.Printf(" %s %s\n", brightBlack("->"), blue(absTarget))
		}
	}
}

func (c *command) install(opts options) {
	force := opts.bool('f', "force")
	copy := opts.bool('c', "copy")
	move := opts.bool('m', "move")
	dropExt := opts.bool('d', "drop-ext")
	c.validate(opts, atLeastOneArg)
	if copy && move {
		c.fatal("%s: cannot use --copy and --move together", c.name)
	}
	for _, arg := range opts.args {
		ic, ok := newInstallCommand(c, arg, dropExt)
		if !ok {
			continue
		}
		if force {
			os.Remove(ic.path)
		}
		if copy {
			ic.copy()
		} else if move {
			ic.move()
		} else {
			ic.symlink()
		}
	}
}

type installCommand struct {
	*command
	arg, name, path, absTarget string
	targetInfo                 fs.FileInfo
}

func newInstallCommand(cmd *command, arg string, dropExt bool) (installCommand, bool) {
	ic := installCommand{command: cmd, arg: arg}
	var err error
	if ic.targetInfo, err = os.Stat(arg); errors.Is(err, fs.ErrNotExist) {
		ic.error("%s: file not found", arg)
	} else if err != nil {
		ic.error("%s: %s", arg, err)
	} else if ic.targetInfo.IsDir() {
		ic.error("%s: is a directory", arg)
	} else if strings.HasPrefix(filepath.Base(arg), ".") {
		ic.error("%s: program must not start with '.'", arg)
	} else if !isExecutable(ic.targetInfo.Mode()) {
		ic.error("%s: not an executable", arg)
	} else if ic.absTarget, err = filepath.Abs(arg); err != nil {
		ic.error("%s: %s", arg, err)
	} else if strings.HasPrefix(ic.absTarget, ic.bin()+string(filepath.Separator)) {
		ic.error("%s: file is already in %s", arg, ic.bin())
	} else {
		ic.name = filepath.Base(ic.absTarget)
		if dropExt {
			ic.name = strings.TrimSuffix(ic.name, filepath.Ext(ic.name))
		}
		ic.path = filepath.Join(ic.bin(), ic.name)
		return ic, true
	}
	return ic, false
}

func (ic *installCommand) copy() {
	fmt.Printf("Copying %s %s %s", ic.name, brightBlack("from"), blue(ic.absTarget))
	info, err := os.Lstat(ic.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Println()
		ic.error("%s: %s", ic.arg, err)
		return
	}
	if err == nil {
		if !ic.sameFileContent(info) {
			fmt.Println()
			ic.error("%s: program exists (overwrite with --force)", ic.arg)
			return
		}
		fmt.Printf(" %s\n", brightBlack("(already installed)"))
		return
	}
	fmt.Println()
	if err := exec.Command("cp", ic.absTarget, ic.path).Run(); err != nil {
		ic.error("%s: copying file: %s", ic.arg, err)
	}
}

func (ic *installCommand) move() {
	fmt.Printf("Moving %s %s %s", ic.name, brightBlack("from"), blue(ic.absTarget))
	info, err := os.Lstat(ic.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Println()
		ic.error("%s: %s", ic.arg, err)
		return
	}
	if err == nil {
		if !ic.sameFileContent(info) {
			fmt.Println()
			ic.error("%s: program exists (overwrite with --force)", ic.arg)
			return
		}
		fmt.Printf(" %s\n", brightBlack("(already installed)"))
		// We still move the file below, for consistency. Why bother checking if
		// the content matches then? So that it succeeds without --force.
	} else {
		fmt.Println()
	}
	if err := os.Rename(ic.absTarget, ic.path); err != nil {
		ic.error("%s: moving file: %s", ic.arg, err)
	}
}

func (ic *installCommand) symlink() {
	relTarget, err := filepath.Rel(ic.bin(), ic.absTarget)
	if err != nil {
		ic.error("%s: %s", ic.arg, err)
		return
	}
	fmt.Printf("Symlinking %s %s %s", ic.name, brightBlack("->"), blue(ic.absTarget))
	err = os.Symlink(relTarget, ic.path)
	if err == nil {
		fmt.Println()
		return
	}
	if !errors.Is(err, os.ErrExist) {
		fmt.Println()
		ic.error("%s: %s", ic.arg, err)
		return
	}
	info, err := os.Lstat(ic.path)
	if err != nil {
		fmt.Println()
		ic.error("%s: %s", ic.arg, err)
		return
	}
	if isSymlink(info.Mode()) {
		existing, err := os.Readlink(ic.path)
		if err != nil {
			fmt.Println()
			ic.error("%s: %s", ic.arg, err)
			return
		}
		if relTarget == existing {
			fmt.Printf(" %s\n", brightBlack("(already installed)"))
			return
		}
	}
	fmt.Println()
	ic.error("%s: program exists (overwrite with --force)", ic.arg)
}

func (ic *installCommand) sameFileContent(existingInfo fs.FileInfo) bool {
	if existingInfo.Size() != ic.targetInfo.Size() {
		return false
	}
	if existingInfo.Mode() != ic.targetInfo.Mode() {
		return false
	}
	err := exec.Command("cmp", "-s", ic.path, ic.absTarget).Run()
	if err == nil {
		return true
	}
	if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 1 {
		return false
	}
	ic.error("%s: runing cmp: %s", ic.arg, err)
	return false
}

func (c *command) remove(opts options) {
	rc := newRemoveCommand(c)
	rc.quiet = opts.bool('q', "quiet")
	rc.targetOnly = opts.bool('t', "target")
	removeAll := opts.bool('a', "all")
	removeSelf := opts.bool('s', "self")
	if !(removeAll || removeSelf) {
		rc.validate(opts, atLeastOneArg)
		rc.removeByArgs(opts)
		return
	}
	rc.validate(opts, noArgs)
	if removeAll {
		rc.removeAll()
	}
	if removeSelf {
		rc.removeSelf()
	}
}

type removeCommand struct {
	*command
	quiet, targetOnly bool
	// Map from program basenames to absolute symlink targets, or to "" for non-symlinks.
	nameToAbsTarget map[string]string
	// Map from absolute program paths to absolute symlink targets, or to "" for non-symlinks.
	pathToAbsTarget map[string]string
	// Map from absolute symlink targets to program basenames.
	absTargetToName map[string]string
	// A key from nameToAbsTarget referring to this program itself.
	selfName string
}

func newRemoveCommand(cmd *command) removeCommand {
	rc := removeCommand{
		command:         cmd,
		nameToAbsTarget: make(map[string]string),
		pathToAbsTarget: make(map[string]string),
		absTargetToName: make(map[string]string),
	}
	selfPath, err := os.Executable()
	if err != nil {
		rc.error("finding self path: %s", err)
	} else if selfPath, err = filepath.EvalSymlinks(selfPath); err != nil {
		rc.error("resolving self path: %s", err)
	}
	for _, file := range rc.files() {
		if skip(file) {
			continue
		}
		path := filepath.Join(rc.bin(), file.Name())
		if !isSymlink(file.Type()) {
			rc.nameToAbsTarget[file.Name()] = ""
			rc.pathToAbsTarget[path] = ""
		} else {
			relOrAbsTarget, err := os.Readlink(path)
			if err != nil {
				rc.fatal("%s: %s", file.Name(), err)
			}
			absTarget := ensureAbs(rc.bin(), relOrAbsTarget)
			rc.nameToAbsTarget[file.Name()] = absTarget
			rc.pathToAbsTarget[path] = absTarget
			rc.absTargetToName[absTarget] = file.Name()
		}
		if rc.selfName == "" && selfPath != "" {
			// We need to follow all symlinks because it's unspecified whether
			// os.Executable() follows a symlink.
			resolved, err := filepath.EvalSymlinks(path)
			// Ignore errors, since in general there could be broken symlinks
			// (and in fact the user might be invoking sim to remove one).
			if err == nil && resolved == selfPath {
				rc.selfName = file.Name()
			}
		}
	}
	return rc
}

func (rc *removeCommand) removeAll() {
	for name, absTarget := range rc.nameToAbsTarget {
		if name == rc.selfName {
			continue
		}
		rc.removeProgram(name, absTarget)
	}
}

func (rc *removeCommand) removeSelf() {
	name := rc.selfName
	if name == "" {
		rc.fatal("remove: --self: %s is not installed", name)
	}
	rc.removeProgram(name, rc.nameToAbsTarget[name])
}

func (rc *removeCommand) removeByArgs(validatedOpts options) {
	find := func(arg string) (string, string, bool) {
		abs, err := filepath.Abs(arg)
		if err != nil {
			rc.fatal("%s: %s", arg, err)
		}
		if !rc.targetOnly {
			if absTarget, ok := rc.nameToAbsTarget[arg]; ok {
				return arg, absTarget, true
			}
			if absTarget, ok := rc.pathToAbsTarget[abs]; ok {
				return filepath.Base(arg), absTarget, true
			}
		}
		if name, ok := rc.absTargetToName[abs]; ok {
			return name, abs, true
		}
		return "", "", false
	}
	for _, arg := range validatedOpts.args {
		name, absTarget, ok := find(arg)
		if !ok {
			if !rc.quiet {
				rc.error("%s: no such program", arg)
			}
			continue
		}
		if name == rc.selfName {
			rc.error("%s: if you really want to remove it, use --self", arg)
			continue
		}
		rc.removeProgram(name, absTarget)
	}
}

func (rc *removeCommand) removeProgram(name, absTarget string) {
	path := filepath.Join(rc.bin(), name)
	if absTarget == "" {
		fmt.Printf("Removing %s\n", name)
	} else {
		fmt.Printf("Removing %s %s %s\n", name, brightBlack("->"), blue(absTarget))
	}
	if err := os.Remove(path); err != nil {
		rc.error("%s: %s", name, err)
	}
}

func (c *command) prune(opts options) {
	c.validate(opts, noArgs)
	for _, file := range c.files() {
		if skip(file) || !isSymlink(file.Type()) {
			continue
		}
		path := filepath.Join(c.bin(), file.Name())
		relOrAbsTarget, err := os.Readlink(path)
		if err != nil {
			c.error("%s", err)
			continue
		}
		absTarget := ensureAbs(c.bin(), relOrAbsTarget)
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("Removing %s %s %s %s\n", file.Name(), brightBlack("->"), red(absTarget), brightBlack("(broken)"))
			if err := os.Remove(path); err != nil {
				c.error("%s: %s", file.Name(), err)
			}
		} else if err != nil {
			c.fatal("%s: %s", file.Name(), err)
		}
	}
}

func (c *command) doctor(opts options) {
	c.validate(opts, noArgs)
	targetToName := make(map[string]string)
	for _, file := range c.files() {
		path := filepath.Join(c.bin(), file.Name())
		if file.IsDir() {
			c.error("%s: unexpected directory", path)
			continue
		}
		if skip(file) {
			continue
		}
		if info, err := os.Stat(path); isSymlink(file.Type()) && errors.Is(err, fs.ErrNotExist) {
			c.error("%s: broken symlink", path)
			continue
		} else if err != nil {
			c.error("%s", err)
			continue
		} else if !isExecutable(info.Mode()) {
			c.error("%s: not an executable", path)
			continue
		}
		if isSymlink(file.Type()) {
			continue
		}
		relOrAbsTarget, err := os.Readlink(path)
		if err != nil {
			c.error("%s", err)
			continue
		}
		if filepath.IsAbs(relOrAbsTarget) {
			c.error("%s: symlink is absolute (should be relative)", path)
			continue
		}
		absTarget := ensureAbs(c.bin(), relOrAbsTarget)
		if otherName, ok := targetToName[absTarget]; ok {
			c.error("both %s and %s map to the same executable, %s", file.Name(), otherName, absTarget)
			continue
		}
		targetToName[absTarget] = file.Name()
	}
}

func (c *command) error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	c.failed = true
}

func (c *command) fatal(format string, args ...interface{}) {
	c.error(format, args...)
	os.Exit(1)
}

type options struct {
	args  []string
	short map[rune]struct{}
	long  map[string]struct{}
}

func parseOptions(raw []string) options {
	opts := options{
		short: make(map[rune]struct{}),
		long:  make(map[string]struct{}),
	}
	for _, arg := range raw {
		if strings.HasPrefix(arg, "--") {
			opts.long[arg[2:]] = struct{}{}
		} else if strings.HasPrefix(arg, "-") {
			for _, r := range arg[1:] {
				opts.short[r] = struct{}{}
			}
		} else {
			opts.args = append(opts.args, arg)
		}
	}
	return opts
}

func (a *options) shift() string {
	var arg string
	arg, a.args = a.args[0], a.args[1:]
	return arg
}

type argValidation int

const (
	noArgs argValidation = iota
	atLeastOneArg
)

func (c *command) validate(opts options, validation argValidation) {
	for r := range opts.short {
		c.fatal("%s: -%c: unrecognized flag", c.name, r)
	}
	for s := range opts.long {
		c.fatal("%s: --%s: unrecognized flag", c.name, s)
	}
	switch validation {
	case noArgs:
		if len(opts.args) > 0 {
			c.fatal("%s: %s: unexpected argument", c.name, opts.args[0])
		}
	case atLeastOneArg:
		if len(opts.args) == 0 {
			c.fatal("%s: expected at least one argument", c.name)
		}
	}
	if c.failed {
		os.Exit(1)
	}
}

func (a *options) bool(short rune, long string) bool {
	var shortOk, longOk bool
	if _, shortOk = a.short[short]; shortOk {
		delete(a.short, short)
	}
	if _, longOk = a.long[long]; longOk {
		delete(a.long, long)
	}
	return shortOk || longOk
}

func (c *command) bin() string {
	if c.binDir != "" {
		return c.binDir
	}
	key := "XDG_BIN_HOME"
	if c.binDir = os.Getenv(key); c.binDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			c.fatal("%s", err)
		}
		c.binDir = filepath.Join(home, ".local", "bin")
	} else if !filepath.IsAbs(c.binDir) {
		c.fatal("%s: %s should be absolute", c.binDir, key)
	}
	return c.binDir
}

func (c *command) files() []fs.DirEntry {
	files, err := os.ReadDir(c.bin())
	if err != nil {
		c.fatal("reading %s: %s", c.bin(), err)
	}
	return files
}

func skip(file fs.DirEntry) bool {
	return file.IsDir() || strings.HasPrefix(file.Name(), ".")
}

func isSymlink(mode fs.FileMode) bool {
	return mode&os.ModeSymlink != 0
}

func isExecutable(mode fs.FileMode) bool {
	return mode&0o111 != 0
}

func ensureAbs(base string, relOrAbs string) string {
	if filepath.IsAbs(relOrAbs) {
		return relOrAbs
	}
	return filepath.Join(base, relOrAbs)
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
