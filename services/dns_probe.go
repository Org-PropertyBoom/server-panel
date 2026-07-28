package services

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// DNS probing for the Cutover Assistant (docs/panel-cutover-assistant.md).
//
// READ-ONLY: this package only performs DNS lookups. It never writes to
// website_hosts (stack-owned), never changes a host's state, and never touches
// Caddy. Everything here is observation used to compose a client message.

// Edge classifications for a tenant hostname's public DNS.
const (
	EdgeOrigin     = "Origin"     // resolves to one of our AWS IPs
	EdgeCloudflare = "Cloudflare" // resolves to Cloudflare anycast
	EdgeElsewhere  = "Elsewhere"  // resolves somewhere else — client drifted away
	EdgeNXDomain   = "NXDOMAIN"   // doesn't resolve — expired/deleted
	EdgeUnknown    = ""           // not looked up yet / lookup failed
)

// originIPs are our AWS origin addresses. They differ per domain, hence a set.
// Override with CUTOVER_ORIGIN_IPS (comma-separated).
func originIPs() map[string]bool {
	raw := strings.TrimSpace(os.Getenv("CUTOVER_ORIGIN_IPS"))
	if raw == "" {
		raw = "52.76.29.0,52.76.123.15,3.1.252.222"
	}
	out := map[string]bool{}
	for _, ip := range strings.Split(raw, ",") {
		if ip = strings.TrimSpace(ip); ip != "" {
			out[ip] = true
		}
	}
	return out
}

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

func isCloudflareIP(ip string) bool {
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

// EdgeInfo is one hostname's resolved edge state.
type EdgeInfo struct {
	Edge string   `json:"edge"`
	IPs  []string `json:"ips,omitempty"`
}

type edgeCacheEntry struct {
	info EdgeInfo
	at   time.Time
}

// DNSProbeService resolves tenant hostnames and caches the answers. The cache is
// what keeps 103 hosts from re-resolving on every page view.
type DNSProbeService struct {
	mu    sync.Mutex
	cache map[string]edgeCacheEntry
}

func NewDNSProbeService() *DNSProbeService {
	return &DNSProbeService{cache: map[string]edgeCacheEntry{}}
}

// edgeCacheTTL — DNS doesn't move fast; override with CUTOVER_DNS_CACHE_HOURS.
func edgeCacheTTL() time.Duration {
	if v := strings.TrimSpace(os.Getenv("CUTOVER_DNS_CACHE_HOURS")); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	}
	return 6 * time.Hour
}

// Edges resolves the edge classification for many hosts at once, serving cached
// answers and looking up only what's stale — concurrently, with a small pool.
func (s *DNSProbeService) Edges(ctx context.Context, hosts []string) map[string]EdgeInfo {
	out := map[string]EdgeInfo{}
	var pending []string

	ttl := edgeCacheTTL()
	s.mu.Lock()
	for _, h := range hosts {
		key := strings.ToLower(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		if e, ok := s.cache[key]; ok && time.Since(e.at) < ttl {
			out[key] = e.info
			continue
		}
		pending = append(pending, key)
	}
	s.mu.Unlock()

	if len(pending) == 0 {
		return out
	}

	var wg sync.WaitGroup
	limit := make(chan struct{}, 16)
	var mu sync.Mutex
	for _, h := range pending {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			limit <- struct{}{}
			info := resolveEdge(ctx, host)
			<-limit
			mu.Lock()
			out[host] = info
			mu.Unlock()
			s.mu.Lock()
			s.cache[host] = edgeCacheEntry{info: info, at: time.Now()}
			s.mu.Unlock()
		}(h)
	}
	wg.Wait()
	return out
}

func resolveEdge(ctx context.Context, host string) EdgeInfo {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	if err != nil || len(addrs) == 0 {
		return EdgeInfo{Edge: EdgeNXDomain}
	}
	var v4 []string
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			v4 = append(v4, a)
		}
	}
	sort.Strings(v4)
	if len(v4) == 0 {
		return EdgeInfo{Edge: EdgeElsewhere, IPs: addrs}
	}
	ours := originIPs()
	for _, a := range v4 {
		if isCloudflareIP(a) {
			return EdgeInfo{Edge: EdgeCloudflare, IPs: v4}
		}
	}
	for _, a := range v4 {
		if ours[a] {
			return EdgeInfo{Edge: EdgeOrigin, IPs: v4}
		}
	}
	return EdgeInfo{Edge: EdgeElsewhere, IPs: v4}
}

// ---- Cutover pre-flight (Tab A, Stage 1) ----

// DNSRecord is one probed label's answer.
type DNSRecord struct {
	Label  string   `json:"label"`  // the probed name, e.g. "_dmarc" or "@"
	Type   string   `json:"type"`   // MX | TXT | A | NS | SOA | DS
	Group  string   `json:"group"`  // display grouping from the spec
	Values []string `json:"values"` // answers; empty means "no record"
}

// CutoverInfo is everything the modal needs for one hostname. Composed from DNS
// only — no writes, no stack-DB access.
type CutoverInfo struct {
	Host string `json:"host"`
	Edge string `json:"edge"`
	// OldIP is the CURRENT apex A record — resolved live, never typed, because it
	// differs per domain.
	OldIP       string      `json:"oldIp,omitempty"`
	OldIPs      []string    `json:"oldIps,omitempty"`
	HasMX       bool        `json:"hasMx"`
	MX          []string    `json:"mx,omitempty"`
	HasWWW      bool        `json:"hasWww"`
	WWW         []string    `json:"www,omitempty"`
	Records     []DNSRecord `json:"records"`
	Nameservers []string    `json:"nameservers,omitempty"`
	// DNSSEC gate: DS present means a nameserver change makes the domain
	// UNRESOLVABLE. DSChecked=false means we could not determine it — treated as
	// NOT safe (the gate stays closed), never as "no DS".
	DSPresent bool     `json:"dsPresent"`
	DSChecked bool     `json:"dsChecked"`
	DSValues  []string `json:"dsValues,omitempty"`
	DSNote    string   `json:"dsNote,omitempty"`
}

// probeLabel describes one entry of the fixed pre-flight list. DNS can't be
// enumerated (zone transfer is refused), so a fixed probe list is the only option.
type probeLabel struct {
	label string
	typ   string
	group string
}

var preflightLabels = []probeLabel{
	{"@", "MX", "Mail routing"},

	{"@", "TXT", "Mail auth / verification"},
	{"_dmarc", "TXT", "Mail auth / verification"},
	{"google._domainkey", "TXT", "Mail auth / verification"},
	{"default._domainkey", "TXT", "Mail auth / verification"},
	{"selector1._domainkey", "TXT", "Mail auth / verification"},
	{"selector2._domainkey", "TXT", "Mail auth / verification"},
	{"resend._domainkey", "TXT", "Mail auth / verification"},
	{"k1._domainkey", "TXT", "Mail auth / verification"},

	{"mail", "A", "Mail hosts"},
	{"smtp", "A", "Mail hosts"},
	{"imap", "A", "Mail hosts"},
	{"pop", "A", "Mail hosts"},
	{"webmail", "A", "Mail hosts"},
	{"autodiscover", "A", "Mail hosts"},
	{"autoconfig", "A", "Mail hosts"},

	{"www", "A", "Web"},
	{"ftp", "A", "Web"},
	{"cpanel", "A", "Web"},
	{"blog", "A", "Web"},
	{"shop", "A", "Web"},
	{"m", "A", "Web"},

	{"_cf-custom-hostname", "TXT", "Ours"},
	{"_acme-challenge", "TXT", "Ours"},

	{"@", "NS", "Delegation"},
	{"@", "DS", "Delegation"},
}

func fqdn(label, domain string) string {
	if label == "@" || label == "" {
		return domain
	}
	return label + "." + domain
}

// Cutover runs the full pre-flight for one hostname.
func (s *DNSProbeService) Cutover(ctx context.Context, host string) CutoverInfo {
	host = strings.ToLower(strings.TrimSpace(host))
	info := CutoverInfo{Host: host, Records: []DNSRecord{}}
	if host == "" {
		return info
	}

	edge := resolveEdge(ctx, host)
	info.Edge = edge.Edge
	info.OldIPs = edge.IPs
	if len(edge.IPs) > 0 {
		info.OldIP = edge.IPs[0]
	}

	var wg sync.WaitGroup
	limit := make(chan struct{}, 12)
	results := make([]DNSRecord, len(preflightLabels))
	for i, p := range preflightLabels {
		wg.Add(1)
		go func(idx int, p probeLabel) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			results[idx] = DNSRecord{Label: p.label, Type: p.typ, Group: p.group, Values: lookupRecord(ctx, fqdn(p.label, host), p.typ)}
		}(i, p)
	}
	wg.Wait()

	for _, r := range results {
		// A nil slice marshals to JSON `null`, and most probed labels have no
		// record — so emit an empty array instead. Callers iterate these directly.
		if r.Values == nil {
			r.Values = []string{}
		}
		info.Records = append(info.Records, r)
		switch {
		case r.Type == "MX" && r.Label == "@" && len(r.Values) > 0:
			info.HasMX, info.MX = true, r.Values
		case r.Type == "A" && r.Label == "www" && len(r.Values) > 0:
			info.HasWWW, info.WWW = true, r.Values
		case r.Type == "NS" && r.Label == "@":
			info.Nameservers = r.Values
		case r.Type == "DS" && r.Label == "@":
			info.DSValues = r.Values
		}
	}

	info.DSPresent, info.DSChecked, info.DSNote = checkDNSSEC(ctx, host)
	if info.DSChecked && info.DSPresent && len(info.DSValues) == 0 {
		info.DSValues = []string{"present"}
	}
	return info
}

// lookupRecord resolves one label. Go's resolver has no DS support, so DS goes
// through dig; everything else uses the standard resolver.
func lookupRecord(ctx context.Context, name, typ string) []string {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	switch typ {
	case "MX":
		mxs, err := net.DefaultResolver.LookupMX(lookupCtx, name)
		if err != nil {
			return nil
		}
		var out []string
		for _, m := range mxs {
			out = append(out, fmt.Sprintf("%d %s", m.Pref, strings.TrimSuffix(m.Host, ".")))
		}
		sort.Strings(out)
		return out
	case "TXT":
		txts, err := net.DefaultResolver.LookupTXT(lookupCtx, name)
		if err != nil {
			return nil
		}
		sort.Strings(txts)
		return txts
	case "NS":
		nss, err := net.DefaultResolver.LookupNS(lookupCtx, name)
		if err != nil {
			return nil
		}
		var out []string
		for _, n := range nss {
			out = append(out, strings.TrimSuffix(n.Host, "."))
		}
		sort.Strings(out)
		return out
	case "DS":
		return digShort(ctx, name, "DS", "")
	default: // A / CNAME-ish host lookup
		addrs, err := net.DefaultResolver.LookupHost(lookupCtx, name)
		if err != nil {
			return nil
		}
		sort.Strings(addrs)
		return addrs
	}
}

// checkDNSSEC reports whether a DS record exists at the registry.
//
// FAIL-CLOSED: if dig is unavailable or the query errors we return
// checked=false, and the caller must keep the gate CLOSED. Reporting "no DS"
// on an unknown would be the single most destructive mistake on this path —
// switching nameservers under DNSSEC makes the domain unresolvable, not degraded.
func checkDNSSEC(ctx context.Context, domain string) (present, checked bool, note string) {
	if _, err := exec.LookPath("dig"); err != nil {
		return false, false, "dig is not installed on this host, so DNSSEC could not be checked — verify manually with `dig DS " + domain + "` before changing nameservers"
	}
	out := digShort(ctx, domain, "DS", "")
	if len(out) > 0 {
		return true, true, ""
	}
	// Distinguish "no DS" from "query failed": re-run asking for the status line.
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(cctx, "dig", "+noall", "+comments", "DS", domain).CombinedOutput()
	if err != nil {
		return false, false, "the DNSSEC lookup failed — verify manually with `dig DS " + domain + "` before changing nameservers"
	}
	text := string(raw)
	if strings.Contains(text, "status: NOERROR") || strings.Contains(text, "status: NXDOMAIN") {
		return false, true, ""
	}
	return false, false, "the DNSSEC lookup did not return a usable status — verify manually with `dig DS " + domain + "`"
}

// digShort runs `dig +short [@server] <type> <name>` and returns non-empty lines.
func digShort(ctx context.Context, name, typ, server string) []string {
	if _, err := exec.LookPath("dig"); err != nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	args := []string{"+short"}
	if strings.TrimSpace(server) != "" {
		args = append(args, "@"+strings.TrimSpace(server))
	}
	args = append(args, typ, name)
	out, err := exec.CommandContext(cctx, "dig", args...).Output()
	if err != nil {
		return nil
	}
	var vals []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			vals = append(vals, line)
		}
	}
	sort.Strings(vals)
	return vals
}

// ---- Zone diff (Tab A, Stage 3) ----

// ZoneDiffRow is one label answered by both nameserver sets.
type ZoneDiffRow struct {
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Current []string `json:"current"`
	Target  []string `json:"target"`
	Match   bool     `json:"match"`
}

// ZoneDiffResult is the Stage 3 gate outcome.
type ZoneDiffResult struct {
	Host     string        `json:"host"`
	Rows     []ZoneDiffRow `json:"rows"`
	AllMatch bool          `json:"allMatch"`
	Mismatch int           `json:"mismatch"`
	Error    string        `json:"error,omitempty"`
}

// ZoneDiff queries the registrar's current nameservers AND the target (Cloudflare)
// pair for every pre-flight label and diffs the answers. All rows must match before
// a nameserver switch is safe: a mismatch means a record was dropped or altered on
// import, and switching would make that loss live.
//
// Needs no credentials — it only queries nameservers — but it is inert until a
// Cloudflare zone exists to query.
func (s *DNSProbeService) ZoneDiff(ctx context.Context, host string, targetNS []string) ZoneDiffResult {
	host = strings.ToLower(strings.TrimSpace(host))
	res := ZoneDiffResult{Host: host, Rows: []ZoneDiffRow{}}
	if host == "" {
		res.Error = "host is required"
		return res
	}
	var targets []string
	for _, ns := range targetNS {
		if ns = strings.TrimSpace(ns); ns != "" {
			targets = append(targets, ns)
		}
	}
	if len(targets) == 0 {
		res.Error = "enter the Cloudflare nameservers to compare against"
		return res
	}
	if _, err := exec.LookPath("dig"); err != nil {
		res.Error = "dig is not installed on this host, so the zone diff cannot run"
		return res
	}

	current := lookupRecord(ctx, host, "NS")
	if len(current) == 0 {
		res.Error = "could not read the domain's current nameservers"
		return res
	}

	var wg sync.WaitGroup
	limit := make(chan struct{}, 8)
	rows := make([]ZoneDiffRow, 0, len(preflightLabels))
	var mu sync.Mutex
	for _, p := range preflightLabels {
		if p.typ == "DS" || p.typ == "NS" {
			continue // delegation itself differs by definition — not a content diff
		}
		wg.Add(1)
		go func(p probeLabel) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			name := fqdn(p.label, host)
			cur := digShort(ctx, name, p.typ, current[0])
			tgt := digShort(ctx, name, p.typ, targets[0])
			if cur == nil { // nil marshals to JSON null; callers iterate these
				cur = []string{}
			}
			if tgt == nil {
				tgt = []string{}
			}
			mu.Lock()
			rows = append(rows, ZoneDiffRow{Label: p.label, Type: p.typ, Current: cur, Target: tgt, Match: sameAnswers(cur, tgt)})
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Label != rows[j].Label {
			return rows[i].Label < rows[j].Label
		}
		return rows[i].Type < rows[j].Type
	})
	res.Rows = rows
	res.AllMatch = true
	for _, r := range rows {
		if !r.Match {
			res.AllMatch = false
			res.Mismatch++
		}
	}
	return res
}

func sameAnswers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
