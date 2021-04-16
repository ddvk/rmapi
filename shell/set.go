package shell

import (
	"errors"
	"flag"
	"fmt"

	"github.com/abiosoft/ishell"
)

func setCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "set",
		Help:      "set metadata",
		Completer: createFsEntryCompleter(),
		Func: func(c *ishell.Context) {
			flagSet := flag.NewFlagSet("set", flag.ContinueOnError)
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

			entry := argRest[0]

			node, _ := ctx.api.Filetree.NodeByPath(entry, ctx.node)

			if node == nil {
				c.Err(errors.New("does not exist"))
				return
			}

			node.Document.CurrentPage = *currentPage

			err := ctx.api.SetEntry(node)

			if err != nil {
				c.Err(errors.New(fmt.Sprint("failed to move entry", err)))
				return
			}

			c.Println("OK")
		},
	}
}
