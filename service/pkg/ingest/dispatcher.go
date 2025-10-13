package ingest

import "progressdb/pkg/ingest/queue"

// RegisterDefaultHandlers wires a simple dispatcher for create/update/delete
// ops that inspects the payload to decide whether the operation is a
// message or a thread operation. This is a convenience for initial wiring.
func RegisterDefaultHandlers(p *Processor) {
	// Register explicit handlers. Enqueueing code must set Op.Handler to one
	// of these HandlerIDs so the processor can deterministically dispatch.
	p.RegisterHandler(queue.HandlerMessageCreate, MutMessageCreate)
	p.RegisterHandler(queue.HandlerMessageUpdate, MutMessageUpdate)
	p.RegisterHandler(queue.HandlerMessageDelete, MutMessageDelete)
	p.RegisterHandler(queue.HandlerReactionAdd, MutReactionAdd)
	p.RegisterHandler(queue.HandlerReactionDelete, MutReactionDelete)
	p.RegisterHandler(queue.HandlerThreadCreate, MutThreadCreate)
	p.RegisterHandler(queue.HandlerThreadUpdate, MutThreadUpdate)
	p.RegisterHandler(queue.HandlerThreadDelete, MutThreadDelete)
}
