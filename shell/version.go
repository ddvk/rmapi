package shell

import (
	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/version"
)

func versionCmd(ctx *ShellCtxt) *ishell.Cmd {
	longHelp := `Usage: version`

	return &ishell.Cmd{
		Name:     "version",
		Help:     "show rmapi version",
		LongHelp: longHelp,
		Func: func(c *ishell.Context) {
			if checkHelp(longHelp, c.Args, c) {
				return
			}
			c.Println("rmapi version:", version.Version)
		},
	}
}
