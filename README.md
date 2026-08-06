# customls

A tiny `ls` replacement I wrote in Go because I got tired of my
terminal output looking the same everywhere. It colorizes directories
and lets you pick your own colors for individual files or folders, then
remembers them in `~/.config/customls/colors.json`.

I alias it to `ls` in my shell so it just happens by default.

## Install

```bash
go build -o customls .
cp customls ~/.local/bin/
```

Or wherever you keep your user binaries.

## Usage

```bash
customls              # list the current directory
customls -a           # include hidden files
customls -l           # long format (perm, size, mtime)
customls -al          # long format including hidden files
customls /some/path   # list another directory
customls set          # interactive: pick items and assign colors
customls colors       # print the color table you've built up
customls order        # toggle "group by color" sort on/off
customls help
```

The `set` command pops up a numbered list of things in the current
directory, you pick one, then pick a color. Pick "remove color" to
clear it. Everything gets saved automatically.

## Colors

Available names: `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`,
`white`, `orange`, `pink`, `purple`. Directories without a custom
assignment fall back to cyan.

## Layout details

- **Short form** is a column-major grid: it fills top-to-bottom first
  then moves to the next column, so items stay alphabetically ordered
  when you read down.
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

You can edit it directly if you want.
