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

Manage programs in $XDG_BIN_HOME.

Commands:
  help        Show this help message
  path        Show install path
  i, install  Install programs
  ls, list    List programs
  rm, remove  Remove programs
  prune       Remove broken symlinks
  doctor      Check for issues
`)
}

func usageInstall() {
	fmt.Printf("Usage: %s install [-hfcmn] PROGRAM ...", os.Args[0])
	fmt.Print(`

Install each PROGRAM in $XDG_BIN_HOME.

Options:
  -h, --help    Show this help message
  -f, --force   Overwrite existing programs
  -c, --copy    Copy instead of symlinking
  -m, --move    Move instead of symlinking
  -n, --no-ext  Remove file extensions
`)
}

func usageList() {
	fmt.Printf("Usage: %s list [-hplqt] [PROGRAM ...]", os.Args[0])
	fmt.Print(`

List each matching PROGRAM in $XDG_BIN_HOME.
PROGRAM can be a basename, a full path, or a symlink target path.

Options:
  -h, --help    Show this help message
  -p, --path    Print full paths to programs
  -l, --long    Print symlink targets
  -q, --quiet   Ignore patterns that match nothing
  -t, --target  Only match symlink target paths
`)
}

func usageRemove() {
	fmt.Printf("Usage: %s remove [-hasqt] [PROGRAM ...]", os.Args[0])
	fmt.Print(`

Remove each matching PROGRAM in $XDG_BIN_HOME.
PROGRAM can be a basename, a full path, or a symlink target path.

Options:
  -h, --help    Show this help message
  -a, --all     Remove all programs except --self
  -s, --self    Remove this program itself
  -q, --quiet   Ignore arguments that match nothing
  -t, --target  Only match symlink target paths
`)
}

func main() {
	opts := parseOptions(os.Args[1:])
	var name string
	if opts.bool('h', "help") || len(opts.args) == 0 {
		name = "help"
	} else {
		name = opts.shift()
	}
	cmd := command{name: name}
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
	case "help":
		c.help(opts)
	case "path":
		c.path(opts)
	case "i", "install":
		c.install(opts)
	case "ls", "list":
		c.list(opts)
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

func (c *command) help(opts options) {
	c.validate(opts, anyArgs)
	var name string
	if len(opts.args) >= 1 {
		name = opts.args[0]
	}
	switch name {
	case "", "help", "path", "prune", "doctor":
		usage()
	case "i", "install":
		usageInstall()
	case "ls", "list":
		usageList()
	case "rm", "remove":
		usageRemove()
	default:
		c.fatal("%s: unrecognized command", name)
	}
}

func (c *command) path(opts options) {
	c.validate(opts, noArgs)
	fmt.Println(c.bin())
}

func (c *command) install(opts options) {
	force := opts.bool('f', "force")
	copy := opts.bool('c', "copy")
	move := opts.bool('m', "move")
	noExt := opts.bool('n', "no-ext")
	c.validate(opts, atLeastOneArg)
	if copy && move {
		c.fatal("%s: cannot use --copy and --move together", c.name)
	}
	for _, arg := range opts.args {
		cmd, ok := newInstallCommand(c, arg, noExt)
		if !ok {
			continue
		}
		if force {
			os.Remove(cmd.path)
		}
		if copy {
			cmd.copy()
		} else if move {
			cmd.move()
		} else {
			cmd.symlink()
		}
	}
}

type installCommand struct {
	*command
	arg, name, path, absTarget string
	targetInfo                 fs.FileInfo
}

func newInstallCommand(cmd *command, arg string, noExt bool) (installCommand, bool) {
	c := installCommand{command: cmd, arg: arg}
	var err error
	if c.targetInfo, err = os.Stat(arg); errors.Is(err, fs.ErrNotExist) {
		c.error("%s: file not found", arg)
	} else if err != nil {
		c.error("%s: %s", arg, err)
	} else if c.targetInfo.IsDir() {
		c.error("%s: is a directory", arg)
	} else if strings.HasPrefix(filepath.Base(arg), ".") {
		c.error("%s: program must not start with '.'", arg)
	} else if !isExecutable(c.targetInfo.Mode()) {
		c.error("%s: not an executable", arg)
	} else if c.absTarget, err = filepath.Abs(arg); err != nil {
		c.error("%s: %s", arg, err)
	} else if strings.HasPrefix(c.absTarget, c.bin()+string(filepath.Separator)) {
		c.error("%s: file is already in %s", arg, c.bin())
	} else {
		c.name = filepath.Base(c.absTarget)
		if noExt {
			c.name = strings.TrimSuffix(c.name, filepath.Ext(c.name))
		}
		c.path = filepath.Join(c.bin(), c.name)
		return c, true
	}
	return c, false
}

func (c *installCommand) copy() {
	fmt.Printf("Copying %s %s %s", c.name, brightBlack("from"), blue(c.absTarget))
	info, err := os.Lstat(c.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Println()
		c.error("%s: %s", c.arg, err)
		return
	}
	if err == nil {
		if !c.sameFileContent(info) {
			fmt.Println()
			c.error("%s: program exists (overwrite with --force)", c.arg)
			return
		}
		fmt.Printf(" %s\n", brightBlack("(already installed)"))
		return
	}
	fmt.Println()
	if err := exec.Command("cp", c.absTarget, c.path).Run(); err != nil {
		c.error("%s: copying file: %s", c.arg, err)
	}
}

func (c *installCommand) move() {
	fmt.Printf("Moving %s %s %s", c.name, brightBlack("from"), blue(c.absTarget))
	info, err := os.Lstat(c.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Println()
		c.error("%s: %s", c.arg, err)
		return
	}
	if err == nil {
		if !c.sameFileContent(info) {
			fmt.Println()
			c.error("%s: program exists (overwrite with --force)", c.arg)
			return
		}
		fmt.Printf(" %s\n", brightBlack("(already installed)"))
		// We still move the file below, for consistency. Why bother checking if
		// the content matches then? So that it succeeds without --force.
	} else {
		fmt.Println()
	}
	if err := os.Rename(c.absTarget, c.path); err != nil {
		c.error("%s: moving file: %s", c.arg, err)
	}
}

func (c *installCommand) symlink() {
	relTarget, err := filepath.Rel(c.bin(), c.absTarget)
	if err != nil {
		c.error("%s: %s", c.arg, err)
		return
	}
	fmt.Printf("Symlinking %s %s %s", c.name, brightBlack("->"), blue(c.absTarget))
	err = os.Symlink(relTarget, c.path)
	if err == nil {
		fmt.Println()
		return
	}
	if !errors.Is(err, os.ErrExist) {
		fmt.Println()
		c.error("%s: %s", c.arg, err)
		return
	}
	info, err := os.Lstat(c.path)
	if err != nil {
		fmt.Println()
		c.error("%s: %s", c.arg, err)
		return
	}
	if isSymlink(info.Mode()) {
		existing, err := os.Readlink(c.path)
		if err != nil {
			fmt.Println()
			c.error("%s: %s", c.arg, err)
			return
		}
		if relTarget == existing {
			fmt.Printf(" %s\n", brightBlack("(already installed)"))
			return
		}
	}
	fmt.Println()
	c.error("%s: program exists (overwrite with --force)", c.arg)
}

func (c *installCommand) sameFileContent(existingInfo fs.FileInfo) bool {
	if existingInfo.Size() != c.targetInfo.Size() {
		return false
	}
	if existingInfo.Mode() != c.targetInfo.Mode() {
		return false
	}
	err := exec.Command("cmp", "-s", c.path, c.absTarget).Run()
	if err == nil {
		return true
	}
	if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 1 {
		return false
	}
	c.error("%s: runing cmp: %s", c.arg, err)
	return false
}

func (c *command) list(opts options) {
	cmd := newLsRmCommand(c)
	cmd.showPath = opts.bool('p', "path")
	cmd.showTarget = opts.bool('l', "long")
	cmd.ignoreFailedMatch = opts.bool('q', "quiet")
	cmd.onlyMatchTarget = opts.bool('t', "target")
	validation := anyArgs
	if opts.bool('a', "all") {
		validation = noArgs
	}
	cmd.validate(opts, validation)
	if len(opts.args) == 0 {
		cmd.listAll()
	} else {
		cmd.listByArgs(opts.args)
	}
	// for _, file := range c.files() {
	// 	if skip(file) {
	// 		continue
	// 	}
	// 	path := filepath.Join(c.bin(), file.Name())
	// 	if showPath {
	// 		fmt.Print(path)
	// 	} else {
	// 		fmt.Print(file.Name())
	// 	}
	// 	if !(isSymlink(file.Type()) && showTarget) {
	// 		fmt.Println()
	// 		continue
	// 	}
	// 	relOrAbsTarget, err := os.Readlink(path)
	// 	if err != nil {
	// 		fmt.Println()
	// 		c.fatal("%s: %s", file.Name(), err)
	// 	}
	// 	absTarget := ensureAbs(c.bin(), relOrAbsTarget)
	// 	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
	// 		fmt.Printf(" %s %s %s\n", brightBlack("->"), red(absTarget), brightBlack("(broken)"))
	// 	} else if err != nil {
	// 		fmt.Println()
	// 		c.fatal("%s: %s", file.Name(), err)
	// 	} else {
	// 		fmt.Printf(" %s %s\n", brightBlack("->"), blue(absTarget))
	// 	}
	// }
}

func (c *command) remove(opts options) {
	cmd := newLsRmCommand(c)
	removeAll := opts.bool('a', "all")
	removeSelf := opts.bool('s', "self")
	cmd.ignoreFailedMatch = opts.bool('q', "quiet")
	cmd.onlyMatchTarget = opts.bool('t', "target")
	cmd.showTarget = true
	if !(removeAll || removeSelf) {
		cmd.validate(opts, atLeastOneArg)
		cmd.removeByArgs(opts.args)
		return
	}
	cmd.validate(opts, noArgs)
	if removeAll {
		cmd.removeAll()
	}
	if removeSelf {
		cmd.removeSelf()
	}
}

type lsRmCommand struct {
	*command
	showPath, showTarget, ignoreFailedMatch, onlyMatchTarget bool
	// Keys of nameToAbsTarget in sorted order.
	names []string
	// Map from program basenames to absolute symlink targets, or to "" for non-symlinks.
	nameToAbsTarget map[string]string
	// Map from absolute program paths to absolute symlink targets, or to "" for non-symlinks.
	pathToAbsTarget map[string]string
	// Map from absolute symlink targets to program basenames.
	absTargetToName map[string]string
	// A key from nameToAbsTarget referring to this program itself.
	selfName string
}

func newLsRmCommand(cmd *command) lsRmCommand {
	c := lsRmCommand{
		command:         cmd,
		nameToAbsTarget: make(map[string]string),
		pathToAbsTarget: make(map[string]string),
		absTargetToName: make(map[string]string),
	}
	selfPath, err := os.Executable()
	if err != nil {
		c.error("finding self path: %s", err)
	} else if selfPath, err = filepath.EvalSymlinks(selfPath); err != nil {
		c.error("resolving self path: %s", err)
	}
	for _, file := range c.files() {
		if skip(file) {
			continue
		}
		c.names = append(c.names, file.Name())
		path := filepath.Join(c.bin(), file.Name())
		if !isSymlink(file.Type()) {
			c.nameToAbsTarget[file.Name()] = ""
			c.pathToAbsTarget[path] = ""
		} else {
			relOrAbsTarget, err := os.Readlink(path)
			if err != nil {
				c.fatal("%s: %s", file.Name(), err)
			}
			absTarget := ensureAbs(c.bin(), relOrAbsTarget)
			c.nameToAbsTarget[file.Name()] = absTarget
			c.pathToAbsTarget[path] = absTarget
			c.absTargetToName[absTarget] = file.Name()
		}
		if c.selfName == "" && selfPath != "" {
			// We need to follow all symlinks because it's unspecified whether
			// os.Executable() follows a symlink.
			resolved, err := filepath.EvalSymlinks(path)
			// Ignore errors, since in general there could be broken symlinks
			// (and in fact the user might be invoking sim to remove one).
			if err == nil && resolved == selfPath {
				c.selfName = file.Name()
			}
		}
	}
	return c
}

func (c *lsRmCommand) listAll() {
	for _, name := range c.names {
		if name == c.selfName {
			continue
		}
		c.listProgram(name, c.nameToAbsTarget[name])
	}
}

func (c *lsRmCommand) listByArgs(args []string) {
	for _, arg := range args {
		name, absTarget, ok := c.find(arg)
		if !ok {
			if !c.ignoreFailedMatch {
				c.error("%s: no such program", arg)
			}
			continue
		}
		c.listProgram(name, absTarget)
	}
}

func (c *lsRmCommand) listProgram(name, absTarget string) {
	program := name
	if c.showPath {
		program = filepath.Join(c.bin(), name)
	}
	if !c.showTarget || absTarget == "" {
		fmt.Printf("%s\n", program)
	} else {
		fmt.Printf("%s %s %s\n", program, brightBlack("->"), blue(absTarget))
	}
}

func (c *lsRmCommand) removeAll() {
	for name, absTarget := range c.nameToAbsTarget {
		if name == c.selfName {
			continue
		}
		c.removeProgram(name, absTarget)
	}
}

func (c *lsRmCommand) removeSelf() {
	name := c.selfName
	if name == "" {
		c.fatal("remove: --self: %s is not installed", name)
	}
	c.removeProgram(name, c.nameToAbsTarget[name])
}

func (c *lsRmCommand) removeByArgs(args []string) {
	for _, arg := range args {
		name, absTarget, ok := c.find(arg)
		if !ok {
			if !c.ignoreFailedMatch {
				c.error("%s: no such program", arg)
			}
			continue
		}
		if name == c.selfName {
			c.error("%s: if you really want to remove it, use --self", arg)
			continue
		}
		c.removeProgram(name, absTarget)
	}
}

func (c *lsRmCommand) removeProgram(name, absTarget string) {
	fmt.Print("Removing ")
	c.listProgram(name, absTarget)
	path := filepath.Join(c.bin(), name)
	if err := os.Remove(path); err != nil {
		c.error("%s: %s", name, err)
	}
}

func (c *lsRmCommand) find(arg string) (string, string, bool) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		c.fatal("%s: %s", arg, err)
	}
	if !c.onlyMatchTarget {
		if absTarget, ok := c.nameToAbsTarget[arg]; ok {
			return arg, absTarget, true
		}
		if absTarget, ok := c.pathToAbsTarget[abs]; ok {
			return filepath.Base(arg), absTarget, true
		}
	}
	if name, ok := c.absTargetToName[abs]; ok {
		return name, abs, true
	}
	return "", "", false
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
	anyArgs argValidation = iota
	noArgs
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
	case anyArgs:
		// Do nothing.
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
