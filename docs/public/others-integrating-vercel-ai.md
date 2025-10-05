---
section: others
title: "Integrating with Vercel AI SDK"
order: 1
visibility: public
---

# Integrating with Vercel AI SDK

This tutorial outlines a simple integration between ProgressDB and the Vercel
AI SDK for building chat-enabled experiences.

Steps:

1. Set up ProgressDB (run locally or use your hosted instance).
2. Create a backend endpoint to `signUser(userId)` using the backend SDK.
3. In your frontend, obtain the user signature from your backend and attach
   `X-User-ID` and `X-User-Signature` to ProgressDB calls.
4. Use the Vercel AI SDK to generate responses and persist conversation turns
   to ProgressDB via the frontend SDK.

Example flow (high-level):

- User sends a chat message in the UI.
- Frontend posts the message to ProgressDB (with signed user headers).
- Frontend calls Vercel AI SDK to generate a response.
- Save the AI response to ProgressDB as another message.

Minimal curl example (frontend/server interaction):

```sh
# Backend: sign a user ID (server-side, using backend SDK / backend key)
curl -X POST https://api.example.com/v1/_sign \
  -H "Authorization: Bearer sk_example" \
  -H "Content-Type: application/json" \
  -d '{"userId":"alice"}'

# Frontend: post a message with X-User headers
curl -X POST https://api.example.com/v1/messages \
  -H "X-User-ID: alice" \
  -H "X-User-Signature: <signature-from-backend>" \
  -H "Content-Type: application/json" \
  -d '{"thread":"general","body":{"text":"Hello from Vercel"}}'
```
