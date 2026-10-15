# customls

A tiny `ls` replacement I wrote in Go because I got tired of my
terminal output looking the same everywhere. It colorizes directories
and lets you pick your own colors for individual files or folders, then
remembers them in `~/.config/customls/colors.json`.

I alias it to `ls` in my shell so it just happens by default. Here's
what a project folder looks like once a few things have colors:

```
$ customls
build       docs        go.mod      main.go     notes       README.md   src
```

(`notes` is pink, `src` is purple, the other dirs are the default cyan,
files are plain.)

```
$ customls -l
drwxr-xr-x     96  Oct 14 21:03  build
drwxr-xr-x    224  Oct 12 09:41  docs
-rw-r--r--     52  Oct  9 18:20  go.mod
-rw-r--r--  12.3K  Oct 14 21:02  main.go
drwxr-xr-x    128  Oct 13 07:15  notes
-rw-r--r--   2.1K  Oct 14 21:03  README.md
drwxr-xr-x    160  Oct 14 20:58  src
```

## Install

With Go:

```bash
go install github.com/JadenB9/customls@latest
```

Or build it yourself:

```bash
git clone https://github.com/JadenB9/customls.git
cd customls
go build -o customls .
cp customls ~/.local/bin/
```

Then in `~/.zshrc`:

```bash
alias ls='customls'
```

## Usage

```bash
customls              # list the current directory
customls -a           # include hidden files
customls -l           # long format (perm, size, mtime)
customls -1           # one entry per line
customls -la ~        # flags combine, path can go anywhere
customls set          # interactive: pick items and assign colors
customls colors       # print the color table you've built up
customls order        # toggle "group by color" sort on/off
customls --version
customls help
```

The `set` command pops up a numbered list of things in the current
directory, you pick one, then pick a color. Pick "remove color" to
clear it. Everything gets saved automatically.

When the output isn't a terminal (`customls | grep foo`, `$(customls)`
in a script) you get one plain name per line with no escape codes, the
same as `ls`. Setting `NO_COLOR` turns color off everywhere.

## Colors

Available names: `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`,
`white`, `orange`, `pink`, `purple`. Directories without a custom
assignment fall back to cyan.

## Layout details

- **Short form** is a column-major grid: it fills top-to-bottom first
  then moves to the next column, so items stay alphabetically ordered
  when you read down. Columns are padded by display width, so accented
  and CJK names don't throw the grid off.
- **Long form** shows permissions, a human-readable size, and the
  modification time, similar to `ls -lh`.
- **Color ordering** groups same-colored items together instead of
  sorting purely alphabetically. Toggle it with `customls order`.

## Config

The config file lives at `~/.config/customls/colors.json` and looks
like:

```json
{
  "colors": {
    "projects": "purple",
    "notes": "pink"
  },
  "order_by_color": false
}
```

You can edit it directly if you want. If it doesn't parse, customls
says so on stderr and carries on with no colors rather than failing,
and `customls colors` points out any entry that names a color that
doesn't exist.

## Building

Plain Go, no dependencies outside the standard library.

```bash
go build ./...
go test ./...
```

Releases are built by GoReleaser from a `v*` tag (see
`.goreleaser.yaml`), which also pushes a formula to
[JadenB9/homebrew-tap](https://github.com/JadenB9/homebrew-tap).
