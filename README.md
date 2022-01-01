# Simmer

Simmer is a command-line tool that manages program symlinks in `$XDG_BIN_HOME` (or `~/.local/bin` by default).

## Get started

Run `make install` and ensure `$XDG_BIN_HOME` is in your `PATH`.

## Usage

- `sim help` prints a help message.
- `sim list` lists installed programs.
    - With `--path`, it prints the full path.
    - With `--link`, it prints symlink target paths.
- `sim install foo.sh` installs `foo`.
    - With `--force`, it overwrites an existing symlink.
        - This is only needed if the new target path is different.
    - With `--keep-extension`, it names the symlink `foo.sh`.
    - With more arguments it installs multiple programs.
- `sim remove foo` removes `foo`.
    - `foo` can be a link name, relative to `$XDG_BIN_HOME`.
    - `foo` can be a target path, relative to the working directory.
    - With more arguments it removes multiple programs.
- `sim prune` removes broken symlinks.
- `sim doctor` checks for issues.

## License

© 2022 Mitchell Kember

Simmer is available under the MIT License; see [LICENSE](LICENSE.md) for details.
