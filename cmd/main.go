package main

import (
	"os"
	"strings"

	"github.com/topxeq/qxlang"
	"github.com/topxeq/tk"
)

func main() {
	argsT := os.Args

	tk.Pl("args: %v", argsT)

	scriptPathT := strings.TrimSpace(tk.GetParam(argsT, 1, ""))

	if scriptPathT == "" {
		tk.Pl("no script file specified")
		return
	}

	scriptT := tk.LoadStringFromFile(scriptPathT)

	if tk.IsErrStr(scriptT) {
		tk.Pl("failed to load script: %v", tk.GetErrStr(scriptT))
		return
	}

	rsT := qxlang.RunCode(scriptT)

	if rsT != nil {
		tk.Pl("%v", rsT)
	}
}
