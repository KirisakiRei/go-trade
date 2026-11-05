# MEMORY.md - Standing Rules & Operating Conventions

## 1. Engine & Environment
- **Core Engine:** Native Golang (`ila-engine`) berlokasi di `/opt/data/ila-engine`.
- **Database:** SQLite WAL di `ila.db`.
- **Binaries:**
  - `bin/ila-cli`: CLI evaluasi mandiri ILA (`--symbol <PAIR> [--htf <HTF> --ltf <LTF>]`).
  - `bin/ila-server`: REST API daemon di port `8080` (passive broker monitor).
  - `bin/ila-scanner`: Scanner periodik 15-menit native Go.
- **Data Feed:** WebSocket TradingView FOREX.com (`FOREXCOM:XAUUSD`, `FOREXCOM:EURUSD`, `FOREXCOM:NAS100`) untuk presisi bentuk candle dan jam tutup real-time.

## 2. Multi-Broker Setup
- **Match-Trader:** REST Client Trader API. Nasdaq 100 dipetakan ke simbol `US100`.
- **cTrader:** Native Go Protobuf TLS Open API.
- **Synchronous Execution:** Kedua akun dieksekusi secara serentak dengan sizing lot 1% independen sesuai ekuitas riil masing-masing broker.
- **Broker-Managed SL/TP:** SL dan TP dipasang langsung di server broker saat open order. Sistem lokal mengawal secara pasif dan dilarang melikuidasi order lokal berdasarkan noise feed luar.

## 3. Presisi Simbol
- **Forex:** Wajib 5 angka desimal (`%.5f`).
- **Gold & Indeks:** 2 angka desimal (`%.2f`).
