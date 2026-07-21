package reconcile

import "testing"

func TestHostUpstreams_ExtractsReverseProxyDials(t *testing.T) {
	// A subroute-nested reverse_proxy (how `caddy adapt` shapes a site block).
	cfg := []byte(`{"apps":{"http":{"servers":{"srv0":{"routes":[
		{"match":[{"host":["cp.propertyweb.co"]}],"handle":[{"handler":"subroute","routes":[
			{"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"localhost:2205"}]}]}
		]}]},
		{"match":[{"host":["app.propertyboom.co"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"127.0.0.1:8002"}]}]}
	]}}}}}`)
	got := hostUpstreams(cfg)
	if len(got["cp.propertyweb.co"]) != 1 || got["cp.propertyweb.co"][0] != "localhost:2205" {
		t.Errorf("cp.propertyweb.co upstreams = %v, want [localhost:2205]", got["cp.propertyweb.co"])
	}
	if len(got["app.propertyboom.co"]) != 1 || got["app.propertyboom.co"][0] != "127.0.0.1:8002" {
		t.Errorf("app.propertyboom.co upstreams = %v, want [127.0.0.1:8002]", got["app.propertyboom.co"])
	}
}

func TestHostUpstreams_NoDialsForStaticSite(t *testing.T) {
	// A host with no reverse_proxy (e.g. a file_server or respond block) → no dials.
	cfg := []byte(`{"apps":{"http":{"servers":{"srv0":{"routes":[
		{"match":[{"host":["static.example.com"]}],"handle":[{"handler":"static_response","body":"ok"}]}
	]}}}}}`)
	got := hostUpstreams(cfg)
	if len(got["static.example.com"]) != 0 {
		t.Errorf("static site should have no upstreams; got %v", got["static.example.com"])
	}
}
