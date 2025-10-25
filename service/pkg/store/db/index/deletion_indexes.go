package index

import (
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// NEW KEY-BASED FUNCTIONS

// MarkSoftDeleted sets a soft delete marker for the given original key
func MarkSoftDeleted(originalKey string) error {
	tr := telemetry.Track("index.mark_soft_deleted")
	defer tr.Finish()

	deleteKey := keys.GenSoftDeleteMarkerKey(originalKey)
	return SaveKey(deleteKey, []byte("1")) // Simple marker value
}

// UnmarkSoftDeleted removes a soft delete marker for the given original key
func UnmarkSoftDeleted(originalKey string) error {
	tr := telemetry.Track("index.unmark_soft_deleted")
	defer tr.Finish()

	deleteKey := keys.GenSoftDeleteMarkerKey(originalKey)
	return DeleteKey(deleteKey)
}

// IsSoftDeleted checks if the given original key is marked as soft deleted
func IsSoftDeleted(originalKey string) (bool, error) {
	deleteKey := keys.GenSoftDeleteMarkerKey(originalKey)
	_, err := GetKey(deleteKey)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
