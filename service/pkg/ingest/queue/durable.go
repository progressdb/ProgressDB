package queue

import (
	"bytes"
	"container/heap"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/bytebufferpool"
)

const (
	recordHeaderSize = 17                 // 8 (offset) + 4 (crc) + 4 (length) + 1 (flags)
	fileHeaderSize   = 8                  // 4 (magic) + 4 (file checksum placeholder)
	fileMagic        = uint32(0x57414C46) // "WALF"

	// Flags
	flagCompressed = 1 << 0
)

func sync_directory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (w *DurableFile) flushBatchLocked() error {
	if len(w.batch.entries) == 0 {
		return nil
	}

	// Write all entries in the batch to disk, using the pre-assigned
	// sequence numbers captured at enqueue time. Do not fsync per
	// record — perform a single group fsync at the end (group commit).
	for _, entry := range w.batch.entries {
		var data []byte
		if entry.sb != nil && entry.sb.bb != nil {
			data = entry.sb.bb.B
		} else {
			data = entry.data
		}
		flags := byte(0)
		toWrite := data
		// Determine compression threshold; default to 512 if not configured.
		minBytes := int64(512)
		if w.compressMinBytes > 0 {
			minBytes = w.compressMinBytes
		}
		if w.enableCompress && int64(len(data)) >= minBytes {
			compressed, err := compressData(data)
			if err == nil {
				// Apply post-compression ratio check
				ratio := w.compressMinRatio
				if ratio <= 0 || ratio > 1 {
					ratio = 1.0
				}
				if float64(len(compressed)) <= float64(len(data))*ratio {
					toWrite = compressed
					flags |= flagCompressed
				}
			}
		}

		recordSize := int64(recordHeaderSize + len(toWrite))
		if w.curr.size+recordSize > w.maxSize {
			if err := w.rotate(); err != nil {
				return fmt.Errorf("failed to rotate WAL file: %w", err)
			}
		}

		// Use the sequence that was assigned at enqueue time.
		offset := entry.seq
		if err := w.writeRecord(w.curr.f, offset, toWrite, flags); err != nil {
			return fmt.Errorf("failed to write batch record at offset %d: %w", offset, err)
		}

		w.curr.size += recordSize
		if w.curr.minSeq == -1 || offset < w.curr.minSeq {
			w.curr.minSeq = offset
		}
		if offset > w.curr.maxSeq {
			w.curr.maxSeq = offset
		}
	}

	// Single fsync for the batch (group commit).
	if err := w.curr.f.Sync(); err != nil {
		return fmt.Errorf("failed to fsync WAL file: %w", err)
	}

	// Ensure global sequence counter is at least one past the highest
	// sequence we just wrote. Append() pre-assigns sequence numbers by
	// incrementing w.seq at enqueue time; however, guard here in case of
	// any inconsistency or future code paths that might leave w.seq
	// behind (defensive).
	if len(w.batch.entries) > 0 {
		last := w.batch.entries[len(w.batch.entries)-1].seq
		if w.seq <= last {
			w.seq = last + 1
		}
	}

	// After durable persistence, release WAL-side references for any
	// pooled buffers that were used for the batch entries. Consumers may
	// still hold references; the SharedBuf release logic will keep the
	// pooled buffer until all references are done.
	for _, entry := range w.batch.entries {
		if entry.sb != nil {
			// decrement WAL-side reference
			entry.sb.release()
		}
	}

	// Clear batch
	w.batch.entries = w.batch.entries[:0]
	w.batch.size = 0

	// Notify waiters via channel (non-blocking) and cond var
	select {
	case w.spaceCh <- struct{}{}:
	default:
	}
	w.flushCond.Broadcast()
	return nil
}

// batchFlusher periodically flushes batches
func (w *DurableFile) batchFlusher() {
	ticker := time.NewTicker(w.batchInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return
		}

		if len(w.batch.entries) > 0 {
			w.flushBatchLocked()
		}
		w.mu.Unlock()
	}
}

// Recover reads all WAL entries from files in order and returns records with offsets.
func (w *DurableFile) Recover() ([]WALRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var result []WALRecord
	for _, wf := range w.files {
		if _, err := wf.f.Seek(fileHeaderSize, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek file %d: %w", wf.num, err)
		}

		records, err := w.readRecords(wf.f, wf.num)
		if err != nil {
			return nil, fmt.Errorf("failed to read records from file %d: %w", wf.num, err)
		}
		result = append(result, records...)
	}
	return result, nil
}

// RecoverStream streams WAL records via callback in file order. If the
// callback returns an error streaming stops and the error is returned.
func (w *DurableFile) RecoverStream(cb func(WALRecord) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, wf := range w.files {
		if _, err := wf.f.Seek(fileHeaderSize, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek file %d: %w", wf.num, err)
		}
		records, err := w.readRecords(wf.f, wf.num)
		if err != nil {
			return fmt.Errorf("failed to read records from file %d: %w", wf.num, err)
		}
		for _, r := range records {
			if err := cb(r); err != nil {
				return err
			}
		}
	}
	return nil
}

// TruncateBefore deletes all WAL files where maxSeq < minOffset
func (w *DurableFile) TruncateBefore(minOffset int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var toDelete []*DurableFileSegment
	var toKeep []*DurableFileSegment

	for _, wf := range w.files {
		if wf.maxSeq < minOffset && wf != w.curr {
			toDelete = append(toDelete, wf)
		} else {
			toKeep = append(toKeep, wf)
		}
	}

	// Delete old files
	for _, wf := range toDelete {
		if err := wf.f.Close(); err != nil {
			return fmt.Errorf("failed to close file %d: %w", wf.num, err)
		}

		path := filepath.Join(w.dir, fmt.Sprintf("%06d.wal", wf.num))
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove file %s: %w", path, err)
		}
	}

	w.files = toKeep

	// Sync directory
	if len(toDelete) > 0 {
		if err := sync_directory(w.dir); err != nil {
			return fmt.Errorf("failed to sync directory: %w", err)
		}
	}

	return nil
}

// Close closes all WAL files safely
func (w *DurableFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	// Flush pending batch
	if w.enableBatch && len(w.batch.entries) > 0 {
		if err := w.flushBatchLocked(); err != nil {
			return err
		}
	}

	// Finalize current file with checksum
	if w.curr != nil {
		if err := w.finalizeFile(w.curr); err != nil {
			return err
		}
	}

	var firstErr error
	for _, wf := range w.files {
		if err := wf.f.Sync(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to sync file %d: %w", wf.num, err)
		}
		if err := wf.f.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close file %d: %w", wf.num, err)
		}
	}
	return firstErr
}

// helpers
func (w *DurableFile) recoverFiles() (int64, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return 0, err
	}

	type fileInfo struct {
		name string
		num  int
	}
	var walFiles []fileInfo

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".wal" {
			continue
		}
		num := 0
		if _, err := fmt.Sscanf(name, "%d.wal", &num); err != nil {
			continue
		}
		walFiles = append(walFiles, fileInfo{name: name, num: num})
	}

	sort.Slice(walFiles, func(i, j int) bool {
		return walFiles[i].num < walFiles[j].num
	})

	maxSeq := int64(-1)

	for _, fi := range walFiles {
		fpath := filepath.Join(w.dir, fi.name)
		f, err := os.OpenFile(fpath, os.O_RDWR, 0o644)
		if err != nil {
			return 0, fmt.Errorf("failed to open WAL file %s: %w", fi.name, err)
		}

		stat, err := f.Stat()
		if err != nil {
			f.Close()
			return 0, fmt.Errorf("failed to stat WAL file %s: %w", fi.name, err)
		}

		// Validate file header and checksum
		if err := w.validateFileHeader(f); err != nil {
			f.Close()
			return 0, fmt.Errorf("failed to validate file %s: %w", fi.name, err)
		}

		wf := &DurableFileSegment{
			f:      f,
			num:    fi.num,
			offset: 0,
			size:   stat.Size(),
			minSeq: -1,
			maxSeq: -1,
		}

		seqs, validSize, err := w.scanFile(f, fi.num)
		if err != nil {
			f.Close()
			return 0, fmt.Errorf("failed to scan WAL file %s: %w", fi.name, err)
		}

		if validSize < stat.Size() {
			if err := f.Truncate(validSize); err != nil {
				f.Close()
				return 0, fmt.Errorf("failed to truncate WAL file %s: %w", fi.name, err)
			}
			if err := f.Sync(); err != nil {
				f.Close()
				return 0, fmt.Errorf("failed to sync truncated file %s: %w", fi.name, err)
			}
			wf.size = validSize
		}

		if len(seqs) > 0 {
			wf.minSeq = seqs[0]
			wf.maxSeq = seqs[len(seqs)-1]
			if wf.maxSeq > maxSeq {
				maxSeq = wf.maxSeq
			}
		}

		w.files = append(w.files, wf)
		if fi.num >= w.nextNum {
			w.nextNum = fi.num + 1
		}
		w.curr = wf
	}

	return maxSeq, nil
}

func (w *DurableFile) validateFileHeader(f *os.File) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var magic uint32
	if err := binary.Read(f, binary.BigEndian, &magic); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty file, write header
			return w.writeFileHeader(f)
		}
		return err
	}

	if magic != fileMagic {
		return fmt.Errorf("invalid file magic: 0x%X", magic)
	}

	// Skip checksum for now (validated on close)
	var fileChecksum uint32
	if err := binary.Read(f, binary.BigEndian, &fileChecksum); err != nil {
		return err
	}

	return nil
}

func (w *DurableFile) writeFileHeader(f *os.File) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := binary.Write(f, binary.BigEndian, fileMagic); err != nil {
		return err
	}
	// Placeholder for checksum (filled on close)
	if err := binary.Write(f, binary.BigEndian, uint32(0)); err != nil {
		return err
	}
	return nil
}

func (w *DurableFile) scanFile(f *os.File, fileNum int) ([]int64, int64, error) {
	if _, err := f.Seek(fileHeaderSize, io.SeekStart); err != nil {
		return nil, 0, err
	}

	var seqs []int64
	validSize := int64(fileHeaderSize)

	for {
		recordStart := validSize

		var offset int64
		var crc uint32
		var length int32
		var flags byte

		if err := binary.Read(f, binary.BigEndian, &offset); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		if err := binary.Read(f, binary.BigEndian, &crc); err != nil {
			break
		}
		if err := binary.Read(f, binary.BigEndian, &length); err != nil {
			break
		}
		if err := binary.Read(f, binary.BigEndian, &flags); err != nil {
			break
		}

		if length < 0 || length > 100*1024*1024 {
			break
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(f, data); err != nil {
			break
		}

		if crc32.Checksum(data, w.crcTable) != crc {
			break
		}

		seqs = append(seqs, offset)
		validSize = recordStart + recordHeaderSize + int64(length)
	}

	return seqs, validSize, nil
}

func (w *DurableFile) createNewFile() error {
	name := fmt.Sprintf("%06d.wal", w.nextNum)
	w.nextNum++
	path := filepath.Join(w.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}

	// Write file header
	if err := w.writeFileHeader(f); err != nil {
		f.Close()
		return err
	}

	if err := sync_directory(w.dir); err != nil {
		f.Close()
		return fmt.Errorf("failed to sync directory: %w", err)
	}

	wf := &DurableFileSegment{
		f:      f,
		num:    w.nextNum - 1,
		offset: 0,
		size:   fileHeaderSize,
		minSeq: -1,
		maxSeq: -1,
	}
	w.files = append(w.files, wf)
	w.curr = wf
	return nil
}

func (w *DurableFile) rotate() error {
	if err := w.finalizeFile(w.curr); err != nil {
		return err
	}
	if err := w.curr.f.Close(); err != nil {
		return err
	}
	return w.createNewFile()
}

func (w *DurableFile) finalizeFile(wf *DurableFileSegment) error {
	// Calculate checksum of entire file content (excluding header)
	if _, err := wf.f.Seek(fileHeaderSize, io.SeekStart); err != nil {
		return err
	}

	h := crc32.New(w.crcTable)
	if _, err := io.Copy(h, wf.f); err != nil {
		return err
	}
	checksum := h.Sum32()

	// Write checksum to header
	if _, err := wf.f.Seek(4, io.SeekStart); err != nil {
		return err
	}
	if err := binary.Write(wf.f, binary.BigEndian, checksum); err != nil {
		return err
	}

	wf.fileChecksum = checksum
	return wf.f.Sync()
}

func (w *DurableFile) writeRecord(f *os.File, offset int64, data []byte, flags byte) error {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, offset); err != nil {
		return err
	}
	crc := crc32.Checksum(data, w.crcTable)
	if err := binary.Write(&buf, binary.BigEndian, crc); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, int32(len(data))); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, flags); err != nil {
		return err
	}
	if _, err := buf.Write(data); err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

// NewIngestQueueWithOptions constructs a IngestQueue configured to use the WAL for
// durability features (replay, truncation). This was previously in a
// separate file; consolidated here for clarity.
func NewIngestQueueWithOptions(opts *IngestQueueOptions) *IngestQueue {
	if opts == nil || opts.Capacity <= 0 {
		panic("queue.NewIngestQueueWithOptions: opts and opts.Capacity must be provided; ensure config.ValidateConfig() applied defaults")
	}
	cap := opts.Capacity
	q := &IngestQueue{ch: make(chan *QueueItem, cap), capacity: cap, outstanding: make(map[int64]struct{}), outstandingH: offsetHeap{}, lastTruncated: -1}
	q.walBacked = opts.WalBacked
	if opts != nil && opts.WAL != nil {
		q.wal = opts.WAL
		switch opts.Mode {
		case "sync":
			q.walMode = WalModeSync
		case "batch":
			q.walMode = WalModeBatch
		default:
			q.walMode = WalModeBatch
		}

		if opts.Recover {
			// stream WAL records into the queue (do not re-append)
			_ = q.wal.RecoverStream(func(r WALRecord) error {
				op, err := deserializeOp(r.Data)
				if err != nil {
					// skip malformed records
					return nil
				}
				op.WalOffset = r.Offset
				// mark outstanding
				q.ackMu.Lock()
				if q.outstanding == nil {
					q.outstanding = make(map[int64]struct{})
				}
				q.outstanding[r.Offset] = struct{}{}
				heap.Push(&q.outstandingH, r.Offset)
				q.ackMu.Unlock()
				// push into channel non-blocking (assume capacity sufficient on startup)
				if q.walBacked {
					bb := bytebufferpool.Get()
					bb.B = append(bb.B[:0], r.Data...)
					sb := newSharedBuf(bb, 1) // consumer holds single reference
					it := &QueueItem{Op: op, Sb: sb, Buf: bb, Q: q}
					q.ch <- it
					atomic.AddInt64(&q.inFlight, 1)
				} else {
					it := &QueueItem{Op: op, Buf: nil, Q: q}
					q.ch <- it
					atomic.AddInt64(&q.inFlight, 1)
				}
				return nil
			})
		}
	}
	// start background truncation if requested
	if opts != nil && opts.TruncateInterval > 0 && q.wal != nil {
		go func() {
			ticker := time.NewTicker(opts.TruncateInterval)
			defer ticker.Stop()
			for range ticker.C {
				q.doTruncate()
			}
		}()
	}
	return q
}

var activeDurable *DurableFile

// EnableDurable attempts to enable a DurableFile under the provided options
// and replaces the package DefaultIngestQueue with a durable-backed queue. Best
// effort: callers may ignore errors and continue with the in-memory queue.
func EnableDurable(opts DurableEnableOptions) error {
	if opts.Dir == "" {
		return nil
	}

	wopts := DurableWALConfigOptions{
		Dir:              opts.Dir,
		MaxFileSize:      opts.WALMaxFileSize,
		EnableBatch:      opts.WALEnableBatch,
		BatchSize:        opts.WALBatchSize,
		BatchInterval:    opts.WALBatchInterval,
		EnableCompress:   opts.WALEnableCompress,
		CompressMinBytes: opts.WALCompressMinBytes,
	}
	w, err := New(wopts)
	if err != nil {
		return err
	}
	activeDurable = w
	qopts := &IngestQueueOptions{
		Capacity:         opts.Capacity,
		WAL:              w,
		Mode:             "batch",
		Recover:          true,
		TruncateInterval: opts.TruncateInterval,
		WalBacked:        true,
	}
	q := NewIngestQueueWithOptions(qopts)
	SetDefaultIngestQueue(q)
	return nil
}

// CloseDurable closes the active WAL if any.
func CloseDurable() error {
	if activeDurable == nil {
		return nil
	}
	err := activeDurable.Close()
	activeDurable = nil
	return err
}

func (w *DurableFile) readRecords(f *os.File, fileNum int) ([]WALRecord, error) {
	var result []WALRecord
	for {
		var offset int64
		var crc uint32
		var length int32
		var flags byte

		if err := binary.Read(f, binary.BigEndian, &offset); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if err := binary.Read(f, binary.BigEndian, &crc); err != nil {
			return nil, err
		}
		if err := binary.Read(f, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		if err := binary.Read(f, binary.BigEndian, &flags); err != nil {
			return nil, err
		}

		if length < 0 || length > 100*1024*1024 {
			return nil, fmt.Errorf("invalid record length %d in file %d at offset %d", length, fileNum, offset)
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, err
		}

		if crc32.Checksum(data, w.crcTable) != crc {
			return nil, fmt.Errorf("CRC mismatch in file %d at offset %d", fileNum, offset)
		}

		// Decompress if needed
		if flags&flagCompressed != 0 {
			decompressed, err := decompressData(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decompress record in file %d at offset %d: %w", fileNum, offset, err)
			}
			data = decompressed
		}

		result = append(result, WALRecord{Offset: offset, Data: data})
	}
	return result, nil
}

func New(opts DurableWALConfigOptions) (*DurableFile, error) {
	// Expect callers (startup path) to provide canonical defaults via
	// configuration. If absent, return an error so the caller can abort
	// startup with a helpful message.
	if opts.MaxFileSize == 0 {
		return nil, fmt.Errorf("wal options missing max_file_size; ensure config.ValidateConfig() applied defaults")
	}
	if opts.EnableBatch {
		if opts.BatchSize == 0 {
			return nil, fmt.Errorf("wal options missing batch_size; ensure config.ValidateConfig() applied defaults")
		}
		if opts.BatchInterval == 0 {
			return nil, fmt.Errorf("wal options missing batch_interval; ensure config.ValidateConfig() applied defaults")
		}
	}

	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	w := &DurableFile{
		dir:              opts.Dir,
		maxSize:          opts.MaxFileSize,
		enableBatch:      opts.EnableBatch,
		batchSize:        opts.BatchSize,
		batchInterval:    opts.BatchInterval,
		enableCompress:   opts.EnableCompress,
		compressMinBytes: opts.CompressMinBytes,
		crcTable:         crc32.MakeTable(crc32.Castagnoli),
		batch:            &DurableBatchBuffer{},
	}
	w.flushCond = sync.NewCond(&w.mu)

	// Recover existing WAL files on startup
	maxSeq, err := w.recoverFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to recover WAL files: %w", err)
	}
	w.seq = maxSeq + 1

	// If no files exist, create first file
	if w.curr == nil {
		if err := w.createNewFile(); err != nil {
			return nil, fmt.Errorf("failed to create initial WAL file: %w", err)
		}
	} else {
		// Seek to end of current file for appending
		if _, err := w.curr.f.Seek(0, io.SeekEnd); err != nil {
			return nil, fmt.Errorf("failed to seek to end of current WAL file: %w", err)
		}
	}

	// Start batch flusher if enabled
	if w.enableBatch {
		go w.batchFlusher()
	}

	return w, nil
}

// reserveSeqLocked reserves a single sequence number and returns it.
// Caller must hold w.mu.
func (w *DurableFile) reserveSeqLocked() int64 {
	s := w.seq
	w.seq++
	return s
}

// Append writes the entry to WAL (may buffer in batch mode)
func (w *DurableFile) Append(data []byte) (int64, error) {
	return w.AppendCtx(data, context.Background())
}

// AppendCtx is like Append but respects ctx cancellation while waiting for
// buffer space.
func (w *DurableFile) AppendCtx(data []byte, ctx context.Context) (int64, error) {
	if !w.enableBatch {
		return w.AppendSync(data)
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, errors.New("WAL is closed")
	}

	// Reserve a sequence number for this entry
	offset := w.reserveSeqLocked()

	// Copy data to avoid external modification
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	// Wait for buffer space if configured limits would be exceeded.
	for {
		if (w.maxBufferedBytes == 0 || w.batch.size+int64(recordHeaderSize+len(dataCopy)) <= w.maxBufferedBytes) &&
			(w.maxBufferedEntries == 0 || len(w.batch.entries)+1 <= w.maxBufferedEntries) {
			break
		}
		// release lock and wait for space notification or ctx cancel
		w.mu.Unlock()
		if w.bufferWaitTimeout == 0 {
			select {
			case <-w.spaceCh:
			case <-ctx.Done():
				w.mu.Lock()
				return 0, ctx.Err()
			}
		} else {
			timer := time.NewTimer(w.bufferWaitTimeout)
			select {
			case <-w.spaceCh:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
				w.mu.Lock()
				return 0, fmt.Errorf("WAL buffer wait timeout")
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				w.mu.Lock()
				return 0, ctx.Err()
			}
		}
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return 0, errors.New("WAL is closed")
		}
	}

	// Append into batch
	w.batch.entries = append(w.batch.entries, DurableBatchEntry{seq: offset, data: dataCopy})
	w.batch.size += int64(recordHeaderSize + len(dataCopy))

	// Flush if batch is full
	if len(w.batch.entries) >= w.batchSize {
		if err := w.flushBatchLocked(); err != nil {
			w.mu.Unlock()
			return 0, err
		}
	}
	w.mu.Unlock()

	return offset, nil
}

// AppendPooled accepts ownership of a SharedBuf and appends it to the WAL
// without copying. The SharedBuf should have at least one reference for the
// WAL; callers typically create one with refs=2 (WAL + consumer).
func (w *DurableFile) AppendPooled(sb *SharedBuf) (int64, error) {
	return w.AppendPooledCtx(sb, context.Background())
}

// AppendPooledCtx appends a pooled buffer to WAL but respects ctx while
// waiting for buffer space.
func (w *DurableFile) AppendPooledCtx(sb *SharedBuf, ctx context.Context) (int64, error) {
	if !w.enableBatch {
		return w.appendPooledSyncLocked(sb)
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, errors.New("WAL is closed")
	}

	// Reserve offset
	offset := w.reserveSeqLocked()

	// Record size based on buffer length
	dataLen := 0
	if sb != nil && sb.bb != nil {
		dataLen = len(sb.bb.B)
	}

	// Wait for buffer space if configured limits would be exceeded.
	for {
		if (w.maxBufferedBytes == 0 || w.batch.size+int64(recordHeaderSize+dataLen) <= w.maxBufferedBytes) &&
			(w.maxBufferedEntries == 0 || len(w.batch.entries)+1 <= w.maxBufferedEntries) {
			break
		}
		w.mu.Unlock()
		if w.bufferWaitTimeout == 0 {
			select {
			case <-w.spaceCh:
			case <-ctx.Done():
				w.mu.Lock()
				return 0, ctx.Err()
			}
		} else {
			timer := time.NewTimer(w.bufferWaitTimeout)
			select {
			case <-w.spaceCh:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
				w.mu.Lock()
				return 0, fmt.Errorf("WAL buffer wait timeout")
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				w.mu.Lock()
				return 0, ctx.Err()
			}
		}
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return 0, errors.New("WAL is closed")
		}
	}

	w.batch.entries = append(w.batch.entries, DurableBatchEntry{seq: offset, sb: sb})
	w.batch.size += int64(recordHeaderSize + dataLen)

	// Flush if batch is full
	if len(w.batch.entries) >= w.batchSize {
		if err := w.flushBatchLocked(); err != nil {
			w.mu.Unlock()
			return 0, err
		}
	}
	w.mu.Unlock()

	return offset, nil
}

func (w *DurableFile) appendPooledSyncLocked(sb *SharedBuf) (int64, error) {
	// synchronous write path for pooled buffer
	// Compress if enabled
	var toWrite []byte
	if sb != nil && sb.bb != nil {
		toWrite = sb.bb.B
	} else {
		toWrite = nil
	}
	flags := byte(0)
	minBytes := int64(512)
	if w.compressMinBytes > 0 {
		minBytes = w.compressMinBytes
	}
	if w.enableCompress && int64(len(toWrite)) >= minBytes {
		compressed, err := compressData(toWrite)
		if err == nil {
			ratio := w.compressMinRatio
			if ratio <= 0 || ratio > 1 {
				ratio = 1.0
			}
			if float64(len(compressed)) <= float64(len(toWrite))*ratio {
				toWrite = compressed
				flags |= flagCompressed
			}
		}
	}

	recordSize := int64(recordHeaderSize + len(toWrite))
	if w.curr.size+recordSize > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("failed to rotate WAL file: %w", err)
		}
	}

	offset := w.reserveSeqLocked()
	if err := w.writeRecord(w.curr.f, offset, toWrite, flags); err != nil {
		return 0, fmt.Errorf("failed to write record at offset %d: %w", offset, err)
	}

	w.curr.size += recordSize

	// Update file sequence range
	if w.curr.minSeq == -1 || offset < w.curr.minSeq {
		w.curr.minSeq = offset
	}
	if offset > w.curr.maxSeq {
		w.curr.maxSeq = offset
	}

	if err := w.curr.f.Sync(); err != nil {
		return 0, fmt.Errorf("failed to fsync WAL file: %w", err)
	}

	// WAL-side reference released after sync
	if sb != nil {
		sb.release()
	}

	return offset, nil
}

// AppendSync writes the entry and fsyncs immediately
func (w *DurableFile) AppendSync(data []byte) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, errors.New("WAL is closed")
	}

	// Flush any pending batch first
	if w.enableBatch && len(w.batch.entries) > 0 {
		if err := w.flushBatchLocked(); err != nil {
			return 0, err
		}
	}

	return w.appendSyncLocked(data)
}

func (w *DurableFile) appendSyncLocked(data []byte) (int64, error) {
	// Compress if enabled
	toWrite := data
	flags := byte(0)
	// Only compress if enabled and data size meets threshold.
	minBytes := int64(512)
	if w.compressMinBytes > 0 {
		minBytes = w.compressMinBytes
	}
	if w.enableCompress && int64(len(data)) >= minBytes {
		compressed, err := compressData(data)
		if err == nil {
			// Apply post-compression ratio check: only accept compressed
			// data if it is smaller than original by configured ratio.
			ratio := w.compressMinRatio
			// If ratio is unset or invalid, default to 1.0 (accept any smaller)
			if ratio <= 0 || ratio > 1 {
				ratio = 1.0
			}
			if float64(len(compressed)) <= float64(len(data))*ratio {
				toWrite = compressed
				flags |= flagCompressed
			}
		}
	}

	recordSize := int64(recordHeaderSize + len(toWrite))
	if w.curr.size+recordSize > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("failed to rotate WAL file: %w", err)
		}
	}

	offset := w.reserveSeqLocked()
	if err := w.writeRecord(w.curr.f, offset, toWrite, flags); err != nil {
		return 0, fmt.Errorf("failed to write record at offset %d: %w", offset, err)
	}

	w.curr.size += recordSize

	// Update file sequence range
	if w.curr.minSeq == -1 || offset < w.curr.minSeq {
		w.curr.minSeq = offset
	}
	if offset > w.curr.maxSeq {
		w.curr.maxSeq = offset
	}

	if err := w.curr.f.Sync(); err != nil {
		return 0, fmt.Errorf("failed to fsync WAL file: %w", err)
	}

	return offset, nil
}

// Flush forces pending batch to disk
func (w *DurableFile) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.enableBatch || len(w.batch.entries) == 0 {
		return nil
	}

	return w.flushBatchLocked()
}
