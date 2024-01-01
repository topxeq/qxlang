package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/topxeq/qxlang"
	"github.com/topxeq/tk"
)

func main() {
	argsT := os.Args

	scriptPathT := strings.TrimSpace(tk.GetParam(argsT, 1, ""))

	if scriptPathT == "" {
		tk.Pl("no script file specified")
		return
	}

	ifGoPathT := tk.IfSwitchExistsWhole(argsT, "-gopath")

	var scriptT string

	if ifGoPathT {
		scriptPathT = filepath.Join(tk.GetEnv("GOPATH"), "src", "github.com", "topxeq", "qxlang", "cmd", "scripts", scriptPathT)
		scriptT = tk.LoadStringFromFile(scriptPathT)
	} else {
		scriptT = tk.LoadStringFromFile(scriptPathT)
	}

	if tk.IsErrStr(scriptT) {
		tk.Pl("failed to load script: %v", tk.GetErrStr(scriptT))
		return
	}

	if tk.IfSwitchExists(argsT, "-debug") {
		qxlang.DebugG = true

		tk.Pl("args: %v", argsT)
	}

	rsT := qxlang.RunCode(scriptT)

	if rsT != nil && rsT != tk.Undefined {
		tk.Pl("%v", rsT)
	}
}
