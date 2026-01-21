package shell

import (
	"errors"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/util"
)

func getCmd(ctx *ShellCtxt) *ishell.Cmd {
	longHelp := `Usage: get <remote_file>`

	return &ishell.Cmd{
		Name:      "get",
		Help:      "copy remote file to local",
		Completer: createEntryCompleter(ctx),
		LongHelp:  longHelp,
		Func: func(c *ishell.Context) {
			if checkHelp(longHelp, c.Args, c) {
				return
			}

			if len(c.Args) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := c.Args[0]

			node, err := ctx.api.Filetree().NodeByPath(srcName, ctx.node)

			if err != nil || node.IsDirectory() {
				c.Err(errors.New("file doesn't exist"))
				return
			}

			c.Println(fmt.Sprintf("downloading: [%s]...", srcName))

			err = ctx.api.FetchDocument(node.Document.ID, fmt.Sprintf("%s.%s", node.Name(), util.RMDOC))

			if err == nil {
				c.Println("OK")
				return
			}

			c.Err(errors.New(fmt.Sprintf("Failed to download file %s with %s", srcName, err.Error())))
		},
	}
}
