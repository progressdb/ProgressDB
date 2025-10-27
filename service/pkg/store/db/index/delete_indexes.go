package index

import (
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

func MarkSoftDeleted(originalKey string) error {
	tr := telemetry.Track("index.mark_soft_deleted")
	defer tr.Finish()

	deleteKey := keys.GenSoftDeleteMarkerKey(originalKey)
	return SaveKey(deleteKey, []byte("1"))
}

func UnmarkSoftDeleted(originalKey string) error {
	tr := telemetry.Track("index.unmark_soft_deleted")
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

func MarkUserOwnsThread(userID, threadID string) error {
	tr := telemetry.Track("index.mark_user_owns_thread")
	defer tr.Finish()

	key := keys.GenUserOwnsThreadKey(userID, threadID)
	return SaveKey(key, []byte("1"))
}

func UnmarkUserOwnsThread(userID, threadID string) error {
	tr := telemetry.Track("index.unmark_user_owns_thread")
	defer tr.Finish()

	key := keys.GenUserOwnsThreadKey(userID, threadID)
	return DeleteKey(key)
}

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

func MarkThreadHasUser(threadID, userID string) error {
	tr := telemetry.Track("index.mark_thread_has_user")
	defer tr.Finish()

	key := keys.GenThreadHasUserKey(threadID, userID)
	return SaveKey(key, []byte("1"))
}

func UnmarkThreadHasUser(threadID, userID string) error {
	tr := telemetry.Track("index.unmark_thread_has_user")
	defer tr.Finish()

	key := keys.GenThreadHasUserKey(threadID, userID)
	return DeleteKey(key)
}

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
