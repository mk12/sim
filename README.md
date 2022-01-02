# Sim

Sim is a command-line tool that manages programs in `$XDG_BIN_HOME`.

It's called "Sim" because it makes **sym**links by default.

## Get started

Run `make install`. Make sure your `PATH` contains `$XDG_BIN_HOME` or `~/.local/bin`.

If you no longer wish to use Sim, run `make uninstall`.

## Usage

```
Usage: sim [COMMAND] [OPTION ...]

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
```

## License

© 2022 Mitchell Kember

Sim is available under the MIT License; see [LICENSE](LICENSE.md) for details.
