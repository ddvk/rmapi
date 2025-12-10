package shell

import (
	"fmt"
	"os"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/api"
	"github.com/juruen/rmapi/model"
)

type ShellCtxt struct {
	Node           *model.Node
	Api            api.ApiCtx
	Path           string
	UseHiddenFiles bool
	UserInfo       api.UserInfo
	JSONOutput     bool
}

func (ctx *ShellCtxt) prompt() string {
	return fmt.Sprintf("[%s]>", ctx.Path)
}

func setCustomCompleter(shell *ishell.Shell) {
	cmdCompleter := make(cmdToCompleter)
	for _, cmd := range shell.Cmds() {
		cmdCompleter[cmd.Name] = cmd.Completer
	}

	completer := shellPathCompleter{cmdCompleter}
	shell.CustomCompleter(completer)
}

func UseHiddenFiles() bool {
	val, ok := os.LookupEnv("RMAPI_USE_HIDDEN_FILES")

	if !ok {
		return false
	}

	return val != "0"
}

func RunShell(apiCtx api.ApiCtx, userInfo *api.UserInfo, args []string, jsonOutput bool) error {
	shell := ishell.New()
	ctx := &ShellCtxt{
		Node:           apiCtx.Filetree().Root(),
		Api:            apiCtx,
		Path:           apiCtx.Filetree().Root().Name(),
		UseHiddenFiles: UseHiddenFiles(),
		UserInfo:       *userInfo,
		JSONOutput:     jsonOutput,
	}

	shell.SetPrompt(ctx.prompt())

	shell.AddCmd(lsCmd(ctx))
	shell.AddCmd(pwdCmd(ctx))
	shell.AddCmd(cdCmd(ctx))
	shell.AddCmd(getCmd(ctx))
	shell.AddCmd(convertCmd(ctx))
	shell.AddCmd(hwrCmd(ctx))
	shell.AddCmd(mgetCmd(ctx))
	shell.AddCmd(mkdirCmd(ctx))
	shell.AddCmd(rmCmd(ctx))
	shell.AddCmd(mvCmd(ctx))
	shell.AddCmd(putCmd(ctx))
	shell.AddCmd(mputCmd(ctx))
	shell.AddCmd(versionCmd(ctx))
	shell.AddCmd(statCmd(ctx))
	shell.AddCmd(getACmd(ctx))
	shell.AddCmd(findCmd(ctx))
	shell.AddCmd(nukeCmd(ctx))
	shell.AddCmd(accountCmd(ctx))
	shell.AddCmd(refreshCmd(ctx))
	shell.AddCmd(refreshTokenCmd(ctx))
	shell.AddCmd(refreshTreeCmd(ctx))
	shell.AddCmd(diffCmd(ctx))

	setCustomCompleter(shell)

	if len(args) > 0 {
		return shell.Process(args...)
	} else {
		shell.Printf("ReMarkable Cloud API Shell, User: %s, SyncVersion: %s\n", userInfo.User, userInfo.SyncVersion)
		shell.Run()

		return nil
	}
}
