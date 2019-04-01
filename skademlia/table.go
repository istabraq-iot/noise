package skademlia

import (
	"bytes"
	"container/list"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"sort"
	"sync"
)

var (
	ErrBucketFull = errors.New("bucket is full")
)

type Table struct {
	self *ID

	buckets    []*Bucket
	bucketSize int
}

func NewTable(self *ID) *Table {
	t := &Table{
		self: self,

		buckets:    make([]*Bucket, len(self.checksum)*8),
		bucketSize: 16,
	}

	for i := range t.buckets {
		t.buckets[i] = new(Bucket)
	}

	b := t.buckets[len(t.buckets)-1]
	b.PushFront(self)

	return t
}

func (t Table) Find(b *Bucket, target *ID) *list.Element {
	if target == nil {
		return nil
	}

	var element *list.Element

	b.RLock()

	for e := b.Front(); e != nil; e = e.Next() {
		if e.Value.(*ID).checksum == target.checksum {
			element = e
			break
		}
	}

	b.RUnlock()

	return element
}

func (t Table) Delete(b *Bucket, target *ID) bool {
	e := t.Find(b, target)

	if e == nil {
		return false
	}

	b.Lock()
	defer b.Unlock()

	return b.Remove(e) != nil
}

func (t Table) Update(target *ID) error {
	if target == nil {
		return nil
	}

	b := t.buckets[getBucketID(t.self.checksum, target.checksum)]
	e := t.Find(b, target)

	if e != nil {
		b.Lock()
		b.MoveToFront(e)
		b.Unlock()

		return nil
	}

	if b.Len() < t.bucketSize {
		b.Lock()
		b.PushFront(target)
		b.Unlock()

		return nil
	}

	return ErrBucketFull
}

func (t Table) FindClosest(target *ID, k int) IDs {
	var checksum [blake2b.Size256]byte

	if target != nil {
		checksum = target.checksum
	}

	var closest []*ID

	f := func(b *Bucket) {
		b.RLock()

		for e := b.Front(); e != nil; e = e.Next() {
			if id := e.Value.(*ID); id.checksum != checksum {
				closest = append(closest, id)
			}
		}

		b.RUnlock()
	}

	idx := getBucketID(t.self.checksum, checksum)

	f(t.buckets[idx])

	for i := 1; len(closest) < k && (idx-i >= 0 || idx+i < len(t.buckets)); i++ {
		if idx-i >= 0 {
			f(t.buckets[idx-i])
		}

		if idx+i < len(t.buckets) {
			f(t.buckets[idx+i])
		}
	}

	sort.Slice(closest, func(i, j int) bool {
		return bytes.Compare(xor(closest[i].checksum[:], checksum[:]), xor(closest[j].checksum[:], checksum[:])) == -1
	})

	if len(closest) > k {
		closest = closest[:k]
	}

	return closest
}

func getBucketID(self, target [blake2b.Size256]byte) int {
	return prefixLen(xor(target[:], self[:]))
}

type Bucket struct {
	sync.RWMutex
	list.List
}

func (b *Bucket) Len() int {
	b.RLock()
	size := b.List.Len()
	b.RUnlock()

	return size
}