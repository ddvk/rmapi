package shell

import (
	"encoding/json"

	"github.com/abiosoft/ishell"
)

func diffCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name: "diff",
		Help: "refreshes the file tree and checks if remote content has changed compared to the last refresh",
		Func: func(c *ishell.Context) {
			result, err := ctx.Api.Diff()
			if err != nil {
				c.Err(err)
				return
			}

			if ctx.JSONOutput {
				jsonData, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					c.Err(err)
					return
				}
				c.Println(string(jsonData))
			} else {
				if result.HasChanges {
					c.Printf("Changes detected:\n")
					if len(result.NewFiles) > 0 {
						c.Printf("  New files: %d\n", len(result.NewFiles))
						for _, id := range result.NewFiles {
							c.Printf("    - %s\n", id)
						}
					}
					if len(result.Modified) > 0 {
						c.Printf("  Modified files: %d\n", len(result.Modified))
						for _, id := range result.Modified {
							c.Printf("    - %s\n", id)
						}
					}
					if len(result.Deleted) > 0 {
						c.Printf("  Deleted files: %d\n", len(result.Deleted))
						for _, id := range result.Deleted {
							c.Printf("    - %s\n", id)
						}
					}
				} else {
					c.Println("No changes detected.")
				}
			}
		},
	}
}

