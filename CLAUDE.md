# CLAUDE.md

Guidance for Claude Code (and other AI assistants) when working in this repository.

## What this is

**Chaos backend** — a production-grade Go REST API built on Clean Architecture.
Gin + GORM + PostgreSQL + Redis, JWT auth with refresh-token rotation, Zap logging,
Viper config, Swagger docs. Same architecture as Space Talk, different product.

The product: a group chat with an AI in it that actually decides things. Friends
argue about a trip; Chaos reads the constraints everyone stated, answers with
two or three comparison cards, and either lets someone pick or puts it to a
vote. It also remembers durable facts about each person, so the next plan
already knows they can't do more than four nights.

- Go 1.26, module `github.com/chaosapp/backend`
- Entry point: `cmd/server/main.go` (composition root — wiring ONLY)
- Run: `make run` (needs `.env`, see `.env.example`) · Test: `make test` · Docs: `make swag`
- Local infra: `docker compose up -d postgres redis`

## Architecture — the one rule set that matters

```
HTTP → middleware → handler → DTO validation → service → repository (interface) → GORM → PostgreSQL
```

- **Business logic lives ONLY in services** (`internal/domain/<x>/service`).
  Never in handlers, routes, middleware, or repositories.
- **Every dependency is an interface injected via constructor** (`New(...)`).
  No globals, no `init()` wiring, no service locators.
- **Repositories own SQL/GORM and authorization-relevant lookups; they never
  see HTTP.** Services never import `gin`.
- **Four shapes per concept, never mixed**: domain entity (`entity/`),
  GORM model (private, in `repository/gorm_model.go`), request DTO, response DTO.
  Mapping: repository ↔ entity internally; `mapper/` maps entity → response DTO.
- **Errors**: services return `*apperrors.AppError` (internal/common/errors).
  Handlers end every failure path with `response.Error(c, err)` — the ONLY place
  errors become HTTP. Add new kinds to errors.go, not ad-hoc statuses.
- **Response envelope** (never deviate):
  success `{"success":true,"message":…,"data":…}`,
  failure `{"success":false,"message":…,"error":KIND,"errors":{field:msg},"data":null}`.

## Layout

```
cmd/server/           main.go — config→logger→db+migrate→cache→bus→wiring→routes→serve
internal/
  config/             Viper env config (typed structs, validated)
  logger/             Zap builder (console dev / JSON prod)
  database/           GORM connect (pooled) + golang-migrate runner (file://migrations)
  cache/              Store port + Redis adapter + Noop (dev without Redis)
  auth/jwt/           token Manager port: separate access/refresh secrets, typ claim
  middleware/         Auth, RequireRoles, RequestID, Logging, Recovery, CORS,
                      RateLimit (cache-backed fixed window), SecureHeaders
  common/             errors, response envelope, pagination, constants, utils
  ai/                 Client port + Azure adapter + Disabled. FOUR calls only:
                      Reply (answer a thread), Name (title one), Extract (mine
                      facts), Ask (recall across a group's threads, with citations)
  domain/user/        entity → repository(iface+gorm) → dto → mapper → service → handler
  domain/auth/        register/login/refresh-rotation/logout/forgot/reset/verify/change/me
  domain/conversation/ threads, messages, option cards, decisions + votes, invites.
                      service/chaos.go is the product-level port over ai.Client
  domain/group/       the standing cast, its shared memory, and "ask across
                      everything this group said" (service/ask.go)
  domain/profile/     the person behind the login + the facts Chaos learnt about them
  storage/            Storage port + local + S3 (R2-compatible via S3_ENDPOINT)
  email/              Sender port + log (dev) + SMTP adapters
  events/ worker/     async event bus + goroutine pool (emails run off-request)
  push/               Sender port + FCM + Notifier (joins device registry to sender)
  realtime/           WebSocket hub; server pushes "changed", clients refetch
  routes/             /api/v1 registration — the only place URLs are declared
migrations/           golang-migrate SQL pairs (auto-applied at boot; NO AutoMigrate)
docs/swagger/         generated — never hand-edit; regenerate with `make swag`
```

## The two rules that are the product

Both live in `domain/conversation/entity` and both have tests. Change them only
on purpose.

- **`ShouldReply`** — when Chaos speaks. Being named always works. Otherwise it
  waits: a question only pulls it in after the group has gone back and forth a
  couple of times, and a thread that has run five messages without it gets one
  turn unasked. Chaos is the friend who answers when it matters, not the one
  who replies to everything. **`direct` short-circuits all of it** — a private
  thread is you talking TO Chaos, and restraint there reads as being ignored.
- **`Decision.Resolve`** — when a vote closes. A winner needs at least two votes
  AND a strict lead. One person clicking their own suggestion is not the group
  deciding, and a tie is precisely the argument the vote was meant to end.

## Adding a new domain module

1. `internal/domain/<x>/{entity,repository,dto,mapper,service,handler}` —
   copy the conversation module's shape.
2. Migration: `make migrate-create name=create_<x>` → write SQL by hand.
3. Wire constructors in `main.go`, add routes in `internal/routes/routes.go`.
4. Swagger annotations on handlers, then `make swag`.
5. Table-driven service tests with hand-written repo fakes.
Existing modules must not change (only main.go + routes.go grow).

## Three kinds of memory, and they are not interchangeable

- **Conversation memory** (`conversations.decided` / `open`) — what this thread
  has settled. Rewritten wholesale each turn.
- **Group memory** (`group_memory`) — what the standing cast knows. Typed by
  people, never by a model, and prepended to every Chaos turn in every
  conversation that group has.
- **Personal facts** (`facts`) — what is true about one person. Mined from what
  they said, and only ever applied to the person who asked.

Mixing them is the main way this product can get creepy: a fact about Rahul
must never become something "the group knows", and a group's budget ceiling
must never become a fact about you.

## Cross-module dependencies

Three modules read slices of each other, and none imports another. Conversation
declares `Facts` (profile satisfies it) and `GroupContext` (group satisfies it);
profile declares `History` and group declares `Threads` and `People` (the
conversation service satisfies all three); `main.go` wires them. Group and
conversation are a genuine cycle, broken in `main.go` by two tiny lazy adapters
rather than a setter on a half-built service. Do the same for any future pair — the
dependency always goes through a port declared by the consumer, which is what
keeps the cycle from ever forming.

## Auth model (do not weaken)

- **Social sign-in needs no secret.** `FIREBASE_PROJECT_ID` alone switches on
  Apple/Google: verifying an ID token uses Google's PUBLIC signing certificates,
  so `internal/auth/firebase` builds a verify-only client with
  `option.WithoutAuthentication()`. `FIREBASE_CREDENTIALS_FILE` is only needed
  for push and for deleting a Firebase identity on account deletion — which is
  why `DeleteUser` says exactly that when it is missing rather than failing
  vaguely. Neither set returns `Disabled{}` and the server still boots.

- Access JWT 15 m / refresh JWT 30 d, different secrets, `typ` claim checked.
- Refresh tokens stored **sha256-hashed** in `refresh_tokens` with rotation
  `family_id`; reusing a rotated-out token revokes the whole family.
- Login/forgot-password answers are identical for unknown emails (no enumeration).
- One-time tokens (`action_tokens`) are hashed, single-use, short-lived.
- Password changes/resets revoke all sessions.

## Conventions & gotchas

- Files < 300 lines, functions < 50; split before you exceed.
- **A model call must never cost someone the thing they typed.** `Send` commits
  the person's message before it asks Chaos anything, and a failed turn returns
  the message with no reply rather than an error.
- **Placeholder users** have no email and no password hash, so they can never be
  signed into. They exist so a group can plan around someone who has not signed
  up; the invite link is what binds a seat to a real account.
- **Confirmed facts are never overwritten by a model** (`UpsertFacts`). The
  person already said this is right, and quietly replacing it would make the
  "Looks right" button meaningless.
- `GET /invites/:id` is the ONE unauthenticated read. It exposes the title and
  members' names, never anything that was said. Keep `service.Previewer` a
  one-method interface so that stays visible at the type level.
- Redis is optional in dev: `cache.Noop{}` silently disables caching AND rate
  limiting — including the Chaos spend cap. Don't rely on limits locally.
- Migrations run at boot from `migrations/` (relative path) — run the binary from
  the repo root, or the Dockerfile's `/app` (it copies migrations).
- **`omitempty` on a `*string` tests the pointer, not the string.** A pointer to
  "" still runs the rest of the chain, so `validate:"omitempty,url"` makes it
  impossible to send "" to clear a field. Where empty is meaningful (see
  `UpdateProfileRequest.AvatarURL`) validate in the service instead.
- `make swag` is part of the definition of done for a handler change. It fails
  loudly on a bad `@Success` type, which is how `response.Envelope` — a name
  that never existed — sat in the link handler until the first regeneration.
- **`Refresh` moves its watermark even when extraction fails.** Otherwise a
  model outage would re-mine the same eighty messages on every profile open and
  eat the whole hourly budget.
- No test suite touches the network, a model, or the DB; keep it that way.
