package transform

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/dreadl0ck/netcap/maltego"
)

func toCaptureProcess() {
	lt := maltego.ParseLocalArguments(os.Args[1:])
	log.Println("capture on interface:", lt.Value)

	// check if a custom snaplen was provided as property
	if snaplen, ok := lt.Values["snaplen"]; ok {

		if snaplen != "" {
			n, err := strconv.Atoi(snaplen)
			if err != nil {
				log.Fatal("invalid snaplen provided: ", err)
			}

			if n <= 0 {
				log.Fatal("invalid snaplen provided: ", n)
			}

			// set value in the base config
			maltegoBaseConfig.SnapLen = n
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	outDir := filepath.Join(home, lt.Value+".net")
	log.Println("writing output to:", outDir)

	// prepare arguments
	args := []string{"capture", "-iface", lt.Value, "-out", outDir, "-fileStorage=files", "-config=/usr/local/etc/netcap/livecapture.conf", "-quiet"}

	// check if a custom bpf was provided as property
	if bpf, ok := lt.Values["bpf"]; ok {

		if bpf != "" {
			args = append(args, "-bpf="+bpf)
		}
	}

	log.Println("args:", args)

	cmd := exec.Command("/usr/local/bin/net", args...)
	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("PID", cmd.Process.Pid)

	returnCaptureProcessEntity(cmd.Process.Pid, outDir, lt.Value)
}

func returnCaptureProcessEntity(pid int, path string, iface string) {
	pidStr := strconv.Itoa(pid)

	// generate maltego transform
	trx := maltego.Transform{}

	name := "Capture Process" + "\nPID: " + pidStr
	ent := trx.AddEntityWithPath("netcap.CaptureProcess", name, path)

	ent.AddProperty("pid", "PID", "strict", pidStr)

	ent.AddProperty("iface", "Interface", "strict", iface)

	trx.AddUIMessage("completed!", maltego.UIMessageInform)
	fmt.Println(trx.ReturnOutput())
}
