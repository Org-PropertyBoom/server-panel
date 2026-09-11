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

	"ppt/server-panel/services/origin"
)

// DNS probing for the Cutover Assistant (docs/panel-cutover-assistant.md).
//
// READ-ONLY: this package only performs DNS lookups. It never writes to
// website_hosts (stack-owned), never changes a host's state, and never touches
// Caddy. Everything here is observation used to compose a client message.

// Edge classifications for a tenant hostname's public DNS (defined in
// services/origin so the classifier is shared and tested with the origin set).
const (
	EdgeOrigin     = origin.EdgeOrigin     // resolves to one of our origin IPs
	EdgeCloudflare = origin.EdgeCloudflare // resolves to Cloudflare anycast
	EdgeElsewhere  = origin.EdgeElsewhere  // resolves somewhere else — client drifted away
	EdgeNXDomain   = origin.EdgeNXDomain   // doesn't resolve — expired/deleted
	EdgeUnknown    = ""                    // not looked up yet / lookup failed
)

// originIPs are our origin addresses: the shared OriginSet (origin_ips.go), the
// same set the reachability probe judges against.
func originIPs() map[string]bool { return OriginSet().IPs() }

func isCloudflareIP(ip string) bool { return origin.IsCloudflareIP(ip) }

// EdgeInfo is one hostname's resolved edge state.
type EdgeInfo struct {
	Edge string   `json:"edge"`
	IPs  []string `json:"ips,omitempty"`
}

// DNSProbeService resolves tenant hostnames and caches the DNS answers. The cache
// is what keeps 103 hosts from re-resolving on every page view. It caches the raw
// answers, not the classification, so a host is always labelled against the current
// origin set (see origin.EdgeCache).
type DNSProbeService struct {
	edges *origin.EdgeCache
}

func NewDNSProbeService() *DNSProbeService {
	return &DNSProbeService{edges: &origin.EdgeCache{
		Lookup: net.DefaultResolver.LookupHost,
		Ours:   originIPs,
		TTL:    edgeCacheTTL,
	}}
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

// Edges resolves the edge classification for many hosts at once. Cached DNS answers
// are reused and only stale ones are looked up, concurrently with a small pool; every
// answer is classified against the current origin set on each call.
func (s *DNSProbeService) Edges(ctx context.Context, hosts []string) map[string]EdgeInfo {
	out := map[string]EdgeInfo{}
	for host, c := range s.edges.Edges(ctx, hosts) {
		out[host] = EdgeInfo{Edge: c.Edge, IPs: c.IPs}
	}
	return out
}

func resolveEdge(ctx context.Context, host string) EdgeInfo {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	edge, ips := origin.ClassifyEdge(addrs, err, originIPs())
	return EdgeInfo{Edge: edge, IPs: ips}
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
