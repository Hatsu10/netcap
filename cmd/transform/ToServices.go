package transform

import (
	"strconv"

	"github.com/dreadl0ck/netcap/maltego"
	"github.com/dreadl0ck/netcap/types"
	"github.com/dreadl0ck/netcap/utils"
)

func toServices() {
	maltego.ServiceTransform(
		nil,
		func(lt maltego.LocalTransform, trx *maltego.Transform, service *types.Service, min, max uint64, path string, mac string, ipaddr string) {
			val := service.IP + ":" + strconv.Itoa(int(service.Port))
			if len(service.Vendor) > 0 {
				val += "\n" + service.Vendor
			}
			if len(service.Product) > 0 {
				val += "\n" + service.Product
			}
			if len(service.Name) > 0 {
				val += "\n" + service.Name
			}
			if len(service.Hostname) > 0 {
				val += "\n" + service.Hostname
			}

			ent := trx.AddEntityWithPath("netcap.Service", val, path)
			ent.AddProperty("timestamp", "Timestamp", "strict", utils.UnixTimeToUTC(service.Timestamp))
			ent.AddProperty("product", "Product", "strict", service.Product)
			ent.AddProperty("version", "Version", "strict", service.Version)
			ent.AddProperty("protocol", "Protocol", "strict", service.Protocol)
			ent.AddProperty("ip", "IP", "strict", service.IP)
			ent.AddProperty("port", "Port", "strict", strconv.Itoa(int(service.Port)))
			ent.AddProperty("hostname", "Hostname", "strict", service.Hostname)
			ent.AddProperty("bytesclient", "BytesClient", "strict", strconv.Itoa(int(service.BytesClient)))
			ent.AddProperty("bytesserver", "BytesServer", "strict", strconv.Itoa(int(service.BytesServer)))
			ent.AddProperty("vendor", "Vendor", "strict", service.Vendor)
			ent.AddProperty("name", "Name", "strict", service.Name)

			if len(service.Banner) > 0 {
				ent.AddDisplayInformation("<pre>"+maltego.EscapeText(service.Banner)+"</pre>", "Transferred Data")
			}
		},
	)
}
