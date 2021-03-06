package bucketeer

import (
	"encoding"
	"fmt"

	"github.com/boltdb/bolt"
)

/*
Bucketeer encapsulates the components needed to resolve a bucket in BoltDB and provides convenience methods for initializing Keyfarers for various key types.
*/
type Bucketeer struct {
	db   *bolt.DB
	path Path
}

/*
New creates a Bucketeer for the provided database and bucket names.
*/
func New(db *bolt.DB, bucketNames ...string) (bb *Bucketeer) {
	bb = &Bucketeer{
		db:   db,
		path: NewPath(bucketNames...),
	}
	return
}

/*
ForPath creates a new Bucketeer for the provided database and bucket path.
*/
func ForPath(db *bolt.DB, path Path) (bb *Bucketeer) {
	bb = &Bucketeer{
		db:   db,
		path: path,
	}
	return
}

/*
EnsurePathBuckets creates any buckets along the provided path if they do not exist.
*/
func (bb *Bucketeer) EnsurePathBuckets() error {
	return EnsurePathBuckets(bb.db, bb.path)
}

/*
EnsureNestedBucket creates a nested bucket if it does not exist. The bucket's full parent path must exist.
*/
func (bb *Bucketeer) EnsureNestedBucket(bucket string) error {
	return EnsureNestedBucket(bb.db, bb.path, bucket)
}

/*
InNestedBucket creates a new Bucketeer for a nested bucket with the provided name.
*/
func (bb *Bucketeer) InNestedBucket(bucket string) *Bucketeer {
	return ForPath(bb.db, bb.path.Nest(bucket))
}

/*
DeleteNestedBucket deletes a nested bucket with the provided name.
*/
func (bb *Bucketeer) DeleteNestedBucket(bucket string) error {
	bf := func(b *bolt.Bucket) error {
		return b.DeleteBucket([]byte(bucket))
	}
	return bb.Update(bf)
}

/*
GetBucketStats retrieves the BucketStats for the current bucket.
*/
func (bb *Bucketeer) GetBucketStats() (stats bolt.BucketStats, err error) {
	bf := func(b *bolt.Bucket) (err error) {
		stats = b.Stats()
		return
	}
	err = bb.View(bf)
	return
}

/*
View executes the provided function in a View transaction.
*/
func (bb *Bucketeer) View(viewFunc func(b *bolt.Bucket) error) error {
	return ViewInBucket(bb.db, bb.path, viewFunc)
}

/*
Update executes the provided function in an Update transaction.
*/
func (bb *Bucketeer) Update(updateFunc func(b *bolt.Bucket) error) error {
	return UpdateInBucket(bb.db, bb.path, updateFunc)
}

/*
UpdateWithSequence executes the provided function in an Update transaction, and supplies the next sequence value for the bucket.
*/
func (bb *Bucketeer) UpdateWithSequence(updateFunc func(b *bolt.Bucket, sequence uint64) error) (sequence uint64, err error) {
	bf := func(b *bolt.Bucket) (err error) {
		if sequence, err = b.NextSequence(); err != nil {
			return
		}
		err = updateFunc(b, sequence)
		return
	}
	err = bb.Update(bf)
	return
}

/*
ForByteKey creates a new Keyfarer for the provided key.
*/
func (bb *Bucketeer) ForKey(key Key) *Keyfarer {
	return NewKeyfarer(bb, key.KeyBytes())
}

/*
ForByteKey creates a new Keyfarer for the provided key name.
*/
func (bb *Bucketeer) ForByteKey(key []byte) *Keyfarer {
	return bb.ForKey(NewByteKey(key))
}

/*
ForStringKey creates a new Keyfarer for the provided key name.
*/
func (bb *Bucketeer) ForStringKey(key string) *Keyfarer {
	return bb.ForKey(NewStringKey(key))
}

/*
ForUint64Key creates a new Keyfarer for the provided key name. The key value is stored in big-endian, fixed-length format so it is byte-sortable with other uint64 keys.
*/
func (bb *Bucketeer) ForUint64Key(key uint64) *Keyfarer {
	return bb.ForKey(NewUint64Key(key))
}

/*
ForInt64Key creates a new Keyfarer for the provided key name. The key value is shifted to always be a positive number, and is stored in big-endian, fixed-length format so it is byte-sortable with other int64 keys.
*/
func (bb *Bucketeer) ForInt64Key(key int64) *Keyfarer {
	return bb.ForKey(NewInt64Key(key))
}

/*
ForTextKey creates a new Keyfarer for the textual form of the provided object. If there is an error marshaling the object to text, this function will panic.
*/
func (bb *Bucketeer) ForTextKey(keyObj encoding.TextMarshaler) *Keyfarer {
	return bb.ForKey(NewTextKey(keyObj))
}

/*
ForBinaryKey creates a new Keyfarer for the binary form of the provided object. If there is an error marshaling the object to binary, this function will panic.
*/
func (bb *Bucketeer) ForBinaryKey(keyObj encoding.BinaryMarshaler) *Keyfarer {
	return bb.ForKey(NewBinaryKey(keyObj))
}

/*
ForJsonKey creates a new Keyfarer for the JSON form of the provided object. If there is an error marshaling the object to JSON, this function will panic.
*/
func (bb *Bucketeer) ForJsonKey(keyObj interface{}) *Keyfarer {
	return bb.ForKey(NewJsonKey(keyObj))
}

/*
EnsurePathBuckets creates any buckets along the provided path if they do not exist.
*/
func EnsurePathBuckets(db *bolt.DB, path Path) (err error) {
	if len(path) == 0 {
		panic("Path must have at least one element")
	}
	txf := func(tx *bolt.Tx) (err error) {
		var b *bolt.Bucket
		b, err = tx.CreateBucketIfNotExists(path[0])
		if err != nil || b == nil || len(path) == 1 {
			return
		}
		for _, bucket := range path[1:] {
			b, err = b.CreateBucketIfNotExists(bucket)
			if err != nil || b == nil {
				return
			}
		}
		return
	}
	err = db.Update(txf)
	return
}

/*
EnsureNestedBucket creates a nested bucket if it does not exist. The bucket's full parent path must exist.
*/
func EnsureNestedBucket(db *bolt.DB, path Path, bucket string) (err error) {
	txf := func(tx *bolt.Tx) (err error) {
		var b *bolt.Bucket
		if b = GetBucket(tx, path); b == nil {
			err = fmt.Errorf("Did not find one or more path buckets: %s", path.String())
			return
		}
		_, err = b.CreateBucketIfNotExists([]byte(bucket))
		return
	}
	err = db.Update(txf)
	return
}

/*
GetBucket retrieves the last (innermost) bucket of the provided path for use within a transaction. The bucket's full parent path must exist.
*/
func GetBucket(tx *bolt.Tx, path Path) (b *bolt.Bucket) {
	if len(path) == 0 {
		panic("Path must have at least one element")
	}
	b = tx.Bucket(path[0])
	if len(path) == 1 || b == nil {
		return
	}
	for _, bucket := range path[1:] {
		if b = b.Bucket(bucket); b == nil {
			return
		}
	}
	return
}

/*
ViewInBucket executes the provided function in a View transaction.
*/
func ViewInBucket(db *bolt.DB, path Path, viewFunc func(b *bolt.Bucket) error) (err error) {
	txf := func(tx *bolt.Tx) (err error) {
		if b := GetBucket(tx, path); b != nil {
			err = viewFunc(b)
		}
		return
	}
	err = db.View(txf)
	return
}

/*
UpdateInBucket executes the provided function in an Update transaction.
*/
func UpdateInBucket(db *bolt.DB, path Path, updateFunc func(b *bolt.Bucket) error) (err error) {
	txf := func(tx *bolt.Tx) (err error) {
		if b := GetBucket(tx, path); b != nil {
			err = updateFunc(b)
		}
		return
	}
	err = db.Update(txf)
	return
}
