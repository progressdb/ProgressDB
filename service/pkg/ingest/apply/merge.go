package apply

// Data merging operations - temporary hold of apply data into their final threadID to value mappings
// These methods handle the accumulation of data changes before batch persistence

// Generic index data
func (b *BatchIndexManager) SetIndexData(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.indexData[key] = append([]byte(nil), data...)
}
