package edgeclient

import (
	"context"
	"crypto/ecdsa"
	"log"
	"math/big"
	"net/url"
	"os"
	"testing"
	"time"

	polytypes "github.com/0xPolygon/polygon-edge/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/maticnetwork/polygon-cli/rpctypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"

	"github.com/joho/godotenv"
)

type Transaction struct {
	Hash string `json:"hash"`
	Type string `json:"type"`
}

// type Transaction edgetypes.Transaction

type RPCResult struct {
	Number       string         `json:"number"`
	Hash         common.Hash    `json:"hash"`
	ParentHash   common.Hash    `json:"parentHash"`
	Transactions []*Transaction `json:"transactions"`
}

type EdgeTest struct {
	EdgeUrl   string
	ctx       context.Context
	evmClient *ethclient.Client
	rpcClient *rpc.Client
}

func (e *EdgeTest) setup() {
	e.EdgeUrl = os.Getenv("EDGE_URL")

	// Parse URL
	parsedUrl, err := url.Parse(e.EdgeUrl)
	if err != nil {
		log.Fatal(err)
	}
	e.ctx = context.Background()

	// Build geth client
	evmClient, err := ethclient.DialContext(e.ctx, parsedUrl.String())
	if err != nil {
		log.Fatal(err)
	}
	e.evmClient = evmClient

	// Build the geth/rpc client
	rpcClient, err := rpc.DialContext(e.ctx, parsedUrl.String())
	if err != nil {
		log.Fatal(err)
	}
	e.rpcClient = rpcClient
}

func NewEdgeTest() EdgeTest {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	e := EdgeTest{}
	e.setup() // REPLACE THIS WITH YOUR Edge Deployment URL
	return e
}

// Returns the block number in which the txn is confirmed.
func (e *EdgeTest) waitForConfirmation(hash common.Hash) *big.Int {
	for {
		time.Sleep(1 * time.Second)
		tx, isPending, err := e.evmClient.TransactionByHash(e.ctx, hash)
		if err != nil {
			log.Fatal(err)
		}

		if !isPending {
			receipt, err := e.evmClient.TransactionReceipt(e.ctx, hash)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("tx confirmed: %s at blocknumber %d", tx.Hash().String(), receipt.BlockNumber.Uint64())
			return receipt.BlockNumber
		}
	}
}

func TestBlockHash(t *testing.T) {
	numBlocks := 10

	edge := NewEdgeTest()

	// We iterate through first 10 blocks and assert that a blocks parenthash matches the previous blocks hash
	// We also assert that we get the same result for block hash using the rpc
	var rpcResult RPCResult
	expectedParentBlockHash := common.HexToHash("0x0")
	for i := 0; i < numBlocks; i++ {
		// Get data from geth client
		log.Printf("Testing block #%d...", i)
		clientResult, err := edge.evmClient.BlockByNumber(edge.ctx, big.NewInt(int64(i)))
		require.NoError(t, err)

		// Get data from RPC
		err = edge.rpcClient.CallContext(edge.ctx, &rpcResult, "eth_getBlockByNumber", "0x"+big.NewInt(int64(i)).Text(16), true)
		require.NoError(t, err)

		assert.Equal(t, expectedParentBlockHash, rpcResult.ParentHash, "ethClient hash's not equal")
		assert.Equal(t, expectedParentBlockHash, clientResult.ParentHash(), "RPC hash's not equal")
		// Set current hash to parent hash for next iteration
		// Assuming rpcResult is the source of truth
		expectedParentBlockHash = clientResult.Hash()
	}
}

// Builds & submits txn using evmClient (geth)
// Validates txnHashes by reading hash data directly from the blocks
func TestTxnHash(t *testing.T) {
	edge := NewEdgeTest()

	// Generate Signer
	envPrivKey := os.Getenv("PRIVATE_KEY")
	privateKey, err := crypto.HexToECDSA(envPrivKey)
	if err != nil {
		log.Fatal(err)
	}
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}

	// Get addresses
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := edge.evmClient.PendingNonceAt(edge.ctx, fromAddress)
	if err != nil {
		log.Fatal(err)
	}
	toAddress := common.HexToAddress(os.Getenv("TO_ADDRESS"))
	log.Printf("From Address: %+v", fromAddress)
	log.Printf("To   Address: %+v", toAddress)
	log.Printf("Nonce: %d", nonce)

	// Get current block number
	blockNumber, _ := edge.evmClient.BlockNumber(edge.ctx)
	log.Printf("Current Block Number: %d", blockNumber)

	evmBlock1, _ := edge.evmClient.BlockByNumber(edge.ctx, big.NewInt(int64(blockNumber)))
	log.Printf("Block %d has %d transactions", evmBlock1.Number(), len(evmBlock1.Transactions()))

	// Get Current Balances
	balance, _ := edge.evmClient.BalanceAt(edge.ctx, fromAddress, big.NewInt(int64(blockNumber)))
	log.Printf("From: Available Balance: %+v -- %d", fromAddress, balance)
	balance, _ = edge.evmClient.BalanceAt(edge.ctx, toAddress, big.NewInt(int64(blockNumber)))
	log.Printf("To:   Available Balance: %+v -- %d", toAddress, balance)

	// Build txn params
	value := big.NewInt(1)    // in wei
	gasLimit := uint64(21000) // in units
	gasPrice, _ := edge.evmClient.SuggestGasPrice(edge.ctx)

	// Build Transaction
	var data []byte
	tx := ethtypes.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, data)
	chainID, _ := edge.evmClient.NetworkID(edge.ctx)
	signedTx, _ := ethtypes.SignTx(tx, ethtypes.NewEIP155Signer(chainID), privateKey)

	// Submit Txn
	err = edge.evmClient.SendTransaction(edge.ctx, signedTx)
	if err != nil {
		log.Fatalf("sendTxn: %+v", err)
	}

	// Get TXN hash of submitted
	txn, isPending, err := edge.evmClient.TransactionByHash(edge.ctx, signedTx.Hash())
	if err != nil {
		log.Fatalf("txnHash: %+v", err)
	}
	assert.True(t, isPending, "txn is not pending")
	log.Printf("tx sent: %s at blocknumber %v", txn.Hash(), blockNumber)
	// Wait for the transaction to be included in a block
	confirmedBlockNumber := edge.waitForConfirmation(txn.Hash())

	// Get block data from block with confirmed TXN
	var rpcBlock RPCResult
	evmBlock, err := edge.evmClient.BlockByNumber(edge.ctx, confirmedBlockNumber)
	if err != nil {
		log.Fatalf("EVM: BlockByNumber: %+v", err)
	}
	err = edge.rpcClient.CallContext(edge.ctx, &rpcBlock, "eth_getBlockByNumber", "0x"+confirmedBlockNumber.Text(16), true)
	if err != nil {
		log.Fatalf("RPC: getBlockByNumber: %+v", err)
	}

	// Validate responses have transactions
	if len(rpcBlock.Transactions) == 0 {
		log.Fatalf("RPC: Block has no txns %+d", blockNumber)
	}
	if len(evmBlock.Transactions()) == 0 {
		log.Fatalf("EVM: Block has no txns %+d", blockNumber)
	}
	if len(evmBlock.Transactions()) != len(rpcBlock.Transactions) {
		log.Fatalf("Txn Mismatch: Differign number of txns")
	}

	for i := 0; i < len(rpcBlock.Transactions); i++ {
		log.Printf("RPC hash (txn #%d): %s", i, rpcBlock.Transactions[i].Hash)
		log.Printf("EVM hash (txn #%d): %s", i, evmBlock.Transactions()[i].Hash().String())
		assert.Equal(t, rpcBlock.Transactions[i].Hash, evmBlock.Transactions()[i].Hash().String())
	}
}

func TestAllBlocks(t *testing.T) {
	edge := NewEdgeTest()
	// Get current block number
	blockNumber, _ := edge.evmClient.BlockNumber(edge.ctx)
	log.Printf("Current Block Number: %d", blockNumber)
	log.Printf("Checking all blocks from 0 to %d", blockNumber)
	for i := uint64(0); i < blockNumber; i++ {
		bigBlockNumber := big.NewInt(int64(i))
		// Get block data from block with confirmed TXN
		var rpcBlock RPCResult
		evmBlock, err := edge.evmClient.BlockByNumber(edge.ctx, bigBlockNumber)
		if err != nil {
			log.Fatalf("EVM: BlockByNumber: %+v", err)
		}
		log.Printf("EVM: BlockNumber %d has %d transactions", i, evmBlock.Transactions().Len())
		for _, m := range evmBlock.Transactions() {
			log.Printf("EVM: Block %d has tx hash %s", i, m.Hash().String())
		}
		err = edge.rpcClient.CallContext(edge.ctx, &rpcBlock, "eth_getBlockByNumber", encodeBig(bigBlockNumber), true)
		if err != nil {
			log.Fatalf("RPC: getBlockByNumber: %+v", err)
		}
		log.Printf("RPC: BlockNumber %d has %d transactions", i, len(rpcBlock.Transactions))
		for _, m := range rpcBlock.Transactions {
			log.Printf("RPC: Block %d has tx hash %s", i, m.Hash)
		}
		for n := 0; n < evmBlock.Transactions().Len(); n++ {
			assert.Equal(t, evmBlock.Transactions()[n].Hash().String(), rpcBlock.Transactions[n].Hash)
		}
	}
}

// EncodeBig encodes bigint as a hex string with 0x prefix.
func encodeBig(bigint *big.Int) string {
	if sign := bigint.Sign(); sign == 0 {
		return "0x0"
	} else if sign > 0 {
		return "0x" + bigint.Text(16)
	} else {
		return "-0x" + bigint.Text(16)[1:]
	}
}

// Running on this commit: https://github.com/0xPolygon/polygon-edge/blob/a0ce3163e2f3b0fd4d0886b1de32197107b26757/types/rlp_marshal.go#L32
func TestEpochBlocks(t *testing.T) {
	edge := NewEdgeTest()
	epochSize := big.NewInt(10) // Every epoch is configured to be 10 blocks

	var rpcBlock rpctypes.RawBlockResponse

	evmBlock, _ := edge.evmClient.BlockByNumber(edge.ctx, epochSize)
	log.Printf("EVM: BlockNumber %d has %d transactions", evmBlock.Number(), evmBlock.Transactions().Len())
	for _, m := range evmBlock.Transactions() {
		log.Printf("EVM: Block %d has tx hash %s", evmBlock.Number(), m.Hash().String())
	}

	edge.rpcClient.CallContext(edge.ctx, &rpcBlock, "eth_getBlockByNumber", encodeBig(evmBlock.Number()), true)

	var evmTxn *ethtypes.Transaction
	var rpcTxn *rpctypes.RawTransactionResponse
	var polyTxn *polytypes.Transaction

	for n := 0; n < evmBlock.Transactions().Len(); n++ {
		evmTxn = evmBlock.Transactions()[n]
		rpcTxn = &rpcBlock.Transactions[n]

		log.Printf("ethClient hash\t\t\t: %+v", evmTxn.Hash().String())         // Use the txnHash given by the ethClient
		log.Printf("RawHash from RPC \t\t: %+v", rpcTxn.Hash.ToHash().String()) // Raw Hash from RPC node

		log.Println("==================== RPC --> PolyTxn ====================") // Should technically equal RawHash from RPC.
		polyTxn = rawToPolyTxn(rpcTxn)
		log.Printf("ArenaHash \t\t\t\t: %+v", arenaHash(polyTxn))                         // Convert rpcTxn -> polyTxn and manually generate hash
		log.Printf("edgeTxn.ComputeHash(): \t: %+v", polyTxn.ComputeHash().Hash.String()) // Use built-in function (should be the same as above)

		log.Println("==================== Geth --> PolyTxn ====================")
		polyTxn = gethToEdgeTxn(evmTxn)
		log.Printf("ArenaHash \t\t\t\t: %+v", arenaHash(polyTxn))                         // Convert rpcTxn -> polyTxn and manually generate hash
		log.Printf("edgeTxn.ComputeHash(): \t: %+v", polyTxn.ComputeHash().Hash.String()) // Use built-in function (should be the same as above)

		assert.Equal(t, evmTxn.Hash().String(), rpcTxn.Hash.ToHash().String())
	}
}

// Converts raw transaction response to a polyedge transaction type.
func rawToPolyTxn(r *rpctypes.RawTransactionResponse) *polytypes.Transaction {
	toAddress := polytypes.BytesToAddress(r.To.ToAddress().Bytes())
	p := &polytypes.Transaction{
		Nonce:    r.Nonce.ToUint64(),
		GasPrice: r.GasPrice.ToBigInt(),
		// EIP1559 things
		// GasTipCap: r.GasTipCap.ToBigInt(),
		// GasFeeCap: r.GasFeeCap.ToBigInt(),
		Gas:   r.Gas.ToUint64(),
		To:    &toAddress,
		Value: r.Value.ToBigInt(),
		Input: r.Input.ToBytes(),
		V:     r.V.ToBigInt(),
		R:     r.R.ToBigInt(),
		S:     r.S.ToBigInt(),
		// From:  polytypes.Address(r.From.ToAddress()),
		Type: polytypes.TxType(r.Type.ToInt64()),
	}

	return p
}

// Takes a geth txn and converts it into a polygon txn type.
func gethToEdgeTxn(gt *ethtypes.Transaction) *polytypes.Transaction {
	toAddress := polytypes.BytesToAddress(gt.To().Bytes())

	p := &polytypes.Transaction{
		Nonce:    gt.Nonce(),
		GasPrice: gt.GasPrice(),
		// EIP1559 things
		// GasTipCap: gt.GasTipCap.ToBigInt(),
		// GasFeeCap: gt.GasFeeCap.ToBigInt(),
		Gas:   gt.Gas(),
		To:    &toAddress,
		Value: gt.Value(),
		Input: gt.Data(),
		V:     big.NewInt(0), //gt.V(), 	   // Missing
		R:     big.NewInt(0), //gt.R(),        // Missing
		S:     big.NewInt(0), //gt.S(),        // Missing
		// From:  polytypes.Address(gt.From.ToAddress()), // Missing
		Type: polytypes.TxType(gt.Type()),
	}
	return p
}

// Copy-pasta of hashing algo used in polygon-edge codebase
// Data that affects the hash
// Nonce
// GasPrice
// Gas
// To (optional: nil)
// Value
// Input
// V, R, S
// Type  (0x0 = legacyTx)
// From  ^ only if Type=StateTx (which epoch commits are not)

func arenaHash(t *polytypes.Transaction) (retHash common.Hash) {
	arena := &fastrlp.Arena{}

	vv := arena.NewArray()

	vv.Set(arena.NewUint(t.Nonce))      //0
	vv.Set(arena.NewBigInt(t.GasPrice)) // 0
	vv.Set(arena.NewUint(t.Gas))        // 1000000

	// Address may be empty
	if t.To != nil {
		vv.Set(arena.NewBytes((*t.To).Bytes())) // 0x0101
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewBigInt(t.Value))    // 0
	vv.Set(arena.NewCopyBytes(t.Input)) // len = 548

	// signature values
	vv.Set(arena.NewBigInt(t.V))
	vv.Set(arena.NewBigInt(t.R))
	vv.Set(arena.NewBigInt(t.S))

	if t.Type == polytypes.StateTx { // 0x0 - Legacy TXN
		vv.Set(arena.NewBytes((t.From).Bytes()))
	}
	sha := sha3.NewLegacyKeccak256()
	buf := vv.MarshalTo(nil)
	sha.Write(buf)

	retHash.SetBytes(sha.Sum(nil))
	return
}
