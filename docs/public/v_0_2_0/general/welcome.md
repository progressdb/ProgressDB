---
section: general
title: "Welcome"
order: 1
visibility: public
---

<!-- ProgressDB Logo -->

# Welcome

> **Storing chat data seems simple**:  
> Model a few tables, drop messages into a standard database, done.

That instinct is understandable - after all, chat *appears* simple:  
- Small payloads  
- Familiar structures  
- Just a timeline of messages  

But **chat isn’t just data**.  
It’s **_personal data_** - often the most sensitive your system will ever handle:  
- AI assistants  
- Customer support  
- Therapy apps  
- Banking conversations  

The stakes are on a completely different level.

---

> **Encryption quickly becomes non-optional.**

Suddenly you’re faced with questions like:  
- *Which* encryption?  
- *Where* should it be applied?  
- *How* do you keep fields encrypted and still support:  
  - Edits  
  - Redactions  
  - Replies and versions  
  - Fast timeline reads  

None of these are “extra features.”  
They’re **fundamental properties** of chat workloads - interdependent, unavoidable.

---

**Assume chat is “just another app table,” and you’ll:**
- Model the wrong schema  
- Pick inappropriate storage  
- Apply the wrong encryption  
- Eventually layer on fragile fixes, patching around a database never meant for the job

<br/>

The goal of ProgressDB is to solve these at the most fundamental level while being fast, delightful to use and egregiously secure.