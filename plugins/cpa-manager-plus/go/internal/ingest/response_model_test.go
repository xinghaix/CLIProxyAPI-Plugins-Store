package ingest

import (
	"fmt"
	"testing"
)

func TestDecodeEventResponseModel(t *testing.T) {
	for _, tc := range []struct{ name, fields, want string }{
		{"missing", "", ""},
		{"pascal", ",\"ResponseModel\":\" upstream-version \"", "upstream-version"},
		{"snake", ",\"response_model\":\" upstream-version \"", "upstream-version"},
		{"same as billed", ",\"ResponseModel\":\"billed\"", "billed"},
		{"blank", ",\"ResponseModel\":\"  \"", ""},
		{"null", ",\"ResponseModel\":null", ""},
		{"precedence", ",\"ResponseModel\":\"preferred\",\"response_model\":\"fallback\"", "preferred"},
		{"explicit blank wins", ",\"ResponseModel\":\" \",\"response_model\":\"fallback\"", ""},
		{"null fallback", ",\"ResponseModel\":null,\"response_model\":\"fallback\"", "fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := `{"Model":" billed ","Alias":" requested ","RequestedAt":"2026-01-01T00:00:00Z","Detail":{"InputTokens":5,"OutputTokens":3},"ResponseHeaders":{"X-Model":["not-authoritative"]}`
			legacy, err := DecodeEvent([]byte(base + "}"))
			if err != nil {
				t.Fatal(err)
			}
			event, err := DecodeEvent([]byte(base + tc.fields + "}"))
			if err != nil {
				t.Fatal(err)
			}
			if event.ResponseModel != tc.want || event.Model != "billed" || event.Alias != "requested" || event.TotalTokens != 8 {
				t.Fatalf("unexpected event: %#v", event)
			}
			if event.Hash != legacy.Hash {
				t.Fatal("response metadata changed event identity")
			}
		})
	}
}

func TestDecodeEventRejectsInvalidResponseModel(t *testing.T) {
	for _, key := range []string{"ResponseModel", "response_model"} {
		for _, value := range []string{"42", "{}", "[]", "true"} {
			if _, err := DecodeEvent([]byte(fmt.Sprintf(`{"Model":"billed",%q:%s}`, key, value))); err == nil {
				t.Fatalf("accepted non-string %s=%s", key, value)
			}
		}
	}
}
