package shell

import (
	"encoding/json"
	"flag"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/api/sync15"
)


func diffTreeCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name: "difference",
		Help: "compares tree.cache with tree.cache.previous and shows what changed",
		Func: func(c *ishell.Context) {
			// Parse flags
			flagSet := flag.NewFlagSet("difference", flag.ContinueOnError)
			jsonFlag := flagSet.Bool("json", false, "output in JSON format")
			jsonFlagShort := flagSet.Bool("j", false, "output in JSON format (short)")
			if err := flagSet.Parse(c.Args); err != nil {
				if err != flag.ErrHelp {
					c.Err(err)
				}
				return
			}

			diff, err := ctx.Api.DiffTreeCache()
			if err != nil {
				c.Err(err)
				return
			}

			// Use command flag or global JSONOutput setting
			useJSON := *jsonFlag || *jsonFlagShort || ctx.JSONOutput

			if useJSON {
				// Output simplified JSON format
				output := sync15.FormatDiffJSON(diff)
				jsonOutput, err := json.MarshalIndent(output, "", "    ")
				if err != nil {
					c.Err(err)
					return
				}
				c.Println(string(jsonOutput))
				return
			}

			// Output human-readable format
			c.Printf("Tree Diff Results:\n")
			c.Printf("==================\n\n")

			if len(diff.New) > 0 {
				c.Printf("New Documents (%d):\n", len(diff.New))
				for _, doc := range diff.New {
					if path, ok := diff.NewPaths[doc.DocumentID]; ok && path != "" {
						c.Printf("  + %s\n", path)
					} else {
						c.Printf("  + %s (ID: %s)\n", doc.Metadata.DocName, doc.DocumentID)
					}
				}
				c.Printf("\n")
			}

			if len(diff.Removed) > 0 {
				c.Printf("Removed Documents (%d):\n", len(diff.Removed))
				for _, doc := range diff.Removed {
					if path, ok := diff.RemovedPaths[doc.DocumentID]; ok && path != "" {
						c.Printf("  - %s\n", path)
					} else {
						c.Printf("  - %s (ID: %s)\n", doc.Metadata.DocName, doc.DocumentID)
					}
				}
				c.Printf("\n")
			}

			if len(diff.Moved) > 0 {
				c.Printf("Moved Documents (%d):\n", len(diff.Moved))
				for _, moved := range diff.Moved {
					if moved.OldPath != "" && moved.Path != "" {
						c.Printf("  → %s\n", moved.Path)
						c.Printf("    Moved from: %s\n", moved.OldPath)
					} else {
						c.Printf("  → %s (ID: %s)\n", moved.Name, moved.DocumentID)
						if parentChange, ok := moved.Changes["parent"]; ok {
							c.Printf("    Moved from parent: %s -> %s\n", parentChange.Old, parentChange.New)
						}
					}
					c.Printf("\n")
				}
			}

			if len(diff.Modified) > 0 {
				c.Printf("Modified Documents (%d):\n", len(diff.Modified))
				for _, mod := range diff.Modified {
					if mod.Path != "" {
						c.Printf("  ~ %s\n", mod.Path)
					} else {
						c.Printf("  ~ %s (ID: %s)\n", mod.Name, mod.DocumentID)
					}
					if mod.OldHash != mod.NewHash {
						c.Printf("    Hash: %s -> %s\n", mod.OldHash[:16]+"...", mod.NewHash[:16]+"...")
					}
					for field, change := range mod.Changes {
						if field != "hash" && field != "parent" {
							c.Printf("    %s: %s -> %s\n", field, change.Old, change.New)
						}
					}
					if mod.OldPath != "" && mod.Path != "" && mod.OldPath != mod.Path {
						c.Printf("    Path changed: %s -> %s\n", mod.OldPath, mod.Path)
					}
					c.Printf("\n")
				}
			}

			if len(diff.New) == 0 && len(diff.Removed) == 0 && len(diff.Modified) == 0 && len(diff.Moved) == 0 {
				c.Printf("No changes detected.\n")
			}
		},
	}
}

