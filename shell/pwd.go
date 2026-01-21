package shell

import "github.com/abiosoft/ishell"

func pwdCmd(ctx *ShellCtxt) *ishell.Cmd {
	longHelp := `Usage: pwd`

	return &ishell.Cmd{
		Name:     "pwd",
		Help:     "print current directory",
		LongHelp: longHelp,
		Func: func(c *ishell.Context) {
			if checkHelp(longHelp, c.Args, c) {
				return
			}
			c.Println(ctx.path)
		},
	}
}
