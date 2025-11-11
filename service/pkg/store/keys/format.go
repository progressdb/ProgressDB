package keys

const (
	// notation dictionary for key formats:
	// t   = thread
	// m   = message
	// v   = version
	// idx = index
	// ms  = messages
	// u   = user
	// p   = participant
	// del = soft delete marker
	// rel = relationship marker
	// All keys are lowercase; segments are separated by ":"
	// <...> = variable segment (e.g. <thread_key>, <message_key>)

	// provisional
	ThreadPrvKey  = "t:%s"      // t:<threadTS>
	MessagePrvKey = "t:%s:m:%s" // t:<threadTS>

	// primary storage key formats
	MessageKey = "t:%s:m:%s:%s" // t:<threadTS>:m:<messageTS>:<seq> -> data
	VersionKey = "v:%s:%s:%s"   // v:<messageKey>:<ts>:<versionSeq> -> data
	ThreadKey  = "t:%s"         // t:<threadTS> -> data

	// thread → message indexes
	ThreadMessageStart = "idx:t:%s:ms:start" // idx:t:<thread_key>:ms:start -> seq
	ThreadMessageEnd   = "idx:t:%s:ms:end"   // idx:t:<thread_key>:ms:end -> seq
	ThreadMessageLC    = "idx:t:%s:ms:lc"    // idx:t:<thread_key>:ms:lc (last created at) -> ts
	ThreadMessageLU    = "idx:t:%s:ms:lu"    // idx:t:<thread_key>:ms:lu (last updated at) -> ts

	// soft delete markers
	SoftDeleteMarker = "del:%s" // del:<original_key> -> key

	// relationship markers
	RelUserOwnsThread = "rel:u:%s:t:%s" // rel:u:<user_id>:t:<thread_key>
	RelThreadHasUser  = "rel:t:%s:u:%s" // rel:t:<thread_key>:u:<user_id>

	// padding widths (fixed for lexicographic ordering)
	SeqPadWidth = 9 // e.g. %09d

	// system keys
	SystemVersionKey    = "system:version"
	SystemInProgressKey = "system:migrating"
)
