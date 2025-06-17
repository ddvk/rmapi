package shell

import (
	"errors"
	"fmt"

	"github.com/abiosoft/ishell"
)

func replacePDFCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "replace-pdf",
		Help:      "replace only the PDF content of an existing document",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Err(errors.New("missing local file or remote entry"))
				return
			}

			srcName := c.Args[0]
			dstPath := c.Args[1]

			node, err := ctx.api.Filetree().NodeByPath(dstPath, ctx.node)
			if err != nil || node.IsDirectory() {
				c.Err(errors.New("remote file doesn't exist"))
				return
			}

			c.Printf("replacing PDF of [%s] with [%s]...", dstPath, srcName)
			if err := ctx.api.ReplaceDocumentFile(node.Document.ID, srcName, true); err != nil {
				c.Err(fmt.Errorf("failed to replace content: %v", err))
				return
			}
			c.Println("OK")
		},
	}
}
