package queue

// Worker loops are moved into their own file to separate runtime worker
// orchestration from core queue responsibilities.

// RunWorker consumes QueueItems one-by-one and invokes the provided handler.
func RunWorker(q *IngestQueue, stop <-chan struct{}, handler func(*QueueOp) error) {
	for {
		select {
		case it, ok := <-q.ch:
			if !ok {
				return
			}
			func(it *QueueItem) {
				defer it.Done()
				_ = handler(it.Op)
			}(it)
		case <-stop:
			return
		}
	}
}

// RunBatchWorker collects up to batchSize items and delivers them to the
// provided batch handler.
func RunBatchWorker(q *IngestQueue, stop <-chan struct{}, batchSize int, handler func([]*QueueOp) error) {
	if batchSize <= 0 {
		panic("queue.RunBatchWorker: batchSize must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	for {
		select {
		case <-stop:
			return
		default:
		}

		var items []*QueueItem

		select {
		case it, ok := <-q.ch:
			if !ok {
				return
			}
			items = append(items, it)
		case <-stop:
			return
		}

	collect:
		for len(items) < batchSize {
			select {
			case it, ok := <-q.ch:
				if !ok {
					break collect
				}
				items = append(items, it)
			default:
				break collect
			}
		}

		func(batch []*QueueItem) {
			defer func() {
				for _, it := range batch {
					it.Done()
				}
			}()
			ops := make([]*QueueOp, len(batch))
			for i, it := range batch {
				ops[i] = it.Op
			}
			_ = handler(ops)
		}(items)
	}
}
