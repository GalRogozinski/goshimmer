package fcob

import (
	"sync"
	"time"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/hive.go/byteutils"
	"github.com/iotaledger/hive.go/marshalutil"
	"github.com/iotaledger/hive.go/objectstorage"
	"github.com/iotaledger/hive.go/stringify"
)

// LevelOfKnowledge defines the Level Of Knowledge type.
type LevelOfKnowledge = uint8

// The different levels of knowledge.
const (
	// Pending implies that the opinion is not formed yet.
	Pending LevelOfKnowledge = iota

	// One implies that voting is required.
	One

	// Two implies that we have locally finalized our opinion but we can still reply to eventual queries.
	Two

	// Three implies that we have locally finalized our opinion and we do not reply to eventual queries.
	Three
)

// region Opinion //////////////////////////////////////////////////////////////////////////////////////////////////////

// Opinion defines the FCoB opinion about a transaction.
type Opinion struct {
	objectstorage.StorableObjectFlags

	// the transactionID associated to this opinion.
	transactionID ledgerstate.TransactionID

	// timestamp is the arrrival/solidification time.
	timestamp time.Time

	// liked is the opinion value.
	liked bool

	// levelOfKnowledge is the degree of certainty of the associated opinion.
	levelOfKnowledge LevelOfKnowledge

	timestampMutex        sync.RWMutex
	likedMutex            sync.RWMutex
	levelOfKnowledgeMutex sync.RWMutex
}

// Timestamp returns the opinion's timestamp.
func (o *Opinion) Timestamp() time.Time {
	o.timestampMutex.RLock()
	defer o.timestampMutex.RUnlock()
	return o.timestamp
}

// SetTimestamp sets the opinion's timestamp.
func (o *Opinion) SetTimestamp(t time.Time) {
	o.timestampMutex.Lock()
	defer o.timestampMutex.Unlock()
	o.timestamp = t
}

// Liked returns the opinion's liked.
func (o *Opinion) Liked() bool {
	o.likedMutex.RLock()
	defer o.likedMutex.RUnlock()
	return o.liked
}

// SetLiked sets the opinion's liked.
func (o *Opinion) SetLiked(l bool) {
	o.likedMutex.Lock()
	defer o.likedMutex.Unlock()
	o.liked = l
}

// LevelOfKnowledge returns the opinion's LevelOfKnowledge.
func (o *Opinion) LevelOfKnowledge() LevelOfKnowledge {
	o.levelOfKnowledgeMutex.RLock()
	defer o.levelOfKnowledgeMutex.RUnlock()
	return o.LevelOfKnowledge()
}

// SetLevelOfKnowledge sets the opinion's LevelOfKnowledge.
func (o *Opinion) SetLevelOfKnowledge(lok LevelOfKnowledge) {
	o.levelOfKnowledgeMutex.Lock()
	defer o.levelOfKnowledgeMutex.Unlock()
	o.levelOfKnowledge = lok
}

// Bytes marshals the Opinion into a sequence of bytes.
func (o *Opinion) Bytes() []byte {
	return byteutils.ConcatBytes(o.ObjectStorageKey(), o.ObjectStorageValue())
}

// String returns a human readable version of the Opinion.
func (o *Opinion) String() string {
	return stringify.Struct("Opinion",
		stringify.StructField("transactionID", o.transactionID),
		stringify.StructField("timestamp", o.Timestamp),
		stringify.StructField("liked", o.Liked),
		stringify.StructField("LevelOfKnowledge", o.LevelOfKnowledge),
	)
}

// Update is disabled and panics if it ever gets called - it is required to match the StorableObject interface.
func (o *Opinion) Update(other objectstorage.StorableObject) {
	panic("updates disabled")
}

// ObjectStorageKey returns the key that is used to store the object in the database. It is required to match the
// StorableObject interface.
func (o *Opinion) ObjectStorageKey() []byte {
	return o.transactionID.Bytes()
}

// ObjectStorageValue marshals the Opinion into a sequence of bytes. The ID is not serialized here as it is
// only used as a key in the ObjectStorage.
func (o *Opinion) ObjectStorageValue() []byte {
	return marshalutil.New().
		WriteTime(o.Timestamp()).
		WriteBool(o.Liked()).
		WriteUint8(o.LevelOfKnowledge()).
		Bytes()
}

// code contract (make sure the struct implements all required methods)
var _ objectstorage.StorableObject = &Opinion{}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region CachedOpinion ////////////////////////////////////////////////////////////////////////////////////////////////

// CachedOpinion is a wrapper for the generic CachedObject returned by the object storage that overrides the accessor
// methods with a type-casted one.
type CachedOpinion struct {
	objectstorage.CachedObject
}

// Retain marks the CachedObject to still be in use by the program.
func (c *CachedOpinion) Retain() *CachedOpinion {
	return &CachedOpinion{c.CachedObject.Retain()}
}

// Unwrap is the type-casted equivalent of Get. It returns nil if the object does not exist.
func (c *CachedOpinion) Unwrap() *Opinion {
	untypedObject := c.Get()
	if untypedObject == nil {
		return nil
	}

	typedObject := untypedObject.(*Opinion)
	if typedObject == nil || typedObject.IsDeleted() {
		return nil
	}

	return typedObject
}

// Consume unwraps the CachedObject and passes a type-casted version to the consumer (if the object is not empty - it
// exists). It automatically releases the object when the consumer finishes.
func (c *CachedOpinion) Consume(consumer func(opinion *Opinion), forceRelease ...bool) (consumed bool) {
	return c.CachedObject.Consume(func(object objectstorage.StorableObject) {
		consumer(object.(*Opinion))
	}, forceRelease...)
}

// String returns a human readable version of the CachedOpinion.
func (c *CachedOpinion) String() string {
	return stringify.Struct("CachedOpinion",
		stringify.StructField("CachedObject", c.Unwrap()),
	)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region ConflictSet //////////////////////////////////////////////////////////////////////////////////////////////////

// ConflictSet is a list of Opinion.
type ConflictSet []*Opinion

// hasDecidedLike returns true if the conflict set contains at least one LIKE with LoK >= 2.
func (c ConflictSet) hasDecidedLike() bool {
	for _, opinion := range c {
		if opinion.Liked() && opinion.LevelOfKnowledge() > One {
			return true
		}
	}
	return false
}

// anchor returns the oldest opinion with LoK <= 1.
func (c ConflictSet) anchor() (opinion *Opinion) {
	oldestTimestamp := time.Unix(1<<63-62135596801, 999999999)
	for _, o := range c {
		if o.LevelOfKnowledge() <= One && o.Timestamp().Before(oldestTimestamp) {
			oldestTimestamp = o.Timestamp()
			opinion = o
		}
	}
	return opinion
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////