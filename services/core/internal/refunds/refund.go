package refunds

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var erc20ABI, _ = abi.JSON(strings.NewReader(`[{"name":"transfer","type":"function","stateMutability":"nonpayable","inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"name":"","type":"bool"}]}]`))

type Sender struct {
	rpc     *ethclient.Client
	auth    *bind.TransactOpts
	from    common.Address
	chainID *big.Int
	// nonceMu serializes nonce allocation through send so concurrent
	// Transfer calls each get a unique nonce.
	nonceMu sync.Mutex
}

func NewSender(rpcURL, privateKey, payout string, chainID int64) (*Sender, error) {
	if privateKey == "" {
		return nil, nil
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid refund operator key: %w", err)
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	if !common.IsHexAddress(payout) || !strings.EqualFold(from.Hex(), payout) {
		return nil, fmt.Errorf("refund operator key does not control PAYOUT_WALLET")
	}
	rpc, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connect refund RPC: %w", err)
	}
	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(chainID))
	if err != nil {
		rpc.Close()
		return nil, err
	}
	return &Sender{rpc: rpc, auth: auth, from: from, chainID: big.NewInt(chainID)}, nil
}

func (s *Sender) Transfer(ctx context.Context, asset, payer, amount, attributionTag string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("automatic refunds are not configured")
	}
	if !common.IsHexAddress(asset) || !common.IsHexAddress(payer) {
		return "", fmt.Errorf("invalid refund asset or payer address")
	}
	value, ok := new(big.Int).SetString(amount, 10)
	if !ok || value.Sign() <= 0 {
		return "", fmt.Errorf("invalid refund amount")
	}
	data, err := erc20ABI.Pack("transfer", common.HexToAddress(payer), value)
	if err != nil {
		return "", err
	}
	data = append(data, attributionSuffix(attributionTag)...)
	gasPrice, err := s.rpc.SuggestGasPrice(ctx)
	if err != nil {
		return "", err
	}

	// Serialize nonce allocation through send so concurrent refunds each
	// get a unique nonce.  WaitMined is outside the lock: it does not
	// allocate a nonce and may block for many seconds.
	s.nonceMu.Lock()
	nonce, err := s.rpc.PendingNonceAt(ctx, s.from)
	if err != nil {
		s.nonceMu.Unlock()
		return "", err
	}
	tx := types.NewTransaction(nonce, common.HexToAddress(asset), big.NewInt(0), 100000, gasPrice, data)
	signed, err := s.auth.Signer(s.from, tx)
	if err != nil {
		s.nonceMu.Unlock()
		return "", err
	}
	sendErr := s.rpc.SendTransaction(ctx, signed)
	s.nonceMu.Unlock()
	if sendErr != nil {
		return "", sendErr
	}
	receipt, err := bind.WaitMined(ctx, s.rpc, signed)
	if err != nil {
		return "", fmt.Errorf("wait for refund transaction %s: %w", signed.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return "", fmt.Errorf("refund transaction %s failed on-chain", signed.Hash().Hex())
	}
	mined, _, err := s.rpc.TransactionByHash(ctx, signed.Hash())
	if err != nil {
		return "", fmt.Errorf("read mined refund transaction %s: %w", signed.Hash().Hex(), err)
	}
	if !bytes.HasSuffix(mined.Data(), attributionSuffix(attributionTag)) {
		return "", fmt.Errorf("refund transaction %s is missing attribution suffix", signed.Hash().Hex())
	}
	return signed.Hash().Hex(), nil
}

func attributionSuffix(code string) []byte {
	codeBytes := []byte(code)
	if len(codeBytes) == 0 || len(codeBytes) > 255 {
		return nil
	}
	marker := []byte{0x80, 0x21, 0x80, 0x21, 0x80, 0x21, 0x80, 0x21, 0x80, 0x21, 0x80, 0x21, 0x80, 0x21, 0x80, 0x21}
	return append(append(append([]byte{}, codeBytes...), byte(len(codeBytes))), append([]byte{0}, marker...)...)
}
