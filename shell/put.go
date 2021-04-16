package shell

import (
	"errors"
	"flag"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/api"
	"github.com/juruen/rmapi/util"
)

func putCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "put",
		Help:      "copy a local document to cloud",
		Completer: createFsEntryCompleter(),
		Func: func(c *ishell.Context) {
			flagSet := flag.NewFlagSet("put", flag.ContinueOnError)
			landscape := flagSet.Bool("l", false, "landscape orientation")
			currentPage := flagSet.Int("c", 0, "current page")

			if err := flagSet.Parse(c.Args); err != nil {
				if err != flag.ErrHelp {
					c.Err(err)
				}
				return
			}

			argRest := flagSet.Args()
			if len(argRest) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := argRest[0]

			docName, _ := util.DocPathToName(srcName)

			node := ctx.node
			var err error

			if len(argRest) == 2 {
				node, err = ctx.api.Filetree.NodeByPath(argRest[1], ctx.node)

				if err != nil || node.IsFile() {
					c.Err(errors.New("directory doesn't exist"))
					return
				}
			}

			_, err = ctx.api.Filetree.NodeByPath(docName, node)
			if err == nil {
				c.Err(errors.New("entry already exists"))
				return
			}

			c.Printf("uploading: [%s]...", srcName)

			dstDir := node.Id()

			options := api.DocumentOptions{
				Landscape:   *landscape,
				CurrentPage: *currentPage,
			}
			document, err := ctx.api.UploadDocument(dstDir, srcName, options)

			if err != nil {
				c.Err(fmt.Errorf("Failed to upload file [%s] %v", srcName, err))
				return
			}

			c.Println("OK")

			ctx.api.Filetree.AddDocument(*document)
		},
	}
}
