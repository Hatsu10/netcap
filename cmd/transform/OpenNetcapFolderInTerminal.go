package transform

import (
	"fmt"
	maltego "github.com/dreadl0ck/netcap/maltego"
	"log"
	"os"
	"os/exec"
	"runtime"
)

func OpenNetcapFolderInTerminal() {

	var (
		lt              = maltego.ParseLocalArguments(os.Args)
		trx             = &maltego.MaltegoTransform{}
		openCommandName = os.Getenv("NETCAP_MALTEGO_OPEN_TERMINAL_CMD")
		args            []string
	)

	// if no command has been supplied via environment variable
	// then default to:
	// - open for macOS
	// - gio open for linux
	if openCommandName == "" {
		if runtime.GOOS == "darwin" {
			openCommandName = "/Applications/iTerm.app/Contents/MacOS/iTerm2"
		} else { // linux TODO:
			openCommandName = "gio"
			args = append(args, "open")
		}
	}

	log.Println("vals", lt.Values)
	path := lt.Values["path"]
	path = path + ".net"
	log.Println("open path:", path)

	log.Println("command for opening path:", openCommandName)
	args = append(args, path)

	out, err := exec.Command(openCommandName, args...).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		log.Fatal(err)
	}
	log.Println(string(out))

	trx.AddUIMessage("completed!", "Inform")
	fmt.Println(trx.ReturnOutput())
}