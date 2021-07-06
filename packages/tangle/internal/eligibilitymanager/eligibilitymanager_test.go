package eligibilitymanager

import (
	"testing"
	"time"

	"github.com/iotaledger/goshimmer/packages/tangle"

	"github.com/iotaledger/hive.go/events"

	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"

	"github.com/iotaledger/hive.go/identity"
)

func TestDependenciesConfirmed(t *testing.T) {
	eligibleEventTriggered := false
	testTangle := tangle.newTestTangle()
	defer testTangle.Shutdown()
	wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID := setupEligibilityTests(t, testTangle)
	scenarioMessagesApproveEmptyID(t, testTangle, wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID)

	testTangle.EligibilityManager.Events.MessageEligible.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		assert.Equal(t, messages["1"].ID(), messageID)
		eligibleEventTriggered = true
	}))

	mockUTXO := ledgerstate.NewUtxoDagMock(t, testTangle.LedgerState.UTXODAG)
	testTangle.LedgerState.UTXODAG = mockUTXO
	mockUTXO.On("InclusionState", transactions["0"].ID()).Return(ledgerstate.Pending)

	isEligibleFlag := runCheckEligibilityAndGetEligibility(t, testTangle, messages["1"].ID())
	assert.False(t, isEligibleFlag, "Message 1 shouldn't be eligible")

	// reset mock, since calls can't be overridden
	mockUTXO.ExpectedCalls = make([]*mock.Call, 0)

	mockUTXO.On("InclusionState", transactions["0"].ID()).Return(ledgerstate.Confirmed)
	isEligibleFlag = runCheckEligibilityAndGetEligibility(t, testTangle, messages["1"].ID())
	assert.True(t, isEligibleFlag, "Message 1 isn't eligible")

	assert.True(t, eligibleEventTriggered, "Eligibility event wasn't triggered")
}

func TestDataMessageAlwaysEligible(t *testing.T) {
	testTangle := tangle.newTestTangle()
	defer testTangle.Shutdown()

	message := testTangle.newTestDataMessage("data")
	testTangle.Storage.StoreMessage(message)

	err := testTangle.EligibilityManager.checkEligibility(message.ID())
	assert.NoError(t, err)

	var eligibilityResult bool
	testTangle.Storage.MessageMetadata(message.ID()).Consume(func(messageMetadata *tangle.MessageMetadata) {
		eligibilityResult = messageMetadata.IsEligible()
	})
	assert.True(t, eligibilityResult, "Data messages should awlays be eligible")
}

func TestDependencyDirectApproval(t *testing.T) {
	testTangle := tangle.newTestTangle()
	defer testTangle.Shutdown()

	wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID := setupEligibilityTests(t, testTangle)

	scenarioMessagesApproveDependency(t, testTangle, wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID)

	mockUTXO := ledgerstate.NewUtxoDagMock(t, testTangle.LedgerState.UTXODAG)
	testTangle.LedgerState.UTXODAG = mockUTXO
	mockUTXO.On("InclusionState", transactions["0"].ID()).Return(ledgerstate.Pending)

	isEligibleFlag := runCheckEligibilityAndGetEligibility(t, testTangle, messages["1"].ID())
	assert.True(t, isEligibleFlag, "Message 1 isn't eligible")
}

func TestUpdateEligibilityAfterDependencyConfirmation(t *testing.T) {
	testTangle := tangle.newTestTangle()
	defer testTangle.Shutdown()
	wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID := setupEligibilityTests(t, testTangle)
	txID := transactions["0"].ID()
	eligibleEventTriggered := false

	scenarioMessagesApproveEmptyID(t, testTangle, wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID)

	testTangle.EligibilityManager.Events.MessageEligible.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		assert.Equal(t, messages["1"].ID(), messageID)
		eligibleEventTriggered = true
	}))

	mockUTXO := ledgerstate.NewUtxoDagMock(t, testTangle.LedgerState.UTXODAG)
	testTangle.LedgerState.UTXODAG = mockUTXO
	mockUTXO.On("InclusionState", txID).Return(ledgerstate.Pending)

	messageID := messages["1"].ID()
	isEligibleFlag := runCheckEligibilityAndGetEligibility(t, testTangle, messageID)
	assert.False(t, isEligibleFlag, "Message 1 shouldn't be eligible")

	// reset mock, since calls can't be overridden
	mockUTXO.ExpectedCalls = make([]*mock.Call, 0)
	mockUTXO.On("InclusionState", txID).Return(ledgerstate.Confirmed)

	err := testTangle.EligibilityManager.updateEligibilityAfterDependencyConfirmation(&txID)
	assert.NoError(t, err)

	testTangle.Storage.MessageMetadata(messageID).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.True(t, isEligibleFlag, "Message 1 isn't eligible")

	assert.True(t, eligibleEventTriggered, "eligibility event wasn't triggered")
}

func TestDoNotUpdateEligibilityAfterPartialDependencyConfirmation(t *testing.T) {
	testTangle := tangle.newTestTangle()
	defer testTangle.Shutdown()
	wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID := setupEligibilityTests(t, testTangle)

	scenarioMoreThanOneDependency(t, testTangle, wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID)

	mockUTXO := ledgerstate.NewUtxoDagMock(t, testTangle.LedgerState.UTXODAG)
	testTangle.LedgerState.UTXODAG = mockUTXO
	tx1ID := transactions["1"].ID()
	tx2ID := transactions["2"].ID()
	mockUTXO.On("InclusionState", tx1ID).Return(ledgerstate.Pending)
	mockUTXO.On("InclusionState", tx2ID).Return(ledgerstate.Pending)

	messageID := messages["3"].ID()
	isEligibleFlag := runCheckEligibilityAndGetEligibility(t, testTangle, messageID)
	assert.False(t, isEligibleFlag)

	// reset mock, since calls can't be overridden
	mockUTXO.ExpectedCalls = make([]*mock.Call, 0)
	mockUTXO.On("InclusionState", tx1ID).Return(ledgerstate.Confirmed)
	mockUTXO.On("InclusionState", tx2ID).Return(ledgerstate.Pending)

	err := testTangle.EligibilityManager.updateEligibilityAfterDependencyConfirmation(&tx1ID)
	assert.NoError(t, err)
	testTangle.Storage.MessageMetadata(messageID).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.False(t, isEligibleFlag)

	// reset mock, since calls can't be overridden
	mockUTXO.ExpectedCalls = make([]*mock.Call, 0)
	mockUTXO.On("InclusionState", tx1ID).Return(ledgerstate.Confirmed)
	mockUTXO.On("InclusionState", tx2ID).Return(ledgerstate.Confirmed)

	err = testTangle.EligibilityManager.updateEligibilityAfterDependencyConfirmation(&tx2ID)
	assert.NoError(t, err)
	testTangle.Storage.MessageMetadata(messageID).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.True(t, isEligibleFlag)
}

func TestConfirmationMakeEligibleOneOfDependentTransaction(t *testing.T) {
	tangle := tangle.newTestTangle()
	defer tangle.Shutdown()
	wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID := setupEligibilityTests(t, tangle)
	scenarioMoreThanOneDependentTransaction(t, tangle, wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID)

	mockUTXO := ledgerstate.NewUtxoDagMock(t, tangle.LedgerState.UTXODAG)
	tangle.LedgerState.UTXODAG = mockUTXO
	mockUTXO.On("InclusionState", transactions["3"].ID()).Return(ledgerstate.Pending)
	mockUTXO.On("InclusionState", transactions["0"].ID()).Return(ledgerstate.Pending)

	isEligibleFlag := runCheckEligibilityAndGetEligibility(t, tangle, messages["1"].ID())
	assert.False(t, isEligibleFlag)

	isEligibleFlag = runCheckEligibilityAndGetEligibility(t, tangle, messages["2"].ID())
	assert.False(t, isEligibleFlag)

	// reset mock, since calls can't be overridden
	mockUTXO.ExpectedCalls = make([]*mock.Call, 0)
	mockUTXO.On("InclusionState", transactions["3"].ID()).Return(ledgerstate.Pending)
	mockUTXO.On("InclusionState", transactions["0"].ID()).Return(ledgerstate.Confirmed)

	confirmedTransactionID := transactions["0"].ID()

	err := tangle.EligibilityManager.updateEligibilityAfterDependencyConfirmation(&confirmedTransactionID)
	assert.NoError(t, err)

	tangle.Storage.MessageMetadata(messages["1"].ID()).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.False(t, isEligibleFlag)
	tangle.Storage.MessageMetadata(messages["2"].ID()).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.True(t, isEligibleFlag, "Message 1 isn't eligible")

	// reset mock, since calls can't be overridden
	mockUTXO.ExpectedCalls = make([]*mock.Call, 0)
	mockUTXO.On("InclusionState", transactions["3"].ID()).Return(ledgerstate.Confirmed)
	mockUTXO.On("InclusionState", transactions["0"].ID()).Return(ledgerstate.Confirmed)

	confirmedTransactionID = transactions["3"].ID()

	err = tangle.EligibilityManager.updateEligibilityAfterDependencyConfirmation(&confirmedTransactionID)
	assert.NoError(t, err)

	tangle.Storage.MessageMetadata(messages["1"].ID()).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.True(t, isEligibleFlag)
	tangle.Storage.MessageMetadata(messages["2"].ID()).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	assert.True(t, isEligibleFlag)
}

func setupEligibilityTests(t *testing.T, tangle *tangle.Tangle) (map[string]tangle.wallet, map[ledgerstate.Address]tangle.wallet, map[string]*tangle.Message, map[string]*ledgerstate.Transaction, map[string]*ledgerstate.UTXOInput, map[string]*ledgerstate.SigLockedSingleOutput, map[ledgerstate.OutputID]ledgerstate.Output) {
	tangle.EligibilityManager.Setup()

	wallets := make(map[string]tangle.wallet)
	walletsByAddress := make(map[ledgerstate.Address]tangle.wallet)
	w := tangle.createWallets(10)
	wallets["GENESIS"] = w[0]
	wallets["A"] = w[1]
	wallets["B"] = w[2]
	wallets["C"] = w[3]
	wallets["D"] = w[4]
	wallets["E"] = w[5]
	wallets["F"] = w[6]
	wallets["H"] = w[7]
	wallets["I"] = w[8]
	wallets["J"] = w[9]
	for _, wlt := range wallets {
		walletsByAddress[wlt.address] = wlt
	}
	genesisBalance := ledgerstate.NewColoredBalances(
		map[ledgerstate.Color]uint64{
			ledgerstate.ColorIOTA: 3,
		})
	genesisEssence := ledgerstate.NewTransactionEssence(
		0,
		time.Unix(tangle.DefaultGenesisTime, 0),
		identity.ID{},
		identity.ID{},
		ledgerstate.NewInputs(ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(ledgerstate.GenesisTransactionID, 0))),
		ledgerstate.NewOutputs(ledgerstate.NewSigLockedColoredOutput(genesisBalance, wallets["GENESIS"].address)),
	)

	genesisTransaction := ledgerstate.NewTransaction(genesisEssence, ledgerstate.UnlockBlocks{ledgerstate.NewReferenceUnlockBlock(0)})
	stored, _, _ := tangle.LedgerState.UTXODAG.StoreTransaction(genesisTransaction)
	assert.True(t, stored, "genesis transaction stored")

	messages := make(map[string]*tangle.Message)
	transactions := make(map[string]*ledgerstate.Transaction)
	inputs := make(map[string]*ledgerstate.UTXOInput)
	outputs := make(map[string]*ledgerstate.SigLockedSingleOutput)
	outputsByID := make(map[ledgerstate.OutputID]ledgerstate.Output)

	// Base message
	inputs["GENESIS"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(genesisTransaction.ID(), 0))
	outputs["0A"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["A"].address)
	outputs["0B"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["B"].address)
	outputs["0C"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["C"].address)
	transactions["0"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["GENESIS"]), ledgerstate.NewOutputs(outputs["0A"], outputs["0B"], outputs["0C"]), outputsByID, walletsByAddress, wallets["GENESIS"])
	messages["0"] = tangle.newTestParentsPayloadMessage(transactions["0"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})
	stored, _, _ = tangle.LedgerState.UTXODAG.StoreTransaction(transactions["0"])
	assert.True(t, stored)
	tangle.Storage.StoreMessage(messages["0"])
	attachment, stored := tangle.Storage.StoreAttachment(transactions["0"].ID(), messages["0"].ID())
	assert.True(t, stored)
	attachment.Release()

	return wallets, walletsByAddress, messages, transactions, inputs, outputs, outputsByID
}

func runCheckEligibilityAndGetEligibility(t *testing.T, tangle *tangle.Tangle, messageID tangle.MessageID) bool {
	err := tangle.EligibilityManager.checkEligibility(messageID)
	assert.NoError(t, err)
	var isEligibleFlag bool
	tangle.Storage.MessageMetadata(messageID).Consume(func(messageMetadata *tangle.MessageMetadata) {
		isEligibleFlag = messageMetadata.IsEligible()
	})
	return isEligibleFlag
}

// create transaction 1 (msg 1) that takes input from tx 0 (msg 0) and does not approve msg 0 directly
func scenarioMessagesApproveEmptyID(t *testing.T, tangle *tangle.Tangle, wallets map[string]tangle.wallet, walletsByAddress map[ledgerstate.Address]tangle.wallet, messages map[string]*tangle.Message, transactions map[string]*ledgerstate.Transaction, inputs map[string]*ledgerstate.UTXOInput, outputs map[string]*ledgerstate.SigLockedSingleOutput, outputsByID map[ledgerstate.OutputID]ledgerstate.Output) {
	inputs["1A"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["A"])))
	outputsByID[inputs["1A"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0A"])[0]

	outputs["1D"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["D"].address)
	transactions["1"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["1A"]), ledgerstate.NewOutputs(outputs["1D"]), outputsByID, walletsByAddress)
	messages["1"] = tangle.newTestParentsPayloadMessage(transactions["1"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	tangle.Storage.StoreMessage(messages["1"])
	stored, _, _ := tangle.LedgerState.UTXODAG.StoreTransaction(transactions["1"])
	assert.True(t, stored)

	attachment, stored := tangle.Storage.StoreAttachment(transactions["1"].ID(), messages["1"].ID())
	assert.True(t, stored)
	attachment.Release()
}

// creates tx and msg 1 that directly approves msg 0 and uses tx0 outputs as its inputs
func scenarioMessagesApproveDependency(t *testing.T, tangle *tangle.Tangle, wallets map[string]tangle.wallet, walletsByAddress map[ledgerstate.Address]tangle.wallet, messages map[string]*tangle.Message, transactions map[string]*ledgerstate.Transaction, inputs map[string]*ledgerstate.UTXOInput, outputs map[string]*ledgerstate.SigLockedSingleOutput, outputsByID map[ledgerstate.OutputID]ledgerstate.Output) {
	inputs["1A"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["A"])))
	outputsByID[inputs["1A"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0A"])[0]
	outputs["1D"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["D"].address)

	transactions["1"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["1A"]), ledgerstate.NewOutputs(outputs["1D"]), outputsByID, walletsByAddress)
	messages["1"] = tangle.newTestParentsPayloadMessage(transactions["1"], []tangle.MessageID{messages["0"].ID()}, []tangle.MessageID{})

	tangle.Storage.StoreMessage(messages["1"])
	stored, _, _ := tangle.LedgerState.UTXODAG.StoreTransaction(transactions["1"])
	assert.True(t, stored)

	attachment, stored := tangle.Storage.StoreAttachment(transactions["1"].ID(), messages["1"].ID())
	assert.True(t, stored)
	attachment.Release()

	return
}

// creates transaction that is dependent on two other transactions and is not connected by direct approval of their messages
func scenarioMoreThanOneDependency(t *testing.T, tangle *tangle.Tangle, wallets map[string]tangle.wallet, walletsByAddress map[ledgerstate.Address]tangle.wallet, messages map[string]*tangle.Message, transactions map[string]*ledgerstate.Transaction, inputs map[string]*ledgerstate.UTXOInput, outputs map[string]*ledgerstate.SigLockedSingleOutput, outputsByID map[ledgerstate.OutputID]ledgerstate.Output) {
	// transaction 1
	inputs["1A"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["A"])))
	outputsByID[inputs["1A"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0A"])[0]
	outputs["1D"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["D"].address)

	transactions["1"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["1A"]), ledgerstate.NewOutputs(outputs["1D"]), outputsByID, walletsByAddress)
	messages["1"] = tangle.newTestParentsPayloadMessage(transactions["1"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	// transaction 2
	inputs["2B"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["B"])))
	outputsByID[inputs["2B"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0B"])[0]
	outputs["2E"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["E"].address)

	transactions["2"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["2B"]), ledgerstate.NewOutputs(outputs["2E"]), outputsByID, walletsByAddress)
	messages["2"] = tangle.newTestParentsPayloadMessage(transactions["2"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	// transaction 3 dependent on transactions 1 and 2
	inputs["3D"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["1"].ID(), tangle.selectIndex(transactions["1"], wallets["D"])))
	outputsByID[inputs["3D"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["1D"])[0]
	inputs["3E"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["2"].ID(), tangle.selectIndex(transactions["2"], wallets["E"])))
	outputsByID[inputs["3E"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["2E"])[0]
	outputs["3F"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["F"].address)

	transactions["3"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["3D"], inputs["3E"]), ledgerstate.NewOutputs(outputs["3F"]), outputsByID, walletsByAddress)
	messages["3"] = tangle.newTestParentsPayloadMessage(transactions["3"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	// store all transactions and messages
	tangle.Storage.StoreMessage(messages["1"])
	stored, _, _ := tangle.LedgerState.UTXODAG.StoreTransaction(transactions["1"])
	assert.True(t, stored)
	attachment, stored := tangle.Storage.StoreAttachment(transactions["1"].ID(), messages["1"].ID())
	attachment.Release()
	assert.True(t, stored)
	tangle.Storage.StoreMessage(messages["3"])
	stored, _, _ = tangle.LedgerState.UTXODAG.StoreTransaction(transactions["3"])
	assert.True(t, stored)
	attachment, stored = tangle.Storage.StoreAttachment(transactions["3"].ID(), messages["3"].ID())
	attachment.Release()
	assert.True(t, stored)
	tangle.Storage.StoreMessage(messages["2"])
	stored, _, _ = tangle.LedgerState.UTXODAG.StoreTransaction(transactions["2"])
	assert.True(t, stored)
	attachment, stored = tangle.Storage.StoreAttachment(transactions["2"].ID(), messages["2"].ID())
	attachment.Release()
	assert.True(t, stored)
}

// two transactions 1 and 2 are dependent on the same transaction 0 and are not connected by direct approval between messages
// transaction is also dependent on other transaction 3 that will get confirmed before 0
func scenarioMoreThanOneDependentTransaction(t *testing.T, tangle *tangle.Tangle, wallets map[string]tangle.wallet, walletsByAddress map[ledgerstate.Address]tangle.wallet, messages map[string]*tangle.Message, transactions map[string]*ledgerstate.Transaction, inputs map[string]*ledgerstate.UTXOInput, outputs map[string]*ledgerstate.SigLockedSingleOutput, outputsByID map[ledgerstate.OutputID]ledgerstate.Output) {
	// transaction 3
	inputs["3B"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["B"])))
	outputsByID[inputs["3B"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0B"])[0]
	outputs["3E"] = ledgerstate.NewSigLockedSingleOutput(2, wallets["E"].address)

	transactions["3"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["3B"]), ledgerstate.NewOutputs(outputs["3E"]), outputsByID, walletsByAddress)
	messages["3"] = tangle.newTestParentsPayloadMessage(transactions["3"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	// transaction 1
	inputs["1A"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["A"])))
	outputsByID[inputs["1A"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0A"])[0]
	inputs["1E"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["3"].ID(), tangle.selectIndex(transactions["3"], wallets["E"])))
	outputsByID[inputs["1E"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["3E"])[0]
	outputs["1D"] = ledgerstate.NewSigLockedSingleOutput(2, wallets["D"].address)

	transactions["1"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["1A"], inputs["1E"]), ledgerstate.NewOutputs(outputs["1D"]), outputsByID, walletsByAddress)
	messages["1"] = tangle.newTestParentsPayloadMessage(transactions["1"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	// transaction 2
	inputs["2C"] = ledgerstate.NewUTXOInput(ledgerstate.NewOutputID(transactions["0"].ID(), tangle.selectIndex(transactions["0"], wallets["C"])))
	outputsByID[inputs["2C"].ReferencedOutputID()] = ledgerstate.NewOutputs(outputs["0C"])[0]
	outputs["2F"] = ledgerstate.NewSigLockedSingleOutput(1, wallets["F"].address)

	transactions["2"] = tangle.makeTransaction(ledgerstate.NewInputs(inputs["2C"]), ledgerstate.NewOutputs(outputs["2F"]), outputsByID, walletsByAddress)
	messages["2"] = tangle.newTestParentsPayloadMessage(transactions["2"], []tangle.MessageID{tangle.EmptyMessageID}, []tangle.MessageID{})

	// store all transactions and messages
	tangle.Storage.StoreMessage(messages["1"])
	stored, _, _ := tangle.LedgerState.UTXODAG.StoreTransaction(transactions["1"])
	assert.True(t, stored)
	attachment, stored := tangle.Storage.StoreAttachment(transactions["1"].ID(), messages["1"].ID())
	attachment.Release()
	assert.True(t, stored)

	tangle.Storage.StoreMessage(messages["2"])
	stored, _, _ = tangle.LedgerState.UTXODAG.StoreTransaction(transactions["2"])
	assert.True(t, stored)
	attachment, stored = tangle.Storage.StoreAttachment(transactions["2"].ID(), messages["2"].ID())
	attachment.Release()
	assert.True(t, stored)

	tangle.Storage.StoreMessage(messages["3"])
	stored, _, _ = tangle.LedgerState.UTXODAG.StoreTransaction(transactions["3"])
	assert.True(t, stored)
	attachment, stored = tangle.Storage.StoreAttachment(transactions["3"].ID(), messages["3"].ID())
	attachment.Release()
	assert.True(t, stored)
}
