# rMAPI

because the upstream project was archived, I'll keep maintaining it here

[![Actions Status](https://github.com/ddvk/rmapi/workflows/Go/badge.svg)](https://github.com/ddvk/rmapi/actions)


rMAPI is a Go app that allows you to access the ReMarkable Cloud API programmatically.

You can interact with the different API end-points through a shell. However, you can also
run commands non-interactively. This may come in handy to script certain workflows such as
taking automatic backups or uploading documents programmatically.


![Console Capture](docs/console.gif)

# Some examples of use

[Tutorial on how to directly print to your reMarkable on Mac with rMAPI](docs/tutorial-print-macosx.md)

# Install

## From sources

Install and build the project:

```
git clone https://github.com/ddvk/rmapi
cd rmapi
go install
```

## Binary

You can download an already built version for either Linux or OSX from [releases](https://github.com/ddvk/rmapi/releases/).

## Brew

```
brew install io41/tap/rmapi
```

## Docker

First clone this repository, then build a local container like

```
docker build -t rmapi .
```

create the .config/rmapi config folder

```
mkdir -p $HOME/.config/rmapi
``` 

and run by mounting the .config/rmapi folder

```
docker run -v $HOME/.config/rmapi/:/home/app/.config/rmapi/ -it rmapi
```

Issue non-interactive commands by appending to the `docker run` command:

```
docker run -v $HOME/.config/rmapi/:/home/app/.config/rmapi/ rmapi help
```

# API support

- [x] list files and directories
- [x] move around directories
- [x] download a specific file
- [x] download a directory and all its files and subdiretores recursively
- [x] create a directory
- [x] delete a file or a directory
- [x] move/rename a file or a directory
- [x] upload a specific file
- [ ] live syncs

# Annotations

- Initial support to generate a PDF with annotations.

# Shell ergonomics

- [x] autocomplete
- [x] globbing
- [x] upload a directory and all its files and subdirectories recursively

# Commands

Start the shell by running `rmapi`

## List current directory

Use `ls` to list the contents of the current directory. Entries are listed with `[d]` if they
are directories, and `[f]` if they are files.

## Change current directory

Use `cd` to change the current directory to any other directory in the hierarchy.

## Find a file

The command  `find` takes one or two arguments.

If only the first argument is passed, all entries from that point are printed recursively.

When the second argument is also passed, a regexp is expected, and only those entries that match the regexp are printed.

Golang standard regexps are used. For instance, to make the regexp case insensitve you can do:

```
find . (?i)foo
```

## Upload a file

Use `put path_to_local_file` to upload a file  to the current directory.

You can also specify the destination directory:

```
put book.pdf /books
```

### Upload flags

- `--force`: Completely replace an existing document (removes all annotations and metadata)
- `--content-only`: Replace only the PDF content while preserving annotations and metadata
- `--coverpage=<0|1>`: Set coverpage (0 to disable, 1 to set first page as cover)

Examples:

```bash
# Upload new file (fails if already exists)
put document.pdf

# Force overwrite existing document completely
put --force document.pdf

# Replace PDF content but keep annotations
put --content-only document.pdf

# Upload with coverpage set to first page
put --coverpage=1 document.pdf

# Replace PDF content in specific directory
put --content-only document.pdf /target-directory

# Upload to specific directory with force
put --force document.pdf /reports
```

**Note**: `--force` and `--content-only` are mutually exclusive. The `--coverpage` flag can be combined with either. If the target document doesn't exist, all flags will create a new document.

## Recursively upload directories and files

Use `mput path_to_dir` to recursively upload all the local files to that directory.

E.g: upload all the files

```
mput (-src sourcfolder) /Papers
```

![Console Capture](docs/mput-console.png)

## Download a file

Use `get path_to_file` to download a file from the cloud to your local computer.

## Recursively download directories and files

Use `mget path_to_dir` to recursively download all the files in that directory.

Chech further options with (mget -h)

E.g: download all the files

```
mget -o dstfolder /
```
Incremental mirror (deletes files not in the cloud so be careful with the output folder)

```
mget -o dstfolder -i -d /
```

## Download a file and generate a PDF with its annoations

Use `geta` to download a file and generate a PDF document
with its annotations.

Please note that its support is very basic for now and only supports one type of pen for now, but
there's work in progress to improve it.

## Create a directoy

Use `mkdir path_to_new_dir` to create a new directory

## Remove a directory or a file

Use `rm directory_or_file` to remove. If it's directory, it needs to be empty in order to be deleted.

You can remove multiple entries at the same time.

## Move/rename a directory or a file

Use `mv source destination` to move or rename a file or directory.

## Stat a directory or file

Use `stat entry` to dump its metadata as reported by the Cloud API.

# Run command non-interactively

Add the commands you want to execute to the arguments of the binary.

E.g: simple script to download all files from the cloud to your local machine

```bash
$ rmapi mget .
```

rMAPI will set the exit code to `0` if the command succeedes, or `1` if it fails.

# Environment variables

- `RMAPI_CONFIG`: filepath used to store authentication tokens. When not set, rmapi uses the file `.rmapi` in the home directory of the current user.
- `RMAPI_TRACE=1`: enable trace logging.
- `RMAPI_USE_HIDDEN_FILES=1`: use and traverse hidden files/directories (they are ignored by default).
- `RMAPI_THUMBNAILS`: generate a thumbnail of the first page of a pdf document
- `RMAPI_AUTH`: override the default authorization url
- `RMAPI_DOC`: override the default document storage url
- `RMAPI_HOST`: override all urls 
- `RMAPI_CONCURRENT`: sync15: maximum number of goroutines/http requests to use (default: 20)
