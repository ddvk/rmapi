package shell

import (
	"errors"
	"fmt"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/util"
	flag "github.com/ogier/pflag"
)

func getCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "get",
		Help:      "copy remote file to local, usage: get [--id] <path|id>",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {
			flagSet := flag.NewFlagSet("get", flag.ContinueOnError)
			var byId bool
			flagSet.BoolVar(&byId, "id", false, "interpret argument as document ID instead of path")
			if err := flagSet.Parse(c.Args); err != nil {
				if err != flag.ErrHelp {
					c.Err(err)
				}
				return
			}
			args := flagSet.Args()

			if len(args) == 0 {
				c.Err(errors.New("missing source file or id"))
				return
			}

			srcArg := args[0]
			var node *model.Node
			var err error

			if byId {
				node = ctx.api.Filetree().NodeById(srcArg)
				if node == nil {
					c.Err(errors.New("document with given ID doesn't exist"))
					return
				}
			} else {
				node, err = ctx.api.Filetree().NodeByPath(srcArg, ctx.node)
				if err != nil {
					c.Err(errors.New("file doesn't exist"))
					return
				}
			}

			if node.IsDirectory() {
				c.Err(errors.New("cannot download a directory"))
				return
			}

			c.Println(fmt.Sprintf("downloading: [%s]...", node.Name()))

			err = ctx.api.FetchDocument(node.Document.ID, fmt.Sprintf("%s.%s", node.Name(), util.RMDOC))

			if err == nil {
				c.Println("OK")
				return
			}

			c.Err(errors.New(fmt.Sprintf("Failed to download file %s with %s", node.Name(), err.Error())))
		},
	}
}
