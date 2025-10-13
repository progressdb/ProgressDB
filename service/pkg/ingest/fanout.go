package ingest

import (
	"progressdb/pkg/logger"
)

// TODO: notify clients
func FanoutNotify(threadID, msgID string) {
	logger.Debug("fanout_notify_stub", "thread", threadID, "msg", msgID)
}
