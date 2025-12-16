package shell

import (
	"errors"

	"github.com/abiosoft/ishell"
)

func refreshTreeCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name: "refresh-tree",
		Help: "refreshes the file tree with remote changes",
		Func: func(c *ishell.Context) {
			has, gen, err := ctx.Api.RefreshTree()
			if err != nil {
				c.Err(err)
				return
			}
			c.Printf("root hash: %s\ngeneration: %d\n", has, gen)
			n, err := ctx.Api.Filetree().NodeByPath(ctx.Path, nil)
			if err != nil {
				c.Err(errors.New("current path is invalid"))

				ctx.Node = ctx.Api.Filetree().Root()
				ctx.Path = ctx.Node.Name()
				c.SetPrompt(ctx.prompt())
				return
			}
			ctx.Node = n
		},
	}
}

