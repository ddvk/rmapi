package shell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/util"
	"github.com/juruen/rmapi/visualize"
)

func convertCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "convert",
		Help:      "download remote file and convert to PNG",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := c.Args[0]

			node, err := ctx.Api.Filetree().NodeByPath(srcName, ctx.Node)

			if err != nil || node.IsDirectory() {
				c.Err(errors.New("file doesn't exist"))
				return
			}

			c.Println(fmt.Sprintf("downloading: [%s]...", srcName))

			// Download the file to a temporary location
			tmpFile, err := os.CreateTemp("", fmt.Sprintf("rmapi-*.%s", util.RMDOC))
			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to create temp file: %s", err.Error())))
				return
			}
			tmpFile.Close()
			rmdocPath := tmpFile.Name()

			err = ctx.Api.FetchDocument(node.Document.ID, rmdocPath)

			if err != nil {
				os.Remove(rmdocPath)
				c.Err(errors.New(fmt.Sprintf("Failed to download file %s with %s", srcName, err.Error())))
				return
			}

			// Clean up temp file after conversion
			defer os.Remove(rmdocPath)

			c.Println("Download OK")
			c.Println(fmt.Sprintf("converting to PNG: [%s]...", srcName))

			// Load the archive
			file, err := os.Open(rmdocPath)
			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to open file %s: %s", rmdocPath, err.Error())))
				return
			}
			defer file.Close()

			fileInfo, err := file.Stat()
			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to stat file %s: %s", rmdocPath, err.Error())))
				return
			}

			// Ensure file is at the beginning
			_, err = file.Seek(0, 0)
			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to seek file %s: %s", rmdocPath, err.Error())))
				return
			}

			// Load the archive (tries standard format first, falls back to new format)
			zipArchive, err := LoadArchive(file, fileInfo.Size())
			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to read archive %s: %s", rmdocPath, err.Error())))
				return
			}

			// Get base name without extension for output files
			// Use the remote file name, not the temp file name
			baseNameWithoutExt := node.Name()
			// Save PNG files in the current directory (same as 'get' command)
			outputDir := "."

			// Convert each page to PNG
			convertedCount := 0
			for i := 0; i < len(zipArchive.Pages); i++ {
				outputPNG := filepath.Join(outputDir, fmt.Sprintf("%s_page_%d.png", baseNameWithoutExt, i))
				c.Printf("  Converting page %d to %s...", i, outputPNG)

				err := visualize.VisualizePage(zipArchive, i, outputPNG)
				if err != nil {
					c.Err(fmt.Errorf("Failed to convert page %d: %s", i, err.Error()))
					continue
				}

				c.Println(" OK")
				convertedCount++
			}

			if convertedCount > 0 {
				c.Printf("Converted %d page(s) to PNG\n", convertedCount)
			} else {
				c.Err(errors.New("No pages were converted"))
			}
		},
	}
}

