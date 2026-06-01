package tools

import (
	"context"
	"testing"
)

func TestBlockchainQueryTool_Name(t *testing.T) {
	tool := NewBlockchainQueryTool()
	if tool.Name() != "blockchain_query" {
		t.Errorf("expected name 'blockchain_query', got %q", tool.Name())
	}
}

func TestBlockchainQueryTool_Parameters(t *testing.T) {
	tool := NewBlockchainQueryTool()
	params := tool.Parameters()

	if params["type"] != "object" {
		t.Error("expected type 'object'")
	}

	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}

	required := []string{"query_type", "address", "network", "tx_hash", "block"}
	for _, key := range required {
		if _, exists := props[key]; !exists {
			t.Errorf("missing parameter %q", key)
		}
	}
}

func TestBlockchainQueryTool_MissingQueryType(t *testing.T) {
	tool := NewBlockchainQueryTool()
	result := tool.Execute(context.Background(), map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for missing query_type")
	}
}

func TestBlockchainQueryTool_UnknownNetwork(t *testing.T) {
	tool := NewBlockchainQueryTool()
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query_type": "balance",
		"address":    "0x0000000000000000000000000000000000000000",
		"network":    "fakenet",
	})
	if !result.IsError {
		t.Error("expected error for unknown network")
	}
}

func TestBlockchainQueryTool_MissingAddress(t *testing.T) {
	tool := NewBlockchainQueryTool()
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query_type": "balance",
		"network":    "ethereum",
	})
	if !result.IsError {
		t.Error("expected error for missing address")
	}
}

func TestBlockchainQueryTool_InvalidAddress(t *testing.T) {
	tool := NewBlockchainQueryTool()
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query_type": "balance",
		"address":    "not-a-hex-address",
		"network":    "ethereum",
	})
	if !result.IsError {
		t.Error("expected error for invalid address format")
	}
}

func TestBlockchainQueryTool_MissingTxHash(t *testing.T) {
	tool := NewBlockchainQueryTool()
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query_type": "transaction",
		"network":    "ethereum",
	})
	if !result.IsError {
		t.Error("expected error for missing tx_hash")
	}
}

func TestBlockchainQueryTool_ResolveCustomRPC(t *testing.T) {
	tool := NewBlockchainQueryTool()
	url := tool.resolveRPC("https://my-custom-node.example.com/rpc")
	if url != "https://my-custom-node.example.com/rpc" {
		t.Errorf("custom RPC URL should pass through, got %q", url)
	}
}

func TestBlockchainQueryTool_ResolveKnownNetworks(t *testing.T) {
	tool := NewBlockchainQueryTool()
	networks := []string{"ethereum", "base", "polygon", "arbitrum", "optimism", "sepolia"}
	for _, net := range networks {
		url := tool.resolveRPC(net)
		if url == "" {
			t.Errorf("expected RPC URL for network %q", net)
		}
	}
}

func TestHexToDecimal(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0x0", "0"},
		{"0xa", "10"},
		{"0xff", "255"},
		{"0x3e8", "1000"},
		{"0xde0b6b3a7640000", "1000000000000000000"},
	}
	for _, tc := range tests {
		result := hexToDecimal(tc.input)
		if result != tc.expected {
			t.Errorf("hexToDecimal(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestWeiToEth(t *testing.T) {
	result := weiToEth("1000000000000000000")
	if result != "1.000000000000000000" {
		t.Errorf("weiToEth(1e18) = %q, want '1.000000000000000000'", result)
	}
}
