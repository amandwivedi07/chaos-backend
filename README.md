# Chaos — backend

Group chat with an AI in it that actually decides things.

Four friends argue about a trip. One can only do four nights, one wants a
beach, one wants nightlife, one wants it under ₹2L. Chaos reads all of that,
answers with three comparison cards, and someone taps "Choose this →". The
40-message thread ends.

Go 1.26 · Gin · GORM · PostgreSQL · Redis · JWT with refresh rotation · Azure OpenAI.

## Run it

```bash
cp .env.example .env          # DATABASE_URL and the two JWT secrets are required
docker compose up -d postgres redis
make run                      # migrations apply at boot
```

`http://localhost:8080/healthz` · `http://localhost:8080/swagger/index.html`

Chaos itself needs `AZURE_OPENAI_ENDPOINT` and `AZURE_OPENAI_API_KEY`. Without
them the server runs fine and logs `Chaos disabled` — the app stays a working
group chat, it just stops answering.

## API

Everything is under `/api/v1` and returns the same envelope:

```json
{ "success": true, "message": "...", "data": {} }
```

| | |
|---|---|
| `POST /auth/register` `/login` `/firebase` `/refresh` `/logout` | sessions |
| `GET /auth/me` · `PATCH /auth/me` | the signed-in account |
| `GET /conversations` · `POST /conversations` | the home list |
| `GET /conversations/:id/messages` | the thread |
| `POST /conversations/:id/messages` | say something — the reply comes back with it |
| `POST /conversations/:id/ask` | make Chaos answer now |
| `POST /conversations/:id/choose` | "Choose this →" on an option card |
| `POST /decisions/:id/vote` | vote on a decision Chaos put to the group |
| `POST /conversations/:id/members` · `GET /conversations/:id/invite` | add people |
| `GET /invites/:id` | what a link recipient sees — the only unauthenticated read |
| `POST /conversations/:id/join` | join from a link |
| `GET /me/profile` · `PATCH /me/profile` | you |
| `POST /me/facts` · `PATCH /me/facts/:id` · `DELETE /me/facts/:id` | what Chaos knows about you |
| `POST /me/facts/learn` | paste an export from ChatGPT / Claude / Gemini |
| `GET /me/prompts/:source` | the ready-made prompt to hand that assistant |
| `GET /ws?token=` | WebSocket: server says "changed", client refetches |

## Layout

```
cmd/server/main.go        composition root — wiring only
internal/domain/          user · auth · conversation · profile · device · media · link
internal/ai/              Client port + Azure adapter. Reply, Name, Extract — that's all
internal/{config,logger,database,cache,storage,email,events,worker,push,realtime}
internal/{middleware,common,routes}
migrations/               golang-migrate pairs, applied at boot
```

See [CLAUDE.md](CLAUDE.md) for the rules that hold this shape together.

## Make

```
make run     make test    make vet    make fmt
make swag    make docker-up          make migrate-create name=add_x
```
# chaos-backend
