package shell

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/filetree"
	"github.com/juruen/rmapi/model"
	flag "github.com/ogier/pflag"
)

// tagSlice implements flag.Value to collect multiple --tag flags
type tagSlice []string

func (t *tagSlice) String() string {
	return strings.Join(*t, ",")
}

func (t *tagSlice) Set(value string) error {
	*t = append(*t, value)
	return nil
}

func findCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "find",
		Help:      "find files recursively, usage: find [options] [dir] [regexp]",
		Completer: createDirCompleter(ctx),
		Func: func(c *ishell.Context) {
			flagSet := flag.NewFlagSet("find", flag.ContinueOnError)
			var compact bool
			var tags tagSlice
			var starred bool
			flagSet.BoolVarP(&compact, "compact", "c", false, "compact format")
			flagSet.Var(&tags, "tag", "filter by tag (can be specified multiple times, matches files with ANY of the tags)")
			flagSet.BoolVar(&starred, "starred", false, "only show starred files")
			if err := flagSet.Parse(c.Args); err != nil {
				if err != flag.ErrHelp {
					c.Err(err)
				}
				return
			}
			argRest := flagSet.Args()

			// Check if --starred flag was actually set
			starredFilterEnabled := false
			flagSet.Visit(func(f *flag.Flag) {
				if f.Name == "starred" {
					starredFilterEnabled = true
				}
			})

			var start, pattern string
			switch len(argRest) {
			case 2:
				pattern = argRest[1]
				fallthrough
			case 1:
				start = argRest[0]
			case 0:
				start = ctx.path
			default:
				c.Err(errors.New("missing arguments; usage find [options] [dir] [regexp]"))
				return
			}

			startNode, err := ctx.api.Filetree().NodeByPath(start, ctx.node)

			if err != nil {
				c.Err(errors.New("start directory doesn't exist"))
				return
			}

			var matchRegexp *regexp.Regexp
			if pattern != "" {
				matchRegexp, err = regexp.Compile(pattern)
				if err != nil {
					c.Err(errors.New("failed to compile regexp"))
					return
				}
			}

			filetree.WalkTree(startNode, filetree.FileTreeVistor{
				Visit: func(node *model.Node, path []string) bool {
					// Filter by starred status if flag was set
					if starredFilterEnabled && node.Document != nil {
						if node.Document.Starred != starred {
							return false
						}
					}

					// Filter by tags if specified (must have ANY of the tags - OR semantics)
					if len(tags) > 0 && node.Document != nil {
						nodeTags := node.Document.Tags
						hasMatch := false
						for _, requiredTag := range tags {
							for _, nodeTag := range nodeTags {
								if nodeTag == requiredTag {
									hasMatch = true
									break
								}
							}
							if hasMatch {
								break
							}
						}
						if !hasMatch {
							// Doesn't have any of the required tags, skip this node
							return false
						}
					}

					entryName := formatEntry(compact, path, node)

					if matchRegexp == nil {
						c.Println(entryName)
						return false
					}

					if !matchRegexp.Match([]byte(entryName)) {
						return false
					}

					c.Println(entryName)

					return false
				},
			})
		},
	}
}
func formatEntry(compact bool, path []string, node *model.Node) string {
	fullpath := filepath.Join(strings.Join(path, "/"), node.Name())
	if compact {
		if node.IsDirectory() {
			return fullpath + "/"
		}

		return fullpath
	}
	var entryType string
	if node.IsDirectory() {
		entryType = "[d] "
	} else {
		entryType = "[f] "
	}
	return entryType + fullpath
}
