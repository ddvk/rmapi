# rmapi REST API Server Documentation

This document describes all available endpoints in the rmapi HTTP server.

## Base URL

All endpoints are prefixed with `/api/` unless otherwise noted.

## Authentication

Most endpoints require authentication. Authenticate first using `/api/auth`, then subsequent requests will use the stored authentication tokens.

---

## Endpoints

### Authentication

#### `GET /api/auth` or `POST /api/auth`
Authenticate with a one-time code from reMarkable.

**Query Parameters (GET):**
- `code` (string, required): 8-digit one-time code from https://my.remarkable.com/device/browser/connect

**Request Body (POST):**
```json
{
  "code": "12345678"
}
```

**Response:**
```json
{
  "message": "Authentication successful",
  "user": "user@example.com"
}
```

**Example:**
```bash
# GET method
curl "http://localhost:8080/api/auth?code=12345678"

# POST method
curl -X POST http://localhost:8080/api/auth \
  -H "Content-Type: application/json" \
  -d '{"code": "12345678"}'
```

---

#### `GET /api/auth/status`
Check authentication status.

**Response:**
```json
{
  "authenticated": true,
  "user": "user@example.com"
}
```

**Example:**
```bash
curl http://localhost:8080/api/auth/status
```

---

### File System Operations

#### `GET /api/ls`
List directory contents.

**Query Parameters:**
- `path` (string, optional): Directory path (defaults to current directory)
- `compact` (bool, optional): Compact output format
- `long` (bool, optional): Long format with details
- `reverse` (bool, optional): Reverse sort order
- `dirFirst` (bool, optional): Show directories first
- `byTime` (bool, optional): Sort by modification time
- `showTemplates` (bool, optional): Include templates

**Response:**
```json
[
  {
    "id": "...",
    "name": "document.pdf",
    "type": "DocumentType",
    "parent": "...",
    ...
  }
]
```

**Example:**
```bash
curl "http://localhost:8080/api/ls?path=/books&long=true"
```

---

#### `GET /api/pwd`
Get current working directory.

**Response:**
```json
{
  "path": "/books"
}
```

**Example:**
```bash
curl http://localhost:8080/api/pwd
```

---

#### `POST /api/cd`
Change current directory.

**Request Body:**
```json
{
  "path": "/books"
}
```

**Response:**
```json
{
  "path": "/books"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/cd \
  -H "Content-Type: application/json" \
  -d '{"path": "/books"}'
```

---

#### `GET /api/get`
Download a file from reMarkable. Returns the binary file content (streams the `.rmdoc` file).

**Query Parameters:**
- `path` (string, required): File path to download

**Response:**
- Content-Type: `application/zip`
- Content-Disposition: `attachment; filename="<filename>.rmdoc"`
- Body: Binary file content (the `.rmdoc` ZIP archive)

**Example:**
```bash
# Download and save to file
curl "http://localhost:8080/api/get?path=/books/document.pdf" -o document.rmdoc

# Stream to stdout
curl "http://localhost:8080/api/get?path=/books/document.pdf"
```

---

#### `POST /api/put`
Upload a file to reMarkable.

**Supports two formats:**

**1. Multipart/form-data:**
- `file` (file, required): File to upload
- `destination` (string, optional): Target directory (defaults to current directory)
- `force` (string, optional): Set to `"true"` to overwrite existing documents
- `contentOnly` (string, optional): Set to `"true"` to replace PDF content while preserving annotations (PDF only)
- `coverpage` (string, optional): Set to `"1"` to use first page as cover

**2. Raw Binary:**
- Request body: Binary file data
- Headers:
  - `Content-Type`: File MIME type (e.g., `application/pdf`)
  - `X-Filename` (required): Filename for the uploaded file
  - `X-Destination` (optional): Target directory
  - `X-Force` (optional): Set to `"true"` to overwrite
  - `X-Content-Only` (optional): Set to `"true"` for content-only replacement
  - `X-Coverpage` (optional): Set to `"1"` for coverpage

**Query Parameters (for raw binary):**
- `destination` (string, optional): Target directory
- `force` (string, optional): Set to `"true"` to overwrite
- `contentOnly` (string, optional): Set to `"true"` for content-only replacement
- `coverpage` (string, optional): Set to `"1"` for coverpage

**Response:**
```json
{
  "message": "File uploaded",
  "node": {
    "id": "...",
    "name": "document",
    ...
  }
}
```

**Examples:**
```bash
# Multipart/form-data
curl -X POST http://localhost:8080/api/put \
  -F "file=@document.pdf" \
  -F "destination=/books" \
  -F "force=true"

# Raw binary with query parameters
curl -X POST "http://localhost:8080/api/put?destination=/books&force=true" \
  -H "Content-Type: application/pdf" \
  -H "X-Filename: document.pdf" \
  --data-binary "@document.pdf"

# Raw binary with headers
curl -X POST http://localhost:8080/api/put \
  -H "Content-Type: application/pdf" \
  -H "X-Filename: document.pdf" \
  -H "X-Destination: /books" \
  -H "X-Force: true" \
  --data-binary "@document.pdf"
```

**Notes:**
- Maximum file size: 32 MB
- `force` and `contentOnly` cannot be used together
- `contentOnly` only works with PDF files
- The document name on reMarkable will be the filename without extension

---

#### `POST /api/mkdir`
Create a directory.

**Query Parameters:**
- `path` (string, required): Directory path to create
- `recursive` (string, optional): Set to `"true"` or `"1"` to create parent directories if they don't exist (like `mkdir -p`)

**Request Body (alternative):**
```json
{
  "path": "/books/2024",
  "recursive": true
}
```

**Response:**
```json
{
  "message": "Directory created",
  "node": {
    "id": "...",
    "name": "2024",
    ...
  }
}
```

**Examples:**
```bash
# Simple directory creation
curl -X POST "http://localhost:8080/api/mkdir?path=/books"

# Recursive directory creation (creates parent directories)
curl -X POST "http://localhost:8080/api/mkdir?path=/books/2024/january&recursive=true"

# Using JSON body
curl -X POST http://localhost:8080/api/mkdir \
  -H "Content-Type: application/json" \
  -d '{"path": "/books/2024", "recursive": true}'
```

**Notes:**
- If directory already exists, returns success (idempotent)
- Without `recursive=true`, fails if parent directories don't exist
- With `recursive=true`, creates all missing parent directories

---

#### `DELETE /api/rm`
Delete a file or directory.

**Query Parameters:**
- `path` (string, required): Path to file or directory to delete
- `recursive` (string, optional): Set to `"true"` to delete directories recursively

**Response:**
```json
{
  "message": "Entries deleted",
  "deleted": ["document.pdf", "folder"]
}
```

**Example:**
```bash
curl -X DELETE "http://localhost:8080/api/rm?path=/books/document.pdf"

# Delete directory recursively
curl -X DELETE "http://localhost:8080/api/rm?path=/books/old&recursive=true"
```

---

#### `POST /api/mv`
Move or rename a file or directory.

**Request Body:**
```json
{
  "source": "/books/document.pdf",
  "destination": "/archive/document.pdf"
}
```

**Response:**
```json
{
  "message": "Entry moved",
  "moved": ["document.pdf"]
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/mv \
  -H "Content-Type: application/json" \
  -d '{
    "source": "/books/document.pdf",
    "destination": "/archive/document.pdf"
  }'
```

**Notes:**
- If destination is a directory, moves source into that directory
- If destination is a path, renames/moves to that location
- Can move multiple files if source matches multiple entries

---

### File Information

#### `GET /api/stat`
Get file or directory metadata.

**Query Parameters:**
- `path` (string, required): Path to file or directory

**Response:**
```json
{
  "ID": "...",
  "Version": 1,
  "Message": "...",
  "Success": true,
  "BlobURLGet": "...",
  "BlobURLGetExpires": "...",
  "ModifiedClient": "...",
  "Type": "DocumentType",
  "VissibleName": "document",
  "CurrentPage": 0,
  "Bookmarked": false,
  "Parent": "...",
  ...
}
```

**Example:**
```bash
curl "http://localhost:8080/api/stat?path=/books/document.pdf"
```

---

#### `GET /api/find`
Search for files and directories.

**Query Parameters:**
- `path` (string, optional): Start directory for search (defaults to current directory)
- `pattern` (string, optional): Regular expression to match against file/directory names
- `compact` (bool, optional): Compact output format
- `starred` (bool, optional): Filter by starred status (`true` = starred, `false` = not starred)
- `tags` (string, optional): Comma-separated list of tags (OR semantics - matches if file has any of the tags)

**Response:**
```json
[
  {
    "id": "...",
    "name": "document.pdf",
    ...
  }
]
```

**Examples:**
```bash
# Find files matching pattern
curl "http://localhost:8080/api/find?path=/books&pattern=.*2024.*"

# Find starred files
curl "http://localhost:8080/api/find?starred=true"

# Find files with specific tags
curl "http://localhost:8080/api/find?tags=work,important"

# Combine filters
curl "http://localhost:8080/api/find?pattern=.*report.*&starred=true&tags=work"
```

---

### Conversion & Processing

#### `GET /api/convert`
Convert a reMarkable document to PNG images.

**Query Parameters:**
- `path` (string, required): Path to file to convert
- `inline` (bool, optional): Set to `"true"` to return PNG data inline instead of downloading

**Response:**
- If `inline=false`: Downloads PNG files
- If `inline=true`: Returns JSON with base64-encoded PNG data

**Example:**
```bash
curl "http://localhost:8080/api/convert?path=/books/document.pdf&inline=true"
```

---

#### `GET /api/hwr`
Perform handwriting recognition on a reMarkable document.

**Query Parameters:**
- `path` (string, required): Path to file to process
- `type` (string, optional): Input type - `Text`, `Math`, or `Diagram` (default: `Text`)
- `lang` (string, optional): Language code (default: `en_US`)
- `page` (int, optional): Specific page number to process (default: all pages)
- `split` (bool, optional): Set to `"true"` to split pages into separate files
- `inline` (bool, optional): Set to `"true"` to return ZIP file with TXT files inline

**Response:**
- If `inline=false`: Downloads ZIP file with TXT files
- If `inline=true`: Returns ZIP file data inline

**Example:**
```bash
curl "http://localhost:8080/api/hwr?path=/books/notes.pdf&type=Text&lang=en_US&inline=true"
```

**Requirements:**
- Requires `RMAPI_HWR_APPLICATIONKEY` and `RMAPI_HWR_HMAC` environment variables

---

### Account & Sync

#### `GET /api/account`
Get account information.

**Response:**
```json
{
  "user": "user@example.com",
  "syncVersion": 5
}
```

**Example:**
```bash
curl http://localhost:8080/api/account
```

---

#### `POST /api/refresh`
Refresh file tree and save diff snapshot.

**Response:**
```json
{
  "rootHash": "...",
  "generation": 123,
  "currentPath": "/books",
  "message": "Tree refreshed and diff snapshot saved"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/refresh
```

---

#### `POST /api/refresh-token`
Refresh authentication token only.

**Response:**
```json
{
  "message": "Token refreshed successfully",
  "user": "user@example.com"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/refresh-token
```

---

#### `POST /api/refresh-tree`
Refresh file tree without saving diff snapshot.

**Response:**
```json
{
  "rootHash": "...",
  "generation": 123,
  "currentPath": "/books",
  "message": "Tree refreshed (diff snapshot not updated)"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/refresh-tree
```

---

#### `GET /api/difference`
Get diff of changes since last snapshot.

**Response:**
```json
{
  "added": [...],
  "deleted": [...],
  "modified": [...]
}
```

**Example:**
```bash
curl http://localhost:8080/api/difference
```

---

### System

#### `GET /api/version`
Get server version.

**Response:**
```json
{
  "version": "0.0.27"
}
```

**Example:**
```bash
curl http://localhost:8080/api/version
```

---

#### `GET /health`
Health check endpoint (no authentication required).

**Response:**
```
OK
```

**Example:**
```bash
curl http://localhost:8080/health
```

---

#### `GET /`
API documentation page (no authentication required).

Returns HTML page with API documentation.

---

## Response Format

### Success Response
```json
{
  "message": "Operation successful",
  "data": { ... }
}
```

### Error Response
```json
{
  "error": "Error message"
}
```

## HTTP Status Codes

- `200 OK`: Success
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Authentication required
- `404 Not Found`: Resource not found
- `409 Conflict`: Resource already exists or conflict
- `500 Internal Server Error`: Server error

## Notes

- All paths use forward slashes (`/`) as separators
- Root directory is `/`
- Most endpoints require authentication (except `/api/auth`, `/api/auth/status`, `/health`, `/`)
- Boolean query parameters accept `"true"` or `"1"` as truthy values
- File uploads support both multipart/form-data and raw binary formats
- Maximum upload size is 32 MB
