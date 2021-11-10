package consensus

import (
	"context"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/crypto/ed25519"

	"github.com/iotaledger/goshimmer/packages/consensus/gof"
	"github.com/iotaledger/goshimmer/packages/jsonmodels"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/tools/integration-tests/tester/framework"
	"github.com/iotaledger/goshimmer/tools/integration-tests/tester/tests"
	"github.com/stretchr/testify/require"
)

// constant var, shouldn't be changed
var tokensPerRequest int

func TestConflictSpam(t *testing.T) {
	ctx, cancel := tests.Context(context.Background(), t)
	defer cancel()
	n, err := f.CreateNetwork(ctx, t.Name(), 4, framework.CreateNetworkConfig{
		Faucet:      true,
		StartSynced: true,
		Activity:    true,
	}, tests.EqualDefaultConfigFunc(t, false))
	require.NoError(t, err)
	defer tests.ShutdownNetwork(ctx, t, n)

	faucet := n.Peers()[0]
	tokensPerRequest = faucet.Config().TokensPerRequest

	tests.AwaitInitialFaucetOutputsPrepared(t, faucet, n.Peers())

	txs := []*ledgerstate.Transaction{}
	for i := 0; i < 3; i++ {
		sendPairWiseConflicts(t, n.Peers(), &txs, i)
	}
	t.Logf("number of txs to verify is %d", len(txs))
	verifyConfirmationsOnPeers(t, n.Peers(), txs)
}

func verifyConfirmationsOnPeers(t *testing.T, peers []*framework.Node, txs []*ledgerstate.Transaction) {
	const unknownGoF = 10
	for _, tx := range txs {
		// current value signifies that we don't know what is the previous gof
		var prevGoF gof.GradeOfFinality = unknownGoF
		for i, peer := range peers {
			var metadata *jsonmodels.TransactionMetadata
			var err error
			require.Eventually(t, func() bool {
				metadata, err = peer.GetTransactionMetadata(tx.ID().Base58())
				return err == nil && metadata != nil
			}, 10*time.Second, time.Second, "Peer %s can't fetch metadata of tx %s. metadata is %v. Error is %w",
				peer.Name(), tx.ID().Base58(), metadata, err)
			t.Logf("GoF is %d for tx %s in peer %s", metadata.GradeOfFinality, tx.ID().Base58(), peer.Name())
			if prevGoF != unknownGoF {
				require.EqualValues(t, prevGoF, metadata.GradeOfFinality,
					"Different gofs on tx %s between peers %s and %s", tx.ID().Base58(),
					peers[i-1].Name(), peer.Name())
			}
			prevGoF = metadata.GradeOfFinality
		}
	}
}

/**
sendPairWiseConflicts receives a list of outputs controlled by a peer with certain peer index.
It send them all to addresses controlled by the next peer, but it does so several time to create pairwise conflicts.
*/
func sendPairWiseConflicts(t *testing.T, peers []*framework.Node, txs *[]*ledgerstate.Transaction, iteration int) {
	if iteration == 3 {
		t.Logf("done sending pairwise conflicts")
		return
	}

	t.Logf("send pairwise conflicts on iteration %d", iteration)

	peerIndex := iteration % len(peers)
	originPeer := peers[peerIndex]
	originAddressIndex := iteration*3 + 4
	originAddress := originPeer.Address(originAddressIndex)
	tests.SendFaucetRequest(t, originPeer, originAddress)

	require.Eventually(t, func() bool {
		return tests.Balance(t, originPeer, originAddress, ledgerstate.ColorIOTA) >= uint64(tokensPerRequest)
	}, tests.Timeout, tests.Tick)

	outputs := getOutputsControlledBy(t, originPeer, originAddress)
	keyPair := originPeer.KeyPair(uint64(originAddressIndex))
	keyPairs := map[string]*ed25519.KeyPair{originAddress.String(): keyPair}
	targetIndex := (iteration + 1) % len(peers)
	targetPeer := peers[targetIndex]
	targetAddresses := []*ledgerstate.Address{}

	for i := iteration * 3; i < iteration*3+3; i++ {
		targetAddress := targetPeer.Address(i)
		targetAddresses = append(targetAddresses, &targetAddress)
	}

	tx1 := tests.CreateTransactionFromOutputs(t, outputs, targetAddresses, keyPairs, targetPeer.ID())
	tx2 := tests.CreateTransactionFromOutputs(t, outputs[:1], targetAddresses, keyPairs, targetPeer.ID())
	tx3 := tests.CreateTransactionFromOutputs(t, outputs[len(outputs)-1:], targetAddresses, keyPairs, targetPeer.ID())

	*txs = append(*txs, tx1, tx2, tx3)

	resp, err := peers[peerIndex].PostTransaction(tx1.Bytes())
	t.Logf("post tx %s on peer %s", tx1.ID().Base58(), peers[peerIndex].Name())
	require.NoError(t, err, "There was an error posting transaction %s to peer %s",
		tx1.ID().Base58(), peers[peerIndex].Name())
	require.Empty(t, resp.Error, "There was an error in the response while posting transaction %s to peer %s",
		tx1.ID().Base58(), peers[peerIndex].Name())
	resp, err = peers[(peerIndex+1)%len(peers)].PostTransaction(tx2.Bytes())
	t.Logf("post tx %s on peer %s", tx2.ID().Base58(), peers[(peerIndex+1)%len(peers)].Name())
	require.NoError(t, err, "There was an error in the response while posting transaction %s to peer %s",
		tx1.ID().Base58(), peers[(peerIndex+1)%len(peers)].Name())
	require.Empty(t, resp.Error, "There was an error in the response while posting transaction %s to peer %s",
		tx2.ID().Base58(), peers[(peerIndex+1)%len(peers)].Name())
	resp, err = peers[(peerIndex+2)%len(peers)].PostTransaction(tx3.Bytes())
	t.Logf("post tx %s on peer %s", tx3.ID().Base58(), peers[(peerIndex+2)%len(peers)].Name())
	require.NoError(t, err, "There was an error posting transaction %s to peer %s",
		tx2.ID().Base58(), peers[(peerIndex+2)%len(peers)].Name())
	require.Empty(t, resp.Error, "There was an error in the response while posting transaction %s to peer %s",
		tx2.ID().Base58())
}

func getOutputsControlledBy(t *testing.T, node *framework.Node, addresses ...ledgerstate.Address) ledgerstate.Outputs {
	outputs := ledgerstate.Outputs{}
	for _, address := range addresses {
		walletOutputs := tests.AddressUnspentOutputs(t, node, address, 1)
		for _, walletOutput := range walletOutputs {
			t.Logf("wallet output is %v", walletOutput)
			output, err := walletOutput.Output.ToLedgerstateOutput()
			require.NoError(t, err, "Failed to convert output to ledgerstate output")
			outputs = append(outputs, output)
		}
	}
	return outputs
}
