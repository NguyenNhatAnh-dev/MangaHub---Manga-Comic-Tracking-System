# MangaHub

- A manga tracking system that demonstrates five network protocols (HTTP, TCP, UDP, WebSocket, gRPC) working together in a single Go application

## Tech Stack

- **Language:** Go 1.21
- **HTTP Framework:** Gin (`github.com/gin-gonic/gin`)
- **WebSocket:** `github.com/gorilla/websocket`
- **gRPC:** `google.golang.org/grpc` (JSON codec, no protoc generation required)
- **Auth:** JWT (`github.com/golang-jwt/jwt/v4`) + bcrypt (`golang.org/x/crypto`)
- **Database:** SQLite (`github.com/mattn/go-sqlite3`)
- **Config:** YAML (`gopkg.in/yaml.v3`)
- **Build:** GNU Make

## Core Features

- **HTTP REST API** — user registration/login (JWT), manga CRUD, library management, reading progress tracking, search with genre/status filters
- **TCP sync server** — persistent JSON-over-TCP connections with auth handshake, real-time progress broadcast to all sessions of the same user via goroutines
- **UDP notification server** — connectionless client registration with genre-based subscription, broadcast notifications to all registered addresses
- **WebSocket chat** — room-based real-time messaging with message history persistence, join/leave events, and ping/pong keepalive
- **gRPC internal service** — `GetManga`, `SearchManga`, `UpdateProgress` RPCs using a custom JSON codec (no generated protobuf code)

## Quick Start

```bash
git clone <repo-url> && cd mangahub
make build
make seed
make run
```

- `make build` compiles all 7 binaries into `bin/`
- `make seed` loads `data/manga.json` (35 manga entries) into SQLite
- `make run` starts all 5 servers in a single process (HTTP :8080, TCP :9090, UDP :9091, gRPC :9092, WS :9093)

## Architecture / Structure

```
mangahub/
├── cmd/
│   ├── api-server/    # All-in-one entry point (starts HTTP + TCP + UDP + gRPC + WS)
│   ├── tcp-server/    # Standalone TCP server
│   ├── udp-server/    # Standalone UDP server
│   ├── grpc-server/   # Standalone gRPC server
│   ├── ws-server/     # Standalone WebSocket server
│   ├── seed/          # Database seeder (loads manga.json → SQLite)
│   └── cli/           # CLI client (all 5 protocols)
├── internal/
│   ├── auth/          # JWT generation/validation, password hashing, middleware
│   ├── config/        # YAML config loader
│   ├── database/      # SQLite init + schema (users, manga, user_progress, chat_messages)
│   ├── httpapi/       # Gin routes: /auth, /manga, /users, /admin
│   ├── manga/         # Manga repository (search, CRUD, library, progress)
│   ├── user/          # User repository
│   ├── tcp/           # TCP server: auth handshake → JSON message loop → progress broadcast
│   ├── udp/           # UDP server: register/unregister clients → broadcast notifications
│   ├── websocket/     # Hub + Client: room management, message persistence, read/write pumps
│   └── grpc/          # gRPC service + JSON codec + client wrapper
├── pkg/
│   ├── models/        # Shared data types (Manga, User, ChatMessage, etc.)
│   └── protocol/      # In-process pub/sub broker (progress + notification channels)
├── proto/             # manga.proto (reference only — project uses JSON codec)
├── data/              # manga.json + mangahub.db
├── config.yaml        # Port and secret configuration
└── Makefile           # build, seed, run, run-http, run-tcp, run-udp, run-grpc, run-ws, cli, test, clean
```

### Inter-service Communication

- `pkg/protocol.Broker` is a shared in-process pub/sub bus
- HTTP `PUT /users/progress` publishes to the broker → TCP server picks it up and broadcasts to connected clients
- HTTP `POST /admin/notify` publishes to the broker → UDP server picks it up and broadcasts to registered addresses
- gRPC `UpdateProgress` also publishes through the same broker
- WebSocket chat is self-contained (hub ↔ clients) with SQLite-backed message history

### CLI Commands

```
mangahub register <user> <email> <pass>     # HTTP
mangahub login <user> <pass>                # HTTP → saves JWT to ~/.mangahub-session.json
mangahub search <query>                     # HTTP
mangahub tcp:sync                           # TCP persistent connection
mangahub udp:subscribe [genres]             # UDP listener
mangahub grpc:get <id>                      # gRPC
mangahub chat [room]                        # WebSocket
```

### Default Ports

| Protocol  | Port |
|-----------|------|
| HTTP      | 8080 |
| TCP       | 9090 |
| UDP       | 9091 |
| gRPC      | 9092 |
| WebSocket | 9093 |
