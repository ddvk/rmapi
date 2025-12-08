package shell

import (
	"errors"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/util"
)

func getCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "get",
		Help:      "copy remote file to local",
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

			err = ctx.Api.FetchDocument(node.Document.ID, fmt.Sprintf("%s.%s", node.Name(), util.RMDOC))

			if err == nil {
				c.Println("OK")
				return
			}

			c.Err(errors.New(fmt.Sprintf("Failed to download file %s with %s", srcName, err.Error())))
		},
	}
}
