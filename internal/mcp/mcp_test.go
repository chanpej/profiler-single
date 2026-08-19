package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeInitializesListsAndForwardsTools(t *testing.T) {
	var forwarded []Request
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var request Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		forwarded = append(forwarded, request)
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"tools": []any{}}})
	}))
	defer bridge.Close()

	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n" +
		"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n" +
		"{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"performance_analysis\",\"arguments\":{}}}\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, bridge.URL, "secret"); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("responses = %q", output.String())
	}
	var initialized struct {
		Result struct {
			ServerInfo map[string]string `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized.Result.ServerInfo["name"] != "UnityPerfAgent" {
		t.Fatalf("server info = %+v", initialized.Result.ServerInfo)
	}
	if len(forwarded) != 3 || forwarded[0].Method != "initialize" || forwarded[1].Method != "tools/list" || forwarded[2].Method != "tools/call" {
		t.Fatalf("forwarded = %+v", forwarded)
	}
}

func TestServeReportsMissingBridgeConfiguration(t *testing.T) {
	if err := Serve(context.Background(), strings.NewReader(""), io.Discard, "", "token"); err == nil {
		t.Fatal("expected missing endpoint error")
	}
}
