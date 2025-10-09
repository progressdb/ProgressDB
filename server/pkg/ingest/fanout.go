package ingest

import (
	"progressdb/pkg/logger"
)

// fanout is a small placeholder for post-apply side-effects (push to
// subscribers / pubsub). Implement a real fanout system later. For now
// this module exposes a single method invoked after apply.
func FanoutNotify(threadID, msgID string) {
	logger.Debug("fanout_notify_stub", "thread", threadID, "msg", msgID)
}
