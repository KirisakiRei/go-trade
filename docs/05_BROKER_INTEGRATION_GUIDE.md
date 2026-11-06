# 05. PANDUAN INTEGRASI MULTI-BROKER (BROKER INTEGRATION)

## 1. Arsitektur Dual-Broker Manager (`pkg/broker/multi_broker.go`)

Sistem mengoperasikan modul **MultiBrokerManager** yang mampu mengeksekusi order secara serentak ke dua akun broker berbeda dengan perhitungan manajemen risiko independen:

```text
                           ┌──────────────────────────┐
                           │   MultiBrokerManager     │
                           └─────────────┬────────────┘
                                         │
                 ┌───────────────────────┴───────────────────────┐
                 ▼                                               ▼
   ┌───────────────────────────┐                   ┌───────────────────────────┐
   │    CTraderBroker (Go)     │                   │  MatchTraderBroker (REST) │
   │   IC Markets Demo #10131354│                  │     Prop Demo #1089632    │
   │  (Protobuf TLS Port 5035) │                   │ (Bearer Token /v1/position│
   └───────────────────────────┘                   └───────────────────────────┘
```

---

## 2. Broker 1: IC Markets cTrader (`pkg/broker/ctrader_broker.go`)

* **Protokol:** 100% Native Golang TCP TLS Google Protocol Buffers (Open API v2).
* **Endpoint:** `demo.ctraderapi.com:5035`.
* **Kredensial Lingkungan (`.env`):**
  * `CTRADER_CLIENT_ID`
  * `CTRADER_CLIENT_SECRET`
  * `CTRADER_ACCOUNT_ID` (`49366554`)
  * `CTRADER_ACCESS_TOKEN`
* **Pemetaan Simbol ID cTrader:**
  * `EURUSD` ➡️ ID `1` (Multiplier: `10.000.000`)
  * `XAUUSD` / `GOLD` ➡️ ID `41` (Multiplier: `10.000`)
  * `USTEC` / `NAS100` ➡️ ID `10014` (Volume step: `10` = 0.10 lot)
* **Eksekusi Order:**
  * Mengirim `ProtoOANewOrderReq` tipe `MARKET` dengan SL dan TP server-side.
* **Audit Deal:**
  * Memanggil `GetDealsByPosition` (`ProtoOADealListReq`) untuk mengambil riwayat deal penutup asli (`ExecutionPrice`, `GrossProfit`).

---

## 3. Broker 2: Match-Trader (`pkg/broker/matchtrader_broker.go`)

* **Protokol:** HTTP REST Client JSON standar.
* **Base URL:** `https://trader-api.match-trader.com/v1`.
* **Kredensial Lingkungan (`.env`):**
  * `MATCHTRADER_TOKEN` (JWT Token Bearer).
* **Pemetaan Simbol:**
  * Nasdaq 100 dipetakan ke **`US100`** (bukan `USTEC`).
* **Endpoint Utama yang Digunakan:**
  * `GET /v1/account-summary`: Tarik data balance, live equity, dan free funds secara real-time.
  * `POST /v1/positions`: Buka posisi instan dengan body:
    ```json
    {
      "symbol": "US100",
      "side": "BUY",
      "volume": 0.10,
      "slPrice": 26900.00,
      "tpPrice": 27500.00,
      "comment": "ILA H4->M15"
    }
    ```
  * `GET /v1/positions`: Tarik daftar posisi terbuka live.
  * `GET /v1/closed-positions?from={ms}&to={ms}`: Tarik riwayat posisi tertutup beserta `closePrice`, `profit`, dan `closeReason` (`CLOSE_REASON_TAKE_PROFIT` atau `CLOSE_REASON_STOP_LOSS`).

---

## 4. Format Tiket Gabungan & Parsing Otomatis

Ketika order dieksekusi secara dual-broker, nomor tiket disimpan dalam format gabungan:
```text
Match-Trader (Trader API REST)(#W525654572927291, 1.05 lot) & IC Markets cTrader (Native Go)(#692956133, 10.47 lot)
```

Fungsi **`broker.ExtractTicketIDs(ticket)`** membedah string tersebut menjadi:
* `cPosID`: `692956133` (numerik `int64` untuk query deal cTrader).
* `mtrID`: `W525654572927291` (string ID untuk query closed position Match-Trader).

Mekanisme ini menjamin sinkronisasi PnL dan status closed trade di database selalu 100% akurat.

---

## 5. Kebijakan Keamanan Kredensial (Hardline Security Policy)

1. **Dilarang Keras Menulis Kredensial di Kode Sumber:**
   * Seluruh token API, client secret, dan password wajib dibaca dari environment variable (`os.Getenv`).
2. **Proteksi `.gitignore`:**
   * File `.env`, file database SQLite (`*.db`), log, dan binary (`bin/`) wajib terdaftar di `.gitignore`.
3. **Template Publik `.env.example`:**
   * Repository publik hanya menyediakan file template dummy `.env.example` tanpa nilai asli.
