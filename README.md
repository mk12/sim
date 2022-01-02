# Sim

Sim is a command-line tool that manages programs in `$XDG_BIN_HOME`.

It's called "Sim" because it makes **sym**links by default.

## Get started

Run `make install`. Make sure your `PATH` contains `$XDG_BIN_HOME` or `~/.local/bin`.

If you no longer wish to use Sim, run `make uninstall`.

## Usage

`sim help`:

```
Usage: sim [-h] COMMAND

Manage programs in $XDG_BIN_HOME.

Commands:
  help        Show this help message
  path        Show install path
  i, install  Install programs
  ls, list    List programs
  rm, remove  Remove programs
  prune       Remove broken symlinks
  doctor      Check for issues
```

`sim help install`:

```
Usage: sim install [-hfcmn] [-r NAME] PROGRAM ...

Install each PROGRAM in $XDG_BIN_HOME.

Options:
  -h, --help         Show this help message
  -f, --force        Overwrite existing programs
  -c, --copy         Copy instead of symlinking
  -m, --move         Move instead of symlinking
  -n, --no-ext       Remove file extensions
  -r, --rename NAME  Rename single PROGRAM to NAME
```

`sim help list`:

```
Usage: sim list [-hpldtq] [PROGRAM ...]

List each matching PROGRAM in $XDG_BIN_HOME.
PROGRAM can be a basename, a full path, or a symlink target path.

Options:
  -h, --help    Show this help message
  -p, --path    Print full paths to programs
  -l, --long    Print symlink targets
  -d, --direct  Do not match on symlink targets
  -t, --target  Only match on symlink targets
  -q, --quiet   Ignore patterns that match nothing
```

`sim help remove`:

```
Usage: sim remove [-hdtq] PROGRAM ...

Remove each matching PROGRAM in $XDG_BIN_HOME.
PROGRAM can be a basename, a full path, or a symlink target path.

Options:
  -h, --help    Show this help message
  -d, --direct  Do not match on symlink targets
  -t, --target  Only match on symlink targets
  -q, --quiet   Ignore patterns that match nothing
```

## License

© 2022 Mitchell Kember

Sim is available under the MIT License; see [LICENSE](LICENSE.md) for details.
