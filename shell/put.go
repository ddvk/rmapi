package shell

import (
	"errors"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/util"
)

func updateCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "update",
		Help:      "update/overwrite an existing document in cloud",
		Completer: createFsEntryCompleter(),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := c.Args[0]
			node := ctx.node
			var err error

			if len(c.Args) == 2 {
				node, err = ctx.api.Filetree().NodeByPath(c.Args[1], ctx.node)
				if err != nil || node.IsFile() {
					c.Err(errors.New("directory doesn't exist"))
					return
				}
			}

			c.Printf("updating: [%s]...", srcName)
			dstDir := node.Id()
			document, err := ctx.api.UploadDocument(dstDir, srcName, true, nil)
			if err != nil {
				c.Err(fmt.Errorf("Failed to update file [%s] %v", srcName, err))
				return
			}

			c.Println("OK")
			ctx.api.Filetree().AddDocument(document)
		},
	}
}

func putCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "put",
		Help:      "copy a local document to cloud",
		Completer: createFsEntryCompleter(),
		LongHelp: `Usage: put [options] <local_file> [remote_directory]

Options:
  --coverpage <0|1>    Set coverpage (0 to disable, 1 to set first page as cover)`,
		Func: func(c *ishell.Context) {
			args := c.Args
			var coverpageFlag *int

			// Parse flags
			for i := 0; i < len(args); i++ {
				if args[i] == "--coverpage" {
					if i+1 >= len(args) {
						c.Err(errors.New("--coverpage requires a value (0 or 1)"))
						return
					}
					switch args[i+1] {
					case "0":
						// Don't set coverpage
					case "1":
						val := 0 // First page is 0 in the document metadata
						coverpageFlag = &val
					default:
						c.Err(errors.New("--coverpage value must be 0 or 1"))
						return
					}
					// Remove flag and value from args
					args = append(args[:i], args[i+2:]...)
					i-- // Adjust index after removal
				}
			}

			if len(args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := args[0]
			docName, _ := util.DocPathToName(srcName)

			node := ctx.node
			var err error

			if len(args) == 2 {
				node, err = ctx.api.Filetree().NodeByPath(args[1], ctx.node)

				if err != nil || node.IsFile() {
					c.Err(errors.New("directory doesn't exist"))
					return
				}
			}

			_, err = ctx.api.Filetree().NodeByPath(docName, node)
			//TODO: force flag and overwrite
			if err == nil {
				c.Err(errors.New("entry already exists"))
				return
			}

			c.Printf("uploading: [%s]...", srcName)

			dstDir := node.Id()

			document, err := ctx.api.UploadDocument(dstDir, srcName, true, coverpageFlag)

			if err != nil {
				c.Err(fmt.Errorf("Failed to upload file [%s] %v", srcName, err))
				return
			}

			c.Println("OK")

			ctx.api.Filetree().AddDocument(document)
		},
	}
}
