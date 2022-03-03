package snapshotcreator

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/iotaledger/goshimmer/client/wallet"
	"github.com/iotaledger/goshimmer/client/wallet/packages/address"
	"github.com/iotaledger/goshimmer/client/wallet/packages/seed"
	"github.com/iotaledger/goshimmer/packages/consensus/gof"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/mana"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/hive.go/bitmask"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/identity"
)

// Pledge defines a pledge to a node.
type Pledge struct {
	// Target address, if nil, a random address will be used instead.
	Address ledgerstate.Address
	// Whether to pledge the genesis amount to the genesis address.
	Genesis bool
	// Amount to use for the pledge, if zero, cfgPledgeTokenAmount is used.
	Amount uint64
}

// Genesis holds information about the genesis output.
type Genesis struct {
	// The genesis seed.
	Seed *seed.Seed
	// The genesis token amount.
	Amount uint64
}

//// consensus integration test snapshot, use with cfgGenesisTokenAmount=800000 for mana distribution: 50%, 25%, 25%
//var nodesToPledge = map[string]Pledge{
//	// peer master
//	"EYsaGXnUVA9aTYL9FwYEvoQ8d1HCJveQVL7vogu6pqCP": {Genesis: true},
//	// "CHfU1NUf6ZvUKDQHTG2df53GR7CvuMFtyt7YymJ6DwS3": {}, // faucet
//	// base58:Bk69VaYsRuiAaKn8hK6KxUj45X5dED3ueRtxfYnsh4Q8
//	"3kwsHfLDb7ifuxLbyMZneXq3s5heRWnXKKGPAARJDaUE": func() Pledge {
//		seedBase58 := "CFE7T9hjePFwg2P3Mqf5ELH3syFMReVbWai6qc4rJsff"
//		seedBytes, err := base58.Decode(seedBase58)
//		must(err)
//		return Pledge{
//			Address: seed.NewSeed(seedBytes).Address(0).Address(),
//			Amount:  1600000,
//		}
//	}(),
//	// base58:HUH4rmxUxMZBBtHJ4QM5Ts6s8DP3HnFpChejntnCxto2
//	"9fC9crffh3xYuw3M114ZtxRFxxCFceG8vdq2RAjDVQCK": func() Pledge {
//		seedBase58 := "5qm7UPdKKv3GqHyUQgHX3eS1VwdNsWEr2JWqe2GjDZx3"
//		seedBytes, err := base58.Decode(seedBase58)
//		must(err)
//		return Pledge{
//			Address: seed.NewSeed(seedBytes).Address(0).Address(),
//			Amount:  800000,
//		}
//	}(),
//}

func anyGenesisNodePledge(nodesToPledge map[string]Pledge) bool {
	for _, pledge := range nodesToPledge {
		if pledge.Genesis {
			return true
		}
	}
	return false
}

type TransactionMap map[ledgerstate.TransactionID]ledgerstate.Record
type AccessManaMap map[identity.ID]ledgerstate.AccessMana

func CreateSnapshot(genesisTokenAmount uint64, seedBytes []byte, pledgeTokenAmount uint64, nodesToPledge map[string]Pledge, snapshotFileName string) {
	genesis := createGenesis(genesisTokenAmount, seedBytes)
	if anyGenesisNodePledge(nodesToPledge) {
		printGenesisInfo(genesis)
	}

	// define maps for snapshot
	transactionsMap := make(TransactionMap)
	accessManaMap := make(AccessManaMap)

	pledgeToDefinedNodes(genesis, pledgeTokenAmount, nodesToPledge, transactionsMap, accessManaMap)
	newSnapshot := &ledgerstate.Snapshot{AccessManaByNode: accessManaMap, Transactions: transactionsMap}
	writeSnapshot(snapshotFileName, newSnapshot)
	verifySnapshot(snapshotFileName)
}

func createGenesis(genesisTokenAmount uint64, seedBytes []byte) *Genesis {
	genesisSeed := seed.NewSeed(seedBytes)
	return &Genesis{
		Seed:   genesisSeed,
		Amount: genesisTokenAmount,
	}
}

func writeSnapshot(snapshotFileName string, newSnapshot *ledgerstate.Snapshot) {
	snapshotFile, err := os.OpenFile(snapshotFileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal("unable to create snapshot file", err)
	}

	n, err := newSnapshot.WriteTo(snapshotFile)
	if err != nil {
		log.Fatal("unable to write snapshot content to file", err)
	}

	log.Printf("Bytes written %d", n)
	if err := snapshotFile.Close(); err != nil {
		panic(err)
	}

	log.Printf("created %s, bye", snapshotFileName)
}

func verifySnapshot(snapshotFileName string) {
	snapshotFile, err := os.OpenFile(snapshotFileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal("unable to create snapshot file ", err)
	}

	readSnapshot := &ledgerstate.Snapshot{}
	if _, err = readSnapshot.ReadFrom(snapshotFile); err != nil {
		log.Fatal("unable to read snapshot file ", err)
	}
	if err := snapshotFile.Close(); err != nil {
		panic(err)
	}

	fmt.Println("\n================= read Snapshot ===============")
	fmt.Printf("\n================= %d Snapshot Txs ===============\n", len(readSnapshot.Transactions))
	for key, txRecord := range readSnapshot.Transactions {
		fmt.Println("===== key =", key)
		fmt.Println(txRecord)
	}
	fmt.Printf("\n================= %d Snapshot Access Manas ===============\n", len(readSnapshot.AccessManaByNode))
	for key, accessManaNode := range readSnapshot.AccessManaByNode {
		fmt.Println("===== key =", key)
		fmt.Println(accessManaNode)
	}
}

// pledges the amount of tokens given or genesis amount to defined nodes.
// this function mutates the transaction and access mana maps accordingly.
// only one node is allowed to have the genesis token amount be pledged to.
func pledgeToDefinedNodes(genesis *Genesis, tokensToPledge uint64, nodesToPledge map[string]Pledge, txMap TransactionMap, aManaMap AccessManaMap) {
	randomSeed := seed.NewSeed()
	balances := ledgerstate.NewColoredBalances(map[ledgerstate.Color]uint64{
		ledgerstate.ColorIOTA: tokensToPledge,
	})

	randAddrOutput := ledgerstate.NewSigLockedColoredOutput(balances, randomSeed.Address(0).Address())

	var inputIndex uint16
	var genesisPledged bool
	for pubKeyStr, pledgeCfg := range nodesToPledge {
		var (
			output       = randAddrOutput
			balances     = balances
			pledgeAmount = tokensToPledge
		)

		switch {
		case pledgeCfg.Genesis:
			if genesisPledged {
				log.Fatal("genesis token amount can only be pledged once, check your config")
			}
			pledgeAmount = genesis.Amount
			output = ledgerstate.NewSigLockedColoredOutput(ledgerstate.NewColoredBalances(map[ledgerstate.Color]uint64{
				ledgerstate.ColorIOTA: genesis.Amount,
			}), genesis.Seed.Address(0).Address())
			genesisPledged = true

		case pledgeCfg.Address != nil && pledgeCfg.Amount != 0:
			pledgeAmount = pledgeCfg.Amount
			output = ledgerstate.NewSigLockedColoredOutput(ledgerstate.NewColoredBalances(map[ledgerstate.Color]uint64{
				ledgerstate.ColorIOTA: pledgeCfg.Amount,
			}), pledgeCfg.Address)

		case pledgeCfg.Address != nil:
			output = ledgerstate.NewSigLockedColoredOutput(balances, pledgeCfg.Address)
		}

		pledge(pubKeyStr, pledgeAmount, inputIndex, output, txMap, aManaMap)
		inputIndex++
	}
}

// pledges the amount defined by output to the node ID derived from the given public key.
// the transaction doing the pledging uses the given inputIndex to define the index of the output used in the genesis transaction.
// the corresponding txs and mana maps are mutated with the generated records.
func pledge(pubKeyStr string, tokensPledged uint64, inputIndex uint16, output *ledgerstate.SigLockedColoredOutput, txMap TransactionMap, aManaMap AccessManaMap) (identity.ID, ledgerstate.Record, *ledgerstate.Transaction) {
	pubKey, err := ed25519.PublicKeyFromString(pubKeyStr)
	if err != nil {
		panic(err)
	}
	nodeID := identity.NewID(pubKey)

	tx := ledgerstate.NewTransaction(ledgerstate.NewTransactionEssence(
		0,
		time.Unix(tangle.DefaultGenesisTime, 0),
		nodeID,
		nodeID,
		ledgerstate.NewInputs(ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(ledgerstate.GenesisTransactionID, inputIndex))),
		ledgerstate.NewOutputs(output),
	), ledgerstate.UnlockBlocks{ledgerstate.NewReferenceUnlockBlock(0)})

	record := ledgerstate.Record{
		Essence:        tx.Essence(),
		UnlockBlocks:   tx.UnlockBlocks(),
		UnspentOutputs: []bool{true},
	}

	txMap[tx.ID()] = record
	accessManaRecord := ledgerstate.AccessMana{
		Value:     float64(tokensPledged),
		Timestamp: time.Unix(tangle.DefaultGenesisTime, 0),
	}
	aManaMap[nodeID] = accessManaRecord

	return nodeID, record, tx
}

func printGenesisInfo(genesis *Genesis) {
	mockedConnector := newMockConnector(
		&wallet.Output{
			Address: genesis.Seed.Address(0),
			Object: ledgerstate.NewSigLockedColoredOutput(ledgerstate.NewColoredBalances(map[ledgerstate.Color]uint64{
				ledgerstate.ColorIOTA: genesis.Amount,
			}), &ledgerstate.ED25519Address{}).SetID(ledgerstate.NewOutputID(ledgerstate.GenesisTransactionID, 0)),
		},
	)

	genesisWallet := wallet.New(wallet.Import(genesis.Seed, 1, []bitmask.BitMask{}, wallet.NewAssetRegistry("test")), wallet.GenericConnector(mockedConnector))
	genesisAddress := genesisWallet.Seed().Address(0).Address()

	log.Println("genesis:")
	log.Printf("-> seed (base58): %s", genesisWallet.Seed().String())
	log.Printf("-> output address (base58): %s", genesisAddress.Base58())
	log.Printf("-> output id (base58): %s", ledgerstate.NewOutputID(ledgerstate.GenesisTransactionID, 0))
	log.Printf("-> token amount: %d", genesis.Amount)
}

type mockConnector struct {
	outputs map[address.Address]map[ledgerstate.OutputID]*wallet.Output
}

func (connector *mockConnector) UnspentOutputs(addresses ...address.Address) (outputs wallet.OutputsByAddressAndOutputID, err error) {
	outputs = make(wallet.OutputsByAddressAndOutputID)
	for _, addr := range addresses {
		for outputID, output := range connector.outputs[addr] {
			// If the GoF is not reached we consider the output unspent
			if !output.GradeOfFinalityReached {
				if _, outputsExist := outputs[addr]; !outputsExist {
					outputs[addr] = make(map[ledgerstate.OutputID]*wallet.Output)
				}

				outputs[addr][outputID] = output
			}
		}
	}

	return
}

func newMockConnector(outputs ...*wallet.Output) (connector *mockConnector) {
	connector = &mockConnector{
		outputs: make(map[address.Address]map[ledgerstate.OutputID]*wallet.Output),
	}

	for _, output := range outputs {
		if _, addressExists := connector.outputs[output.Address]; !addressExists {
			connector.outputs[output.Address] = make(map[ledgerstate.OutputID]*wallet.Output)
		}

		connector.outputs[output.Address][output.Object.ID()] = output
	}

	return
}

func (connector *mockConnector) RequestFaucetFunds(addr address.Address, powTarget int) (err error) {
	// generate random transaction id
	return
}

func (connector *mockConnector) SendTransaction(tx *ledgerstate.Transaction) (err error) {
	// mark outputs as spent
	return
}

func (connector *mockConnector) GetAllowedPledgeIDs() (pledgeIDMap map[mana.Type][]string, err error) {
	return
}

func (connector *mockConnector) GetUnspentAliasOutput(addr *ledgerstate.AliasAddress) (output *ledgerstate.AliasOutput, err error) {
	return
}

func (connector *mockConnector) GetTransactionGoF(txID ledgerstate.TransactionID) (gradeOfFinality gof.GradeOfFinality, err error) {
	return
}