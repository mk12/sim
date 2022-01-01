# Sim

Sim is a command-line tool that manages program symlinks in `$XDG_BIN_HOME`.

## Get started

Run `make install`. Make sure your `PATH` contains `$XDG_BIN_HOME` or `~/.local/bin`.

If you no longer wish to use Sim, run `make uninstall`.

## Usage

- `sim help` prints a help message.
- `sim list` lists installed programs.
    - With `--path`, it prints the full path.
    - With `--link`, it prints symlink target paths.
- `sim install foo.sh` installs `foo`.
    - With more arguments it installs multiple programs.
    - With `--force`, it overwrites an existing symlink.
        - This is only needed if the new target path is different.
    - With `--keep-ext`, it names the symlink `foo.sh`.
- `sim remove foo` removes `foo`.
    - `foo` can be a link name, relative to `$XDG_BIN_HOME`.
    - ... or a link path, relative to the working directory.
    - ... or a target path, relative to the working directory.
    - With more arguments it removes multiple programs.
    - With `--all`, it removes all programs except `sim` itself.
    - With `--self`, it removes `sim` itself.
- `sim prune` removes broken symlinks.
- `sim doctor` checks for issues.

## License

© 2022 Mitchell Kember

Sim is available under the MIT License; see [LICENSE](LICENSE.md) for details.
