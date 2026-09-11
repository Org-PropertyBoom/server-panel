package origin

import (
	"net"
	"sort"
)

// Edge classifications for a tenant hostname's public DNS.
const (
	EdgeOrigin     = "Origin"     // resolves to one of our origin IPs
	EdgeCloudflare = "Cloudflare" // resolves to Cloudflare anycast
	EdgeElsewhere  = "Elsewhere"  // resolves somewhere else — client drifted away
	EdgeNXDomain   = "NXDOMAIN"   // doesn't resolve — expired/deleted
)

// cloudflareNets are Cloudflare's published IPv4 ranges. Membership decides the
// "Cloudflare" classification; the cutover A records (104.21.69.82 /
// 172.67.206.132) fall inside 104.16.0.0/13 and 172.64.0.0/13.
var cloudflareNets = func() []*net.IPNet {
	cidrs := []string{
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
		"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	}
	var out []*net.IPNet
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// IsCloudflareIP reports whether ip is inside Cloudflare's published IPv4 ranges.
func IsCloudflareIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range cloudflareNets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// ClassifyEdge turns one host's lookup result into an Edge label plus the IPs worth
// showing. Cloudflare wins over Origin (a proxied host's visitors hit Cloudflare).
func ClassifyEdge(addrs []string, lookupErr error, ours map[string]bool) (edge string, ips []string) {
	if lookupErr != nil || len(addrs) == 0 {
		return EdgeNXDomain, nil
	}
	var v4 []string
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			v4 = append(v4, a)
		}
	}
	sort.Strings(v4)
	if len(v4) == 0 {
		return EdgeElsewhere, addrs
	}
	for _, a := range v4 {
		if IsCloudflareIP(a) {
			return EdgeCloudflare, v4
		}
	}
	for _, a := range v4 {
		if ours[Canonical(a)] {
			return EdgeOrigin, v4
		}
	}
	return EdgeElsewhere, v4
}
