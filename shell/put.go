package shell

import (
	"errors"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/util"
)

func putCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "put",
		Help:      "copy a local document to cloud (use --force to overwrite, --content-only to replace PDF content)",
		Completer: createFsEntryCompleter(),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			// Parse flags
			var force, contentOnly bool
			var args []string
			for _, arg := range c.Args {
				if arg == "--force" {
					force = true
				} else if arg == "--content-only" {
					contentOnly = true
				} else {
					args = append(args, arg)
				}
			}

			// Validate flags are mutually exclusive
			if force && contentOnly {
				c.Err(errors.New("--force and --content-only cannot be used together"))
				return
			}

			if len(args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := args[0]

			// Handle --content-only mode (replace PDF content)
			if contentOnly {
				// Validate that source file is a PDF
				_, ext := util.DocPathToName(srcName)
				if ext != "pdf" {
					c.Err(errors.New("--content-only can only be used with PDF files"))
					return
				}

				docName, _ := util.DocPathToName(srcName)
				node := ctx.node
				var err error

				// Parse destination directory if provided
				if len(args) == 2 {
					node, err = ctx.api.Filetree().NodeByPath(args[1], ctx.node)
					if err != nil || node.IsFile() {
						c.Err(errors.New("directory doesn't exist"))
						return
					}
				}

				existingNode, err := ctx.api.Filetree().NodeByPath(docName, node)
				if err != nil {
					// Document doesn't exist, create new one
					c.Printf("uploading: [%s]...", srcName)
					dstDir := node.Id()
					document, err := ctx.api.UploadDocument(dstDir, srcName, true)
					if err != nil {
						c.Err(fmt.Errorf("failed to upload file [%s]: %v", srcName, err))
						return
					}
					c.Println("OK")
					ctx.api.Filetree().AddDocument(document)
					return
				}

				if existingNode.IsDirectory() {
					c.Err(errors.New("cannot replace directory with file"))
					return
				}

				c.Printf("replacing PDF content of [%s] with [%s]...", docName, srcName)
				if err := ctx.api.ReplaceDocumentFile(existingNode.Document.ID, srcName, true); err != nil {
					c.Err(fmt.Errorf("failed to replace content: %v", err))
					return
				}
				c.Println("OK")
				return
			}

			// Handle regular upload or --force mode
			docName, _ := util.DocPathToName(srcName)
			node := ctx.node
			var err error

			// Parse destination directory if provided
			if len(args) == 2 {
				node, err = ctx.api.Filetree().NodeByPath(args[1], ctx.node)
				if err != nil || node.IsFile() {
					c.Err(errors.New("directory doesn't exist"))
					return
				}
			}

			// Check if file exists and handle --force
			existingNode, err := ctx.api.Filetree().NodeByPath(docName, node)
			if err == nil {
				// File exists
				if !force {
					c.Err(errors.New("entry already exists (use --force to recreate, --content-only to replace content)"))
					return
				}
				// Use --force: completely replace document (delete old, upload new)
				if existingNode.IsDirectory() {
					c.Err(errors.New("cannot overwrite directory with file"))
					return
				}
				c.Printf("replacing: [%s]...", srcName)

				// Delete existing document
				if err := ctx.api.DeleteEntry(existingNode, false, false); err != nil {
					c.Err(fmt.Errorf("failed to delete existing file: %v", err))
					return
				}
				ctx.api.Filetree().DeleteNode(existingNode)

				// Upload new document
				dstDir := node.Id()
				document, err := ctx.api.UploadDocument(dstDir, srcName, true)
				if err != nil {
					c.Err(fmt.Errorf("failed to upload replacement file [%s]: %v", srcName, err))
					return
				}

				c.Println("OK")
				ctx.api.Filetree().AddDocument(document)
				return
			}

			// File doesn't exist, upload new document
			c.Printf("uploading: [%s]...", srcName)
			dstDir := node.Id()
			document, err := ctx.api.UploadDocument(dstDir, srcName, true)

			if err != nil {
				c.Err(fmt.Errorf("Failed to upload file [%s] %v", srcName, err))
				return
			}

			c.Println("OK")

			ctx.api.Filetree().AddDocument(document)
		},
	}
}
