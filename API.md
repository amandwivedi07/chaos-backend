# Chaos Backend — Architecture & API

Go 1.26 · Gin · GORM · PostgreSQL · Redis · Azure OpenAI
Module `github.com/chaosapp/backend`

| | |
|---|---|
| **Production** | `https://app.joinchaos.ai` |
| **Base path** | `/api/v1` |
| **Interactive schemas** | `https://app.joinchaos.ai/swagger/index.html` |
| **Health** | `GET /healthz` (unversioned, no auth) |

Swagger is generated from handler annotations (`make swag`) and is the exhaustive
field-level reference. This document explains the shape of the system and what
each endpoint is *for* — the part a schema cannot tell you.

---

## 1. What the product is

A group chat with an AI in it that actually decides things. Friends argue about
a trip; Chaos reads the constraints everyone stated, answers with two or three
comparison cards, and either lets someone pick or puts it to a vote. It also
remembers durable facts about each person, so the next plan already knows they
cannot do more than four nights.

Two functions carry the product, both in
`internal/domain/conversation/entity`, both unit-tested:

- **`ShouldReply`** — when Chaos speaks. Being named always works. Otherwise it
  waits: a question only pulls it in after the group has gone back and forth a
  couple of times, and a thread that has run five messages without it gets one
  turn unasked. A `direct` conversation short-circuits all of it, because a
  private thread is you talking *to* Chaos and restraint there reads as being
  ignored.
- **`Decision.Resolve`** — when a vote closes. A winner needs **at least two
  votes and a strict lead**. One person clicking their own suggestion is not the
  group deciding, and a tie is precisely the argument the vote was meant to end.

Change either only on purpose.

---

## 2. Architecture

```
HTTP → middleware → handler → DTO validation → service → repository (interface) → GORM → PostgreSQL
```

The rules that matter:

- **Business logic lives only in services** (`internal/domain/<x>/service`).
  Never in handlers, routes, middleware, or repositories.
- **Every dependency is an interface injected via constructor.** No globals, no
  `init()` wiring, no service locators. `cmd/server/main.go` is the composition
  root and does wiring *only*.
- **Repositories own SQL and authorization-relevant lookups; they never see
  HTTP.** Services never import `gin`.
- **Four shapes per concept, never mixed**: domain entity (`entity/`), GORM
  model (private, in `repository/gorm_model.go`), request DTO, response DTO.
  Repository maps ↔ entity internally; `mapper/` maps entity → response DTO.
- **Errors**: services return `*apperrors.AppError`. Handlers end every failure
  path with `response.Error(c, err)` — the only place errors become HTTP.

### Cross-module dependencies

Three modules read slices of each other and **none imports another**. The
dependency always goes through a port declared by the *consumer*:

| Module | Declares (needs) | Satisfied by |
|---|---|---|
| conversation | `Facts`, `GroupContext` | profile, group |
| profile | `History` | conversation |
| group | `Threads`, `People` | conversation |

Group and conversation are a genuine cycle, broken in `main.go` with two small
lazy adapters rather than a setter on a half-built service. Do the same for any
future pair — that is what keeps the cycle from forming in the first place.

### Layout

```
cmd/server/           main.go — config → logger → db+migrate → cache → bus → wiring → routes → serve
internal/
  config/             Viper env config (typed, validated)
  logger/             Zap (console dev / JSON prod)
  database/           GORM connect (pooled) + golang-migrate runner
  cache/              Store port + Redis adapter + Noop
  auth/jwt/           token Manager: separate access/refresh secrets, typ claim
  auth/firebase/      Apple/Google ID-token verifier
  middleware/         Auth, RequireRoles, RequestID, Logging, Recovery, CORS,
                      RateLimit, SecureHeaders
  common/             errors, response envelope, pagination, constants
  ai/                 Client port + Azure adapter + Disabled
  domain/user|auth|conversation|group|profile|device|media|link/
  storage/            Storage port + local + S3 (R2-compatible)
  email/              Sender port + log (dev) + SMTP
  events/ worker/     async event bus + goroutine pool
  push/               FCM sender + Notifier
  realtime/           WebSocket hub
  routes/             /api/v1 registration — the only place URLs are declared
migrations/           golang-migrate SQL pairs, applied at boot
```

---

## 3. Conventions

### Response envelope

Never deviate from this shape.

**Success**
```json
{ "success": true, "message": "Signed in", "data": { } }
```

**Failure**
```json
{
  "success": false,
  "message": "Validation failed",
  "data": null,
  "error": "VALIDATION_ERROR",
  "errors": { "email": "must be a valid email" }
}
```

`errors` is present only for field-level validation failures.

### Error kinds

`error` carries one of: `BAD_REQUEST`, `VALIDATION_ERROR`, `UNAUTHORIZED`,
`FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `RATE_LIMITED`, `DATABASE_ERROR`,
`UNAVAILABLE`, `INTERNAL_ERROR`.

Add new kinds to `internal/common/errors`, never an ad-hoc status in a handler.

### Rate limiting

A cache-backed fixed window applies to **all** of `/api/v1`, currently
`RATE_LIMIT_RPM=120`. Note that with `cache.Noop{}` (no Redis) rate limiting and
the Chaos spend cap are *silently disabled* — do not rely on limits locally.

### Auth header

```
Authorization: Bearer <access_token>
```

---

## 4. Auth model

Do not weaken any of this.

- Access JWT **15 m**, refresh JWT **30 d** (`720h`), **different secrets**, and
  a `typ` claim that is checked — an access token cannot be used to refresh.
- Refresh tokens are stored **sha256-hashed** with a rotation `family_id`.
  Reusing a rotated-out token **revokes the whole family**.
- Login and forgot-password answer identically for unknown emails, so the API
  cannot be used to enumerate accounts.
- One-time tokens (`action_tokens`) are hashed, single-use, short-lived.
- Password changes and resets revoke every session.
- **Social sign-in needs no secret.** `FIREBASE_PROJECT_ID` alone enables
  Apple/Google: verifying an ID token uses Google's *public* certificates, so
  the verifier is built with `option.WithoutAuthentication()`.
  `FIREBASE_CREDENTIALS_FILE` is only needed for push and for deleting a
  Firebase identity on account deletion.
- **Placeholder users** have no email and no password hash, so they can never be
  signed into. They exist so a group can plan around someone who has not signed
  up; the invite link binds that seat to a real account.

---

## 5. API reference

All paths are relative to `/api/v1`. **Auth** column: `—` public,
`user` requires a bearer token, `admin` additionally requires the admin role.

### Authentication

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/auth/register` | — | Create an account. Returns user + token pair. |
| POST | `/auth/login` | — | Email + password. |
| POST | `/auth/firebase` | — | Exchange a Firebase ID token (Apple/Google) for a Chaos session. First sign-in creates the account — there is no separate social registration step. |
| POST | `/auth/refresh` | — | Rotate the refresh token. Reuse of a rotated token revokes the family. |
| POST | `/auth/logout` | — | Revoke the supplied refresh token. |
| POST | `/auth/forgot-password` | — | Always answers the same, known email or not. |
| POST | `/auth/reset-password` | — | Consume a one-time token; revokes all sessions. |
| POST | `/auth/verify-email` | — | Consume a one-time token. |
| GET | `/auth/me` | user | The signed-in account. |
| PATCH | `/auth/me` | user | Update name, handle, avatar. |
| POST | `/auth/change-password` | user | Revokes all sessions. |
| DELETE | `/auth/account` | user | Irreversible. Scrubs the profile, revokes every session, clears push tokens, removes the Firebase identity. Messages already sent into shared conversations remain, unattributed. |

**Register / login response**
```json
{
  "success": true,
  "data": {
    "user":   { "id": "…", "name": "Rhea", "email": "…", "palette_id": "sun" },
    "tokens": { "access_token": "…", "refresh_token": "…" }
  }
}
```

### Conversations

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/conversations` | user | List, newest first. |
| POST | `/conversations` | user | Create. Body: `title?`, `emoji?`, `member_ids[]?`. |
| GET | `/conversations/:id` | user | One conversation with members and state. |
| PATCH | `/conversations/:id` | user | Rename / re-emoji. |
| POST | `/conversations/:id/leave` | user | Leave. |
| GET | `/conversations/:id/messages` | user | Paginated transcript. |
| POST | `/conversations/:id/messages` | user | Send. Body: `text`, `speaking_as?`. **Chaos may or may not reply** — see `ShouldReply`. |
| POST | `/conversations/:id/ask` | user | Force a Chaos turn regardless of `ShouldReply`. |
| POST | `/conversations/:id/seen` | user | Mark read. |
| POST | `/conversations/:id/choose` | user | Pick an option card; says so in the thread rather than flipping a silent flag. |
| POST | `/decisions/:id/vote` | user | Body: `option_id`. Resolution needs ≥2 votes and a strict lead. |

**The send contract that matters:** a model call must never cost someone the
thing they typed. `Send` commits the person's message *before* it asks Chaos
anything, and a failed model turn returns the message with no reply rather than
an error.

### Membership & invites

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/conversations/:id/members` | user | Add by `member_ids[]`, or by `name` to create a placeholder seat. |
| GET | `/conversations/:id/invite` | user | The invite link for this conversation. |
| POST | `/conversations/:id/join` | user | Take a seat from an invite. Body: `name`, `photo_url?`. |
| GET | `/invites/:id` | **—** | **The one unauthenticated read.** Title, emoji and member *names* only — never anything that was said. Keep `service.Previewer` a one-method interface so that stays visible at the type level. |

```json
{
  "conversation_id": "461ab673-…",
  "title": "Chaos, explain this to all of us.",
  "emoji": "✦",
  "url": "https://app.joinchaos.ai/join/461ab673-…",
  "member_names": ["Aman Dwivedi", "Rhea"]
}
```

> **Known gap.** That `url` points at the API host, which does not serve
> `/join/:id` — it 404s — and neither platform registers a deep link, so the
> link cannot open the app. Invites are effectively non-functional until a
> `/join/:id` page, a URL scheme and link handling are added. The invite base
> URL is also derived from `STORAGE_PUBLIC_BASE_URL`, which simultaneously
> builds avatar URLs; it needs its own config value before it can be repointed.

### Groups

The standing cast and what they already know.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/groups` | user | List. |
| POST | `/groups` | user | Body: `name`, `emoji?`, `member_ids[]?`. |
| GET | `/groups/:id` | user | Group with members, memory and conversations. |
| PATCH | `/groups/:id` | user | Rename. |
| DELETE | `/groups/:id` | user | Delete. Conversations survive (`group_id` → NULL). |
| POST | `/groups/:id/members` | user | Add a member. |
| DELETE | `/groups/:id/members/:memberId` | user | Remove. |
| POST | `/groups/:id/memory` | user | Body: `text`. Shared, e.g. "Budget ceiling is ₹2L per person." |
| DELETE | `/groups/:id/memory/:memoryId` | user | Forget it. |
| POST | `/groups/:id/ask` | user | Body: `question`. Searches **every** conversation the group has had and answers with citations. |
| GET | `/people/collaborators` | user | People you share the most conversations with. |

**Ask returns its sources.** The service resolves the model's citations back to
real threads and **drops anything invented** — a recall nobody can check is
worse than no recall.

### Profile — what Chaos knows about you

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/me/profile` | user | Profile plus learned facts. |
| PATCH | `/me/profile` | user | Update. |
| POST | `/me/facts` | user | Body: `text` (up to 12000 chars — this is also the paste-from-another-AI path). |
| PATCH | `/me/facts/:id` | user | Body: `value?`, `confirmed?`. |
| DELETE | `/me/facts/:id` | user | Forget. |
| POST | `/me/facts/learn` | user | Mine facts from pasted text. |
| POST | `/me/facts/refresh` | user | Mine facts from your own recent messages since the watermark. |
| GET | `/me/prompts/:source` | user | The copy-paste prompt for ChatGPT / Claude / Gemini. |

Two rules here:

- **Confirmed facts are never overwritten by a model.** The person already said
  this is right; quietly replacing it would make the "Looks right" button
  meaningless.
- **`Refresh` moves its watermark even when extraction fails**, or a model
  outage would re-mine the same eighty messages on every profile open and eat
  the whole hourly budget.

### Misc

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/directory/search` | user | Find users to add. |
| POST | `/media` | user | Multipart upload; returns a public URL. |
| GET | `/links/preview` | user | Server-side URL preview. Signed-in only, deliberately — it makes the server fetch a URL on request. |
| POST | `/me/devices` | user | Register a push token. |
| DELETE | `/me/devices/:token` | user | Unregister. |

### Admin

`admin` role required on all of these.

| Method | Path |
|---|---|
| POST | `/users` |
| GET | `/users` |
| GET | `/users/:id` |
| PATCH | `/users/:id` |
| DELETE | `/users/:id` |

---

## 6. Realtime

```
GET wss://app.joinchaos.ai/api/v1/ws?token=<access_token>
```

The token goes in the **query string**, not a header — a browser `WebSocket`
cannot set headers. The upgrade is a normal HTTP/1.1 upgrade; it will not work
over HTTP/2.

The server pushes a minimal event and the client refetches. Payloads are
deliberately tiny — the socket says *something changed*, it is not a data
channel:

```json
{ "type": "changed", "conversation_id": "461ab673-…" }
```

**The socket is also the presence signal.** While it is open the server holds
back push notifications for that user, which is why the app drops it on pause
and reconnects on resume. Do not "optimise" that away.

---

## 7. The AI layer

`internal/ai` is a port with an Azure OpenAI adapter and a `Disabled`
implementation. Leave `AZURE_OPENAI_ENDPOINT` or `AZURE_OPENAI_API_KEY` empty
and Chaos switches itself off rather than erroring — the app stays a working
group chat, it just stops answering.

**Exactly four calls, one file each:**

| Call | Does |
|---|---|
| `Reply` | Answer a thread, optionally with option cards or a vote |
| `Name` | Title a conversation from its first message |
| `Extract` | Mine durable facts from what someone said |
| `Ask` | Recall across a group's threads, with citations |

Two adapter constraints worth knowing: it uses `max_completion_tokens` (not
`max_tokens`), and with `response_format: json_object` the system prompt **must
contain the word "json"** or the API rejects the request — `chat()` asserts this
rather than letting it fail at runtime.

---

## 8. Three kinds of memory

They are not interchangeable, and mixing them is the main way this product can
get creepy.

| Kind | Stored in | Scope | Lifecycle |
|---|---|---|---|
| **Conversation memory** | `conversations.decided` / `open` | One thread | Rewritten wholesale each turn |
| **Group memory** | `group_memory` | One standing group | Typed by people, never by a model; prepended to every Chaos turn in that group |
| **Personal facts** | `facts` | One person | Mined from their own messages; only ever applied to the person who asked |

A fact about Rahul must never become something "the group knows", and a group's
budget ceiling must never become a fact about you.

---

## 9. Configuration

Set in `.env`; see `.env.example`. Required: `DATABASE_URL` and the two JWT
secrets.

| Variable | Notes |
|---|---|
| `APP_ENV` | `development` / `staging` / `production` |
| `PORT` | Listen port |
| `ALLOWED_ORIGINS` | CORS allowlist; `*` permitted in dev only |
| `RATE_LIMIT_RPM` | Fixed window across all of `/api/v1` |
| `DATABASE_URL` | Postgres DSN |
| `REDIS_ADDR` | Optional. Absent → `cache.Noop{}`, which silently disables caching **and** rate limiting |
| `JWT_ACCESS_SECRET` / `JWT_REFRESH_SECRET` | Must differ |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL` | `15m` / `720h` |
| `FIREBASE_PROJECT_ID` | Alone enables Apple/Google sign-in |
| `FIREBASE_CREDENTIALS_FILE` | Only for push and identity deletion |
| `AZURE_OPENAI_*` | Endpoint, key, chat deployment, api version |
| `AI_PER_HOUR_LIMIT` | Chaos spend cap (needs Redis to be enforced) |
| `EMAIL_DRIVER` | `log` or `smtp`. **`log` in production means reset emails go nowhere.** |
| `STORAGE_DRIVER` | `local` or `s3` |
| `STORAGE_PUBLIC_BASE_URL` | Public base for uploads — and, currently, for invite links |

---

## 10. Running it

```bash
cp .env.example .env
docker compose up -d postgres redis
make run          # migrations apply at boot
make test
make swag         # regenerate docs/swagger — part of "done" for any handler change
```

Migrations run at boot from `migrations/` using a **relative** path, so run the
binary from the repo root (or the Dockerfile's `/app`, which copies them).
There is no `AutoMigrate`.

### Production

```bash
rsync -az --exclude .git --exclude .env --exclude uploads \
  -e "ssh -i ~/.ssh/source-entvin_key.pem" \
  ./ risharya@172.200.162.118:/opt/chaos/
ssh -i ~/.ssh/source-entvin_key.pem risharya@172.200.162.118 \
  'cd /opt/chaos && sudo docker compose -f docker-compose.prod.yml up -d --build'
```

Caddy terminates TLS and proxies to the API on `127.0.0.1:8090`. Postgres and
Redis have no host ports. `docker-compose.prod.yml` is standalone rather than an
overlay on purpose: Compose *appends* `ports` across `-f` files, so an overlay
would keep the dev mappings and publish Postgres to the internet.

---

## 11. Adding a domain module

1. `internal/domain/<x>/{entity,repository,dto,mapper,service,handler}` — copy
   the conversation module's shape.
2. `make migrate-create name=create_<x>`, then write the SQL by hand.
3. Wire constructors in `main.go`, add routes in `internal/routes/routes.go`.
4. Swagger annotations on handlers, then `make swag`.
5. Table-driven service tests with hand-written repository fakes.

Existing modules must not change — only `main.go` and `routes.go` grow. No test
touches the network, a model, or the database; keep it that way.

### Gotchas already paid for

- **`omitempty` on a `*string` tests the pointer, not the string.** A pointer to
  `""` still runs the rest of the chain, so `validate:"omitempty,url"` makes it
  impossible to send `""` to clear a field. Where empty is meaningful (see
  `UpdateProfileRequest.AvatarURL`) validate in the service instead.
- **GORM's `Scan` does not walk anonymous embedded structs.** A flat struct is
  required for raw scans — an embedded one silently comes back zeroed.
- **`make swag` fails loudly on a bad `@Success` type**, which is how a
  reference to a response type that never existed sat undetected until the first
  regeneration.
