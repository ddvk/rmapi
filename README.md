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

Alternatively, pass the `--json` flag and receive output in JSON.

```typescript
interface Node {
  id: string; // empty string for root node
  name: string;
  // TemplateType are downloaded reMarkable methods
  // CollectionType refers to directories or the root node
  // DocumentType is any PDF-document, Ebook or notebook    
  type: "CollectionType" | "DocumentType" | "TemplateType";
  version: number;         // Only relevant for type=DocumentType
  modifiedClient: string;  // RFC3339Nano timestamp, empty for root
  currentPage: number;     // 0-indexed, only meaningful for type=DocumentType
  parent: string;          // parent ID, empty string for root children
  tags: string[];          // A list of tags. Note, this does -not- include per-page tags. Also works for directories
  starred: boolean;        // Whether this item is starred or not. Also works for directories.
}

type LsOutput = Node[];
```

## Change current directory

Use `cd` to change the current directory to any other directory in the hierarchy.

## Find a file


The find command can be used to search through all of your reMarkable files recursively. 
The first argument is the directory, and the second argument is your search query.
The search query is optional, when left out, the command will list files recursively.

- Flags:
  - `--tag=<string>` only show files that include this tag. You can supply multiple tag parameters
  - `--starred` if supplied, only show starred files
  - `--json` show output in JSON, see the `ls` documentation for more detail

### Find examples

```bash
# Find all starred files
find --starred

# Find starred files in the root directory
find --starred /

# Find files with the "read-later" tag in the current directory or below (recursively)
find --tag="read-later" .

# Searching is performed using standard Go regular expressions
# Find files using a regular expression, for example when you have a particular format for diary files
find / "Diary-.*"
# For example, if you date your journals like this "Journal-DD-MM-YYYY", you can search for all journals in 2024 using
find / "Journal-..-..-2024"
# Or just search for all files with 2024 in the filename
find / ".*2024.*"
# If you want to search ignore character casing, you can do that as follows:
find / "(?i)case_insensitive_search"

# Find files with either "Work" or "Personal" tag
find --tag="Work" --tag="Personal"

# Find starred files with a specific tag
find --starred --tag="Important"

# Combine with regexp search
find --tag="Projects/2024" . ".*report.*"

# Tags can contain special characters like /, \, ", etc.
# Just quote them as you would any shell argument
find --tag="Work/Projects" --tag="tag,with,comma"
# This tag contains a double quote, you can escape it using \"
find --tag="tag-\"with-double-quote"
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
- `RMAPI_FORCE_SCHEMA_VERSION`: force a specific schema version (3 or 4) for the root index, overriding server detection
