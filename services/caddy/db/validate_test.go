package db

import "testing"

func guard() Guard {
	known := map[string]bool{"phalcon": true, "laravel": true, "golang": true}
	return Guard{
		IsProtected: func(h string) bool { return h == "app.propertyboom.co" },
		StackKnown:  func(s string) bool { return known[s] },
	}
}

func TestValidateSystemHost_OK(t *testing.T) {
	in, err := ValidateSystemHost(SystemHostInput{Host: "  LA-App.PropertyBoom.co ", ServerStack: "Laravel", Target: "127.0.0.1:8004", IsActive: true}, guard())
	if err != nil {
		t.Fatal(err)
	}
	if in.Host != "la-app.propertyboom.co" || in.ServerStack != "laravel" {
		t.Errorf("normalization failed: %+v", in)
	}
}

func TestValidateSystemHost_AllowsAnyServiceLabel(t *testing.T) {
	// A system host proxies to ANY container, so server_stack is a free label
	// (not restricted to the code stacks); empty defaults to "system".
	in, err := ValidateSystemHost(SystemHostInput{Host: "dbs.cobds.com", ServerStack: "nocodb", Target: "127.0.0.1:9001"}, guard())
	if err != nil {
		t.Fatalf("infra service label must be allowed: %v", err)
	}
	if in.ServerStack != "nocodb" {
		t.Errorf("ServerStack = %q, want nocodb", in.ServerStack)
	}
	in2, err := ValidateSystemHost(SystemHostInput{Host: "y.com", ServerStack: "", Target: "127.0.0.1:9002"}, guard())
	if err != nil {
		t.Fatal(err)
	}
	if in2.ServerStack != "system" {
		t.Errorf("empty stack should default to system, got %q", in2.ServerStack)
	}
}

func TestValidateSystemHost_Rejections(t *testing.T) {
	g := guard()
	cases := map[string]SystemHostInput{
		"empty host":        {Host: "", ServerStack: "phalcon", Target: "127.0.0.1:8002"},
		"protected domain":  {Host: "app.propertyboom.co", ServerStack: "phalcon", Target: "127.0.0.1:8002"},
		"empty target":      {Host: "x.com", ServerStack: "phalcon", Target: ""},
		"target not hostpt": {Host: "x.com", ServerStack: "phalcon", Target: "not-a-host-port"},
		"target is url":     {Host: "x.com", ServerStack: "phalcon", Target: "http://127.0.0.1:8002"},
	}
	for name, in := range cases {
		if _, err := ValidateSystemHost(in, g); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestValidateRedirect_OK_DefaultsCode(t *testing.T) {
	in, err := ValidateRedirect(RedirectInput{Host: "Old.com", Target: "https://new.com", Code: 0}, guard())
	if err != nil {
		t.Fatal(err)
	}
	if in.Host != "old.com" || in.Code != 301 {
		t.Errorf("expected normalized host + default 301; got %+v", in)
	}
}

func TestValidateRedirect_Rejections(t *testing.T) {
	g := guard()
	cases := map[string]RedirectInput{
		"empty host":      {Host: "", Target: "https://new.com", Code: 301},
		"protected":       {Host: "app.propertyboom.co", Target: "https://new.com", Code: 301},
		"empty target":    {Host: "x.com", Target: "", Code: 301},
		"relative target": {Host: "x.com", Target: "/somewhere", Code: 301},
		"bad code":        {Host: "x.com", Target: "https://new.com", Code: 307},
		"self redirect":   {Host: "x.com", Target: "https://x.com/moved", Code: 301},
	}
	for name, in := range cases {
		if _, err := ValidateRedirect(in, g); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}
