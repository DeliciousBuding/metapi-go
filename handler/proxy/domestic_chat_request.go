package proxyhandler

import (
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func nativeChatRequestProfile(site store.Site, path string) string {
	endpoint, ok := proxy.EndpointFromPath(path)
	if !ok || endpoint != proxy.EndpointChat {
		return ""
	}
	return service.NativeChatRequestProfile(site.URL, site.Platform)
}
