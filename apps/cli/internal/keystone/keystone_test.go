package keystone

import "testing"

func TestTokenURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://h/identity":     "http://h/identity/v3/auth/tokens",
		"http://h/identity/v3/": "http://h/identity/v3/auth/tokens",
		"http://h:5000/v3":      "http://h:5000/v3/auth/tokens",
	} {
		if got := tokenURL(in); got != want {
			t.Errorf("tokenURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEndpoint(t *testing.T) {
	s := &Session{Catalog: []Service{{
		Type: "compute",
		Endpoints: []Endpoint{
			{Interface: "internal", Region: "RegionOne", URL: "http://internal/compute"},
			{Interface: "public", Region: "RegionTwo", URL: "http://two/compute"},
			{Interface: "public", Region: "RegionOne", URL: "http://one/compute"},
		},
	}}}
	tests := []struct{ iface, region, want string }{
		{"", "RegionOne", "http://one/compute"},
		{"publicURL", "RegionOne", "http://one/compute"},
		{"internal", "", "http://internal/compute"},
		{"public", "", "http://two/compute"},
	}
	for _, tt := range tests {
		got, err := s.Endpoint("compute", tt.iface, tt.region)
		if err != nil || got != tt.want {
			t.Errorf("Endpoint(%q, %q) = %q, %v; want %q", tt.iface, tt.region, got, err, tt.want)
		}
	}
	if _, err := s.Endpoint("network", "", ""); err == nil {
		t.Error("want error for a service type missing from the catalog")
	}
}

func TestValidate(t *testing.T) {
	ok := Credentials{AuthURL: "http://h", Username: "u", Password: "p", ProjectName: "admin"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	noProject := ok
	noProject.ProjectName = ""
	if noProject.Validate() == nil {
		t.Error("want error without a project")
	}
	appCred := Credentials{AuthURL: "http://h", AppCredID: "id", AppCredSecret: "s"}
	if err := appCred.Validate(); err != nil {
		t.Error(err)
	}
}
