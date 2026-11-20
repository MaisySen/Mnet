package multies

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBridgeJSONSchemaWalk(t *testing.T) {
	type nested struct {
		Key string `json:"key"`
	}
	type response struct {
		Items  []nested  `json:"items"`
		At     time.Time `json:"at"`
		Flag   *bool     `json:"flag"`
		Number int       `json:"number"`
	}
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"items":[{"key":"x"}],"at":"2026-11-21T00:00:00Z","flag":false,"number":1}`, true},
		{`{"items":null}`, true},
		{`{"items":[{"key":"x","key":"y"}]}`, false},
		{`{"items":[{"Key":"x"}]}`, false},
		{`{"items":[{"key":"x","other":1}]}`, false},
		{`{"items":[null]}`, false},
		{`{"at":{}}`, false},
		{`{"flag":null}`, false},
		{`{"number":"1"}`, false},
		{`{"number":1,"\u006eumber":2}`, false},
		{`{"number":1} {}`, false},
		{`[]`, false},
		{`null`, false},
		{`{"items":[]`, false},
	} {
		var out response
		if got := validateBridgeJSON([]byte(tc.body), &out); got != tc.want {
			t.Errorf("schema check %s: got %v", tc.body, got)
		}
	}
	if validateBridgeJSON([]byte(`{}`), nil) || validateBridgeJSON([]byte(`{}`), response{}) {
		t.Fatal("invalid output shape")
	}
}

func FuzzBridgeJSONValidation(f *testing.F) {
	f.Add([]byte(`{"workspaceId":"workspace","devices":[],"ownDevicesOnly":true,"inputAuthorized":false,"truncated":false}`))
	f.Add([]byte(`{"devices":[{"id":"a","id":"b"}]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 65536 {
			return
		}
		var out ApprovedDeviceList
		if validateBridgeJSON(raw, &out) {
			if !json.Valid(raw) {
				t.Fatal("accepted invalid JSON")
			}
			if json.Unmarshal(raw, &out) != nil {
				t.Fatal("accepted undecodable schema")
			}
		}
	})
}
