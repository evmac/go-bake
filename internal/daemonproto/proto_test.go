package daemonproto

import (
	"encoding/json"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	cases := []Request{
		{Run: "build"},
		{Up: true},
		{Down: true},
		{Down: true, Daemon: "redis"},
		{},
	}
	for _, req := range cases {
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal %+v: %v", req, err)
		}
		var got Request
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", b, err)
		}
		if got != req {
			t.Errorf("round-trip: got %+v, want %+v", got, req)
		}
	}
}

func TestResponseRoundTrip(t *testing.T) {
	cases := []Response{
		{ExitCode: 0},
		{ExitCode: 1, Error: "something failed"},
		{ExitCode: 2, Error: "missing run, up, or down in request"},
	}
	for _, resp := range cases {
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal %+v: %v", resp, err)
		}
		var got Response
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", b, err)
		}
		if got != resp {
			t.Errorf("round-trip: got %+v, want %+v", got, resp)
		}
	}
}

func TestRequestOmitEmpty(t *testing.T) {
	req := Request{Run: "build"}
	b, _ := json.Marshal(req)
	s := string(b)
	if contains(s, "up") || contains(s, "down") || contains(s, "daemon") {
		t.Errorf("omitempty should hide zero fields: %s", s)
	}
}

func TestResponseOmitEmpty(t *testing.T) {
	resp := Response{ExitCode: 0}
	b, _ := json.Marshal(resp)
	s := string(b)
	if contains(s, "error") {
		t.Errorf("omitempty should hide zero error: %s", s)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
