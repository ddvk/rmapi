package shell

import (
	"errors"
	"fmt"
	"github.com/abiosoft/ishell"
)

func getnCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "getn",
		Help:      "get notes only",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {
			// Parse cmd args
			if len(c.Args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}
			srcName := c.Args[0]

			// Download document as zip
			node, err := ctx.api.Filetree.NodeByPath(srcName, ctx.node)
			if err != nil || node.IsDirectory() {
				c.Err(errors.New("file doesn't exist"))
				return
			}

			err = getAnnotatedDocument(ctx, node, "", true)
			if err != nil {
				c.Err(err)
				return
			}

			fmt.Println("OK")
		},
	}
}
