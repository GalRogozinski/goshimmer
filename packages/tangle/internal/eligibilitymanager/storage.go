package eligibilitymanager

import (
	"sync"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/hive.go/marshalutil"
	"github.com/iotaledger/hive.go/objectstorage"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/hive.go/types"
)

// region UnconfirmedTxDependency //////////////////////////////////////////////////////////////////////////////////////

var UnconfirmedTxDependencyPartitionKeys = objectstorage.PartitionKey(ledgerstate.TransactionIDLength)

// UnconfirmedTxDependency maps a transaction to all of the transactions that create its inputs which are not yet confirmed
type UnconfirmedTxDependency struct {
	objectstorage.StorableObjectFlags

	dependencyTxID ledgerstate.TransactionID
	dependentTxIDs ledgerstate.TransactionIDs
	mutex          sync.RWMutex
}

// NewUnconfirmedTxDependency creates an empty mapping for txID
func NewUnconfirmedTxDependency(txID *ledgerstate.TransactionID) *UnconfirmedTxDependency {
	return &UnconfirmedTxDependency{
		dependencyTxID: *txID,
		dependentTxIDs: make(ledgerstate.TransactionIDs, 0),
	}
}

// AddDependency adds a transaction id dependency
func (u *UnconfirmedTxDependency) AddDependency(txID *ledgerstate.TransactionID) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.dependentTxIDs[*txID] = types.Void
}

// DeleteDependency deletes a transaction id dependency
func (u *UnconfirmedTxDependency) DeleteDependency(txID ledgerstate.TransactionID) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	if _, ok := u.dependentTxIDs[txID]; ok {
		delete(u.dependentTxIDs, txID)
	}
}

func (u *UnconfirmedTxDependency) Update(other objectstorage.StorableObject) {
	panic("implement me")
}

func (u *UnconfirmedTxDependency) ObjectStorageKey() []byte {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	return u.dependencyTxID.Bytes()
}

func (u *UnconfirmedTxDependency) ObjectStorageValue() []byte {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	marshalUtil := marshalutil.New(ledgerstate.TransactionIDLength * len(u.dependentTxIDs))
	for dependency := range u.dependentTxIDs {
		marshalUtil.WriteBytes(dependency.Bytes())
	}
	return marshalUtil.Bytes()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region CachedUnconfirmedTxDependency ////////////////////////////////////////////////////////////////////////////////

// CachedUnconfirmedTxDependency is a wrapper for the generic CachedObject returned by the object storage that overrides the
// accessor methods with a type-casted one.
type CachedUnconfirmedTxDependency struct {
	objectstorage.CachedObject
}

// TODO what for?
// ID returns the dependency transactionID of the UnconfirmedTxDependency.
func (c *CachedUnconfirmedTxDependency) ID() (id ledgerstate.TransactionID) {
	id, _, err := ledgerstate.TransactionIDFromBytes(c.Key())
	if err != nil {
		panic(err)
	}
	return
}

// Retain marks the CachedObject to still be in use by the program.
func (c *CachedUnconfirmedTxDependency) Retain() *CachedUnconfirmedTxDependency {
	return &CachedUnconfirmedTxDependency{c.CachedObject.Retain()}
}

// Unwrap is the type-casted equivalent of Get. It returns nil if the object does not exist.
func (c *CachedUnconfirmedTxDependency) Unwrap() *UnconfirmedTxDependency {
	untypedObject := c.Get()
	if untypedObject == nil {
		return nil
	}

	typedObject := untypedObject.(*UnconfirmedTxDependency)
	if typedObject == nil || typedObject.IsDeleted() {
		return nil
	}

	return typedObject
}

// Consume unwraps the CachedObject and passes a type-casted version to the consumer (if the object is not empty - it
// exists). It automatically releases the object when the consumer finishes.
func (c *CachedUnconfirmedTxDependency) Consume(consumer func(unconfirmedTxDependency *UnconfirmedTxDependency), forceRelease ...bool) (consumed bool) {
	return c.CachedObject.Consume(func(object objectstorage.StorableObject) {
		consumer(object.(*UnconfirmedTxDependency))
	}, forceRelease...)
}

// String returns a human readable version of the CachedMissingMessage.
func (c *CachedUnconfirmedTxDependency) String() string {
	return stringify.Struct("CachedUnconfirmedTxDependency",
		stringify.StructField("CachedObject", c.Unwrap()),
	)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
