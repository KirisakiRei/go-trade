package ila

// Test Urutan 4 pada C2 IRL: ACUAN BERGESER.
//
// Inilah pembeda utama definisi draft (c2-closure-irl.md) dari logika lama.
// Pada logika lama, sistem selalu menuntut sapuan terhadap candle TEPAT
// SEBELUMNYA. Draft tidak: yang diuji adalah LEVEL ACUAN, dan acuan itu sendiri
// bergeser setiap kali sebuah candle menyentuhnya namun gagal ditolak.
//
// Akibat praktisnya, setup yang sah menurut draft bisa ditolak sistem lama:
//
//   candle X menyentuh acuan, close melewati acuan tetapi masih di dalam zona
//   -> C2 lama gugur, acuan pindah ke high(X)
//   candle Y cukup menyapu high(X) dan close di bawahnya -> C2 SAH
//
// Logika lama akan menolak candle Y karena Y tidak menyapu high candle X-1.

import (
	"testing"
	"time"

	"ila-engine/pkg/indicators"
)

// SIBI zona 4400.32 (Bottom) - 4405.77 (Top).
// C3 index 2 dengan High 4400.32 -> acuan awal 4400.32.
func sibi4400(base time.Time) (FVG, []indicators.Candle) {
	fvg := FVG{
		Type:             FVGBearish,
		Top:              4405.77,
		Bottom:           4400.32,
		Candle1Index:     0,
		Candle2Index:     1,
		Candle3Index:     2,
		InvalidatedIndex: -1,
	}
	cs := []indicators.Candle{
		bangunCandle(base, 0, 4430, 4440, 4405.77, 4412), // C1: Low = atap zona
		bangunCandle(base, 1, 4431.07, 4431.07, 4394.15, 4394.15),
		bangunCandle(base, 2, 4394.15, 4400.32, 4390, 4396), // C3: High = lantai zona / acuan awal
	}
	return fvg, cs
}

// Kasus H dokumen: C2 gugur karena acuan dilewati, lalu C2 baru terbentuk.
func TestC2IRLUrutan4AcuanBergeserLaluC2Baru(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	fvg, cs := sibi4400(base)

	cs = append(cs,
		// Candle X (idx 3): high 4402.67 menembus acuan 4400.32, close 4402.10
		// masih di bawah atap 4405.77 -> URUTAN 4.
		bangunCandle(base, 3, 4398, 4402.67, 4397, 4402.10),
		// Candle Y (idx 4): high 4404.00 menyapu high X (4402.67), close 4399 ditolak
		// -> C2 SAH dengan acuan baru 4404.00.
		bangunCandle(base, 4, 4402.10, 4404.00, 4396, 4399),
	)
	// Candle penutup supaya idx 4 bukan candle terakhir.
	cs = append(cs, bangunCandle(base, 5, 4399, 4403.50, 4390, 4392))

	res := EvaluateC2IRL(fvg, cs)

	// C2 terakhir harus di idx 4 dengan acuan 4404.00.
	if res.Status != C2Aktif {
		t.Fatalf("harapan C2_AKTIF, dapat %s (%s)", res.Status, res.Detail)
	}
	if res.CandleIndex != 4 {
		t.Fatalf("C2 seharusnya di idx 4 (menyapu acuan bergeser), dapat %d", res.CandleIndex)
	}
	if res.Reference != 4404.00 {
		t.Fatalf("acuan setelah C2 idx4 harus 4404.00, dapat %.2f", res.Reference)
	}
	if res.Pergeseran != 2 {
		t.Fatalf("harapan 2 pergeseran (candle X gugur + candle Y jadi C2), dapat %d", res.Pergeseran)
	}
}

// Candle yang close TEPAT di acuan BUKAN C2 (dokumen tegas), dan acuan
// bergeser karena close tidak berada di bawah acuan.
func TestC2IRLCloseTepatDiAcuanBukanC2(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	fvg, cs := sibi4400(base)

	cs = append(cs,
		// high 4401.00 menembus acuan 4400.32, close TEPAT 4400.32.
		bangunCandle(base, 3, 4398, 4401.00, 4395, 4400.32),
	)
	res := EvaluateC2IRL(fvg, cs)

	if res.Status == C2Aktif {
		t.Fatalf("close tepat di acuan bukan C2, tetapi dapat C2_AKTIF di idx %d", res.CandleIndex)
	}
	if res.Reference != 4401.00 {
		t.Fatalf("acuan harus bergeser ke high candle itu (4401.00), dapat %.2f", res.Reference)
	}
}

// Candle yang tidak menyentuh acuan diabaikan; C2 aktif sebelumnya tetap.
func TestC2IRLCandleTidakSentuhAcuanDiabaikan(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	fvg, cs := sibi4400(base)

	cs = append(cs,
		// idx 3: C2 pertama, high 4402 menembus acuan, close 4399 ditolak.
		bangunCandle(base, 3, 4398, 4402, 4395, 4399),
		// idx 4: high 4401.50 TIDAK menyentuh acuan baru 4402 -> diabaikan.
		bangunCandle(base, 4, 4399, 4401.50, 4396, 4398),
	)
	cs = append(cs, bangunCandle(base, 5, 4398, 4401, 4390, 4392))

	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif {
		t.Fatalf("harapan C2_AKTIF, dapat %s", res.Status)
	}
	if res.CandleIndex != 3 {
		t.Fatalf("C2 harus tetap di idx 3 (idx 4 tidak menyentuh acuan), dapat %d", res.CandleIndex)
	}
}

// Invalidasi bersifat final: close di atas zona menutup FVG, candle setelahnya
// tidak bisa menjadi C2 walaupun harga kembali masuk zona.
func TestC2IRLInvalidLaluBalikBukanC2(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	fvg, cs := sibi4400(base)

	cs = append(cs,
		// close 4410 di atas atap 4405.77 -> FVG INVALID.
		bangunCandle(base, 3, 4398, 4412, 4397, 4410),
		// harga balik masuk zona dan ditolak, tetapi FVG sudah mati.
		bangunCandle(base, 4, 4410, 4405, 4390, 4395),
	)
	res := EvaluateC2IRL(fvg, cs)

	if res.Status != C2Invalid {
		t.Fatalf("harapan INVALID (evaluasi berhenti di invalidasi), dapat %s", res.Status)
	}
	if res.InvalidIndex != 3 {
		t.Fatalf("harapan InvalidIndex 3, dapat %d", res.InvalidIndex)
	}
}

// FVG yang sudah Invalidated di FindFVGs tidak boleh menghasilkan C2 sama sekali.
func TestC2IRLFVGInvalidatedTidakAdaC2(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	fvg, cs := sibi4400(base)
	fvg.Invalidated = true
	fvg.InvalidatedIndex = 3

	cs = append(cs, bangunCandle(base, 3, 4398, 4412, 4397, 4410))
	res := EvaluateC2IRL(fvg, cs)

	if res.Status != C2Invalid {
		t.Fatalf("harapan INVALID karena FVG.Invalidated, dapat %s", res.Status)
	}
	if res.InvalidIndex != 3 {
		t.Fatalf("InvalidIndex harus diambil dari FVG, dapat %d", res.InvalidIndex)
	}
}

// BISI (bullish) cermin: acuan awal = low candle 3, acuan bergeser ke low candle.
func TestC2IRLBullishAcuanBergeser(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	// BISI zona 4400.32 (bawah) - 4405.77 (atas): C3 Low 4405.77 = acuan awal.
	fvg := FVG{
		Type:             FVGBullish,
		Top:              4405.77,
		Bottom:           4400.32,
		Candle1Index:     0,
		Candle2Index:     1,
		Candle3Index:     2,
		InvalidatedIndex: -1,
	}
	cs := []indicators.Candle{
		bangunCandle(base, 0, 4375, 4400.32, 4370, 4395), // C1: High = lantai zona
		bangunCandle(base, 1, 4395, 4430, 4390, 4425),
		bangunCandle(base, 2, 4425, 4430, 4405.77, 4420), // C3: Low = atap zona / acuan awal
		// idx 3: low 4403 menembus acuan 4405.77, close 4404.50 di atas acuan ->
		// C2 terbentuk, acuan bergeser ke low 4403.
		bangunCandle(base, 3, 4420, 4422, 4403.00, 4410),
		// idx 4: low 4401.50 menyapu acuan baru 4403, close 4408 ditolak -> C2 baru.
		bangunCandle(base, 4, 4410, 4415, 4401.50, 4408),
		// penutup
		bangunCandle(base, 5, 4408, 4418, 4406, 4416),
	}

	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif {
		t.Fatalf("harapan C2_AKTIF, dapat %s (%s)", res.Status, res.Detail)
	}
	if res.Reference != 4401.50 {
		t.Fatalf("acuan akhir harus low C2 terakhir 4401.50, dapat %.2f", res.Reference)
	}
}

// Candle running menghentikan evaluasi: status belum final.
func TestC2IRLCandleRunningBerhenti(t *testing.T) {
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	fvg, cs := sibi4400(base)

	cs = append(cs,
		bangunCandle(base, 3, 4398, 4402, 4395, 4399), // C2 sah
	)
	running := bangunCandle(base, 4, 4399, 4410, 4394, 4408)
	running.IsClosed = false
	cs = append(cs, running)

	res := EvaluateC2IRL(fvg, cs)
	if res.Status != C2Aktif || res.CandleIndex != 3 {
		t.Fatalf("harapan C2 tetap di idx 3 (idx 4 running), dapat %s idx=%d", res.Status, res.CandleIndex)
	}
}
