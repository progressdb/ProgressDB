package indexdb

import (
	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/keys"
)

func MarkSoftDeleted(originalKey string) error {
	tr := telemetry.Track("indexdb.mark_soft_deleted")
	defer tr.Finish()

	deleteKey := keys.GenSoftDeleteMarkerKey(originalKey)
	return SaveKey(deleteKey, []byte("1"))
}

func DeleteSoftDeleteMarker(originalKey string) error {
	tr := telemetry.Track("indexdb.unmark_soft_deleted")
	defer tr.Finish()

	deleteKey := keys.GenSoftDeleteMarkerKey(originalKey)
	return DeleteKey(deleteKey)
}

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

func MarkUserOwnsThread(userID, threadKey string) error {
	tr := telemetry.Track("indexdb.mark_user_owns_thread")
	defer tr.Finish()

	key := keys.GenUserOwnsThreadKey(userID, threadKey)
	return SaveKey(key, []byte("1"))
}

func UnmarkUserOwnsThread(userID, threadKey string) error {
	tr := telemetry.Track("indexdb.unmark_user_owns_thread")
	defer tr.Finish()

	key := keys.GenUserOwnsThreadKey(userID, threadKey)
	return DeleteKey(key)
}

func DoesUserOwnThread(userID, threadKey string) (bool, error) {
	key := keys.GenUserOwnsThreadKey(userID, threadKey)
	_, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func MarkThreadHasUser(threadKey, userID string) error {
	tr := telemetry.Track("indexdb.mark_thread_has_user")
	defer tr.Finish()

	key := keys.GenThreadHasUserKey(threadKey, userID)
	return SaveKey(key, []byte("1"))
}

func UnmarkThreadHasUser(threadKey, userID string) error {
	tr := telemetry.Track("indexdb.unmark_thread_has_user")
	defer tr.Finish()

	key := keys.GenThreadHasUserKey(threadKey, userID)
	return DeleteKey(key)
}

func DoesThreadHaveUser(threadKey, userID string) (bool, error) {
	key := keys.GenThreadHasUserKey(threadKey, userID)
	_, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
