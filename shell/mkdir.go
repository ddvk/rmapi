package shell

import (
	"errors"
	"fmt"
	"path"

	"github.com/abiosoft/ishell"
)

func mkdirCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "mkdir",
		Help:      "create a directory",
		Completer: createDirCompleter(ctx),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New("missing directory"))
				return
			}

			target := c.Args[0]

			_, err := ctx.Api.Filetree().NodeByPath(target, ctx.Node)

			if err == nil {
				c.Println("entry already exists")
				return
			}

			parentDir := path.Dir(target)
			newDir := path.Base(target)

			if newDir == "/" || newDir == "." {
				c.Err(errors.New("invalid directory name"))
				return
			}

			parentNode, err := ctx.Api.Filetree().NodeByPath(parentDir, ctx.Node)

			if err != nil || parentNode.IsFile() {
				c.Err(errors.New("directory doesn't exist"))
				return
			}

			parentId := parentNode.Id()
			if parentNode.IsRoot() {
				parentId = ""
			}

			document, err := ctx.Api.CreateDir(parentId, newDir, true)

			if err != nil {
				c.Err(errors.New(fmt.Sprint("failed to create directory", err)))
				return
			}

			ctx.Api.Filetree().AddDocument(document)
		},
	}
}
