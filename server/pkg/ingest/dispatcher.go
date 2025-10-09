package ingest

import qpkg "progressdb/pkg/ingest/queue"

// RegisterDefaultHandlers wires a simple dispatcher for create/update/delete
// ops that inspects the payload to decide whether the operation is a
// message or a thread operation. This is a convenience for initial wiring.
func RegisterDefaultHandlers(p *Processor) {
	// Register explicit handlers. Enqueueing code must set Op.Handler to one
	// of these HandlerIDs so the processor can deterministically dispatch.
	p.RegisterHandler(qpkg.HandlerMessageCreate, MutMessageCreate)
	p.RegisterHandler(qpkg.HandlerMessageUpdate, MutMessageUpdate)
	p.RegisterHandler(qpkg.HandlerMessageDelete, MutMessageDelete)
	p.RegisterHandler(qpkg.HandlerReactionAdd, MutReactionAdd)
	p.RegisterHandler(qpkg.HandlerReactionDelete, MutReactionDelete)
	p.RegisterHandler(qpkg.HandlerThreadCreate, MutThreadCreate)
	p.RegisterHandler(qpkg.HandlerThreadUpdate, MutThreadUpdate)
	p.RegisterHandler(qpkg.HandlerThreadDelete, MutThreadDelete)
}
