package shell

import (
	"errors"
	"flag"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/util"
)

func putCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "put",
		Help:      "copy a local document to cloud",
		Completer: createFsEntryCompleter(),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Println("missing source file")
				return
			}

			flagSet := flag.NewFlagSet("put", flag.ContinueOnError)
			force := flagSet.Bool("f", false, "force upload (overwrite)")
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

			id := ""
			version := 1
			docNode, err := ctx.api.Filetree.NodeByPath(docName, node)
			if err == nil && !*force {
				fmt.Println(docNode.Id())
				fmt.Println(docNode.Name())
				fmt.Println(docNode.Version())
				c.Err(errors.New("entry already exists"))
				return
			}
			//if overwriting
			if docNode != nil {
				id = docNode.Id()
				version = docNode.Version() + 1
			}

			c.Printf("uploading: [%s]...", srcName)

			dstDir := node.Id()

			document, err := ctx.api.UploadDocument(id, dstDir, srcName, version)

			if err != nil {
				c.Err(errors.New(fmt.Sprint("Failed to upload file", srcName, err.Error())))
				return
			}

			c.Println("OK")

			ctx.api.Filetree.AddDocument(*document)
		},
	}
}
