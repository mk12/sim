package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func usage() {
	fmt.Printf("Usage: %s [COMMAND] [OPTION ...]", os.Args[0])
	fmt.Print(`

Manage program symlinks in $XDG_BIN_HOME

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
    -l, --link      Print symlink targets

  install
    PROGRAM ...     Paths to programs
    -f, --force     Overwrite existing symlinks
    -k, --keep-ext  Keep extensions in symlink names

  remove
    PROGRAM ...     Symlink names, symlink paths, or target paths
    -q, --quiet     Ignore arguments that match nothing
    -t, --target    Match target paths only
    -a, --all       Remove all programs
    -s, --self      Remove this program itself
`)
}

func main() {
	var arg string
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}
	if arg == "" || arg == "help" || arg == "-h" || arg == "--help" {
		usage()
		return
	}
	cmd := command{name: arg}
	args := parseArguments(os.Args[2:])
	cmd.dispatch(args)
	if cmd.failed {
		os.Exit(1)
	}
}

type command struct {
	name   string
	failed bool
	binDir string
}

func (c *command) dispatch(args arguments) {
	switch c.name {
	case "path":
		c.path(args)
	case "ls", "list":
		c.list(args)
	case "i", "install":
		c.install(args)
	case "rm", "remove":
		c.remove(args)
	case "prune":
		c.prune(args)
	case "doctor":
		c.doctor(args)
	default:
		c.fatal("%s: unrecognized command", c.name)
	}
}

func (c *command) path(args arguments) {
	c.validate(args, noPositional)
	fmt.Println(c.bin())
}

func (c *command) list(args arguments) {
	showPath := args.bool("-p", "--path")
	showLink := args.bool("-l", "--link")
	c.validate(args, noPositional)
	for _, file := range c.files() {
		if file.IsDir() || file.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(c.bin(), file.Name())
		if showPath {
			fmt.Print(path)
		} else {
			fmt.Print(file.Name())
		}
		if showLink {
			relOrAbsTarget, err := os.Readlink(path)
			if err != nil {
				c.fatal("%s: %s", file.Name(), err)
			}
			absTarget := ensureAbs(c.bin(), relOrAbsTarget)
			if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
				fmt.Printf(" %s %s %s", brightBlack("->"), red(absTarget), brightBlack("(broken)"))
			} else if err != nil {
				c.fatal("%s: %s", file.Name(), err)
			} else {
				fmt.Printf(" %s %s", brightBlack("->"), blue(absTarget))
			}
		}
		fmt.Println()
	}
}

func (c *command) install(args arguments) {
	force := args.bool("-f", "--force")
	keepExt := args.bool("-k", "--keep-ext")
	c.validate(args, atLeastOnePositional)
	for _, arg := range args.pos {
		var (
			info                 fs.FileInfo
			absTarget, relTarget string
			err                  error
		)
		if info, err = os.Stat(arg); errors.Is(err, fs.ErrNotExist) {
			c.error("%s: file not found", arg)
			continue
		} else if err != nil {
			c.error("%s: %s", arg, err)
			continue
		} else if info.IsDir() {
			c.error("%s: is a directory", arg)
			continue
		} else if !isExecutable(info.Mode()) {
			c.error("%s: not an executable", arg)
			continue
		} else if absTarget, err = filepath.Abs(arg); err != nil {
			c.error("%s: %s", arg, err)
			continue
		} else if strings.HasPrefix(absTarget, c.bin()+string(filepath.Separator)) {
			c.error("%s: file is already in %s", arg, c.bin())
			continue
		} else if relTarget, err = filepath.Rel(c.bin(), absTarget); err != nil {
			c.error("%s: %s", arg, err)
			continue
		}
		name := info.Name()
		if !keepExt {
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		fmt.Printf("Installing %s %s %s", name, brightBlack("->"), blue(absTarget))
		path := filepath.Join(c.bin(), name)
		if force {
			os.Remove(path)
		}
		if err := os.Symlink(relTarget, path); errors.Is(err, os.ErrExist) {
			existing, err := os.Readlink(path)
			if err != nil {
				fmt.Println()
				c.error("%s: %s", arg, err)
			} else if relTarget != existing {
				fmt.Println()
				c.error("%s: program exists (overwrite with --force)", arg)
			} else {
				fmt.Printf(" %s\n", brightBlack("(already installed)"))
			}
		} else if err != nil {
			fmt.Println()
			c.error("%s: %s", arg, err)
		} else {
			fmt.Println()
		}
	}
}

func (c *command) remove(args arguments) {
	rc := newRemoveCommand(c)
	rc.quiet = args.bool("-q", "--quiet")
	rc.targetOnly = args.bool("-t", "--target")
	removeAll := args.bool("-a", "--all")
	removeSelf := args.bool("-s", "--self")
	if !(removeAll || removeSelf) {
		rc.validate(args, atLeastOnePositional)
		rc.removeSpecific(args)
		return
	}
	rc.validate(args, noPositional)
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
	// Maps between symlinks and their targets.
	nameToAbsTarget, pathToAbsTarget, absTargetToName map[string]string
	// Name of the symlink to this program itself.
	self string
}

func newRemoveCommand(cmd *command) removeCommand {
	rc := removeCommand{
		command:         cmd,
		nameToAbsTarget: make(map[string]string),
		pathToAbsTarget: make(map[string]string),
		absTargetToName: make(map[string]string),
	}
	self, err := os.Executable()
	if err != nil {
		rc.error("finding self path: %s", err)
	} else if self, err = filepath.EvalSymlinks(self); err != nil {
		rc.error("resolving self path: %s", err)
	}
	for _, file := range rc.files() {
		if file.IsDir() || file.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(rc.bin(), file.Name())
		relOrAbsTarget, err := os.Readlink(path)
		if err != nil {
			rc.fatal("%s: %s", file.Name(), err)
		}
		absTarget := ensureAbs(rc.bin(), relOrAbsTarget)
		rc.nameToAbsTarget[file.Name()] = absTarget
		rc.pathToAbsTarget[path] = absTarget
		rc.absTargetToName[absTarget] = file.Name()
		if rc.self == "" && self != "" {
			// We need to follow all symlinks because it's unspecified whether
			// os.Executable() follows a symlink.
			resolved, err := filepath.EvalSymlinks(absTarget)
			if err != nil {
				rc.error("resolving %s: %s", file.Name(), err)
			} else if resolved == self {
				rc.self = file.Name()
			}
		}
	}
	return rc
}

func (rc *removeCommand) removeAll() {
	for name, absTarget := range rc.nameToAbsTarget {
		if name == rc.self {
			continue
		}
		path := filepath.Join(rc.bin(), name)
		fmt.Printf("Removing %s %s %s\n", name, brightBlack("->"), blue(absTarget))
		if err := os.Remove(path); err != nil {
			rc.error("%s: %s", name, err)
		}
	}
}

func (rc *removeCommand) removeSelf() {
	name := rc.self
	if name == "" {
		rc.fatal("remove --self: already removed")
	}
	absTarget, ok := rc.nameToAbsTarget[name]
	if !ok {
		panic("self not in nameToAbsTarget")
	}
	fmt.Printf("Removing %s %s %s\n", name, brightBlack("->"), blue(absTarget))
	path := filepath.Join(rc.bin(), name)
	if err := os.Remove(path); err != nil {
		rc.error("%s: %s", name, err)
	}
}

func (rc *removeCommand) removeSpecific(validatedArgs arguments) {
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
	for _, arg := range validatedArgs.pos {
		name, absTarget, ok := find(arg)
		if !ok {
			if !rc.quiet {
				rc.error("%s: no such program", arg)
			}
			continue
		}
		if name == rc.self {
			rc.error("%s: if you really want to remove it, use --self", arg)
			continue
		}
		path := filepath.Join(rc.bin(), name)
		fmt.Printf("Removing %s %s %s\n", name, brightBlack("->"), blue(absTarget))
		if err := os.Remove(path); err != nil {
			rc.error("%s: %s", arg, err)
		}
	}
}

func (c *command) prune(args arguments) {
	c.validate(args, noPositional)
	for _, file := range c.files() {
		if file.IsDir() {
			continue
		}
		path := filepath.Join(c.bin(), file.Name())
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("Removing %s\n", file.Name())
			if err := os.Remove(path); err != nil {
				c.error("%s: %s", file.Name(), err)
			}
		} else if err != nil {
			c.fatal("%s: %s", file.Name(), err)
		}
	}
}

func (c *command) doctor(args arguments) {
	c.validate(args, noPositional)
	targetToName := make(map[string]string)
	for _, file := range c.files() {
		path := filepath.Join(c.bin(), file.Name())
		if file.IsDir() {
			c.error("%s: unexpected directory", path)
			continue
		}
		if file.Type().IsRegular() && strings.HasPrefix(file.Name(), ".") {
			continue
		}
		if file.Type()&os.ModeSymlink == 0 {
			c.error("%s: not a symlink", path)
			continue
		}
		if info, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			c.error("%s: broken symlink", path)
			continue
		} else if err != nil {
			c.error("%s", err)
			continue
		} else if !isExecutable(info.Mode()) {
			c.error("%s: not an executable", path)
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

func (c *command) validate(args arguments, opts validateOpts) {
	for flag := range args.flags {
		c.fatal("%s: %s: unrecognized flag", c.name, flag)
	}
	switch opts {
	case noPositional:
		if len(args.pos) > 0 {
			c.fatal("%s: %s: unexpected argument", c.name, args.pos[0])
		}
	case atLeastOnePositional:
		if len(args.pos) == 0 {
			c.fatal("%s: expected at least one argument", c.name)
		}
	}
	if c.failed {
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
