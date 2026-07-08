package job

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validJSON = `{"title":"Kochi to Goa","subtitle":"1,200 km by road","script":"Our journey begins.","vehicle":"car"}`

func TestParseValid(t *testing.T) {
	j, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if j.Title != "Kochi to Goa" || j.Subtitle != "1,200 km by road" ||
		j.Script != "Our journey begins." || j.Vehicle != VehicleCar {
		t.Fatalf("unexpected job: %+v", j)
	}
}

func TestParseUnknownFieldsIgnored(t *testing.T) {
	if _, err := Parse([]byte(`{"title":"a","subtitle":"b","script":"c","vehicle":"boat","id":42}`)); err != nil {
		t.Fatalf("unknown fields must be ignored: %v", err)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"bad vehicle":    `{"title":"a","subtitle":"b","script":"c","vehicle":"rocket"}`,
		"empty title":    `{"title":"","subtitle":"b","script":"c","vehicle":"car"}`,
		"empty subtitle": `{"title":"a","subtitle":"","script":"c","vehicle":"car"}`,
		"empty script":   `{"title":"a","subtitle":"b","script":"","vehicle":"car"}`,
		"missing fields": `{"title":"a"}`,
		"not json":       `nope`,
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "job.json")
	if err := os.WriteFile(p, []byte(validJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if j.Vehicle != VehicleCar {
		t.Fatalf("got %+v", j)
	}
}

func TestLoadFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(validJSON))
	}))
	defer srv.Close()
	j, err := Load(srv.URL + "/job.json")
	if err != nil {
		t.Fatalf("Load URL: %v", err)
	}
	if j.Title != "Kochi to Goa" {
		t.Fatalf("got %+v", j)
	}
}

func TestLoadURLNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := Load(srv.URL); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}
