package shell

import (
	"github.com/abiosoft/ishell"
)

func refreshTokenCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name: "refresh-token",
		Help: "refreshes the authentication token",
		Func: func(c *ishell.Context) {
			err := ctx.Api.RefreshToken()
			if err != nil {
				c.Err(err)
				return
			}
			c.Println("Token refreshed successfully")
		},
	}
}

