package shell

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/abiosoft/ishell"
	"github.com/peerdavid/rmapi/filetree"
	"github.com/peerdavid/rmapi/model"
)

func mirrorCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "mirror",
		Help:      "mirror from remarkable cloud into a given directory and DELETES all local files which does not exist in the cloud!",
		Completer: createDirCompleter(ctx),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New(("missing source dir")))
				return
			}

			srcName := c.Args[0]

			node, err := ctx.api.Filetree.NodeByPath(srcName, ctx.node)

			if err != nil || node.IsFile() {
				c.Err(errors.New("directory doesn't exist"))
				return
			}
			
			// Read all lokal files
			var lokalFiles map[string]bool
			lokalFiles = make(map[string]bool)
			err = filepath.Walk(".",
				func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				
				// Is file
				if info.Mode().IsRegular(){
					lokalFiles["./" + path] = true
				}
				return nil
			})

			
			if err != nil {
				c.Err(errors.New("failed to load lokal files for mirror."))
				return
			}

			visitor := filetree.FileTreeVistor{
				func(currentNode *model.Node, currentPath []string) bool {
					idxDir := 0
					if srcName == "." && len(currentPath) > 0 {
						idxDir = 1
					}
					
					dst := "./" + filetree.BuildPath(currentPath[idxDir:], currentNode.Name())
					dir := path.Dir(dst)
					os.MkdirAll(dir, 0766)

					if currentNode.IsDirectory() {
						return filetree.ContinueVisiting
					}
					
					lokalFiles[dst + ".pdf"] = false
					c.Printf("downloading [%s]...", dst)
					
					err = getAnnotatedDocument(ctx, currentNode, fmt.Sprintf("%s", dir), false)

					if err == nil {
						c.Println(" OK")
						return filetree.ContinueVisiting
					}

					c.Err(errors.New(fmt.Sprintf("Failed to download file %s", currentNode.Name())))

					return filetree.ContinueVisiting
				},
			}

			filetree.WalkTree(node, visitor)

			// Delete lokal files
			for file, delete := range lokalFiles { 
				if delete == false{
					continue
				}

				err = os.Remove(file)
				if err != nil {
					c.Err(errors.New("could not delete lokal file " + file))
				}
			}
		},
	}
}
