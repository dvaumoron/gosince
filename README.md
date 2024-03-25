# gosince

gosince shows the introducing version of a go package or symbol, has a flag to call `go doc` about it.

## Getting started

Install via [Homebrew](https://brew.sh/)

```console
$ brew tap dvaumoron/tap
$ brew install gosince
```

Or get the [last binary](https://github.com/dvaumoron/gosince/releases) depending on your OS.

```console
$ gosince SliceHeader
found reflect SliceHeader added in go1 and deprecated in go1.21
```

```console
$ gosince -h
gosince shows the introducing version of a go package or symbol, find more details at : https://github.com/dvaumoron/gosince

Usage of gosince:
gosince <pkg>
gosince <sym>
gosince <pkg>.<sym>[.<methodOrField>]
gosince <pkg> <sym>[.<methodOrField>]

Usage:
  gosince expr1 [expr2] [flags]

Flags:
  -p, --cache-path string    Local path to cache the retrieved api information (default "/home/dvaumoron/.gosince")
  -d, --go-doc               Call go doc command
  -h, --help                 help for gosince
  -a, --source-addr string   Location of Go source (default "https://raw.githubusercontent.com/golang/go/master")
  -v, --verbose              Verbose output
      --version              version for gosince
```

## Environment Variables

### GOSINCE_CACHE_PATH

String (Default: ${HOME}/.gosince)

The path to a directory where **gosince** cache locally api informations.

### GOSINCE_SOURCE_URL

String (Default: ${https://raw.githubusercontent.com/golang/go/master})

URL to download Go source (**gosince** rely on `api/go1*.txt` files)
