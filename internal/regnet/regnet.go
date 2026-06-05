// Package regnet contains networking helper functions for interacting with registries.
package regnet

import (
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/regclient/regclient/types/errs"
)

func AllowRedirect(src, dest url.URL) error {
	if src.Scheme == "https" && dest.Scheme != "https" {
		return fmt.Errorf("redirect from an https to non-https server is not allowed (%s)%.0w", dest.String(), errs.ErrHTTPRedirectRefused)
	}
	if !IsLocal(src.Host) && IsLocal(dest.Host) {
		return fmt.Errorf("redirect to a local domain is not allowed (%s)%.0w", dest.String(), errs.ErrHTTPRedirectRefused)
	}
	return nil
}

func IsLocal(hostPort string) bool {
	// skip check on any requests going through a proxy, ProxyFromEnv assumes http, and localhost is unlikely to have an https cert
	if u, err := http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Host: hostPort}}); err == nil && u != nil {
		return false
	}
	// strip trailing port
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	// parse IP
	ip := net.ParseIP(host)
	if ip != nil {
		return isIPLocal(ip)
	}
	// else resolve the hostname and then check each IP
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip != nil && isIPLocal(ip) {
			return true
		}
	}
	return false
}

func isIPLocal(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
