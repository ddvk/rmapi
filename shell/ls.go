package shell

import (
	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/model"
)

func displayNodes(c *ishell.Context, e *model.Node) {
	eType := "d"
	if e.IsFile() {
		eType = "f"
	}
	c.Printf("[%s]\t%s\n", eType, e.Name())
}

func lsCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "ls",
		Help:      "list directory",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {
			node := ctx.node

			if len(c.Args) == 1 {
				target := c.Args[0]

				nodes, err := ctx.api.Filetree().NodesByPath(target, ctx.node)

				if err != nil {
					c.Err(err)
				}

				// if len(nodes) == 0 {
				// 	c.Err(errors.New("directory doesn't exist"))
				// 	return
				// }

				for _, e := range nodes {
					displayNodes(c, e)
				}
				return
			}

			for _, e := range node.Children {
				displayNodes(c, e)
			}
		},
	}
}
