package shell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/hwr"
	"github.com/juruen/rmapi/util"
)

func hwrCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "hwr",
		Help:      "download remote file and perform handwriting recognition",
		Completer: createEntryCompleter(ctx),
		LongHelp: `Usage: hwr [options] <remote_file>

Options:
  --type=<Text|Math|Diagram>  Content type (default: Text)
  --lang=<lang>               Language code (default: en_US)
  --page=<N>                  Page number (default: all pages, use 0 for last opened)
  --split                     Output each page to a separate .txt file`,
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			// Parse options
			inputType := "Text"
			lang := "en_US"
			page := -1
			splitPages := false

			args := c.Args
			for i, arg := range args {
				if arg == "--split" {
					splitPages = true
					args = append(args[:i], args[i+1:]...)
					break
				}
				if len(arg) > 7 && arg[:7] == "--type=" {
					inputType = arg[7:]
					args = append(args[:i], args[i+1:]...)
					break
				}
				if len(arg) > 6 && arg[:6] == "--lang=" {
					lang = arg[6:]
					args = append(args[:i], args[i+1:]...)
					break
				}
				if len(arg) > 7 && arg[:7] == "--page=" {
					fmt.Sscanf(arg[7:], "%d", &page)
					args = append(args[:i], args[i+1:]...)
					break
				}
			}

			if len(args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := args[0]

			node, err := ctx.Api.Filetree().NodeByPath(srcName, ctx.Node)
			if err != nil || node.IsDirectory() {
				c.Err(errors.New("file doesn't exist"))
				return
			}

			// Check for API credentials
			applicationKey := os.Getenv("RMAPI_HWR_APPLICATIONKEY")
			if applicationKey == "" {
				c.Err(errors.New("RMAPI_HWR_APPLICATIONKEY environment variable is required"))
				return
			}
			hmacKey := os.Getenv("RMAPI_HWR_HMAC")
			if hmacKey == "" {
				c.Err(errors.New("RMAPI_HWR_HMAC environment variable is required"))
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
				c.Err(errors.New(fmt.Sprintf("Failed to download file %s: %s", srcName, err.Error())))
				return
			}

			// Clean up temp file after processing
			defer os.Remove(rmdocPath)

			c.Println("Download OK")
			c.Println(fmt.Sprintf("performing handwriting recognition: [%s]...", srcName))

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

			file.Seek(0, 0)
			zipArchive, err := LoadArchive(file, fileInfo.Size())
			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to read archive %s: %s", rmdocPath, err.Error())))
				return
			}

			// Get base name without extension for output files
			baseNameWithoutExt := node.Name()
			outputDir := "."
			outputFile := filepath.Join(outputDir, baseNameWithoutExt)

			cfg := hwr.Config{
				Page:       page,
				Lang:       lang,
				InputType:  inputType,
				OutputFile: outputFile,
				SplitPages: splitPages,
				BatchSize:  3,
			}

			outputFiles, err := hwr.Hwr(zipArchive, cfg, applicationKey, hmacKey)
			if err != nil {
				c.Err(fmt.Errorf("HWR failed: %s", err.Error()))
				return
			}

			if len(outputFiles) > 0 {
				c.Printf("Recognition complete. Output file(s):\n")
				for _, f := range outputFiles {
					c.Printf("  %s\n", f)
				}
			} else {
				c.Err(errors.New("No output files were created"))
			}
		},
	}
}

