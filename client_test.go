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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joho/godotenv"
)

type RPCResult struct {
	Number       string      `json:"number"`
	Hash         common.Hash `json:"hash"`
	ParentHash   common.Hash `json:"parentHash"`
	Transactions []struct {
		Hash string `json:"hash"`
	} `json:"transactions"`
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
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, data)
	chainID, _ := edge.evmClient.NetworkID(edge.ctx)
	signedTx, _ := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)

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
		err = edge.rpcClient.CallContext(edge.ctx, &rpcBlock, "eth_getBlockByNumber", "0x"+bigBlockNumber.Text(16), true)
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
