package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BlockchainQueryTool queries blockchain networks via JSON-RPC and REST APIs.
// Supports EVM-compatible chains (Ethereum, Base, Polygon, etc.) and custom chains.
type BlockchainQueryTool struct {
	timeout time.Duration
	client  *http.Client
}

// Well-known public RPC endpoints (no API key required).
var defaultRPCEndpoints = map[string]string{
	"ethereum": "https://eth.llamarpc.com",
	"base":     "https://mainnet.base.org",
	"polygon":  "https://polygon-rpc.com",
	"arbitrum": "https://arb1.arbitrum.io/rpc",
	"optimism": "https://mainnet.optimism.io",
	"sepolia":  "https://rpc.sepolia.org",
}

func NewBlockchainQueryTool() *BlockchainQueryTool {
	return &BlockchainQueryTool{
		timeout: 15 * time.Second,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *BlockchainQueryTool) Name() string {
	return "blockchain_query"
}

func (t *BlockchainQueryTool) Description() string {
	return "Query blockchain data: balances, transaction details, block info, and gas prices. Supports EVM chains (Ethereum, Base, Polygon, Arbitrum, Optimism) and custom RPC endpoints."
}

func (t *BlockchainQueryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"balance", "transaction", "block", "gas_price", "block_number"},
				"description": "Type of blockchain query",
			},
			"address": map[string]interface{}{
				"type":        "string",
				"description": "Wallet or contract address (for balance queries)",
			},
			"tx_hash": map[string]interface{}{
				"type":        "string",
				"description": "Transaction hash (for transaction queries)",
			},
			"block": map[string]interface{}{
				"type":        "string",
				"description": "Block number or 'latest' (for block queries)",
			},
			"network": map[string]interface{}{
				"type":        "string",
				"description": "Network name (ethereum, base, polygon, arbitrum, optimism, sepolia) or a custom RPC URL",
			},
		},
		"required": []string{"query_type"},
	}
}

func (t *BlockchainQueryTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	queryType, _ := args["query_type"].(string)
	if queryType == "" {
		return ErrorResult("query_type is required")
	}

	network, _ := args["network"].(string)
	if network == "" {
		network = "ethereum"
	}

	rpcURL := t.resolveRPC(network)
	if rpcURL == "" {
		return ErrorResult(fmt.Sprintf("Unknown network %q. Use: ethereum, base, polygon, arbitrum, optimism, sepolia, or provide a full RPC URL.", network))
	}

	switch queryType {
	case "balance":
		return t.queryBalance(ctx, rpcURL, args)
	case "transaction":
		return t.queryTransaction(ctx, rpcURL, args)
	case "block":
		return t.queryBlock(ctx, rpcURL, args)
	case "gas_price":
		return t.queryGasPrice(ctx, rpcURL)
	case "block_number":
		return t.queryBlockNumber(ctx, rpcURL)
	default:
		return ErrorResult(fmt.Sprintf("Unknown query_type %q", queryType))
	}
}

func (t *BlockchainQueryTool) resolveRPC(network string) string {
	if strings.HasPrefix(network, "http://") || strings.HasPrefix(network, "https://") {
		return network
	}
	return defaultRPCEndpoints[strings.ToLower(network)]
}

// JSON-RPC request/response types.
type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (t *BlockchainQueryTool) rpcCall(ctx context.Context, rpcURL, method string, params []interface{}) (json.RawMessage, error) {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RPC call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (t *BlockchainQueryTool) queryBalance(ctx context.Context, rpcURL string, args map[string]interface{}) *ToolResult {
	address, _ := args["address"].(string)
	if address == "" {
		return ErrorResult("address is required for balance queries")
	}
	if !strings.HasPrefix(address, "0x") {
		return ErrorResult("address must start with 0x")
	}

	result, err := t.rpcCall(ctx, rpcURL, "eth_getBalance", []interface{}{address, "latest"})
	if err != nil {
		return ErrorResult(fmt.Sprintf("Balance query failed: %v", err))
	}

	var hexBalance string
	if err := json.Unmarshal(result, &hexBalance); err != nil {
		return ErrorResult(fmt.Sprintf("Parse balance: %v", err))
	}

	weiBalance := hexToDecimal(hexBalance)
	ethBalance := weiToEth(weiBalance)

	return UserResult(fmt.Sprintf("Address: %s\nBalance: %s ETH\nWei: %s", address, ethBalance, weiBalance))
}

func (t *BlockchainQueryTool) queryTransaction(ctx context.Context, rpcURL string, args map[string]interface{}) *ToolResult {
	txHash, _ := args["tx_hash"].(string)
	if txHash == "" {
		return ErrorResult("tx_hash is required for transaction queries")
	}

	result, err := t.rpcCall(ctx, rpcURL, "eth_getTransactionByHash", []interface{}{txHash})
	if err != nil {
		return ErrorResult(fmt.Sprintf("Transaction query failed: %v", err))
	}

	if string(result) == "null" {
		return ErrorResult("Transaction not found")
	}

	var tx map[string]interface{}
	if err := json.Unmarshal(result, &tx); err != nil {
		return ErrorResult(fmt.Sprintf("Parse transaction: %v", err))
	}

	from, _ := tx["from"].(string)
	to, _ := tx["to"].(string)
	value, _ := tx["value"].(string)
	blockNum, _ := tx["blockNumber"].(string)

	ethValue := weiToEth(hexToDecimal(value))

	return UserResult(fmt.Sprintf("Tx: %s\nFrom: %s\nTo: %s\nValue: %s ETH\nBlock: %s",
		txHash, from, to, ethValue, blockNum))
}

func (t *BlockchainQueryTool) queryBlock(ctx context.Context, rpcURL string, args map[string]interface{}) *ToolResult {
	block, _ := args["block"].(string)
	if block == "" {
		block = "latest"
	}

	var blockParam string
	if block == "latest" || block == "pending" || block == "earliest" {
		blockParam = block
	} else {
		if !strings.HasPrefix(block, "0x") {
			blockParam = "0x" + fmt.Sprintf("%x", mustParseInt(block))
		} else {
			blockParam = block
		}
	}

	result, err := t.rpcCall(ctx, rpcURL, "eth_getBlockByNumber", []interface{}{blockParam, false})
	if err != nil {
		return ErrorResult(fmt.Sprintf("Block query failed: %v", err))
	}

	if string(result) == "null" {
		return ErrorResult("Block not found")
	}

	var blk map[string]interface{}
	if err := json.Unmarshal(result, &blk); err != nil {
		return ErrorResult(fmt.Sprintf("Parse block: %v", err))
	}

	number, _ := blk["number"].(string)
	timestamp, _ := blk["timestamp"].(string)
	txCount := 0
	if txs, ok := blk["transactions"].([]interface{}); ok {
		txCount = len(txs)
	}
	miner, _ := blk["miner"].(string)
	gasUsed, _ := blk["gasUsed"].(string)

	ts := hexToDecimal(timestamp)
	unixTS := mustParseInt(ts)
	timeStr := time.Unix(int64(unixTS), 0).UTC().Format(time.RFC3339)

	return UserResult(fmt.Sprintf("Block: %s (%s)\nTimestamp: %s\nTransactions: %d\nMiner: %s\nGas Used: %s",
		number, hexToDecimal(number), timeStr, txCount, miner, hexToDecimal(gasUsed)))
}

func (t *BlockchainQueryTool) queryGasPrice(ctx context.Context, rpcURL string) *ToolResult {
	result, err := t.rpcCall(ctx, rpcURL, "eth_gasPrice", []interface{}{})
	if err != nil {
		return ErrorResult(fmt.Sprintf("Gas price query failed: %v", err))
	}

	var hexPrice string
	if err := json.Unmarshal(result, &hexPrice); err != nil {
		return ErrorResult(fmt.Sprintf("Parse gas price: %v", err))
	}

	weiPrice := hexToDecimal(hexPrice)
	gweiPrice := weiToGwei(weiPrice)

	return UserResult(fmt.Sprintf("Gas Price: %s Gwei (%s wei)", gweiPrice, weiPrice))
}

func (t *BlockchainQueryTool) queryBlockNumber(ctx context.Context, rpcURL string) *ToolResult {
	result, err := t.rpcCall(ctx, rpcURL, "eth_blockNumber", []interface{}{})
	if err != nil {
		return ErrorResult(fmt.Sprintf("Block number query failed: %v", err))
	}

	var hexNum string
	if err := json.Unmarshal(result, &hexNum); err != nil {
		return ErrorResult(fmt.Sprintf("Parse block number: %v", err))
	}

	return UserResult(fmt.Sprintf("Latest Block: %s (%s)", hexToDecimal(hexNum), hexNum))
}

// Hex/decimal conversion helpers — avoid big.Int dependency for portability.
func hexToDecimal(hex string) string {
	hex = strings.TrimPrefix(hex, "0x")
	if hex == "" || hex == "0" {
		return "0"
	}

	// Simple hex to decimal for reasonable values
	var result uint64
	for _, c := range hex {
		result *= 16
		switch {
		case c >= '0' && c <= '9':
			result += uint64(c - '0')
		case c >= 'a' && c <= 'f':
			result += uint64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			result += uint64(c-'A') + 10
		}
	}
	return fmt.Sprintf("%d", result)
}

func weiToEth(wei string) string {
	v := mustParseInt(wei)
	whole := v / 1_000_000_000_000_000_000
	frac := v % 1_000_000_000_000_000_000
	return fmt.Sprintf("%d.%018d", whole, frac)
}

func weiToGwei(wei string) string {
	v := mustParseInt(wei)
	whole := v / 1_000_000_000
	frac := v % 1_000_000_000
	return fmt.Sprintf("%d.%09d", whole, frac)
}

func mustParseInt(s string) uint64 {
	var result uint64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + uint64(c-'0')
		}
	}
	return result
}
