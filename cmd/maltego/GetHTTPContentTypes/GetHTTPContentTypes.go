package main

import (
	maltego "github.com/dreadl0ck/netcap/cmd/maltego/maltego"
	"github.com/dreadl0ck/netcap/types"
)

func main() {
	maltego.HTTPTransform(
		nil,
		func(lt maltego.LocalTransform, trx *maltego.MaltegoTransform, http *types.HTTP, minPackets, maxPackets uint64, profilesFile string, ipaddr string) {
			if http.SrcIP == ipaddr || http.DstIP == ipaddr {

				if http.ContentTypeDetected != "" {
					// using ContentTypeDetected instead the one that was set on the HTTP request / response
					ent := trx.AddEntity("netcap.ContentType", http.ContentTypeDetected)
					ent.SetType("netcap.ContentType")
					ent.SetValue(http.ContentTypeDetected)

					di := "<h3>Content Type</h3><p>Timestamp: " + http.Timestamp + "</p>"
					ent.AddDisplayInformation(di, "Netcap Info")

					ent.AddProperty("ipaddr", "IPAddress", "strict", ipaddr)
					ent.AddProperty("path", "Path", "strict", profilesFile)

					//ent.SetLinkLabel(strconv.FormatInt(dns..NumPackets, 10) + " pkts")
					ent.SetLinkColor("#000000")
					//ent.SetLinkThickness(maltego.GetThickness(ip.NumPackets))
				}
				if http.ResContentTypeDetected != "" {
					// using ContentTypeDetected instead the one that was set on the HTTP request / response
					ent := trx.AddEntity("netcap.ContentType", http.ResContentTypeDetected)
					ent.SetType("netcap.ContentType")
					ent.SetValue(http.ResContentTypeDetected)

					di := "<h3>Content Type</h3><p>Timestamp: " + http.Timestamp + "</p>"
					ent.AddDisplayInformation(di, "Netcap Info")

					ent.AddProperty("ipaddr", "IPAddress", "strict", ipaddr)
					ent.AddProperty("path", "Path", "strict", profilesFile)

					//ent.SetLinkLabel(strconv.FormatInt(dns..NumPackets, 10) + " pkts")
					ent.SetLinkColor("#000000")
					//ent.SetLinkThickness(maltego.GetThickness(ip.NumPackets))
				}
			}
		},
	)
}
