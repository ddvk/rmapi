package shell

import (
	"errors"

	"github.com/abiosoft/ishell"
)

func refreshCmd(ctx *ShellCtxt) *ishell.Cmd {
	longHelp := `Usage: refresh`

	return &ishell.Cmd{
		Name:     "refresh",
		Help:     "refreshes the tree with remote changes",
		LongHelp: longHelp,
		Func: func(c *ishell.Context) {
			if checkHelp(longHelp, c.Args, c) {
				return
			}
			has, gen, err := ctx.api.Refresh()
			if err != nil {
				c.Err(err)
				return
			}
			c.Printf("root hash: %s\ngeneration: %d\n", has, gen)
			n, err := ctx.api.Filetree().NodeByPath(ctx.path, nil)
			if err != nil {
				c.Err(errors.New("current path is invalid"))

				ctx.node = ctx.api.Filetree().Root()
				ctx.path = ctx.node.Name()
				c.SetPrompt(ctx.prompt())
				return
			}
			ctx.node = n
		},
	}
}
