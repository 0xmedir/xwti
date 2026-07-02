package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/canopy-network/go-plugin/contract"
	"github.com/canopy-network/go-plugin/crypto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	queryRPCURL = "http://localhost:50002"
	adminRPCURL = "http://localhost:50003"
	networkID   = uint64(1)
	chainID     = uint64(1)
	password    = "testpassword123"
	nickname    = "xwti_oracle_test"
)

type keyGroup struct {
	Address    string `json:"address"`
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

func main() {
	address, err := getAddressForNickname(adminRPCURL, nickname, password)
	if err != nil {
		panic(fmt.Sprintf("failed to get address: %v", err))
	}
	fmt.Printf("Using oracle account: %s\n", address)

	key, err := keystoreGetKey(adminRPCURL, address, password)
	if err != nil {
		panic(fmt.Sprintf("failed to get key: %v", err))
	}

	height, err := getHeight(queryRPCURL)
	if err != nil {
		panic(fmt.Sprintf("failed to get height: %v", err))
	}
	fmt.Printf("Current height: %d\n", height)

	txHash, err := sendSubmitPriceTx(queryRPCURL, key, 7245, "manual_test", networkID, chainID, height)
	if err != nil {
		panic(fmt.Sprintf("failed to send submit_price tx: %v", err))
	}
	fmt.Printf("submit_price tx sent: %s\n", txHash)
}

func getAddressForNickname(rpcURL, nickname, password string) (string, error) {
	reqJSON := fmt.Sprintf(`{"nickname":"%s","password":"%s"}`, nickname, password)
	respBody, err := postRawJSON(rpcURL+"/v1/admin/keystore-get", reqJSON)
	if err != nil {
		return "", err
	}
	var kg keyGroup
	if err := json.Unmarshal(respBody, &kg); err != nil {
		return "", fmt.Errorf("failed to parse response: %v, body: %s", err, string(respBody))
	}
	return kg.Address, nil
}

func sendSubmitPriceTx(rpcURL string, signerKey *keyGroup, priceUsdCents uint64, source string, networkID, chainID, height uint64) (string, error) {
	signerAddrBytes, err := hex.DecodeString(signerKey.Address)
	if err != nil {
		return "", fmt.Errorf("bad address hex: %v", err)
	}

	msgProto := &contract.MessageSubmitPrice{
		OracleAddress: signerAddrBytes,
		PriceUsd:      priceUsdCents,
		Timestamp:     time.Now().Unix(),
		Source:        source,
	}

	typeURL := "type.googleapis.com/types.MessageSubmitPrice"

	msgBytes, err := proto.Marshal(msgProto)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %v", err)
	}

	msgAny := &anypb.Any{TypeUrl: typeURL, Value: msgBytes}

	fee := uint64(10000)
	txTime := uint64(time.Now().UnixMicro())

	signBytes, err := crypto.GetSignBytes("submit_price", msgAny, txTime, height, fee, "", networkID, chainID)
	if err != nil {
		return "", fmt.Errorf("failed to get sign bytes: %v", err)
	}

	privKey, err := crypto.StringToBLS12381PrivateKey(signerKey.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %v", err)
	}
	signature := privKey.Sign(signBytes)

	pubKeyBytes, err := hex.DecodeString(signerKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode public key: %v", err)
	}

	tx := map[string]interface{}{
		"type":       "submit_price",
		"msgTypeUrl": typeURL,
		"msgBytes":   hex.EncodeToString(msgBytes),
		"signature": map[string]string{
			"publicKey": hex.EncodeToString(pubKeyBytes),
			"signature": hex.EncodeToString(signature),
		},
		"time":          txTime,
		"createdHeight": height,
		"fee":           fee,
		"memo":          "",
		"networkID":     networkID,
		"chainID":       chainID,
	}

	txJSONBytes, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal transaction: %v", err)
	}

	respBody, err := postRawJSON(rpcURL+"/v1/tx", string(txJSONBytes))
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %v", err)
	}

	var txHash string
	if err := json.Unmarshal(respBody, &txHash); err != nil {
		return "", fmt.Errorf("failed to parse response: %v, body: %s", err, string(respBody))
	}
	return txHash, nil
}

func keystoreGetKey(rpcURL, address, password string) (*keyGroup, error) {
	reqJSON := fmt.Sprintf(`{"address":"%s","password":"%s"}`, address, password)
	respBody, err := postRawJSON(rpcURL+"/v1/admin/keystore-get", reqJSON)
	if err != nil {
		return nil, err
	}
	var kg keyGroup
	if err := json.Unmarshal(respBody, &kg); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v, body: %s", err, string(respBody))
	}
	return &kg, nil
}

func getHeight(rpcURL string) (uint64, error) {
	respBody, err := postRawJSON(rpcURL+"/v1/query/height", "{}")
	if err != nil {
		return 0, err
	}
	var result struct {
		Height uint64 `json:"height"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %v", err)
	}
	return result.Height, nil
}

func postRawJSON(url string, jsonBody string) ([]byte, error) {
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
