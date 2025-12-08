package shell

import (
	"errors"
	"fmt"
	"path"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/model"
)

func mvCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "mv",
		Help:      "mv file or directory",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Err(errors.New("missing source and/or destination"))
				return
			}

			src := c.Args[0]
			dst := c.Args[1]

			srcNodes, err := ctx.Api.Filetree().NodesByPath(src, ctx.Node, false)

			if err != nil {
				c.Err(err)
				return
			}
			if len(srcNodes) < 1 {
				c.Err(errors.New("no nodes found"))
				return
			}

			dstNode, _ := ctx.Api.Filetree().NodeByPath(dst, ctx.Node)

			if dstNode != nil && dstNode.IsFile() {
				c.Err(errors.New("destination entry already exists"))
				return
			}

			// We are moving the node to another directory
			if dstNode != nil && dstNode.IsDirectory() {
				for _, node := range srcNodes {
					if IsSubdir(node, dstNode) {
						c.Err(fmt.Errorf("cannot move: %s in itself", node.Name()))
						return
					}

					n, err := ctx.Api.MoveEntry(node, dstNode, node.Name())

					if err != nil {
						c.Err(fmt.Errorf("failed to move entry %w", err))
						return
					}

					ctx.Api.Filetree().MoveNode(node, n)
				}
				err = ctx.Api.SyncComplete()
				if err != nil {
					c.Err(fmt.Errorf("cannot notify, %w", err))
				}
				return
			}

			if len(srcNodes) > 1 {
				c.Err(errors.New("cannot rename multiple nodes, only first match will be renamed"))
			}

			srcNode := srcNodes[0]

			// We are renaming the node
			parentDir := path.Dir(dst)
			newEntry := path.Base(dst)

			parentNode, err := ctx.Api.Filetree().NodeByPath(parentDir, ctx.Node)

			if err != nil || parentNode.IsFile() {
				c.Err(fmt.Errorf("cannot move, %w", err))
				return
			}

			n, err := ctx.Api.MoveEntry(srcNode, parentNode, newEntry)

			if err != nil {
				c.Err(fmt.Errorf("failed to move entry, %w", err))
				return
			}
			err = ctx.Api.SyncComplete()
			if err != nil {
				c.Err(fmt.Errorf("cannot notify, %w", err))
			}

			ctx.Api.Filetree().MoveNode(srcNode, n)
		},
	}
}

// IsSubdir check for moves e.g. a in a/sub1 which result in data loss
func IsSubdir(parent *model.Node, child *model.Node) bool {
	for child != nil {
		if parent.Id() == child.Id() {
			return true
		}
		child = child.Parent
	}
	return false
}
