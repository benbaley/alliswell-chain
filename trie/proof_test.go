// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"bytes"
	crand "crypto/rand"
	mrand "math/rand"
	"sort"
	"testing"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/ethdb/memorydb"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
)

func init() {
	mrand.Seed(time.Now().Unix())
}

// makeProvers creates Merkle trie provers based on different implementations to
// test all variations.
func makeProvers(trie *Trie) []func(key []byte) *memorydb.Database {
	var provers []func(key []byte) *memorydb.Database

	// Create a direct trie based Merkle prover
	provers = append(provers, func(key []byte) *memorydb.Database {
		proof := memorydb.New()
		trie.Prove(key, 0, proof)
		return proof
	})
	// Create a leaf iterator based Merkle prover
	provers = append(provers, func(key []byte) *memorydb.Database {
		proof := memorydb.New()
		if it := NewIterator(trie.NodeIterator(key)); it.Next() && bytes.Equal(key, it.Key) {
			for _, p := range it.Prove() {
				proof.Put(crypto.Keccak256(p), p)
			}
		}
		return proof
	})
	return provers
}

func TestProof(t *testing.T) {
	trie, vals := randomTrie(500)
	root := trie.Hash()
	for i, prover := range makeProvers(trie) {
		for _, kv := range vals {
			proof := prover(kv.k)
			if proof == nil {
				t.Fatalf("prover %d: missing key %x while constructing proof", i, kv.k)
			}
			val, err := VerifyProof(root, kv.k, proof)
			if err != nil {
				t.Fatalf("prover %d: failed to verify proof for key %x: %v\nraw proof: %x", i, kv.k, err, proof)
			}
			if !bytes.Equal(val, kv.v) {
				t.Fatalf("prover %d: verified value mismatch for key %x: have %x, want %x", i, kv.k, val, kv.v)
			}
		}
	}
}

func TestOneElementProof(t *testing.T) {
	trie := new(Trie)
	updateString(trie, "k", "v")
	for i, prover := range makeProvers(trie) {
		proof := prover([]byte("k"))
		if proof == nil {
			t.Fatalf("prover %d: nil proof", i)
		}
		if proof.Len() != 1 {
			t.Errorf("prover %d: proof should have one element", i)
		}
		val, err := VerifyProof(trie.Hash(), []byte("k"), proof)
		if err != nil {
			t.Fatalf("prover %d: failed to verify proof: %v\nraw proof: %x", i, err, proof)
		}
		if !bytes.Equal(val, []byte("v")) {
			t.Fatalf("prover %d: verified value mismatch: have %x, want 'k'", i, val)
		}
	}
}

type entrySlice []*kv

func (p entrySlice) Len() int           { return len(p) }
func (p entrySlice) Less(i, j int) bool { return bytes.Compare(p[i].k, p[j].k) < 0 }
func (p entrySlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func TestRangeProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)
	for i := 0; i < 500; i++ {
		start := mrand.Intn(len(entries))
		end := mrand.Intn(len(entries)-start) + start
		if start == end {
			continue
		}
		firstProof, lastProof := memorydb.New(), memorydb.New()
		if err := trie.Prove(entries[start].k, 0, firstProof); err != nil {
			t.Fatalf("Failed to prove the first node %v", err)
		}
		if err := trie.Prove(entries[end-1].k, 0, lastProof); err != nil {
			t.Fatalf("Failed to prove the last node %v", err)
		}
		var keys [][]byte
		var vals [][]byte
		for i := start; i < end; i++ {
			keys = append(keys, entries[i].k)
			vals = append(vals, entries[i].v)
		}
		err := VerifyRangeProof(trie.Hash(), keys[0], keys, vals, firstProof, lastProof)
		if err != nil {
			t.Fatalf("Case %d(%d->%d) expect no error, got %v", i, start, end-1, err)
		}
	}
}

// TestRangeProof tests normal range proof with the first edge proof
// as the non-existent proof. The test cases are generated randomly.
func TestRangeProofWithNonExistentProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)
	for i := 0; i < 500; i++ {
		start := mrand.Intn(len(entries))
		end := mrand.Intn(len(entries)-start) + start
		if start == end {
			continue
		}
		firstProof, lastProof := memorydb.New(), memorydb.New()

		first := decreseKey(common.CopyBytes(entries[start].k))
		if start != 0 && bytes.Equal(first, entries[start-1].k) {
			continue
		}
		if err := trie.Prove(first, 0, firstProof); err != nil {
			t.Fatalf("Failed to prove the first node %v", err)
		}
		if err := trie.Prove(entries[end-1].k, 0, lastProof); err != nil {
			t.Fatalf("Failed to prove the last node %v", err)
		}
		var keys [][]byte
		var vals [][]byte
		for i := start; i < end; i++ {
			keys = append(keys, entries[i].k)
			vals = append(vals, entries[i].v)
		}
		err := VerifyRangeProof(trie.Hash(), first, keys, vals, firstProof, lastProof)
		if err != nil {
			t.Fatalf("Case %d(%d->%d) expect no error, got %v", i, start, end-1, err)
		}
	}
}

// TestRangeProofWithInvalidNonExistentProof tests such scenarios:
// - The last edge proof is an non-existent proof
// - There exists a gap between the first element and the left edge proof
func TestRangeProofWithInvalidNonExistentProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)

	// Case 1
	start, end := 100, 200
	first, last := decreseKey(common.CopyBytes(entries[start].k)), increseKey(common.CopyBytes(entries[end].k))
	firstProof, lastProof := memorydb.New(), memorydb.New()
	if err := trie.Prove(first, 0, firstProof); err != nil {
		t.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(last, 0, lastProof); err != nil {
		t.Fatalf("Failed to prove the last node %v", err)
	}
	var k [][]byte
	var v [][]byte
	for i := start; i < end; i++ {
		k = append(k, entries[i].k)
		v = append(v, entries[i].v)
	}
	err := VerifyRangeProof(trie.Hash(), first, k, v, firstProof, lastProof)
	if err == nil {
		t.Fatalf("Expected to detect the error, got nil")
	}

	// Case 2
	start, end = 100, 200
	first = decreseKey(common.CopyBytes(entries[start].k))

	firstProof, lastProof = memorydb.New(), memorydb.New()
	if err := trie.Prove(first, 0, firstProof); err != nil {
		t.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(entries[end-1].k, 0, lastProof); err != nil {
		t.Fatalf("Failed to prove the last node %v", err)
	}
	start = 105 // Gap created
	k = make([][]byte, 0)
	v = make([][]byte, 0)
	for i := start; i < end; i++ {
		k = append(k, entries[i].k)
		v = append(v, entries[i].v)
	}
	err = VerifyRangeProof(trie.Hash(), first, k, v, firstProof, lastProof)
	if err == nil {
		t.Fatalf("Expected to detect the error, got nil")
	}
}

// TestOneElementRangeProof tests the proof with only one
// element. The first edge proof can be existent one or
// non-existent one.
func TestOneElementRangeProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)

	// One element with existent edge proof
	start := 1000
	firstProof, lastProof := memorydb.New(), memorydb.New()
	if err := trie.Prove(entries[start].k, 0, firstProof); err != nil {
		t.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(entries[start].k, 0, lastProof); err != nil {
		t.Fatalf("Failed to prove the last node %v", err)
	}
	err := VerifyRangeProof(trie.Hash(), entries[start].k, [][]byte{entries[start].k}, [][]byte{entries[start].v}, firstProof, lastProof)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// One element with non-existent edge proof
	start = 1000
	first := decreseKey(common.CopyBytes(entries[start].k))
	firstProof, lastProof = memorydb.New(), memorydb.New()
	if err := trie.Prove(first, 0, firstProof); err != nil {
		t.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(entries[start].k, 0, lastProof); err != nil {
		t.Fatalf("Failed to prove the last node %v", err)
	}
	err = VerifyRangeProof(trie.Hash(), first, [][]byte{entries[start].k}, [][]byte{entries[start].v}, firstProof, lastProof)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

// TestEmptyRangeProof tests the range proof with "no" element.
// The first edge proof must be a non-existent proof.
func TestEmptyRangeProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)

	var cases = []struct {
		pos int
		err bool
	}{
		{len(entries) - 1, false},
		{500, true},
	}
	for _, c := range cases {
		firstProof := memorydb.New()
		first := increseKey(common.CopyBytes(entries[c.pos].k))
		if err := trie.Prove(first, 0, firstProof); err != nil {
			t.Fatalf("Failed to prove the first node %v", err)
		}
		err := VerifyRangeProof(trie.Hash(), first, nil, nil, firstProof, nil)
		if c.err && err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !c.err && err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}
}

// TestAllElementsProof tests the range proof with all elements.
// The edge proofs can be nil.
func TestAllElementsProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)

	var k [][]byte
	var v [][]byte
	for i := 0; i < len(entries); i++ {
		k = append(k, entries[i].k)
		v = append(v, entries[i].v)
	}
	err := VerifyRangeProof(trie.Hash(), k[0], k, v, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Even with edge proofs, it should still work.
	firstProof, lastProof := memorydb.New(), memorydb.New()
	if err := trie.Prove(entries[0].k, 0, firstProof); err != nil {
		t.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(entries[len(entries)-1].k, 0, lastProof); err != nil {
		t.Fatalf("Failed to prove the last node %v", err)
	}
	err = VerifyRangeProof(trie.Hash(), k[0], k, v, firstProof, lastProof)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

// TestSingleSideRangeProof tests the range starts from zero.
func TestSingleSideRangeProof(t *testing.T) {
	for i := 0; i < 64; i++ {
		trie := new(Trie)
		var entries entrySlice
		for i := 0; i < 4096; i++ {
			value := &kv{randBytes(32), randBytes(20), false}
			trie.Update(value.k, value.v)
			entries = append(entries, value)
		}
		sort.Sort(entries)

		var cases = []int{0, 1, 50, 100, 1000, 2000, len(entries) - 1}
		for _, pos := range cases {
			firstProof, lastProof := memorydb.New(), memorydb.New()
			if err := trie.Prove(common.Hash{}.Bytes(), 0, firstProof); err != nil {
				t.Fatalf("Failed to prove the first node %v", err)
			}
			if err := trie.Prove(entries[pos].k, 0, lastProof); err != nil {
				t.Fatalf("Failed to prove the first node %v", err)
			}
			k := make([][]byte, 0)
			v := make([][]byte, 0)
			for i := 0; i <= pos; i++ {
				k = append(k, entries[i].k)
				v = append(v, entries[i].v)
			}
			err := VerifyRangeProof(trie.Hash(), common.Hash{}.Bytes(), k, v, firstProof, lastProof)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		}
	}
}

// TestBadRangeProof tests a few cases which the proof is wrong.
// The prover is expected to detect the error.
func TestBadRangeProof(t *testing.T) {
	trie, vals := randomTrie(4096)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)

	for i := 0; i < 500; i++ {
		start := mrand.Intn(len(entries))
		end := mrand.Intn(len(entries)-start) + start
		if start == end {
			continue
		}
		firstProof, lastProof := memorydb.New(), memorydb.New()
		if err := trie.Prove(entries[start].k, 0, firstProof); err != nil {
			t.Fatalf("Failed to prove the first node %v", err)
		}
		if err := trie.Prove(entries[end-1].k, 0, lastProof); err != nil {
			t.Fatalf("Failed to prove the last node %v", err)
		}
		var keys [][]byte
		var vals [][]byte
		for i := start; i < end; i++ {
			keys = append(keys, entries[i].k)
			vals = append(vals, entries[i].v)
		}
		testcase := mrand.Intn(6)
		var index int
		switch testcase {
		case 0:
			// Modified key
			index = mrand.Intn(end - start)
			keys[index] = randBytes(32) // In theory it can't be same
		case 1:
			// Modified val
			index = mrand.Intn(end - start)
			vals[index] = randBytes(20) // In theory it can't be same
		case 2:
			// Gapped entry slice
			index = mrand.Intn(end - start)
			keys = append(keys[:index], keys[index+1:]...)
			vals = append(vals[:index], vals[index+1:]...)
			if len(keys) <= 1 {
				continue
			}
		case 3:
			// Switched entry slice, same effect with gapped
			index = mrand.Intn(end - start)
			keys[index] = entries[len(entries)-1].k
			vals[index] = entries[len(entries)-1].v
		case 4:
			// Set random key to nil
			index = mrand.Intn(end - start)
			keys[index] = nil
		case 5:
			// Set random value to nil
			index = mrand.Intn(end - start)
			vals[index] = nil
		}
		err := VerifyRangeProof(trie.Hash(), keys[0], keys, vals, firstProof, lastProof)
		if err == nil {
			t.Fatalf("%d Case %d index %d range: (%d->%d) expect error, got nil", i, testcase, index, start, end-1)
		}
	}
}

// TestGappedRangeProof focuses on the small trie with embedded nodes.
// If the gapped node is embedded in the trie, it should be detected too.
func TestGappedRangeProof(t *testing.T) {
	trie := new(Trie)
	var entries []*kv // Sorted entries
	for i := byte(0); i < 10; i++ {
		value := &kv{common.LeftPadBytes([]byte{i}, 32), []byte{i}, false}
		trie.Update(value.k, value.v)
		entries = append(entries, value)
	}
	first, last := 2, 8
	firstProof, lastProof := memorydb.New(), memorydb.New()
	if err := trie.Prove(entries[first].k, 0, firstProof); err != nil {
		t.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(entries[last-1].k, 0, lastProof); err != nil {
		t.Fatalf("Failed to prove the last node %v", err)
	}
	var keys [][]byte
	var vals [][]byte
	for i := first; i < last; i++ {
		if i == (first+last)/2 {
			continue
		}
		keys = append(keys, entries[i].k)
		vals = append(vals, entries[i].v)
	}
	err := VerifyRangeProof(trie.Hash(), keys[0], keys, vals, firstProof, lastProof)
	if err == nil {
		t.Fatal("expect error, got nil")
	}
}

// mutateByte changes one byte in b.
func mutateByte(b []byte) {
	for r := mrand.Intn(len(b)); ; {
		new := byte(mrand.Intn(255))
		if new != b[r] {
			b[r] = new
			break
		}
	}
}

func increseKey(key []byte) []byte {
	for i := len(key) - 1; i >= 0; i-- {
		key[i]++
		if key[i] != 0x0 {
			break
		}
	}
	return key
}

func decreseKey(key []byte) []byte {
	for i := len(key) - 1; i >= 0; i-- {
		key[i]--
		if key[i] != 0xff {
			break
		}
	}
	return key
}

func BenchmarkProve(b *testing.B) {
	trie, vals := randomTrie(100)
	var keys []string
	for k := range vals {
		keys = append(keys, k)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kv := vals[keys[i%len(keys)]]
		proofs := memorydb.New()
		if trie.Prove(kv.k, 0, proofs); proofs.Len() == 0 {
			b.Fatalf("zero length proof for %x", kv.k)
		}
	}
}

func BenchmarkVerifyProof(b *testing.B) {
	trie, vals := randomTrie(100)
	root := trie.Hash()
	var keys []string
	var proofs []*memorydb.Database
	for k := range vals {
		keys = append(keys, k)
		proof := memorydb.New()
		trie.Prove([]byte(k), 0, proof)
		proofs = append(proofs, proof)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		im := i % len(keys)
		if _, err := VerifyProof(root, []byte(keys[im]), proofs[im]); err != nil {
			b.Fatalf("key %x: %v", keys[im], err)
		}
	}
}

func BenchmarkVerifyRangeProof10(b *testing.B)   { benchmarkVerifyRangeProof(b, 10) }
func BenchmarkVerifyRangeProof100(b *testing.B)  { benchmarkVerifyRangeProof(b, 100) }
func BenchmarkVerifyRangeProof1000(b *testing.B) { benchmarkVerifyRangeProof(b, 1000) }
func BenchmarkVerifyRangeProof5000(b *testing.B) { benchmarkVerifyRangeProof(b, 5000) }

func benchmarkVerifyRangeProof(b *testing.B, size int) {
	trie, vals := randomTrie(8192)
	var entries entrySlice
	for _, kv := range vals {
		entries = append(entries, kv)
	}
	sort.Sort(entries)

	start := 2
	end := start + size
	firstProof, lastProof := memorydb.New(), memorydb.New()
	if err := trie.Prove(entries[start].k, 0, firstProof); err != nil {
		b.Fatalf("Failed to prove the first node %v", err)
	}
	if err := trie.Prove(entries[end-1].k, 0, lastProof); err != nil {
		b.Fatalf("Failed to prove the last node %v", err)
	}
	var keys [][]byte
	var values [][]byte
	for i := start; i < end; i++ {
		keys = append(keys, entries[i].k)
		values = append(values, entries[i].v)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := VerifyRangeProof(trie.Hash(), keys[0], keys, values, firstProof, lastProof)
		if err != nil {
			b.Fatalf("Case %d(%d->%d) expect no error, got %v", i, start, end-1, err)
		}
	}
}

func randomTrie(n int) (*Trie, map[string]*kv) {
	trie := new(Trie)
	vals := make(map[string]*kv)
	for i := byte(0); i < 100; i++ {
		value := &kv{common.LeftPadBytes([]byte{i}, 32), []byte{i}, false}
		value2 := &kv{common.LeftPadBytes([]byte{i + 10}, 32), []byte{i}, false}
		trie.Update(value.k, value.v)
		trie.Update(value2.k, value2.v)
		vals[string(value.k)] = value
		vals[string(value2.k)] = value2
	}
	for i := 0; i < n; i++ {
		value := &kv{randBytes(32), randBytes(20), false}
		trie.Update(value.k, value.v)
		vals[string(value.k)] = value
	}
	return trie, vals
}

func randBytes(n int) []byte {
	r := make([]byte, n)
	crand.Read(r)
	return r
}
