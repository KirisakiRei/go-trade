# Dockerfile engine ILA (cmd/engine).
#
# Kenapa CGO dimatikan: engine tidak butuh libc dinamis. Statis membuat image
# akhir cukup `scratch`-like dan tidak pernah rusak karena selisih versi glibc
# antara tahap build dan tahap jalan.
#
# Kenapa GOFLAGS=-p=1: VPS engine ber-RAM 909 MB. Kompilasi paralel Go di mesin
# sekecil itu menghabiskan memori dan OOM-killer membunuh build tanpa pesan yang
# jelas. Satu paket sekali saja jauh lebih lambat tapi selalu selesai.
#
# Berkas proto sudah di-commit di gen/, jadi protoc TIDAK dibutuhkan saat build.
# Ini disengaja: menambah toolchain lain memperbesar permukaan yang bisa gagal.

FROM golang:1.23-bookworm AS build

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOFLAGS=-p=1 \
    GOTOOLCHAIN=local

# go.mod/go.sum lebih dulu agar lapisan unduhan dependensi dipakai ulang saat
# hanya kode yang berubah.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -trimpath: jalur build tidak ikut ke biner, sehingga tidak membocorkan
# susunan folder mesin build.
RUN go build -trimpath -ldflags="-s -w" -o /out/engine ./cmd/engine

FROM debian:bookworm-slim

# ca-certificates: engine menarik candle lewat HTTPS/WSS TradingView.
# tzdata: jam pasar dan TTL sinyal bergantung pada zona waktu yang benar.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

# User tanpa hak istimewa: engine tidak butuh root untuk apa pun.
RUN useradd -u 1001 -m -s /usr/sbin/nologin app

COPY --from=build /out/engine /app/engine

# Folder log ditulis sebagai user app; volume dari host harus bisa ditulisi.
RUN mkdir -p /var/log/ila && chown -R app:app /var/log/ila

USER app
WORKDIR /app

ENTRYPOINT ["/app/engine"]
