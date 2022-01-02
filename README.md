# Sim

Sim is a command-line tool that manages program symlinks in `$XDG_BIN_HOME`.

## Get started

Run `make install`. Make sure your `PATH` contains `$XDG_BIN_HOME` or `~/.local/bin`.

If you no longer wish to use Sim, run `make uninstall`.

## Usage

```
Usage: sim [COMMAND] [OPTION ...]

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
```

## License

© 2022 Mitchell Kember

Sim is available under the MIT License; see [LICENSE](LICENSE.md) for details.
