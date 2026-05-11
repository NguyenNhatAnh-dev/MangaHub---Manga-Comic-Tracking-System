#!/usr/bin/env bash
set -e

BIN=./bin/mangahub-cli
HTTP_HEALTH="http://localhost:8080/health"

echo "=== MangaHub end-to-end demo script ==="
echo

if [ ! -f "$BIN" ]; then
  echo "Building binaries first..."
  make build
fi

echo "[1/8] Health check"
$BIN health

echo
echo "[2/8] Register a demo user (ignored if already exists)"
$BIN register demo demo@example.com password123 || true

echo
echo "[3/8] Login"
$BIN login demo password123

echo
echo "[4/8] Search for 'one'"
$BIN search one

echo
echo "[5/8] Add One Piece to library and update progress"
$BIN library:add one-piece reading 1
$BIN progress one-piece 5

echo
echo "[6/8] View library"
$BIN library

echo
echo "[7/8] gRPC GetManga"
$BIN grpc:get one-piece

echo
echo "[8/8] Trigger UDP notification (subscribe in another terminal first)"
$BIN udp:notify one-piece "New chapter 1100 released!"

echo
echo "=== Done. For interactive features, try in separate terminals: ==="
echo "  $BIN tcp:sync           (listens for progress broadcasts)"
echo "  $BIN udp:subscribe       (listens for UDP notifications)"
echo "  $BIN chat               (websocket chat)"
