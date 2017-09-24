package proxy

import (
	"net"
	"net/http"

	"golang.org/x/net/proxy"
)

func NetDial(dialer proxy.Dialer, network, address string) (net.Conn, error) {
	if dialer != nil {
		return dialer.Dial(network, address)
	}
	return net.Dial(network, address)
}

func HttpGet(dialer proxy.Dialer, url string) (r *http.Response, e error) {
	return HttpClient(dialer).Get(url)
}

func HttpClient(dialer proxy.Dialer) (client *http.Client) {
	if dialer == nil {
		dialer = proxy.Direct
	}
	tr := &http.Transport{Dial: dialer.Dial}
	client = &http.Client{Transport: tr}
	return
}
