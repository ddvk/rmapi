package shell

import (
	"github.com/abiosoft/ishell"
)

func accountCmd(ctx *ShellCtxt) *ishell.Cmd {
	longHelp := `Usage: account`

	return &ishell.Cmd{
		Name:     "account",
		Help:     "account info",
		LongHelp: longHelp,
		Func: func(c *ishell.Context) {
			if checkHelp(longHelp, c.Args, c) {
				return
			}
			c.Printf("User: %s, SyncVersion: %v\n", ctx.UserInfo.User, ctx.UserInfo.SyncVersion)
		},
	}
}
