package daemon

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLocalUpExactRequestShapeAndValues(t *testing.T) {
	for _, test := range []struct {
		body string
		want localUpRequest
	}{
		{`{}`, localUpRequest{}},
		{`{"server":"https://control.example","network":"home","name":"laptop","auth_key":"fixture-only","advertise_exit_node":true,"advertise_routes":["10.7.0.0/24","10.8.0.0/24"],"exit_node":"gateway","reset_routing":false}`, localUpRequest{Server: "https://control.example", Network: "home", Name: "laptop", AuthKey: "fixture-only", AdvertiseExit: true, AdvertiseRoutes: []string{"10.7.0.0/24", "10.8.0.0/24"}, UseExitNode: "gateway"}},
		{`{"reset_routing":true}`, localUpRequest{ResetRouting: true}},
		{" \n{\"advertise_routes\":[],\"server\":\"\"}\t ", localUpRequest{}},
	} {
		got, err := decodeLocalUp(json.NewDecoder(strings.NewReader(test.body)))
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("valid local Up request changed: %+v %v", got, err)
		}
	}
}

func TestLocalUpRejectsAmbiguousAndMistypedFields(t *testing.T) {
	for _, body := range []string{"", `null`, `[]`, `true`, `{} {}`, `{} false`, `{"network":"home"`,
		`{"server":"one","server":"two"}`, `{"server":"one","SERVER":"two"}`, `{"SERVER":"other"}`,
		`{"network":"one","network":"two"}`, `{"reset_routing":true,"reset_routing":false}`, `{"advertise_routes":[],"advertise_routes":[]}`,
		`{"server":null}`, `{"network":null}`, `{"name":null}`, `{"auth_key":null}`, `{"exit_node":null}`, `{"reset_routing":null}`, `{"advertise_exit_node":null}`, `{"advertise_routes":null}`,
		`{"advertise_routes":[null]}`, `{"advertise_routes":["10.1.0.0/24",7]}`, `{"advertise_routes":{}}`, `{"advertise_routes":"10.1.0.0/24"}`,
		`{"unexpected":true}`, `{"name":7}`, `{"reset_routing":"true"}`, `{"exit_node":[]}`,
	} {
		if _, err := decodeLocalUp(json.NewDecoder(strings.NewReader(body))); err == nil {
			t.Errorf("ambiguous request accepted: %s", body)
		}
	}
}
