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

// RELATIONSHIP MANAGEMENT FUNCTIONS

// MarkUserOwnsThread sets a relationship marker for user owning a thread
func MarkUserOwnsThread(userID, threadID string) error {
	tr := telemetry.Track("index.mark_user_owns_thread")
	defer tr.Finish()

	key := keys.GenUserOwnsThreadKey(userID, threadID)
	return SaveKey(key, []byte("1")) // Simple marker value
}

// UnmarkUserOwnsThread removes a relationship marker for user owning a thread
func UnmarkUserOwnsThread(userID, threadID string) error {
	tr := telemetry.Track("index.unmark_user_owns_thread")
	defer tr.Finish()

	key := keys.GenUserOwnsThreadKey(userID, threadID)
	return DeleteKey(key)
}

// DoesUserOwnThread checks if user owns the thread
func DoesUserOwnThread(userID, threadID string) (bool, error) {
	key := keys.GenUserOwnsThreadKey(userID, threadID)
	_, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MarkThreadHasUser sets a relationship marker for thread having a user participant
func MarkThreadHasUser(threadID, userID string) error {
	tr := telemetry.Track("index.mark_thread_has_user")
	defer tr.Finish()

	key := keys.GenThreadHasUserKey(threadID, userID)
	return SaveKey(key, []byte("1")) // Simple marker value
}

// UnmarkThreadHasUser removes a relationship marker for thread having a user participant
func UnmarkThreadHasUser(threadID, userID string) error {
	tr := telemetry.Track("index.unmark_thread_has_user")
	defer tr.Finish()

	key := keys.GenThreadHasUserKey(threadID, userID)
	return DeleteKey(key)
}

// DoesThreadHaveUser checks if thread has the user as participant
func DoesThreadHaveUser(threadID, userID string) (bool, error) {
	key := keys.GenThreadHasUserKey(threadID, userID)
	_, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
