package shell

import (
	"errors"
	"fmt"
	"path"

	"github.com/abiosoft/ishell"
)

func mkdir(target string, ctx *ShellCtxt)(error){
	_, err := ctx.api.Filetree.NodeByPath(target, ctx.node)
	if err == nil {
		return nil
	}

	parentDir := path.Dir(target)
	newDir := path.Base(target)

	if newDir == "/" || newDir == "." {
		return errors.New("invalid directory name")
	}

	parentNode, err := ctx.api.Filetree.NodeByPath(parentDir, ctx.node)

	if err != nil || parentNode.IsFile() {
		return errors.New("directory doesn't exist")
	}

	parentId := parentNode.Id()
	if parentNode.IsRoot() {
		parentId = ""
	}

	document, err := ctx.api.CreateDir(parentId, newDir)

	if err != nil {
		return errors.New(fmt.Sprint("failed to create directory", err))
	}

	ctx.api.Filetree.AddDocument(document)
	return nil
}

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
			err := mkdir(target, ctx)
			if err != nil {
				c.Err(err)
				return
			}
		},
	}
}
