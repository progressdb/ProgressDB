package queue

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	recordHeaderSize = 17         // 8 (offset) + 4 (crc) + 4 (length) + 1 (flags)
	fileHeaderSize   = 8          // 4 (magic) + 4 (file checksum placeholder)
	fileMagic        = 0x57414C46 // "WALF"

	// Flags
	flagCompressed = 1 << 0
)

// WAL is the write-ahead log interface.
type WAL interface {
	Append([]byte) (int64, error)              // append entry (buffered in batch mode)
	AppendSync([]byte) (int64, error)          // append + fsync immediately
	Flush() error                              // flush pending batch
	Recover() ([]WALRecord, error)             // replay all entries with offsets
	RecoverStream(func(WALRecord) error) error // stream records via callback
	TruncateBefore(int64) error                // delete files with all offsets < N
	Close() error                              // close WAL safely
}

// WALRecord represents a recovered WAL entry with its sequence offset and data.
type WALRecord struct {
	Offset int64
	Data   []byte
}

// Options configure the WAL.
type Options struct {
	Dir            string
	MaxFileSize    int64
	EnableBatch    bool          // Enable batched writes
	BatchSize      int           // Max records per batch
	BatchInterval  time.Duration // Max time before auto-flush
	EnableCompress bool          // Enable gzip compression
}

// walFile represents a single WAL file.
type walFile struct {
	f            *os.File
	num          int
	offset       int64
	size         int64
	minSeq       int64 // Minimum sequence in this file
	maxSeq       int64 // Maximum sequence in this file
	fileChecksum uint32
}

// batchBuffer holds pending writes
type batchBuffer struct {
	entries []batchEntry
	size    int64
}

type batchEntry struct {
	seq  int64
	data []byte
}

// FileWAL implements WAL interface.
type FileWAL struct {
	dir            string
	maxSize        int64
	enableBatch    bool
	batchSize      int
	batchInterval  time.Duration
	enableCompress bool

	mu       sync.Mutex
	curr     *walFile
	files    []*walFile
	nextNum  int
	seq      int64
	crcTable *crc32.Table

	// Batch mode
	batch      *batchBuffer
	batchTimer *time.Timer
	flushCond  *sync.Cond
	closed     bool
}

func New(opts Options) (*FileWAL, error) {
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

	w := &FileWAL{
		dir:            opts.Dir,
		maxSize:        opts.MaxFileSize,
		enableBatch:    opts.EnableBatch,
		batchSize:      opts.BatchSize,
		batchInterval:  opts.BatchInterval,
		enableCompress: opts.EnableCompress,
		crcTable:       crc32.MakeTable(crc32.Castagnoli),
		batch:          &batchBuffer{},
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

// Append writes the entry to WAL (may buffer in batch mode)
func (w *FileWAL) Append(data []byte) (int64, error) {
	if !w.enableBatch {
		return w.AppendSync(data)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, errors.New("WAL is closed")
	}

	// Add to batch
	offset := w.seq
	w.seq++

	// Copy data to avoid external modification
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	w.batch.entries = append(w.batch.entries, batchEntry{
		seq:  offset,
		data: dataCopy,
	})
	w.batch.size += int64(recordHeaderSize + len(data))

	// Flush if batch is full
	if len(w.batch.entries) >= w.batchSize {
		if err := w.flushBatchLocked(); err != nil {
			return 0, err
		}
	}

	return offset, nil
}

// AppendSync writes the entry and fsyncs immediately
func (w *FileWAL) AppendSync(data []byte) (int64, error) {
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

func (w *FileWAL) appendSyncLocked(data []byte) (int64, error) {
	// Compress if enabled
	toWrite := data
	flags := byte(0)
	if w.enableCompress && len(data) > 512 { // Only compress larger records
		compressed, err := compressData(data)
		if err == nil && len(compressed) < len(data) {
			toWrite = compressed
			flags |= flagCompressed
		}
	}

	recordSize := int64(recordHeaderSize + len(toWrite))
	if w.curr.size+recordSize > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("failed to rotate WAL file: %w", err)
		}
	}

	offset := w.seq
	if err := w.writeRecord(w.curr.f, offset, toWrite, flags); err != nil {
		return 0, fmt.Errorf("failed to write record at offset %d: %w", offset, err)
	}

	w.curr.size += recordSize
	w.seq++

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
func (w *FileWAL) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.enableBatch || len(w.batch.entries) == 0 {
		return nil
	}

	return w.flushBatchLocked()
}

func (w *FileWAL) flushBatchLocked() error {
	if len(w.batch.entries) == 0 {
		return nil
	}

	for _, entry := range w.batch.entries {
		if _, err := w.appendSyncLocked(entry.data); err != nil {
			return err
		}
	}

	// Clear batch
	w.batch.entries = w.batch.entries[:0]
	w.batch.size = 0

	w.flushCond.Broadcast()
	return nil
}

// batchFlusher periodically flushes batches
func (w *FileWAL) batchFlusher() {
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
func (w *FileWAL) Recover() ([]WALRecord, error) {
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
func (w *FileWAL) RecoverStream(cb func(WALRecord) error) error {
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
func (w *FileWAL) TruncateBefore(minOffset int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var toDelete []*walFile
	var toKeep []*walFile

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
		if err := syncDir(w.dir); err != nil {
			return fmt.Errorf("failed to sync directory: %w", err)
		}
	}

	return nil
}

// Close closes all WAL files safely
func (w *FileWAL) Close() error {
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

// --- internal helpers ---

func (w *FileWAL) recoverFiles() (int64, error) {
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

		wf := &walFile{
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

func (w *FileWAL) validateFileHeader(f *os.File) error {
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

func (w *FileWAL) writeFileHeader(f *os.File) error {
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

func (w *FileWAL) scanFile(f *os.File, fileNum int) ([]int64, int64, error) {
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

func (w *FileWAL) createNewFile() error {
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

	if err := syncDir(w.dir); err != nil {
		f.Close()
		return fmt.Errorf("failed to sync directory: %w", err)
	}

	wf := &walFile{
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

func (w *FileWAL) rotate() error {
	if err := w.finalizeFile(w.curr); err != nil {
		return err
	}
	if err := w.curr.f.Close(); err != nil {
		return err
	}
	return w.createNewFile()
}

func (w *FileWAL) finalizeFile(wf *walFile) error {
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

func (w *FileWAL) writeRecord(f *os.File, offset int64, data []byte, flags byte) error {
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

func (w *FileWAL) readRecords(f *os.File, fileNum int) ([]WALRecord, error) {
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

func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressData(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
