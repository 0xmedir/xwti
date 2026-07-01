package contract

import (
	"encoding/binary"
	"math/rand"
)

// oraclePrefix and indicatorPrefix are plugin-owned custom state prefixes.
// Must be >= 16 (1-15 reserved for Canopy core — see AGENTS.md).
// Declared here AND must be added to ContractConfig.CustomStatePrefixes
// in contract.go, or Canopy panics at handshake.
var oraclePrefix = []byte{100}
var indicatorPrefix = []byte{101}

// KeyForPrice returns the state key for the latest WTI price
func KeyForPrice() []byte {
	return oraclePrefix
}

// KeyForIndicator returns the state key for an indicator snapshot,
// keyed by timeframe (e.g. "5m", "1h") so each timeframe has its own slot.
func KeyForIndicator(timeframe string) []byte {
	return JoinLenPrefix(indicatorPrefix, []byte(timeframe))
}

// --- MessageSubmitPrice ---

// CheckMessageSubmitPrice statelessly validates a price submission
func (c *Contract) CheckMessageSubmitPrice(msg *MessageSubmitPrice) *PluginCheckResponse {
	if len(msg.OracleAddress) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	if msg.PriceUsd == 0 {
		return &PluginCheckResponse{Error: ErrInvalidAmount()}
	}
	return &PluginCheckResponse{
		Recipient:         msg.OracleAddress,
		AuthorizedSigners: [][]byte{msg.OracleAddress},
	}
}

// DeliverMessageSubmitPrice applies a price submission to state.
// Solo-oracle mode: any signer that passes CheckTx can submit (the wallet
// key you sign with IS the authorized oracle for now). Multi-oracle mode
// later adds a registered-oracle lookup here instead.
func (c *Contract) DeliverMessageSubmitPrice(msg *MessageSubmitPrice, fee uint64) *PluginDeliverResponse {
	var (
		oracleKey  = KeyForAccount(msg.OracleAddress)
		priceKey   = KeyForPrice()
		feePoolKey = KeyForFeePool(c.Config.ChainId)
		oracleQID  = rand.Uint64()
		feeQID     = rand.Uint64()
	)
	resp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: oracleQID, Key: oracleKey},
			{QueryId: feeQID, Key: feePoolKey},
		},
	})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if resp.Error != nil {
		return &PluginDeliverResponse{Error: resp.Error}
	}
	var oracleBytes, feePoolBytes []byte
	for _, r := range resp.Results {
		if len(r.Entries) == 0 {
			continue
		}
		switch r.QueryId {
		case oracleQID:
			oracleBytes = r.Entries[0].Value
		case feeQID:
			feePoolBytes = r.Entries[0].Value
		}
	}
	oracle := new(Account)
	feePool := new(Pool)
	if err = Unmarshal(oracleBytes, oracle); err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if err = Unmarshal(feePoolBytes, feePool); err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if oracle.Amount < fee {
		return &PluginDeliverResponse{Error: ErrInsufficientFunds()}
	}
	oracle.Amount -= fee
	feePool.Amount += fee

	priceBytes := encodePrice(msg)

	oracleBytes, _ = Marshal(oracle)
	feePoolBytes, _ = Marshal(feePool)
	writeResp, err := c.plugin.StateWrite(c, &PluginStateWriteRequest{
		Sets: []*PluginSetOp{
			{Key: oracleKey, Value: oracleBytes},
			{Key: feePoolKey, Value: feePoolBytes},
			{Key: priceKey, Value: priceBytes},
		},
	})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	return &PluginDeliverResponse{Error: writeResp.Error}
}

// encodePrice packs price fields into bytes for storage:
// priceUSDCents(8) + timestamp(8) + source
func encodePrice(msg *MessageSubmitPrice) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], msg.PriceUsd)
	binary.BigEndian.PutUint64(buf[8:16], uint64(msg.Timestamp))
	buf = append(buf, []byte(msg.Source)...)
	return buf
}

// --- MessageSubmitIndicator ---

// CheckMessageSubmitIndicator statelessly validates an indicator submission
func (c *Contract) CheckMessageSubmitIndicator(msg *MessageSubmitIndicator) *PluginCheckResponse {
	if len(msg.OracleAddress) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	if msg.Timeframe == "" {
		return &PluginCheckResponse{Error: ErrInvalidAmount()}
	}
	if msg.Rsi > 10000 {
		return &PluginCheckResponse{Error: ErrInvalidAmount()}
	}
	return &PluginCheckResponse{
		Recipient:         msg.OracleAddress,
		AuthorizedSigners: [][]byte{msg.OracleAddress},
	}
}

// DeliverMessageSubmitIndicator applies an indicator snapshot to state
func (c *Contract) DeliverMessageSubmitIndicator(msg *MessageSubmitIndicator, fee uint64) *PluginDeliverResponse {
	var (
		oracleKey    = KeyForAccount(msg.OracleAddress)
		indicatorKey = KeyForIndicator(msg.Timeframe)
		feePoolKey   = KeyForFeePool(c.Config.ChainId)
		oracleQID    = rand.Uint64()
		feeQID       = rand.Uint64()
	)
	resp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: oracleQID, Key: oracleKey},
			{QueryId: feeQID, Key: feePoolKey},
		},
	})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if resp.Error != nil {
		return &PluginDeliverResponse{Error: resp.Error}
	}
	var oracleBytes, feePoolBytes []byte
	for _, r := range resp.Results {
		if len(r.Entries) == 0 {
			continue
		}
		switch r.QueryId {
		case oracleQID:
			oracleBytes = r.Entries[0].Value
		case feeQID:
			feePoolBytes = r.Entries[0].Value
		}
	}
	oracle := new(Account)
	feePool := new(Pool)
	if err = Unmarshal(oracleBytes, oracle); err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if err = Unmarshal(feePoolBytes, feePool); err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if oracle.Amount < fee {
		return &PluginDeliverResponse{Error: ErrInsufficientFunds()}
	}
	oracle.Amount -= fee
	feePool.Amount += fee

	indicatorBytes := encodeIndicator(msg)

	oracleBytes, _ = Marshal(oracle)
	feePoolBytes, _ = Marshal(feePool)
	writeResp, err := c.plugin.StateWrite(c, &PluginStateWriteRequest{
		Sets: []*PluginSetOp{
			{Key: oracleKey, Value: oracleBytes},
			{Key: feePoolKey, Value: feePoolBytes},
			{Key: indicatorKey, Value: indicatorBytes},
		},
	})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	return &PluginDeliverResponse{Error: writeResp.Error}
}

// encodeIndicator packs indicator fields into bytes:
// rsi(8) + macd(8) + macdSignal(8) + bbUpper(8) + bbLower(8) + ema20(8) + ema25(8) + timestamp(8)
func encodeIndicator(msg *MessageSubmitIndicator) []byte {
	buf := make([]byte, 64)
	binary.BigEndian.PutUint64(buf[0:8], msg.Rsi)
	binary.BigEndian.PutUint64(buf[8:16], msg.Macd)
	binary.BigEndian.PutUint64(buf[16:24], msg.MacdSignal)
	binary.BigEndian.PutUint64(buf[24:32], msg.BbUpper)
	binary.BigEndian.PutUint64(buf[32:40], msg.BbLower)
	binary.BigEndian.PutUint64(buf[40:48], msg.Ema20)
	binary.BigEndian.PutUint64(buf[48:56], msg.Ema25)
	binary.BigEndian.PutUint64(buf[56:64], uint64(msg.Timestamp))
	return buf
}
