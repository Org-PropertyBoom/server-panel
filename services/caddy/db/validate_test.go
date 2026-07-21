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

func TestValidateSystemHost_Rejections(t *testing.T) {
	g := guard()
	cases := map[string]SystemHostInput{
		"empty host":        {Host: "", ServerStack: "phalcon", Target: "127.0.0.1:8002"},
		"protected domain":  {Host: "app.propertyboom.co", ServerStack: "phalcon", Target: "127.0.0.1:8002"},
		"empty stack":       {Host: "x.com", ServerStack: "", Target: "127.0.0.1:8002"},
		"unknown stack":     {Host: "x.com", ServerStack: "perl", Target: "127.0.0.1:8002"},
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
	}
	for name, in := range cases {
		if _, err := ValidateRedirect(in, g); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}
