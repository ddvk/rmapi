package shell

import (
	"fmt"
	"os"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/api"
	"github.com/juruen/rmapi/model"
)

type ShellCtxt struct {
	node           *model.Node
	api            api.ApiCtx
	path           string
	useHiddenFiles bool
	UserInfo       api.UserInfo
	JSONOutput     bool
	commands       []*ishell.Cmd
}

func (ctx *ShellCtxt) prompt() string {
	return fmt.Sprintf("[%s]>", ctx.path)
}

func (ctx *ShellCtxt) addCmd(shell *ishell.Shell, cmd *ishell.Cmd) {
	shell.AddCmd(cmd)
	ctx.commands = append(ctx.commands, cmd)
}

func setCustomCompleter(shell *ishell.Shell) {
	cmdCompleter := make(cmdToCompleter)
	for _, cmd := range shell.Cmds() {
		cmdCompleter[cmd.Name] = cmd.Completer
	}

	completer := shellPathCompleter{cmdCompleter}
	shell.CustomCompleter(completer)
}

func useHiddenFiles() bool {
	val, ok := os.LookupEnv("RMAPI_USE_HIDDEN_FILES")

	if !ok {
		return false
	}

	return val != "0"
}

func RunShell(apiCtx api.ApiCtx, userInfo *api.UserInfo, args []string, jsonOutput bool) error {
	shell := ishell.New()
	ctx := &ShellCtxt{
		node:           apiCtx.Filetree().Root(),
		api:            apiCtx,
		path:           apiCtx.Filetree().Root().Name(),
		useHiddenFiles: useHiddenFiles(),
		UserInfo:       *userInfo,
		JSONOutput:     jsonOutput,
	}

	shell.SetPrompt(ctx.prompt())

	ctx.addCmd(shell, helpCmd(shell, ctx))
	ctx.addCmd(shell, accountCmd(ctx))
	ctx.addCmd(shell, versionCmd(ctx))
	ctx.addCmd(shell, refreshCmd(ctx))

	ctx.addCmd(shell, pwdCmd(ctx))
	ctx.addCmd(shell, cdCmd(ctx))
	ctx.addCmd(shell, lsCmd(ctx))
	ctx.addCmd(shell, statCmd(ctx))
	ctx.addCmd(shell, findCmd(ctx))

	ctx.addCmd(shell, getCmd(ctx))
	ctx.addCmd(shell, getACmd(ctx))
	ctx.addCmd(shell, mgetCmd(ctx))

	ctx.addCmd(shell, mvCmd(ctx))
	ctx.addCmd(shell, putCmd(ctx))
	ctx.addCmd(shell, mputCmd(ctx))
	ctx.addCmd(shell, mkdirCmd(ctx))
	ctx.addCmd(shell, rmCmd(ctx))
	ctx.addCmd(shell, nukeCmd(ctx))
	setCustomCompleter(shell)

	if len(args) > 0 {
		return shell.Process(args...)
	} else {
		shell.Printf("ReMarkable Cloud API Shell, User: %s, SyncVersion: %s\n", userInfo.User, userInfo.SyncVersion)
		shell.Run()

		return nil
	}
}
