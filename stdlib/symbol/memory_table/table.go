package memory_table

import (
	"bytes"
	"sync"
	"sync/atomic"

	"github.com/art-media-platform/amp.SDK/stdlib/bufs"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol"
)

func createTable(opts TableOpts) (symbol.Table, error) {
	if opts.Issuer == nil {
		opts.Issuer = symbol.NewVolatileIssuer(opts.IssuerInitsAt)
	} else {
		opts.Issuer.AddRef()
	}

	st := &symbolTable{
		opts:          opts,
		curBufPoolIdx: -1,
		valueCache:    make(map[uint64]kvEntry, opts.WorkingSizeHint),
		tokenCache:    make(map[symbol.ID]kvEntry, opts.WorkingSizeHint),
	}

	st.refCount.Store(1)
	return st, nil
}

func (st *symbolTable) Issuer() symbol.Issuer {
	return st.opts.Issuer
}

func (st *symbolTable) AddRef() {
	st.refCount.Add(1)
}

func (st *symbolTable) Close() error {
	if st.refCount.Add(-1) > 0 {
		return nil
	}
	return st.close()
}

func (st *symbolTable) close() error {
	err := st.opts.Issuer.Close()
	st.opts.Issuer = nil

	st.valueCache = nil
	st.tokenCache = nil
	st.bufPools = nil
	return err
}

type kvEntry struct {
	symID   symbol.ID
	poolIdx int32
	poolOfs int32
	len     int32
}

func (st *symbolTable) equals(kv *kvEntry, buf []byte) bool {
	sz := int32(len(buf))
	if sz != kv.len {
		return false
	}
	return bytes.Equal(st.bufPools[kv.poolIdx][kv.poolOfs:kv.poolOfs+sz], buf)
}

func (st *symbolTable) bufForEntry(kv kvEntry) []byte {
	if kv.symID == 0 {
		return nil
	}
	return st.bufPools[kv.poolIdx][kv.poolOfs : kv.poolOfs+kv.len]
}

// symbolTable implements symbol.Table
type symbolTable struct {
	opts          TableOpts
	refCount      atomic.Int32
	valueCacheMu  sync.RWMutex          // Protects valueCache
	valueCache    map[uint64]kvEntry    // Maps a entry value hash to a kvEntry
	tokenCacheMu  sync.RWMutex          // Protects tokenCache
	tokenCache    map[symbol.ID]kvEntry // Maps an ID ("token") to an entry
	curBufPool    []byte
	curBufPoolSz  int32
	curBufPoolIdx int32
	bufPools      [][]byte
}

func (st *symbolTable) getIDFromCache(buf []byte) symbol.ID {
	hash := bufs.HashBuf(buf)

	st.valueCacheMu.RLock()
	defer st.valueCacheMu.RUnlock()

	kv, found := st.valueCache[hash]
	for found {
		if st.equals(&kv, buf) {
			return kv.symID
		}
		hash++
		kv, found = st.valueCache[hash]
	}

	return 0
}

func (st *symbolTable) allocAndBindToID(buf []byte, bindID symbol.ID) kvEntry {
	hash := bufs.HashBuf(buf)

	st.valueCacheMu.Lock()
	defer st.valueCacheMu.Unlock()

	kv, found := st.valueCache[hash]
	for found {
		if st.equals(&kv, buf) {
			break
		}
		hash++
		kv, found = st.valueCache[hash]
	}

	// No-op if already present
	if found && kv.symID == bindID {
		return kv
	}

	// At this point we know [hash] will be the destination element
	// Add a copy of the buf in our backing buf (in the heap).
	// If we run out of space in our pool, we start a new pool
	kv.symID = bindID
	{
		kv.len = int32(len(buf))
		if int(st.curBufPoolSz+kv.len) > cap(st.curBufPool) {
			allocSz := max(st.opts.PoolSz, kv.len)
			st.curBufPool = make([]byte, allocSz)
			st.curBufPoolSz = 0
			st.curBufPoolIdx++
			st.bufPools = append(st.bufPools, st.curBufPool)
		}
		kv.poolIdx = st.curBufPoolIdx
		kv.poolOfs = st.curBufPoolSz
		copy(st.curBufPool[kv.poolOfs:kv.poolOfs+kv.len], buf)
		st.curBufPoolSz += kv.len
	}

	// Place the now-backed copy at the open hash spot and return the alloced value
	st.valueCache[hash] = kv

	st.tokenCacheMu.Lock()
	st.tokenCache[kv.symID] = kv
	st.tokenCacheMu.Unlock()

	return kv
}

func (st *symbolTable) GetSymbolID(val []byte, autoIssue bool) (symbol.ID, bool) {
	symID := st.getIDFromCache(val)
	if symID != 0 {
		return symID, false
	}

	symID, _ = st.getsetValueIDPair(val, 0, autoIssue)
	return symID, symID != 0
}

func (st *symbolTable) SetSymbolID(val []byte, symID symbol.ID) (symbol.ID, bool) {
	// If symID == 0, then behave like GetSymbolID(val, true)
	return st.getsetValueIDPair(val, symID, symID == 0)
}

// getsetValueIDPair loads and returns the ID for the given value, and/or writes the ID and value assignment to the db,
// also updating the cache in the process.
//
//	if symID == 0:
//	  if the given value has an existing value-ID association:
//	      the existing ID is cached and returned (autoIssue is ignored).
//	  if the given value does NOT have an existing value-ID association:
//	      if autoIssue == false, the call has no effect and 0 is returned.
//	      if autoIssue == true, a new ID is issued and new value-to-ID and ID-to-value assignments are written,
//
//	if symID != 0:
//	    if autoIssue == false, a new value-to-ID assignment is (over)written and any existing ID-to-value assignment remains.
//	    if autoIssue == true, both value-to-ID and ID-to-value assignments are (over)written.
func (st *symbolTable) getsetValueIDPair(val []byte, symID symbol.ID, autoIssue bool) (symbol.ID, bool) {

	// The empty string is always mapped to ID 0
	if len(val) == 0 {
		return 0, false
	}

	issued := false
	if symID == 0 && autoIssue {
		issued = true
		symID, _ = st.opts.Issuer.MintNext()
	}

	// Update the cache
	if symID != 0 {
		st.allocAndBindToID(val, symID)
	}
	return symID, issued
}

func (st *symbolTable) GetSymbol(symID symbol.ID, io []byte) []byte {
	if symID == 0 {
		return nil
	}

	st.tokenCacheMu.RLock()
	kv := st.tokenCache[symID]
	st.tokenCacheMu.RUnlock()

	// At this point, if symID wasn't found, kv will be zero and causing nil to be returned
	symBuf := st.bufForEntry(kv)
	if symBuf == nil {
		return nil
	}
	return append(io, symBuf...)
}

func max(a, b int32) int32 {
	if a > b {
		return a
	} else {
		return b
	}
}
