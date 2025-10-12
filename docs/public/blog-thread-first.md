---
section: blog
title: "Thread First — a model for all chat experiences"
order: 1
visibility: public
---

# Thread First — a model for all chat experiences

If you ask the internet how to model a chat experience, you will get an approach based around messages and their user or authors.  
This is simple, works, and scales if your only author is a user for the most part.

## The limits of message-first

The message-first model assumes that each message—and its metadata—is the primary unit of organization.  

Everything else—replies, reactions, AI responses, summaries, etc.—hangs off the message unit.  

It sounds reasonable, until you try to build any new chat features, especially in the age of AI:

- AI or bot messages that have no direct relationship with the message unit.
- Or edge cases that fall outside the norm (no popular or standard design).

The concept of replies, reactions, comments uniquely tied to a message unit, usually requires creating another table or collection to store these, 
- relinking or migrating them leads to a sprawl of new models, or hackish ways to keep it all operationally sane.
- Operational complexity grows over time.


For example, to build shared threads like OpenAI’s, you might end up creating a new collection or caching layer just to connect messages that logically belong together.



##### This complexity is multiplied if you need to support different models of chat:

- **Direct chat (1:1)**
- **Group chat (N:N bounded)**
- **Broadcast/channel (1:N)**
- **Federated/public (M:M open)**
- **Bot/assistant chat (1:service)**

The modelling becomes quickly multiplied and requires new modelling and, in some cases, tearing down old models and migrating data to the new model—and this does not cease for the most part.

Layer on other moving targets like moderation, encryption, etc., and you usually end up updating downstream services to sync with those changes.

This is mostly fine, but when you multiply these moving parts by more requirements, it quickly leads to a modelling sprawl.

It’s fairly standard for most teams—whether because they’re resourced or just because that’s “how it’s done”—to power through it. If anything, it’s accepted as a job requirement.

But is it really the best way? Is it okay?

For all of us—solo or team, well-resourced or time-inconvenienced—does the default really help everyone? No.


---

## Thread first

A thread-first model is both a data structure **and** a storage model/setup that combines the message into a monotonic unit in the underlying store to brings order to things.

Instead of the message being the unit for organization,  
- what if **the thread** is the unit of organization?

> NOTE: most databases _don't_ support this concept at the storage level; some do as an effect, but with limitations.

**Think:**  
- We have `thread_id` prefixes organizing the messages, but not as a collection or single table—this structure _represents access to the data_.  
- The ownership or participation in a thread lives separately from the message itself.

Compared to the earlier chat models, this detachment allows for all modes of chat without extra work to evolve user/message relationships.  

- Most importantly, it *standardizes* the chat model so it can be replicated everywhere, for different chat contexts.

**Note:**  
The concept here isn’t saying it’s “better” in all ways—it’s saying it’s _more efficient_, and, if you pair it with the right storage medium, it solves for most chat patterns more efficiently.  
- Its also not saying you can’t do this with current systems.
- Its basically solving for the **standard**.
  - Something everyone can accept, use & it works excellent - out of the box - for chat experiences.

---

## The storage medium

Getting the model right is only half the battle.

The real challenge begins when you try to **store and access** it efficiently.

Once the modelling is done, you’re still bound by how your database handles reads, writes, and scans.

- SQL and NoSQL systems don’t natively support **prefix-based** or **monotonic threaded access** — threads lose their natural shape once they’re flattened into tables or collections.
- Indexes and joins end up rebuilding what the storage layer never preserved.
- Redis streams or sorted sets get close, but they’re still **bounded** or **ephemeral** — not a true underlying structure.

A thread-first model needs a storage medium where appends, reads, and scans are natural — **not simulated.**

A proper thread-first store should make this a single contiguous read (a range scan by prefix), maintaining **lexicographic ordering**.

This is trivial in LSM-based KV stores like Pebble, RocksDB, or LevelDB, but it’s *unnatural* in SQL or document databases — they emulate order rather than live in it.

The result is complexity in something that should be simple: scanning, appending, and replaying a thread in order.

**The storage is where the chat problem begins.**

---

The core of this post focuses on the chat model and its relationship to storage—the “space between” what’s modelled and how it’s accessed.