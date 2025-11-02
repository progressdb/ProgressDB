package iterator

// import (
// 	"fmt"
// 	"strings"

// 	"github.com/cockroachdb/pebble"
// 	"progressdb/pkg/store/pagination"
// )

// // Direction represents iteration direction
// type Direction int

// const (
// 	DirectionForward Direction = iota
// 	DirectionBackward
// )

// // SortOrder represents sort order
// type SortOrder string

// const (
// 	SortOrderAsc  SortOrder = "asc"
// 	SortOrderDesc SortOrder = "desc"
// )

// // SortField represents sort field
// type SortField string

// const (
// 	SortFieldCreatedAt SortField = "created_at"
// 	SortFieldUpdatedAt SortField = "updated_at"
// )

// // IteratorConfig holds configuration for iteration
// type IteratorConfig struct {
// 	Prefix     string
// 	LowerBound []byte
// 	UpperBound []byte
// }

// // QueryExecutor handles bidirectional pagination queries
// type QueryExecutor struct {
// 	db *pebble.DB
// }

// // NewQueryExecutor creates a new query executor
// func NewQueryExecutor(db *pebble.DB) *QueryExecutor {
// 	return &QueryExecutor{db: db}
// }

// // ExecuteQuery executes a pagination query and returns item keys
// func (qe *QueryExecutor) ExecuteQuery(config IteratorConfig, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
// 	// Validate request
// 	if err := validateRequest(req); err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Determine query type and execute
// 	switch {
// 	case req.Anchor != "":
// 		return qe.executeAnchorQuery(config, req)
// 	case req.Before != "":
// 		return qe.executeBeforeQuery(config, req)
// 	case req.After != "":
// 		return qe.executeAfterQuery(config, req)
// 	default:
// 		return qe.executeInitialLoad(config, req)
// 	}
// }

// // validateRequest validates the pagination request
// func validateRequest(req pagination.PaginationRequest) error {
// 	// Only one of anchor, before, after can be set
// 	refCount := 0
// 	if req.Anchor != "" {
// 		refCount++
// 	}
// 	if req.Before != "" {
// 		refCount++
// 	}
// 	if req.After != "" {
// 		refCount++
// 	}

// 	if refCount > 1 {
// 		return fmt.Errorf("only one of anchor, before, after can be specified")
// 	}

// 	// Validate sort_by
// 	if req.SortBy != "" && req.SortBy != string(SortFieldCreatedAt) && req.SortBy != string(SortFieldUpdatedAt) {
// 		return fmt.Errorf("sort_by must be 'created_at' or 'updated_at'")
// 	}

// 	// Validate order_by
// 	if req.OrderBy != "" && req.OrderBy != string(SortOrderAsc) && req.OrderBy != string(SortOrderDesc) {
// 		return fmt.Errorf("order_by must be 'asc' or 'desc'")
// 	}

// 	// Set defaults
// 	if req.Limit == 0 {
// 		req.Limit = 50
// 	}
// 	if req.Limit > 1000 {
// 		req.Limit = 1000
// 	}

// 	return nil
// }

// // executeAnchorQuery executes an anchor query (bidirectional)
// func (qe *QueryExecutor) executeAnchorQuery(config IteratorConfig, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
// 	halfLimit := req.Limit / 2

// 	// Fetch items before anchor
// 	beforeItems, hasMoreBefore, err := qe.fetchBefore(config, req.Anchor, halfLimit, req)
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Fetch items after anchor (remaining limit)
// 	afterLimit := req.Limit - len(beforeItems)
// 	afterItems, hasMoreAfter, err := qe.fetchAfter(config, req.Anchor, afterLimit, req)
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Merge results: before + anchor + after
// 	allItems := make([]string, 0, len(beforeItems)+1+len(afterItems))
// 	allItems = append(allItems, beforeItems...)
// 	allItems = append(allItems, req.Anchor)
// 	allItems = append(allItems, afterItems...)

// 	// Build response
// 	var startAnchor, endAnchor string
// 	if len(allItems) > 0 {
// 		startAnchor = allItems[0]
// 		endAnchor = allItems[len(allItems)-1]
// 	}

// 	response := pagination.PaginationResponse{
// 		StartAnchor: startAnchor,
// 		EndAnchor:   endAnchor,
// 		HasBefore:   hasMoreBefore,
// 		HasAfter:    hasMoreAfter,
// 		OrderBy:     req.OrderBy,
// 		Count:       len(allItems),
// 	}

// 	return allItems, response, nil
// }

// // executeBeforeQuery executes a before query (directional)
// func (qe *QueryExecutor) executeBeforeQuery(config IteratorConfig, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
// 	items, hasMoreBefore, err := qe.fetchBefore(config, req.Before, req.Limit, req)
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Check if there are items after the reference point
// 	hasMoreAfter, err := qe.checkHasAfter(config, req.Before, req)
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Build response
// 	var startAnchor, endAnchor string
// 	if len(items) > 0 {
// 		startAnchor = items[len(items)-1] // Oldest (last in backward iteration)
// 		endAnchor = items[0]              // Newest (first in backward iteration)
// 	}

// 	response := pagination.PaginationResponse{
// 		StartAnchor: startAnchor,
// 		EndAnchor:   endAnchor,
// 		HasBefore:   hasMoreBefore,
// 		HasAfter:    hasMoreAfter,
// 		OrderBy:     req.OrderBy,
// 		Count:       len(items),
// 	}

// 	return items, response, nil
// }

// // executeAfterQuery executes an after query (directional)
// func (qe *QueryExecutor) executeAfterQuery(config IteratorConfig, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
// 	items, hasMoreAfter, err := qe.fetchAfter(config, req.After, req.Limit, req)
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Check if there are items before the reference point
// 	hasMoreBefore, err := qe.checkHasBefore(config, req.After, req)
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}

// 	// Build response
// 	var startAnchor, endAnchor string
// 	if len(items) > 0 {
// 		startAnchor = items[0]          // Newest (first in forward iteration)
// 		endAnchor = items[len(items)-1] // Oldest (last in forward iteration)
// 	}

// 	response := pagination.PaginationResponse{
// 		StartAnchor: startAnchor,
// 		EndAnchor:   endAnchor,
// 		HasBefore:   hasMoreBefore,
// 		HasAfter:    hasMoreAfter,
// 		OrderBy:     req.OrderBy,
// 		Count:       len(items),
// 	}

// 	return items, response, nil
// }

// // executeInitialLoad executes an initial load query
// func (qe *QueryExecutor) executeInitialLoad(config IteratorConfig, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
// 	iter, err := qe.db.NewIter(&pebble.IterOptions{
// 		LowerBound: config.LowerBound,
// 		UpperBound: config.UpperBound,
// 	})
// 	if err != nil {
// 		return nil, pagination.PaginationResponse{}, err
// 	}
// 	defer iter.Close()

// 	var items []string
// 	var iterFunc func(*pebble.Iterator) bool

// 	// Determine iteration direction and start point
// 	if req.OrderBy == string(SortOrderDesc) {
// 		iterFunc = func(iter *pebble.Iterator) bool { return iter.Last() }
// 	} else {
// 		iterFunc = func(iter *pebble.Iterator) bool { return iter.First() }
// 	}

// 	// Iterate and collect items
// 	for valid := iterFunc(iter); valid && len(items) < req.Limit; valid = iter.Next() {
// 		key := string(iter.Key())
// 		if config.Prefix != "" && !strings.HasPrefix(key, config.Prefix) {
// 			continue
// 		}
// 		items = append(items, key)
// 	}

// 	// Check for more items
// 	hasMore := iter.Valid() && (config.Prefix == "" || strings.HasPrefix(string(iter.Key()), config.Prefix))

// 	// Build response
// 	var startAnchor, endAnchor string
// 	if len(items) > 0 {
// 		if req.OrderBy == string(SortOrderDesc) {
// 			startAnchor = items[len(items)-1] // Oldest
// 			endAnchor = items[0]              // Newest
// 		} else {
// 			startAnchor = items[0]          // Newest
// 			endAnchor = items[len(items)-1] // Oldest
// 		}
// 	}

// 	response := pagination.PaginationResponse{
// 		StartAnchor: startAnchor,
// 		EndAnchor:   endAnchor,
// 		HasBefore:   req.OrderBy == string(SortOrderAsc) && hasMore,
// 		HasAfter:    req.OrderBy == string(SortOrderDesc) && hasMore,
// 		OrderBy:     req.OrderBy,
// 		Count:       len(items),
// 	}

// 	return items, response, nil
// }

// // fetchBefore fetches items before the reference point
// func (qe *QueryExecutor) fetchBefore(config IteratorConfig, reference string, limit int, req pagination.PaginationRequest) ([]string, bool, error) {
// 	iter, err := qe.db.NewIter(&pebble.IterOptions{
// 		LowerBound: config.LowerBound,
// 		UpperBound: config.UpperBound,
// 	})
// 	if err != nil {
// 		return nil, false, err
// 	}
// 	defer iter.Close()

// 	var items []string
// 	referenceKey := []byte(reference)

// 	// Seek to reference point and go backward
// 	var valid bool
// 	if req.OrderBy == string(SortOrderDesc) {
// 		// Descending order: seek less than, then prev
// 		valid = iter.SeekLT(referenceKey)
// 	} else {
// 		// Ascending order: seek less than, then prev
// 		valid = iter.SeekLT(referenceKey)
// 	}

// 	// Collect items going backward
// 	for valid && len(items) < limit {
// 		key := string(iter.Key())
// 		if config.Prefix != "" && !strings.HasPrefix(key, config.Prefix) {
// 			break
// 		}
// 		items = append(items, key)
// 		valid = iter.Prev()
// 	}

// 	// Check for more items
// 	hasMore := valid && (config.Prefix == "" || strings.HasPrefix(string(iter.Key()), config.Prefix))

// 	// Reverse items to maintain correct order
// 	if len(items) > 0 {
// 		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
// 			items[i], items[j] = items[j], items[i]
// 		}
// 	}

// 	return items, hasMore, nil
// }

// // fetchAfter fetches items after the reference point
// func (qe *QueryExecutor) fetchAfter(config IteratorConfig, reference string, limit int, req pagination.PaginationRequest) ([]string, bool, error) {
// 	iter, err := qe.db.NewIter(&pebble.IterOptions{
// 		LowerBound: config.LowerBound,
// 		UpperBound: config.UpperBound,
// 	})
// 	if err != nil {
// 		return nil, false, err
// 	}
// 	defer iter.Close()

// 	var items []string
// 	referenceKey := []byte(reference)

// 	// Seek to reference point and go forward
// 	var valid bool
// 	if req.OrderBy == string(SortOrderAsc) {
// 		// Ascending order: seek greater than or equal, then next
// 		valid = iter.SeekGE(referenceKey)
// 		if valid && string(iter.Key()) == reference {
// 			valid = iter.Next() // Skip the reference itself
// 		}
// 	} else {
// 		// Descending order: seek greater than or equal, then next (skip reference)
// 		valid = iter.SeekGE(referenceKey)
// 		if valid && string(iter.Key()) == reference {
// 			valid = iter.Next() // Skip the reference itself
// 		}
// 	}

// 	// Collect items going forward
// 	for valid && len(items) < limit {
// 		key := string(iter.Key())
// 		if config.Prefix != "" && !strings.HasPrefix(key, config.Prefix) {
// 			break
// 		}
// 		items = append(items, key)
// 		valid = iter.Next()
// 	}

// 	// Check for more items
// 	hasMore := valid && (config.Prefix == "" || strings.HasPrefix(string(iter.Key()), config.Prefix))

// 	return items, hasMore, nil
// }

// // checkHasBefore checks if there are items before the reference point
// func (qe *QueryExecutor) checkHasBefore(config IteratorConfig, reference string, req pagination.PaginationRequest) (bool, error) {
// 	iter, err := qe.db.NewIter(&pebble.IterOptions{
// 		LowerBound: config.LowerBound,
// 		UpperBound: config.UpperBound,
// 	})
// 	if err != nil {
// 		return false, err
// 	}
// 	defer iter.Close()

// 	referenceKey := []byte(reference)
// 	var valid bool

// 	if req.OrderBy == string(SortOrderDesc) {
// 		valid = iter.SeekLT(referenceKey)
// 	} else {
// 		valid = iter.SeekLT(referenceKey)
// 	}

// 	return valid && (config.Prefix == "" || strings.HasPrefix(string(iter.Key()), config.Prefix)), nil
// }

// // checkHasAfter checks if there are items after the reference point
// func (qe *QueryExecutor) checkHasAfter(config IteratorConfig, reference string, req pagination.PaginationRequest) (bool, error) {
// 	iter, err := qe.db.NewIter(&pebble.IterOptions{
// 		LowerBound: config.LowerBound,
// 		UpperBound: config.UpperBound,
// 	})
// 	if err != nil {
// 		return false, err
// 	}
// 	defer iter.Close()

// 	referenceKey := []byte(reference)
// 	var valid bool

// 	if req.OrderBy == string(SortOrderAsc) {
// 		valid = iter.SeekGE(referenceKey)
// 		if valid && string(iter.Key()) == reference {
// 			valid = iter.Next()
// 		}
// 	} else {
// 		valid = iter.SeekGE(referenceKey)
// 		if valid && string(iter.Key()) == reference {
// 			valid = iter.Next()
// 		}
// 	}

// 	return valid && (config.Prefix == "" || strings.HasPrefix(string(iter.Key()), config.Prefix)), nil
// }
