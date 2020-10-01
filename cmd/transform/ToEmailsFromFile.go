package transform

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/mcnijman/go-emailaddress"

	"github.com/dreadl0ck/netcap/maltego"
)

func toEmailsFromFile() {
	var (
		lt   = maltego.ParseLocalArguments(os.Args)
		trx  = &maltego.Transform{}
		path = strings.TrimPrefix(lt.Values["location"], "file://")
		err  error
	)
	log.Println(lt.Values)

	if path == "" {
		path, err = url.QueryUnescape(lt.Values["properties.url"])
		if err != nil {
			log.Fatal(err)
		}
	}

	path = strings.TrimPrefix(path, "file://")

	log.Println("file path:", path)

	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	results := emailaddress.Find(data, false)
	if len(results) == 0 {
		log.Println("No emails found")
		trx.AddUIMessage("completed!", maltego.UIMessageInform)
		fmt.Println(trx.ReturnOutput())
		os.Exit(0)
	}

	log.Println("results", results)

	for _, r := range results {
		trx.AddEntityWithPath("netcap.Email", r.String(), path)
	}

	trx.AddUIMessage("completed!", maltego.UIMessageInform)
	fmt.Println(trx.ReturnOutput())
}
