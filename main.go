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
	fmt.Printf("Usage: %s [-h] COMMAND", os.Args[0])
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
	fmt.Printf("Usage: %s install [-hfcmn] [-r NAME] PROGRAM ...", os.Args[0])
	fmt.Print(`

Install each PROGRAM in $XDG_BIN_HOME.

Options:
    -h, --help         Show this help message
    -f, --force        Overwrite existing programs
    -c, --copy         Copy instead of symlinking
    -m, --move         Move instead of symlinking
    -n, --no-ext       Remove file extensions
    -r, --rename NAME  Rename single PROGRAM to NAME
`)
}

func usageList() {
	fmt.Printf("Usage: %s list [-hpldtq] [PROGRAM ...]", os.Args[0])
	fmt.Print(`

List each matching PROGRAM in $XDG_BIN_HOME.
PROGRAM can be a basename, a full path, or a symlink target path.

Options:
    -h, --help    Show this help message
    -p, --path    Print full paths to programs
    -l, --long    Print symlink targets
    -d, --direct  Do not match on symlink targets
    -t, --target  Only match on symlink targets
    -q, --quiet   Ignore patterns that match nothing
`)
}

func usageRemove() {
	fmt.Printf("Usage: %s remove [-hdtq] PROGRAM ...", os.Args[0])
	fmt.Print(`

Remove each matching PROGRAM in $XDG_BIN_HOME.
PROGRAM can be a basename, a full path, or a symlink target path.

Options:
    -h, --help    Show this help message
    -d, --direct  Do not match on symlink targets
    -t, --target  Only match on symlink targets
    -q, --quiet   Ignore patterns that match nothing
`)
}

func main() {
	opts := parseOptions(os.Args[1:])
	var name string
	if opts.bool('h', "help") || len(os.Args) == 1 {
		name = "help"
	} else if opts.first != -1 {
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

func (c *command) dispatch(opts *options) {
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
	case "":
		c.fatal("missing command")
	default:
		c.fatal("%s: unrecognized command", c.name)
	}
}

func (c *command) help(opts *options) {
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

func (c *command) path(opts *options) {
	c.validate(opts, noArgs)
	fmt.Println(c.bin())
}

func (c *command) install(opts *options) {
	force := opts.bool('f', "force")
	copy := opts.bool('c', "copy")
	move := opts.bool('m', "move")
	noExt := opts.bool('n', "no-ext")
	rename := opts.string('r', "rename")
	c.validate(opts, atLeastOneArg)
	if copy && move {
		c.fatal("%s: cannot use --copy and --move together", c.name)
	}
	if noExt && rename != "" {
		c.fatal("%s: cannot use --no-ext and --rename together", c.name)
	}
	if rename != "" && len(opts.args) != 1 {
		c.fatal("%s: --rename requires a single program", c.name)
	}
	for _, arg := range opts.args {
		cmd, ok := newInstallCommand(c, arg, noExt, rename)
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
	targetStat                 fs.FileInfo
}

func newInstallCommand(cmd *command, arg string, noExt bool, rename string) (installCommand, bool) {
	c := installCommand{command: cmd, arg: arg}
	var err error
	if c.targetStat, err = os.Stat(arg); errors.Is(err, fs.ErrNotExist) {
		c.error("%s: file not found", arg)
	} else if err != nil {
		c.error("%s: %s", arg, err)
	} else if c.targetStat.IsDir() {
		c.error("%s: is a directory", arg)
	} else if strings.HasPrefix(filepath.Base(arg), ".") {
		c.error("%s: program must not start with '.'", arg)
	} else if !isExecutable(c.targetStat.Mode()) {
		c.error("%s: not an executable", arg)
	} else if c.absTarget, err = filepath.Abs(arg); err != nil {
		c.error("%s: %s", arg, err)
	} else {
		if rename != "" {
			c.name = rename
		} else {
			c.name = filepath.Base(c.absTarget)
			if noExt {
				c.name = strings.TrimSuffix(c.name, filepath.Ext(c.name))
			}
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
		if c.sameFileContent(info) {
			fmt.Printf(" %s\n", brightBlack("(already installed)"))
		} else {
			fmt.Println()
			c.error("%s: %s exists (overwrite with --force)", c.arg, c.name)
		}
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
			c.error("%s: %s exists (overwrite with --force)", c.arg, c.name)
			return
		}
		fmt.Printf(" %s\n", brightBlack("(already installed)"))
		// We still move the file below, for consistency. Why bother checking if
		// the content matches then? So that it succeeds without --force.
	} else if info, err := os.Lstat(c.absTarget); err != nil {
		fmt.Println()
		c.error("%s: %s", c.arg, err)
		return
	} else if isSymlink(info.Mode()) {
		fmt.Println()
		c.error("%s: cannot install symlinks with --move", c.arg)
		return
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
	c.error("%s: %s exists (overwrite with --force)", c.arg, c.name)
}

func (c *installCommand) sameFileContent(existingInfo fs.FileInfo) bool {
	if existingInfo.Size() != c.targetStat.Size() {
		return false
	}
	if existingInfo.Mode() != c.targetStat.Mode() {
		return false
	}
	err := exec.Command("cmp", "-s", c.path, c.absTarget).Run()
	if err == nil {
		return true
	}
	if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 1 {
		return false
	}
	c.error("%s: running cmp: %s", c.arg, err)
	return false
}

func (c *command) list(opts *options) {
	cmd := newLsRmCommand(c)
	cmd.showPath = opts.bool('p', "path")
	cmd.showTarget = opts.bool('l', "long")
	cmd.directOnly = opts.bool('d', "direct")
	cmd.targetOnly = opts.bool('t', "target")
	cmd.ignoreNoMatch = opts.bool('q', "quiet")
	cmd.validate(opts, anyArgs)
	if cmd.directOnly && cmd.targetOnly {
		cmd.fatal("%s: cannot use --direct and --target together", cmd.name)
	}
	if len(opts.args) > 0 {
		cmd.perform(cmd.listProgram, opts.args)
		return
	}
	for _, name := range cmd.names {
		cmd.listProgram(match{name, cmd.nameToAbsTarget[name]})
	}
}

func (c *command) remove(opts *options) {
	cmd := newLsRmCommand(c)
	cmd.showTarget = true
	cmd.directOnly = opts.bool('d', "direct")
	cmd.targetOnly = opts.bool('t', "target")
	cmd.ignoreNoMatch = opts.bool('q', "quiet")
	cmd.validate(opts, atLeastOneArg)
	cmd.perform(cmd.removeProgram, opts.args)
}

type lsRmCommand struct {
	*command
	showPath, showTarget, directOnly, targetOnly, ignoreNoMatch bool
	// Keys of nameToAbsTarget in sorted order.
	names []string
	// Map from program basenames to absolute symlink targets, or to "" for non-symlinks.
	nameToAbsTarget map[string]string
	// Map from absolute program paths to absolute symlink targets, or to "" for non-symlinks.
	pathToAbsTarget map[string]string
	// Map from absolute symlink targets to program basenames.
	absTargetToNames map[string][]string
}

func newLsRmCommand(cmd *command) lsRmCommand {
	c := lsRmCommand{
		command:          cmd,
		nameToAbsTarget:  make(map[string]string),
		pathToAbsTarget:  make(map[string]string),
		absTargetToNames: make(map[string][]string),
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
			continue
		}
		relOrAbsTarget, err := os.Readlink(path)
		if err != nil {
			c.fatal("%s: %s", file.Name(), err)
		}
		absTarget := ensureAbs(c.bin(), relOrAbsTarget)
		c.nameToAbsTarget[file.Name()] = absTarget
		c.pathToAbsTarget[path] = absTarget
		c.absTargetToNames[absTarget] = append(c.absTargetToNames[absTarget], file.Name())
	}
	return c
}

func (c *lsRmCommand) perform(action func(match), args []string) {
	seen := make(map[string]struct{})
	for _, arg := range args {
		matches := c.find(arg)
		if !c.ignoreNoMatch && len(matches) == 0 {
			c.error("%s: no match found", arg)
		}
		for _, m := range matches {
			if _, ok := seen[m.name]; ok {
				continue
			}
			seen[m.name] = struct{}{}
			action(m)
		}
	}
}

type match struct {
	name, absTarget string
}

func (c *lsRmCommand) listProgram(match match) {
	program := match.name
	if c.showPath {
		program = filepath.Join(c.bin(), match.name)
	}
	if !c.showTarget || match.absTarget == "" {
		fmt.Printf("%s\n", program)
	} else if _, err := os.Stat(match.absTarget); errors.Is(err, fs.ErrNotExist) {
		fmt.Printf("%s %s %s %s\n", program, brightBlack("->"), red(match.absTarget), brightBlack("(broken)"))
	} else if err != nil {
		c.error("%s: %s", match.name, err)
	} else {
		fmt.Printf("%s %s %s\n", program, brightBlack("->"), blue(match.absTarget))
	}
}

func (c *lsRmCommand) removeProgram(match match) {
	fmt.Print("Removing ")
	c.listProgram(match)
	path := filepath.Join(c.bin(), match.name)
	if err := os.Remove(path); err != nil {
		c.error("%s: %s", match.name, err)
	}
}

func (c *lsRmCommand) find(arg string) []match {
	abs, err := filepath.Abs(arg)
	if err != nil {
		c.fatal("%s: %s", arg, err)
	}
	var matches []match
	matchDirect := !c.targetOnly
	matchTarget := !c.directOnly
	if matchDirect {
		if absTarget, ok := c.nameToAbsTarget[arg]; ok {
			matches = append(matches, match{arg, absTarget})
		}
		if absTarget, ok := c.pathToAbsTarget[abs]; ok {
			matches = append(matches, match{filepath.Base(arg), absTarget})
		}
	}
	if matchTarget {
		for _, name := range c.absTargetToNames[abs] {
			matches = append(matches, match{name, abs})
		}
	}
	return matches
}

func (c *command) prune(opts *options) {
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

func (c *command) doctor(opts *options) {
	c.validate(opts, noArgs)
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
		if !isSymlink(file.Type()) {
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
	args []string
	// Maps each flag to the index of its potential argument in args, or to -1
	// if it is followed by another flag or by nothing.
	short map[rune]int
	long  map[string]int
	// Index of the first non-flag arg in args, or -1 if there is none.
	first int
	// Errors to report during validation.
	errors []string
}

func (opts *options) error(format string, args ...interface{}) {
	opts.errors = append(opts.errors, fmt.Sprintf(format, args...))
}

func parseOptions(raw []string) *options {
	opts := options{
		short: make(map[rune]int),
		long:  make(map[string]int),
		first: -1,
	}
	var index int
	nop := func() {}
	setArgIndex := func() { opts.first = index }
	processFlags := true
	for _, arg := range raw {
		if processFlags {
			if arg == "--" {
				processFlags = false
				setArgIndex = nop
				continue
			}
			if len(arg) >= 3 && strings.HasPrefix(arg, "--") {
				key := arg[2:]
				if _, ok := opts.long[key]; ok {
					opts.error("--%s: duplicate flag", key)
					setArgIndex = nop
				} else {
					opts.long[key] = -1
					setArgIndex = func() {
						opts.long[key] = index
					}
				}
				continue
			}
			if len(arg) >= 2 && strings.HasPrefix(arg, "-") {
				chars := []rune(arg[1:])
				for _, r := range chars {
					if _, ok := opts.short[r]; ok {
						opts.error("-%c: duplicate flag", r)
					} else {
						opts.short[r] = -1
					}
				}
				if len(chars) == 1 {
					setArgIndex = func() {
						opts.short[chars[0]] = index
					}
				} else {
					setArgIndex = nop
				}
				continue
			}
		}
		opts.args = append(opts.args, arg)
		setArgIndex()
		setArgIndex = nop
		index++
	}
	return &opts
}

func (o *options) shift() string {
	if o.first == -1 {
		panic("nothing to shift")
	}
	arg := o.args[o.first]
	o.removeArg(o.first)
	o.first = -1
	return arg
}

func (o *options) bool(short rune, long string) bool {
	var shortOk, longOk bool
	if _, shortOk = o.short[short]; shortOk {
		delete(o.short, short)
	}
	if _, longOk = o.long[long]; longOk {
		delete(o.long, long)
	}
	if shortOk && longOk {
		o.error("duplicate flags -%c and --%s", short, long)
	}
	return shortOk || longOk
}

func (o *options) string(short rune, long string) string {
	var (
		i               int
		shortOk, longOk bool
		value           string
	)
	if i, shortOk = o.short[short]; shortOk {
		delete(o.short, short)
		if i == -1 {
			o.error("-%c: missing argument", short)
		} else {
			value = o.args[i]
			o.removeArg(i)
		}
	}
	if _, longOk = o.long[long]; longOk {
		delete(o.long, long)
		if i == -1 {
			o.error("--%s: missing argument", long)
		} else {
			value = o.args[i]
			o.removeArg(i)
		}
	}
	if shortOk && longOk {
		o.error("duplicate flags -%c and --%s", short, long)
	}
	return value
}

func (o *options) removeArg(index int) {
	o.args = append(o.args[:index], o.args[index+1:]...)
	for k, i := range o.short {
		if i == index {
			panic("flags should have distinct arg indexes")
		}
		if i > index {
			o.short[k]--
		}
	}
	for k, i := range o.long {
		if i == index {
			panic("flags should have distinct arg indexes")
		}
		if i > index {
			o.long[k]--
		}
	}
}

type argValidation int

const (
	anyArgs argValidation = iota
	noArgs
	atLeastOneArg
)

func (c *command) validate(opts *options, validation argValidation) {
	for r := range opts.short {
		c.error("%s: -%c: unrecognized flag", c.name, r)
	}
	for s := range opts.long {
		c.error("%s: --%s: unrecognized flag", c.name, s)
	}
	switch validation {
	case anyArgs:
		// Do nothing.
	case noArgs:
		if len(opts.args) > 0 {
			c.error("%s: %s: unexpected argument", c.name, opts.args[0])
		}
	case atLeastOneArg:
		if len(opts.args) == 0 {
			c.error("%s: expected at least one argument", c.name)
		}
	}
	for _, err := range opts.errors {
		c.error("%s: %s", c.name, err)
	}
	if c.failed {
		os.Exit(1)
	}
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
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return true
	}
	if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeCharDevice == 0 {
		return true
	}
	return false
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
