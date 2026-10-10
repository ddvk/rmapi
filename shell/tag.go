package shell

import (
	"errors"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/ogier/pflag"
)

func tagCmd(ctx *ShellCtxt) *ishell.Cmd {
	longHelp := `Usage: tag [options] <file> [tag[,tag...]]

Adds the given document tags to a file that is already on the device. The
document is kept in place (same ID, annotations untouched); only its tag list
is rewritten. Tags are comma-separated; use quotes for names with spaces.

With no options the tags are added to the ones already set. --remove takes
the given tags away, --set replaces the whole list (--set with no tags clears
every tag). The resulting tag list is printed.`

	return &ishell.Cmd{
		Name:      "tag",
		Help:      "add, remove or replace document tags",
		Completer: createFileCompleter(ctx),
		LongHelp:  longHelp,
		Func: func(c *ishell.Context) {
			flags := pflag.NewFlagSet("tag", pflag.ContinueOnError)
			remove := flags.Bool("remove", false, "Remove the given tags instead of adding them")
			set := flags.Bool("set", false, "Replace the current tags with the given ones")

			if !processFlagSet(flags, longHelp, c.Args, c) {
				return
			}
			if *remove && *set {
				c.Err(errors.New("--remove and --set cannot be used together"))
				return
			}

			args := flags.Args()
			if len(args) == 0 {
				c.Err(errors.New("missing file"))
				return
			}
			tags := parseTags(strings.Join(args[1:], ","))
			if len(tags) == 0 && !*set {
				c.Err(errors.New("missing tags"))
				return
			}

			node, err := ctx.api.Filetree().NodeByPath(args[0], ctx.node)
			if err != nil || node.IsDirectory() {
				c.Err(errors.New("file doesn't exist"))
				return
			}

			var next []string
			switch {
			case *set:
				next = tags
			case *remove:
				next = removeTags(node.Document.Tags, tags)
			default:
				next = mergeTags(node.Document.Tags, tags)
			}

			c.Printf("tagging [%s]...", node.Name())
			if err := ctx.api.SetDocumentTags(node.Document.ID, next, true); err != nil {
				c.Err(err)
				return
			}
			node.Document.Tags = next
			c.Println("OK")
			c.Println("tags:", strings.Join(next, ", "))
		},
	}
}

// mergeTags appends the tags in add that are not already in current, keeping
// current's order.
func mergeTags(current, add []string) []string {
	out := append([]string(nil), current...)
	for _, tag := range add {
		if !containsTag(out, tag) {
			out = append(out, tag)
		}
	}
	return out
}

// removeTags returns current without any of the tags in drop.
func removeTags(current, drop []string) []string {
	out := []string{}
	for _, tag := range current {
		if !containsTag(drop, tag) {
			out = append(out, tag)
		}
	}
	return out
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
