package ila

import (
	"fmt"
	"math"
	"strings"
	"time"

	"ila-engine/pkg/indicators"
)

// Evaluator provides deterministic evaluation for the Imaginary Liquidity (ILA) method.
type Evaluator struct{}

var nyLocation *time.Location

func init() {
	var err error
	nyLocation, err = time.LoadLocation("America/New_York")
	if err != nil {
		nyLocation = time.FixedZone("EST", -5*3600)
	}
}

func formatCandleTime(t time.Time) string {
	if nyLocation != nil {
		return t.In(nyLocation).Format("15:04 NY")
	}
	return t.UTC().Add(-5 * time.Hour).Format("15:04 NY")
}

func formatCandleDateTime(t time.Time) string {
	if nyLocation != nil {
		return t.In(nyLocation).Format("02 Jan 15:04 NY")
	}
	return t.UTC().Add(-5 * time.Hour).Format("02 Jan 15:04 NY")
}

func formatILAVal(val float64) string {
	if val < 50.0 {
		return fmt.Sprintf("%.5f", val)
	}
	return fmt.Sprintf("%.2f", val)
}

// getFVGTolerance menentukan toleransi spread/noise sebelum sebuah ekor dianggap
// benar-benar menutup FVG. Tanpa toleransi, selisih mikroskopis quote antar broker
// sudah cukup untuk menghanguskan FVG yang secara riil belum tertutup.
//
// Ambang mengikuti skala harga karena satu angka tidak berlaku untuk semua instrumen.
// Kelas instrumen dikenali dari besaran harga, selaras dengan minSLFloorFor di
// pkg/sequence: pair JPY berharga ratusan dengan 3 desimal (1 pip = 0.01), sedangkan
// forex 5 desimal berharga satuan (1 pip = 0.0001). Memakai satu ambang "harga < 50"
// akan memberi pair JPY toleransi 50x lebih lebar dari seharusnya.
func getFVGTolerance(price float64) float64 {
	switch {
	case price >= 10000.0:
		return 2.00 // Indeks: 2.0 poin
	case price >= 400.0:
		return 0.50 // Gold / logam: $0.50
	case price >= 50.0:
		return 0.01 // Forex JPY (3 desimal): 1 pip
	default:
		return 0.00010 // Forex 5 desimal: 1 pip
	}
}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// umurMaksimalFVG adalah batas umur sebuah FVG, dihitung dalam candle HTF sejak
// Candle 3 pembentuknya sampai candle terakhir yang dievaluasi.
//
// FVG hanya berguna selama harga masih berurusan dengan zonanya. Semua FVG yang
// dipertimbangkan memang belum termitigasi, tetapi bisa saja harga sudah
// meninggalkannya jauh dan tidak pernah kembali. FVG seperti itu tetap "hidup"
// secara teknis, lalu dipakai sebagai acuan dan melahirkan entry dari struktur
// berhari-hari lalu.
//
// Enam candle dipilih dari pengukuran pada feed asli, bukan tebakan. Batas empat
// sempat dicoba dan membuang setup sah (EURUSD 12 Nov 2026 09:45 NY, FVG acuan
// baru berumur lima candle, entry SELL-nya berakhir TP). Mengukur dalam jumlah
// candle, bukan jam atau poin, membuat batas ini berlaku sama untuk semua
// timeframe dan semua pasangan tanpa angka khusus per simbol.
const umurMaksimalFVG = 6

// fvgMasihDijangkauHarga menjawab apakah harga masih berurusan dengan zona FVG.
//
// Ini syarat yang sama dengan definisi sweep itu sendiri: sebuah candle hanya
// bisa menyapu likuiditas di dalam FVG bila ia benar-benar masuk ke zona itu.
// Menguji sentuhan terhadap batas zona, bukan jarak poin, membuat syarat ini
// otomatis mengikuti skala tiap instrumen.
func fvgMasihDijangkauHarga(fvg FVG, candles []indicators.Candle) bool {
	tol := getFVGTolerance(fvg.Top)

	// Mulai dari candle SETELAH Candle 3, bukan dari Candle 3 itu sendiri.
	//
	// Candle 3 pembentuk FVG secara definisi berada di dalam zonanya (High-nya
	// adalah batas bawah FVG bearish, Low-nya batas atas FVG bullish). Menghitung
	// candle itu membuat setiap FVG yang baru lahir langsung dianggap "dijangkau
	// harga" tanpa satu pun candle setelahnya yang benar-benar menyentuh zona.
	for i := fvg.Candle3Index + 1; i < len(candles); i++ {
		c := candles[i]
		if fvg.Type == FVGBearish {
			// FVG bearish (SIBI) berada DI BAWAH candle displacement. Harga baru
			// bisa menyapu likuiditas di zona ini setelah naik masuk ke dalamnya,
			// yaitu saat High menembus batas bawah zona.
			if c.High >= fvg.Bottom-tol {
				return true
			}
			continue
		}
		// FVG bullish (BISI) berada DI ATAS candle displacement; harga
		// menyentuhnya saat turun menembus batas atas zona.
		if c.Low <= fvg.Top+tol {
			return true
		}
	}
	return false
}

// FindFVGs scans candle history and returns all 3-candle Fair Value Gaps.
func (e *Evaluator) FindFVGs(candles []indicators.Candle) []FVG {
	n := len(candles)
	if n < 3 {
		return nil
	}

	var fvgs []FVG

	for i := 2; i < n; i++ {
		c1 := candles[i-2]
		c2 := candles[i-1]
		c3 := candles[i]

		// FVG hanya sah dibentuk oleh 3 candle yang sudah CLOSED resmi
		if !c1.IsClosed || !c2.IsClosed || !c3.IsClosed {
			continue
		}

		// Bearish FVG: Low(C1) > High(C3) (C2 is a massive downward expansion candle)
		if c1.Low > c3.High && c2.Close < c2.Open {
			fvg := FVG{
				Type:             FVGBearish,
				Top:              c1.Low,
				Bottom:           c3.High,
				Candle1Index:     i - 2,
				Candle2Index:     i - 1,
				Candle3Index:     i,
				Candle1Time:      c1.Timestamp,
				Candle2Time:      c2.Timestamp,
				Candle3Time:      c3.Timestamp,
				InvalidatedIndex: -1,
			}

			tol := getFVGTolerance(fvg.Top)

			// Check if subsequently mitigated by candles k > i
			// Aturan Baku ILA (versi hermes): FVG termitigasi/hangus jika
			// 1. Ada candle closed dengan BODY menembus batas luar FVG (Close > Top), ATAU
			// 2. Ada candle dengan ekor (wick) yang menutup penuh FVG melampaui toleransi spread/noise (High > Top + tol).
			// Penusukan ekor dalam batas toleransi dengan body tertahan di dalam/bawah adalah rejection wick/sweep, BUKAN mitigasi.
			for k := i + 1; k < n; k++ {
				if candles[k].IsClosed && (candles[k].Close > fvg.Top || candles[k].High > fvg.Top+tol) {
					fvg.Mitigated = true
					break
				}
			}

			// INVALIDASI versi dokumen baku ILA 2.0: HANYA close yang menembus batas
			// atas zona. Berbeda dari Mitigated di atas yang memakai wick + toleransi.
			// Keduanya sengaja dipisah: Mitigated menyaring kandidat sweep (perilaku
			// live), Invalidated menutup FVG untuk evaluasi C2 Closure.
			for k := i + 1; k < n; k++ {
				if !candles[k].IsClosed {
					break
				}
				if candles[k].Close > fvg.Top {
					fvg.Invalidated = true
					fvg.InvalidatedIndex = k
					fvg.InvalidatedTime = candles[k].Timestamp
					break
				}
			}

			fvgs = append(fvgs, fvg)
		}

		// Bullish FVG: High(C1) < Low(C3) (C2 is a massive upward expansion candle)
		if c1.High < c3.Low && c2.Close > c2.Open {
			fvg := FVG{
				Type:             FVGBullish,
				Top:              c3.Low,
				Bottom:           c1.High,
				Candle1Index:     i - 2,
				Candle2Index:     i - 1,
				Candle3Index:     i,
				Candle1Time:      c1.Timestamp,
				Candle2Time:      c2.Timestamp,
				Candle3Time:      c3.Timestamp,
				InvalidatedIndex: -1,
			}

			tol := getFVGTolerance(fvg.Bottom)

			// Check if subsequently mitigated by candles k > i
			// Aturan Baku ILA (versi hermes): FVG termitigasi/hangus jika
			// 1. Ada candle closed dengan BODY menembus batas luar FVG (Close < Bottom), ATAU
			// 2. Ada candle dengan ekor (wick) yang menutup penuh FVG melampaui toleransi spread/noise (Low < Bottom - tol).
			for k := i + 1; k < n; k++ {
				if candles[k].IsClosed && (candles[k].Close < fvg.Bottom || candles[k].Low < fvg.Bottom-tol) {
					fvg.Mitigated = true
					break
				}
			}

			// INVALIDASI versi dokumen baku ILA 2.0: HANYA close yang menembus batas
			// bawah zona (cermin dari sisi bearish).
			for k := i + 1; k < n; k++ {
				if !candles[k].IsClosed {
					break
				}
				if candles[k].Close < fvg.Bottom {
					fvg.Invalidated = true
					fvg.InvalidatedIndex = k
					fvg.InvalidatedTime = candles[k].Timestamp
					break
				}
			}

			fvgs = append(fvgs, fvg)
		}
	}

	return fvgs
}

// DetectHTFIlaSweep checks if an HTF candle retraced into an active FVG and swept the wick of the prior candle.
// indexOfFVG mengembalikan posisi fvg di dalam daftar berdasarkan tiga indeks pembentuknya.
// Dipakai untuk mencari FVG sebelumnya secara kronologis tanpa bergantung pada status mitigasi.
// Mengembalikan -1 bila tidak ditemukan.
func indexOfFVG(fvgs []FVG, fvg FVG) int {
	for i, f := range fvgs {
		if f.Candle1Index == fvg.Candle1Index && f.Candle2Index == fvg.Candle2Index && f.Candle3Index == fvg.Candle3Index && f.Type == fvg.Type {
			return i
		}
	}
	return -1
}

func (e *Evaluator) DetectHTFIlaSweep(candles []indicators.Candle) *HTFSweep {
	n := len(candles)
	if n < 4 {
		return nil
	}

	fvgs := e.FindFVGs(candles)
	if len(fvgs) == 0 {
		return nil
	}

	// Strict ILA rule: Filter out fully mitigated FVGs, focus ONLY on the latest active UNMITIGATED FVG
	var activeFVGs []FVG
	for _, f := range fvgs {
		if !f.Mitigated {
			activeFVGs = append(activeFVGs, f)
		}
	}
	if len(activeFVGs) == 0 {
		return nil // Tidak ada FVG aktif yang valid (semua sudah termitigasi)
	}

	// Kunci pada FVG aktif TERBARU, yaitu yang candle pembentuknya paling akhir.
	//
	// Urutan daftar sudah kronologis, tetapi memilih "elemen terakhir" saja salah:
	// begitu FVG terbaru termitigasi, pilihannya melompat ke FVG aktif paling tua
	// yang masih tersisa. Yang dicari selalu yang paling muda.
	//
	// URUTAN INI DISENGAJA: pilih dulu, baru diuji syaratnya.
	//
	// Versi sebelumnya menyaring dulu (umur dan jarak) lalu memilih yang termuda
	// dari yang lolos. Akibatnya, bila FVG termuda gagal syarat, mesin diam-diam
	// mundur ke FVG yang lebih tua dan tetap membuka entry -- persis perilaku
	// "mengeksekusi FVG jadul" yang sedang dibereskan. Manusia membaca chart dari
	// struktur terakhir; bila struktur terakhir belum memenuhi syarat, jawabannya
	// adalah menunggu, bukan menggali struktur sebelumnya.
	latestFVG := activeFVGs[0]
	for _, f := range activeFVGs[1:] {
		if f.Candle3Index > latestFVG.Candle3Index {
			latestFVG = f
		}
	}

	// Batas umur FVG. Dihitung relatif (jumlah candle), jadi berlaku sama untuk
	// semua timeframe dan semua pasangan.
	idxTerakhir := n - 1
	if idxTerakhir-latestFVG.Candle3Index > umurMaksimalFVG {
		return nil // FVG termuda sudah terlalu tua; tidak mundur ke yang lebih tua
	}

	// FVG yang harga sudah lama meninggalkan zonanya bukan acuan yang jujur:
	// tidak ada likuiditas di sana yang bisa disapu.
	//
	// Bila FVG termuda tidak dijangkau harga, acuan diperiksa pada FVG lebih tua
	// DARI YANG TERMUDA ke belakang dan berhenti pada sweep sah pertama. Ini bukan
	// "menyisir FVG usang": penelusuran berhenti di struktur terdekat yang benar-
	// benar disapu pasar.
	//
	// Kasus nyata XAUUSD H4 12 Nov 2026: FVG bullish kecil 3886.93-3888.47
	// (lahir 10:00) tidak pernah disentuh lagi, sementara FVG bearish besar
	// 3887.64-3924.74 baru disapu candle 18:00 dan 22:00. Sweep bearish itu
	// yang benar, dan itulah yang dipakai.
	if !fvgMasihDijangkauHarga(latestFVG, candles) {
		return e.cariSweepPadaFVGLebihTua(candles, fvgs, activeFVGs, latestFVG, idxTerakhir)
	}

	// Deteksi apakah terjadi Double / Stacked FVG (displacements berturut-turut pada arah yang sama).
	// Pada kondisi Double FVG, Candle 3 dari FVG terbaru adalah batas/lantai FVG yang sah disapu
	// oleh candle berikutnya saat melakukan retracement masuk ke area FVG.
	//
	// Catatan: pembanding harus FVG kronologis tepat sebelum `latestFVG` (diambil dari
	// `fvgs`, termasuk yang sudah termitigasi). Bila memakai `activeFVGs`, FVG yang terlewat
	// karena termitigasi membuat pembanding melompat ke FVG yang jauh lebih tua dan dua FVG
	// yang sebenarnya tidak berurutan bisa salah dianggap Double FVG.
	//
	// Syarat ketat: FVG sebelumnya WAJIB masih aktif (belum termitigasi) DAN pembentuknya
	// benar-benar bersambung. FVG yang sudah termitigasi berarti harganya sudah melewati area
	// tersebut, sehingga Candle 3-nya tidak lagi layak dijadikan lantai sweep yang sah.
	isDoubleFVG := false
	if idx := indexOfFVG(fvgs, latestFVG); idx > 0 {
		prevFVG := fvgs[idx-1]
		consecutive := latestFVG.Candle1Index == prevFVG.Candle3Index ||
			latestFVG.Candle2Index-prevFVG.Candle2Index == 1
		if prevFVG.Type == latestFVG.Type && !prevFVG.Mitigated && consecutive {
			isDoubleFVG = true
		}
	}

	// ILA 2.0 menelusuri MAKSIMAL DUA FVG aktif: FVG terbaru lebih dulu, lalu FVG
	// kronologis tepat sebelumnya. Alasannya, satu candle HTF bisa sekaligus menutup
	// sweep sah pada FVG lama sambil membentuk FVG baru di arah berlawanan. Kasus nyata
	// XAUUSD H4: candle #58 menyapu ekor #57 di dalam FVG bearish 3887.64-3924.74,
	// tetapi candle yang sama juga membentuk FVG bullish 3886.93-3888.47. Bila hanya
	// FVG terbaru yang diperiksa, sweep bearish yang sah ikut terbuang.
	//
	// Aturan pemilihan: sweep TERLAMA yang sah menang ("sell yang terjadi lebih dulu
	// dieksekusi, buy-nya dilewati"). Batas penelusuran tetap dua FVG agar mesin tidak
	// mundur ke struktur usang.
	kandidat := []struct {
		fvg         FVG
		isDoubleFVG bool
	}{{fvg: latestFVG, isDoubleFVG: isDoubleFVG}}

	// FVG sebelumnya diambil dari daftar aktif (belum termitigasi) yang lebih tua
	// dari latestFVG. FVG yang sudah termitigasi berarti harga telah melewati areanya,
	// sehingga Candle 3-nya tidak lagi layak dijadikan acuan sweep.
	//
	// Dua FVG sah berbagi satu candle: Candle 3 FVG lama sekaligus Candle 1 FVG baru
	// (displacement bersambung). Karena itu FVG sebelumnya diterima bila Candle 3-nya
	// tidak melewati Candle 1 FVG terbaru (Candle3Index <= latest.Candle1Index),
	// bukan harus lebih kecil secara ketat.
	for i := len(activeFVGs) - 2; i >= 0 && len(kandidat) < 2; i-- {
		fvg := activeFVGs[i]
		if fvg.Candle3Index > latestFVG.Candle1Index {
			continue // bukan FVG sebelumnya secara kronologis
		}
		// Status Double FVG hanya berlaku untuk pasangan FVG terbaru; FVG lain
		// diperlakukan sebagai FVG tunggal agar aturan Candle 3 tetap ketat.
		kandidat = append(kandidat, struct {
			fvg         FVG
			isDoubleFVG bool
		}{fvg: fvg, isDoubleFVG: false})
	}

	// Kumpulkan sweep sah dari setiap kandidat, lalu pilih yang sweep-nya paling awal.
	// Sweep yang masih berjalan (candle belum closed) diperlakukan sama seperti sebelumnya:
	// dianggap kandidat terkuat karena konfirmasi belum final.
	var terpilih *HTFSweep
	var belumAdaSweep *HTFSweep
	for i := range kandidat {
		sweep := e.detectSweepPadaFVG(candles, fvgs, kandidat[i].fvg, kandidat[i].isDoubleFVG)
		if sweep == nil {
			continue
		}
		if !sweep.Detected && !sweep.SweepInProgress {
			// Tidak ada sweep pada FVG ini; simpan sebagai acuan arah bila kandidat terbaru.
			if i == 0 && belumAdaSweep == nil {
				belumAdaSweep = sweep
			}
			continue
		}
		if terpilih == nil || sweep.SweepTime.Before(terpilih.SweepTime) {
			terpilih = sweep
		}
	}
	if terpilih != nil {
		return terpilih
	}
	if belumAdaSweep != nil {
		return belumAdaSweep
	}

	// Jika belum ada sweep yang terbentuk, gunakan latestFVG sebagai acuan arah
	dirStr := "BEARISH"
	detailStr := fmt.Sprintf("FVG terakhir adalah Bearish (%s - %s). Menunggu candle masuk dan menyapu High wick candle sebelumnya.", formatILAVal(latestFVG.Bottom), formatILAVal(latestFVG.Top))
	if latestFVG.Type == FVGBullish {
		dirStr = "BULLISH"
		detailStr = fmt.Sprintf("FVG terakhir adalah Bullish (%s - %s). Menunggu candle masuk dan menyapu Low wick candle sebelumnya.", formatILAVal(latestFVG.Bottom), formatILAVal(latestFVG.Top))
	}

	return &HTFSweep{
		Detected:  false,
		Direction: dirStr,
		FVG:       &latestFVG,
		Details:   detailStr,
	}
}

// cariSweepPadaFVGLebihTua menangani satu keadaan khusus: FVG aktif termuda
// sudah tidak dijangkau harga, sehingga tidak mungkin jadi acuan sweep.
//
// Dalam keadaan itu acuan diperiksa pada FVG aktif yang lebih tua, DARI YANG
// PALING MUDA ke belakang, dan berhenti pada FVG pertama yang sweep-nya sah.
// Satu FVG lebih tua boleh jadi acuan hanya bila sweep-nya memang terjadi saat
// atau setelah FVG itu lahir (Candle 3) dan masih dalam batas umur.
//
// Mengembalikan nil bila tak satu pun menghasilkan sweep sah. Sengaja tidak
// mengembalikan "acuan arah" dari FVG tertua: itu yang dulu membuat evaluator
// menelusuri candle sangat jauh ke belakang mencari sweep yang sudah usang.
func (e *Evaluator) cariSweepPadaFVGLebihTua(
	candles []indicators.Candle,
	fvgs []FVG,
	activeFVGs []FVG,
	fvgTermuda FVG,
	idxTerakhir int,
) *HTFSweep {
	for i := len(activeFVGs) - 1; i >= 0; i-- {
		fvgLebihTua := activeFVGs[i]
		if fvgLebihTua.Candle3Index >= fvgTermuda.Candle3Index {
			continue // kandidat utama sendiri atau lebih baru
		}
		if idxTerakhir-fvgLebihTua.Candle3Index > umurMaksimalFVG {
			continue // terlalu tua untuk dijadikan acuan
		}

		isDoubleFVG := false
		if idx := indexOfFVG(fvgs, fvgLebihTua); idx > 0 {
			prevFVG := fvgs[idx-1]
			consecutive := fvgLebihTua.Candle1Index == prevFVG.Candle3Index ||
				fvgLebihTua.Candle2Index-prevFVG.Candle2Index == 1
			if prevFVG.Type == fvgLebihTua.Type && !prevFVG.Mitigated && consecutive {
				isDoubleFVG = true
			}
		}

		sweep := e.detectSweepPadaFVG(candles, fvgs, fvgLebihTua, isDoubleFVG)
		if sweep != nil && sweep.Detected {
			return sweep
		}
	}
	return nil
}

// detectSweepPadaFVG menjalankan aturan sweep ILA 2.0 untuk SATU FVG acuan,
// lalu menempelkan status C2 Closure sebagai informasi pelengkap.
//
// Pembagian tugasnya sengaja tegas: detectSweepPadaFVGInti memutuskan sweep
// (Detected / Direction / StopLoss / TargetDOL) persis seperti sebelumnya, dan
// wrapper ini HANYA menambah field C2Status/C2Detail. Tidak ada satu pun
// keputusan entry yang dibaca dari C2, sehingga perilaku live tidak berubah.
func (e *Evaluator) detectSweepPadaFVG(candles []indicators.Candle, fvgs []FVG, fvg FVG, isDoubleFVG bool) *HTFSweep {
	sweep := e.detectSweepPadaFVGInti(candles, fvgs, fvg, isDoubleFVG)
	if sweep == nil {
		return nil
	}
	c2 := EvaluateC2IRL(fvg, candles)
	sweep.C2Status = c2.Status
	sweep.C2Detail = c2.Detail
	return sweep
}

// detectSweepPadaFVGInti adalah logika sweep asli tanpa perubahan perilaku.
func (e *Evaluator) detectSweepPadaFVGInti(candles []indicators.Candle, fvgs []FVG, fvg FVG, isDoubleFVG bool) *HTFSweep {
	n := len(candles)
	latestFVG := fvg

	// isDoubleFVG sudah ditentukan pemanggil (DetectHTFIlaSweep / cariSweepPadaFVGLebihTua)
	// dari daftar FVG kronologis. Tidak dihitung ulang di sini.

	for k := n - 1; k > latestFVG.Candle2Index; k-- {
		// 1. PENCEGAHAN SWEEP ZOMBIE:
		// Sweep hanya relevan jika terjadi di:
		// - Candle running saat ini (k == n-1: SweepInProgress)
		// - Candle yang baru closed (k == n-2: aktif di Candle 5 mencari CISD)
		// - Candle n-3 (Candle 5 baru saja closed tanpa entry -> dievaluasi EXPIRED di EvaluateILA)
		// Jika k < n-3, artinya sudah lewat lebih dari 1 candle setelah Candle 5 tutup -> sweep zombie/usang.
		if k < n-3 {
			continue
		}

		// 2. CANDLE ACUAN HARUS ADA:
		// Candle k butuh minimal satu candle setelah candle 3 FVG untuk diuji
		// sebagai acuan. Dokumen baku TIDAK mensyaratkan candle k-1 harus sudah
		// berada di dalam zona; yang diuji adalah LEVEL ACUAN FVG, dan acuan itu
		// sudah tersedia begitu candle 3 tertutup.
		//
		// Syarat lama "k-1 == Candle3Index && !isDoubleFVG -> tolak" dihapus sesuai
		// dokumen: pada XAUUSD H4 28 Aug, candle setelah candle 3 langsung menyapu
		// acuan dengan sah (pasar tutup Jumat sehingga tidak ada candle di antara),
		// dan dokumen menyatakan setup itu valid.
		if k-1 < latestFVG.Candle3Index {
			continue
		}

		ck := candles[k]
		cPrev := candles[k-1] // Bersebelahan langsung (adjacent)

		// LEVEL ACUAN menurut dokumen baku ILA 2.0 (c2-closure-irl.md).
		//
		// Dokumen menguji sapuan terhadap LEVEL ACUAN FVG, bukan terhadap high/low
		// candle tepat sebelumnya. Acuan itu BERGESER setiap kali sebuah candle
		// menyentuhnya lalu close melewatinya tetapi masih di dalam zona
		// (Urutan 4), dan berpindah ke high/low candle C2 terakhir (Urutan 3).
		//
		// Dihitung pada irisan candle sampai k-1 saja: candle k sendiri adalah
		// candle yang sedang diuji, sehingga status C2-nya belum boleh dipakai.
		acuan := referenceAwalIRL(latestFVG, candles)
		for s := latestFVG.Candle3Index + 1; s < k; s++ {
			cs := candles[s]
			if !cs.IsClosed {
				break
			}
			if latestFVG.Type == FVGBearish {
				if cs.High < acuan {
					continue // tidak menyentuh acuan, diabaikan
				}
				// Close di bawah acuan -> C2; acuan pindah ke high candle itu.
				// Close di atas acuan namun masih di dalam zona -> C2 gugur,
				// acuan tetap bergeser ke high candle itu (Urutan 4).
				acuan = cs.High
				continue
			}
			if cs.Low > acuan {
				continue
			}
			acuan = cs.Low
		}

		// 0. Cek apakah FVG sudah tertembus penuh (Body Breakout atau Wick menembus
		// melampaui toleransi). Versi hermes dipertahankan: body close di luar zona
		// ATAU wick menembus jauh melampaui toleransi spread.
		tol := getFVGTolerance(latestFVG.Top)
		if latestFVG.Type == FVGBearish && ck.IsClosed && (ck.Close > latestFVG.Top || ck.High > latestFVG.Top+tol) {
			return nil // Bearish FVG sudah tertembus penuh by body / wick tembus jauh
		}
		if latestFVG.Type == FVGBullish && ck.IsClosed && (ck.Close < latestFVG.Bottom || ck.Low < latestFVG.Bottom-tol) {
			return nil // Bullish FVG sudah tertembus penuh by body / wick tembus jauh
		}

		// BEARISH ILA SWEEP (definisi C2 Closure):
		// - latestFVG is Bearish
		// - ck enters FVG: ck.High >= latestFVG.Bottom
		// - ck menyapu LEVEL ACUAN: ck.High > acuan
		// - ck does NOT close above FVG top (displacement holds): ck.Close <= latestFVG.Top
		// - ck ditolak: close kembali di bawah acuan
		//
		// Pembanding adalah `acuan` (level FVG yang bergeser), bukan `cPrev.High`.
		// Syarat "candle sebelumnya wajib sudah di dalam zona" dihapus karena
		// dokumen tidak memintanya. Kedua hal ini yang membuat setup sah seperti
		// XAUUSD H4 11 Sep ditolak sistem lama.
		//
		// WICK SWEEP ONLY tetap diberlakukan: body ck tidak boleh menembus ke atas
		// acuan, cukup ekornya yang menusuk lalu ditolak (ck.Close < acuan).
		if latestFVG.Type == FVGBearish && ck.High >= latestFVG.Bottom && ck.High > acuan && ck.Close <= latestFVG.Top && ck.Close < acuan {
			ssl := latestFVG.Bottom
			for s := latestFVG.Candle2Index; s < k; s++ {
				if candles[s].Low < ssl {
					ssl = candles[s].Low
				}
			}

			// VALIDASI SIKLUS AKTIF (Aturan Baku ILA):
			// Sweep tetap sah dan aktif selama:
			// 1. Belum ada candle berikutnya yang menembus ekor sweep (ck.High)
			// 2. FVG belum tertembus melampaui batas toleransi (Close > Top atau High > Top + tol)
			// Catatan: Jika harga sudah menembus target SSL lebih dulu sebelum CISD terbentuk,
			// setup TIDAK dibatalkan, melainkan diberlakukan "Skenario 3 DOL Breached" (kunci TP ke 1:2.00 R:R).
			invalidated := false
			for post := k + 1; post < n; post++ {
				if candles[post].High > ck.High || (candles[post].IsClosed && (candles[post].Close > latestFVG.Top || candles[post].High > latestFVG.Top+tol)) {
					invalidated = true
					break
				}
			}
			if invalidated {
				continue
			}

			if !ck.IsClosed {
				return &HTFSweep{
					Detected:         false,
					SweepInProgress:  true,
					Direction:        "BEARISH",
					FVG:              &latestFVG,
					SweepCandle:      ck,
					SweptCandle:      cPrev,
					SweepCandleIndex: k,
					SweepTime:        ck.Timestamp,
					StopLoss:         ck.High,
					TargetDOL:        ssl,
					Details: fmt.Sprintf("Candle %s sedang melakukan sweep wick di Bearish FVG (belum ganti candle / masih running). Menunggu candle HTF closed resmi untuk cari entry di candle berikutnya!",
						formatCandleDateTime(ck.Timestamp)),
				}
			}

			return &HTFSweep{
				Detected:         true,
				Direction:        "BEARISH",
				FVG:              &latestFVG,
				SweepCandle:      ck,
				SweptCandle:      cPrev,
				SweepCandleIndex: k,
				SweepTime:        ck.Timestamp,
				StopLoss:         ck.High,
				TargetDOL:        ssl,
				Details: fmt.Sprintf("Candle %s masuk ke Bearish FVG Terakhir (%s - %s) dan menyapu High wick candle sebelumnya (%s > %s). Target DOL: %s (SSL).",
					formatCandleDateTime(ck.Timestamp), formatILAVal(latestFVG.Bottom), formatILAVal(latestFVG.Top), formatILAVal(ck.High), formatILAVal(cPrev.High), formatILAVal(ssl)),
			}
		}

		// BULLISH ILA SWEEP (definisi C2 Closure, cermin dari sisi bearish):
		// - latestFVG is Bullish
		// - ck enters FVG: ck.Low <= latestFVG.Top
		// - ck menyapu LEVEL ACUAN: ck.Low < acuan
		// - ck does NOT close below FVG bottom: ck.Close >= latestFVG.Bottom
		// - ck ditolak: close kembali di atas acuan
		//
		// WICK SWEEP ONLY tetap diberlakukan: body ck tidak boleh menembus ke bawah
		// acuan, cukup ekornya yang menusuk lalu ditolak (ck.Close > acuan).
		if latestFVG.Type == FVGBullish && ck.Low <= latestFVG.Top && ck.Low < acuan && ck.Close >= latestFVG.Bottom && ck.Close > acuan {
			bsl := latestFVG.Top
			for s := latestFVG.Candle2Index; s < k; s++ {
				if candles[s].High > bsl {
					bsl = candles[s].High
				}
			}

			// VALIDASI SIKLUS AKTIF (Aturan Baku ILA):
			invalidated := false
			for post := k + 1; post < n; post++ {
				if candles[post].Low < ck.Low || (candles[post].IsClosed && (candles[post].Close < latestFVG.Bottom || candles[post].Low < latestFVG.Bottom-tol)) {
					invalidated = true
					break
				}
			}
			if invalidated {
				continue
			}

			if !ck.IsClosed {
				return &HTFSweep{
					Detected:         false,
					SweepInProgress:  true,
					Direction:        "BULLISH",
					FVG:              &latestFVG,
					SweepCandle:      ck,
					SweptCandle:      cPrev,
					SweepCandleIndex: k,
					SweepTime:        ck.Timestamp,
					StopLoss:         ck.Low,
					TargetDOL:        bsl,
					Details: fmt.Sprintf("Candle %s sedang melakukan sweep wick di Bullish FVG (belum ganti candle / masih running). Menunggu candle HTF closed resmi untuk cari entry di candle berikutnya!",
						formatCandleDateTime(ck.Timestamp)),
				}
			}

			return &HTFSweep{
				Detected:         true,
				Direction:        "BULLISH",
				FVG:              &latestFVG,
				SweepCandle:      ck,
				SweptCandle:      cPrev,
				SweepCandleIndex: k,
				SweepTime:        ck.Timestamp,
				StopLoss:         ck.Low,
				TargetDOL:        bsl,
				Details: fmt.Sprintf("Candle %s masuk ke Bullish FVG Terakhir (%s - %s) dan menyapu Low wick candle sebelumnya (%s < %s). Target DOL: %s (BSL).",
					formatCandleDateTime(ck.Timestamp), formatILAVal(latestFVG.Bottom), formatILAVal(latestFVG.Top), formatILAVal(ck.Low), formatILAVal(cPrev.Low), formatILAVal(bsl)),
			}
		}
	}

	// Jika belum ada sweep yang terbentuk, gunakan latestFVG sebagai acuan arah
	dirStr := "BEARISH"
	detailStr := fmt.Sprintf("FVG terakhir adalah Bearish (%s - %s). Menunggu candle masuk dan menyapu High wick candle sebelumnya.", formatILAVal(latestFVG.Bottom), formatILAVal(latestFVG.Top))
	if latestFVG.Type == FVGBullish {
		dirStr = "BULLISH"
		detailStr = fmt.Sprintf("FVG terakhir adalah Bullish (%s - %s). Menunggu candle masuk dan menyapu Low wick candle sebelumnya.", formatILAVal(latestFVG.Bottom), formatILAVal(latestFVG.Top))
	}

	return &HTFSweep{
		Detected:  false,
		Direction: dirStr,
		FVG:       &latestFVG,
		Details:   detailStr,
	}
}

// DetectCISD scans LTF candles for Change In State of Delivery (body breaks body).
// Strictly using the first candle of the consecutive opposing run tracing backwards from the extreme of Candle 5.
func (e *Evaluator) DetectCISD(candles []indicators.Candle, direction string, afterTime time.Time) *CISD {
	cisd, _ := e.DetectDynamicCISD(candles, direction, afterTime, indicators.Candle{})
	return cisd
}

// DetectDynamicCISD evaluates the dynamic consecutive leg CISD:
// Tracing backwards from the extreme of Candle 5 to the origin (first candle) of the opposing leg.
func (e *Evaluator) DetectDynamicCISD(candles []indicators.Candle, direction string, afterTime time.Time, sweepCandle indicators.Candle) (*CISD, bool) {
	n := len(candles)
	if n < 2 {
		return nil, false
	}

	if direction == "BULLISH" {
		// 1. Cari candle terendah (lowest Low) di Candle 5 (c.Timestamp >= afterTime)
		lowestIdx := -1
		lowestVal := math.MaxFloat64

		for i := 0; i < n; i++ {
			c := candles[i]
			if !afterTime.IsZero() && c.Timestamp.Before(afterTime) {
				continue
			}
			if c.Low < lowestVal {
				lowestVal = c.Low
				lowestIdx = i
			}
		}

		if lowestIdx == -1 {
			lowestIdx = n - 1
			lowestVal = candles[lowestIdx].Low
		}

		// 2. Invalidation Check: Apakah low di Candle 5 turun terlalu jauh menembus wick Candle 4?
		if sweepCandle.Low > 0 && lowestVal < sweepCandle.Low {
			return nil, true // Invalidated!
		}

		// 3. Telusuri MUNDUR dari lowestIdx untuk mencari candle bearish PERTAMA dari deretan penurunan ini.
		// SYARAT KETAT: lowestIdx wajib terjadi di dalam Candle 5 (Timestamp >= afterTime).
		// Batas bawah penelusuran origin memakai awal lilin sweep HTF (Candle 4), BUKAN awal Candle 5.
		// Deretan candle lawan yang bersambung langsung menuju titik ekstrim boleh berawal di Candle 4;
		// bila dipotong di batas Candle 5, origin yang benar ikut terbuang dan level CISD bergeser
		// lebih tinggi sehingga entry terjadi terlalu dini. Penelusuran tetap berhenti di lilin sweep:
		// tidak boleh menyeberang ke siklus sebelumnya.
		// Bila sweepCandle kosong (pemanggil lama), batas ini tidak memasang potongan buatan.
		if !afterTime.IsZero() && candles[lowestIdx].Timestamp.Before(afterTime) {
			return nil, false // Low belum terbentuk di dalam Candle 5
		}

		// 3. Telusuri MUNDUR dari lowestIdx untuk mencari candle bearish PERTAMA dari deretan penurunan ini.
		// Aturan Baku: Deretan candle lawan (opposing consecutive run) yang bersambung langsung
		// menuju titik ekstrim diizinkan berawal dari candle sebelumnya (di dalam siklus sweep Candle 4),
		// asalkan deretan tersebut murni bersambung tanpa terputus oleh candle lawan.
		curr := lowestIdx
		if candles[curr].Close > candles[curr].Open && curr > 0 {
			curr--
		}

		var firstBear *indicators.Candle
		for curr >= 0 && candles[curr].Close < candles[curr].Open {
			if !sweepCandle.Timestamp.IsZero() && candles[curr].Timestamp.Before(sweepCandle.Timestamp) {
				break // Tidak menyeberang sebelum lilin sweep HTF (Candle 4)
			}
			firstBear = &candles[curr]
			curr--
		}

		if firstBear == nil {
			// Belum ada deretan candle bearish yang bersambung menuju ekstrem.
			//
			// Dulu ada blok cadangan di sini yang mengambil candle bearish PERTAMA
			// yang ditemui saat menelusuri mundur. Blok itu keliru karena bisa
			// meminjam candle dari Candle 4 lalu menjadikan candle ekstrem sendiri
			// sebagai penembusnya, sehingga lahir CISD dari deretan yang tidak ada.
			// Kasus nyata: USTEC 9 Okt 2026 21:00 NY, entry SELL palsu SL 8,7 poin.
			// Tanpa deretan, memang belum ada CISD.
			return nil, false
		}

		cisdLevel := firstBear.Open
		cisd := &CISD{
			Detected:  false,
			Direction: "BULLISH",
			RefCandle: *firstBear,
			CISDLevel: cisdLevel,
			ExtremeSL: lowestVal,
			Details: fmt.Sprintf("Origin penurunan di Candle %s (Open %s). Menunggu candle ditutup > %s by body (SL: %s).",
				formatCandleTime(firstBear.Timestamp), formatILAVal(cisdLevel), formatILAVal(cisdLevel), formatILAVal(lowestVal)),
		}

		// 4. Cari apakah ada candle closed setelah lowestIdx yang menembus cisdLevel by body
		for k := lowestIdx; k < n; k++ {
			c := candles[k]
			if c.IsClosed && c.Close > cisdLevel {
				cisd.Detected = true
				cisd.BreakCandle = c
				cisd.BreakTime = c.Timestamp
				cisd.EntryPrice = c.Close
				cisd.Details = fmt.Sprintf("Bullish CISD Valid: Candle %s (Close %s) resmi menembus origin bearish %s (Open %s) by body! SL di %s.",
					formatCandleTime(c.Timestamp), formatILAVal(c.Close), formatCandleTime(firstBear.Timestamp), formatILAVal(cisdLevel), formatILAVal(lowestVal))
				return cisd, false
			}
		}

		return cisd, false

	} else if direction == "BEARISH" {
		// 1. Cari candle tertinggi (highest High) di Candle 5 (c.Timestamp >= afterTime)
		highestIdx := -1
		highestVal := -math.MaxFloat64

		for i := 0; i < n; i++ {
			c := candles[i]
			if !afterTime.IsZero() && c.Timestamp.Before(afterTime) {
				continue
			}
			if c.High > highestVal {
				highestVal = c.High
				highestIdx = i
			}
		}

		if highestIdx == -1 {
			highestIdx = n - 1
			highestVal = candles[highestIdx].High
		}

		// 2. Invalidation Check: Apakah high di Candle 5 menembus wick Candle 4?
		if sweepCandle.High > 0 && highestVal > sweepCandle.High {
			return nil, true // Invalidated!
		}

		// 3. Telusuri MUNDUR dari highestIdx untuk mencari candle bullish PERTAMA dari deretan kenaikan ini.
		// SYARAT KETAT: highestIdx wajib terjadi di dalam Candle 5 (Timestamp >= afterTime).
		// Batas bawah penelusuran origin memakai awal lilin sweep HTF (Candle 4), BUKAN awal Candle 5.
		// Deretan candle lawan yang bersambung langsung menuju titik ekstrim boleh berawal di Candle 4;
		// bila dipotong di batas Candle 5, origin yang benar ikut terbuang dan level CISD bergeser
		// lebih rendah sehingga entry terjadi terlalu dini. Penelusuran tetap berhenti di lilin sweep:
		// tidak boleh menyeberang ke siklus sebelumnya.
		// Bila sweepCandle kosong (pemanggil lama), batas ini tidak memasang potongan buatan.
		if !afterTime.IsZero() && candles[highestIdx].Timestamp.Before(afterTime) {
			return nil, false // High belum terbentuk di dalam Candle 5
		}

		// 3. Telusuri MUNDUR dari highestIdx untuk mencari candle bullish PERTAMA dari deretan kenaikan ini.
		// Aturan Baku: Deretan candle lawan (opposing consecutive run) yang bersambung langsung
		// menuju titik ekstrim diizinkan berawal dari candle sebelumnya (di dalam siklus sweep Candle 4),
		// asalkan deretan tersebut murni bersambung tanpa terputus oleh candle lawan.
		curr := highestIdx
		if candles[curr].Close < candles[curr].Open && curr > 0 {
			curr--
		}

		var firstBull *indicators.Candle
		for curr >= 0 && candles[curr].Close > candles[curr].Open {
			if !sweepCandle.Timestamp.IsZero() && candles[curr].Timestamp.Before(sweepCandle.Timestamp) {
				break // Tidak menyeberang sebelum lilin sweep HTF (Candle 4)
			}
			firstBull = &candles[curr]
			curr--
		}

		if firstBull == nil {
			// Belum ada deretan candle bullish yang bersambung menuju ekstrem.
			//
			// Dulu ada blok cadangan di sini yang mengambil candle bullish PERTAMA
			// yang ditemui saat menelusuri mundur. Blok itu keliru karena bisa
			// meminjam candle dari Candle 4 lalu menjadikan candle ekstrem sendiri
			// sebagai penembusnya, sehingga lahir CISD dari deretan yang tidak ada.
			// Tanpa deretan, memang belum ada CISD.
			return nil, false
		}

		cisdLevel := firstBull.Open
		cisd := &CISD{
			Detected:  false,
			Direction: "BEARISH",
			RefCandle: *firstBull,
			CISDLevel: cisdLevel,
			ExtremeSL: highestVal,
			Details: fmt.Sprintf("Origin kenaikan di Candle %s (Open %s). Menunggu candle ditutup < %s by body (SL: %s).",
				formatCandleTime(firstBull.Timestamp), formatILAVal(cisdLevel), formatILAVal(cisdLevel), formatILAVal(highestVal)),
		}

		// 4. Cari apakah ada candle closed setelah highestIdx yang menembus cisdLevel by body
		for k := highestIdx; k < n; k++ {
			c := candles[k]
			if c.IsClosed && c.Close < cisdLevel {
				cisd.Detected = true
				cisd.BreakCandle = c
				cisd.BreakTime = c.Timestamp
				cisd.EntryPrice = c.Close
				cisd.Details = fmt.Sprintf("Bearish CISD Valid: Candle %s (Close %s) resmi menembus origin bullish %s (Open %s) by body! SL di %s.",
					formatCandleTime(c.Timestamp), formatILAVal(c.Close), formatCandleTime(firstBull.Timestamp), formatILAVal(cisdLevel), formatILAVal(highestVal))
				return cisd, false
			}
		}

		return cisd, false
	}

	return nil, false
}

func timeframeDuration(tf string) time.Duration {
	switch strings.ToUpper(strings.TrimSpace(tf)) {
	case "M1", "1":
		return 1 * time.Minute
	case "M5", "5":
		return 5 * time.Minute
	case "M15", "15":
		return 15 * time.Minute
	case "M30", "30":
		return 30 * time.Minute
	case "H1", "60", "60M":
		return 1 * time.Hour
	case "H4", "240", "240M":
		return 4 * time.Hour
	case "D1", "D":
		return 24 * time.Hour
	case "W", "W1", "1W":
		return 7 * 24 * time.Hour
	default:
		return 4 * time.Hour
	}
}

// EvaluateILA executes the full paired multi-timeframe ILA logic.
func (e *Evaluator) EvaluateILA(htfCandles, ltfCandles []indicators.Candle, htfName, ltfName, symbol string) *ILASetup {
	setup := &ILASetup{
		Symbol:      symbol,
		HTF:         htfName,
		LTF:         ltfName,
		Bias:        "NEUTRAL",
		Status:      "WAIT_HTF_SWEEP",
		EvaluatedAt: time.Now().UTC(),
		// Bias ERL bersifat informasi (Swing High/Low 3 candle). Tidak dipakai
		// memutuskan status maupun entry; lihat catatan di ERLBiasOf.
		ERLBias: ERLBiasOf(htfCandles),
	}

	// 1. Evaluate HTF
	sweep := e.DetectHTFIlaSweep(htfCandles)
	if sweep == nil {
		setup.NextAction = fmt.Sprintf("⏳ Menunggu pembentukan FVG baru di timeframe %s.", htfName)
		setup.Summary = fmt.Sprintf("Belum ditemukan formasi FVG aktif di timeframe %s.", htfName)
		return setup
	}

	setup.HTFSweep = sweep
	setup.Bias = sweep.Direction

	if sweep.SweepInProgress {
		setup.Status = "WAIT_HTF_CANDLE_CLOSE"
		setup.NextAction = fmt.Sprintf("⏳ Candle sweep di %s masih berjalan (belum ganti candle). Menunggu candle %s closed resmi untuk cari entry di candle berikutnya!", htfName, htfName)
		setup.Summary = fmt.Sprintf("Timeframe %s: %s", htfName, sweep.Details)
		return setup
	}

	if !sweep.Detected {
		setup.Status = "WAIT_HTF_SWEEP"
		setup.NextAction = fmt.Sprintf("⏳ %s", sweep.Details)
		setup.Summary = fmt.Sprintf("Timeframe %s: %s", htfName, sweep.Details)
		return setup
	}

	// 2. Batasan Jendela Waktu Eksekusi Dinamis (Candle 5 & Candle 6):
	//
	// Semula jendela ditutup seketika Candle 5 berganti ke Candle 6. Itu mematikan setup
	// di detik terakhirnya: pada EURUSD H4-M15, candle M15 08:45 NY ditutup 09:00 NY dan
	// menembus origin CISD by body, tetapi feed H4 pada detik 09:00:00 NY sudah memunculkan
	// Candle 6 running, sehingga setup dianggap kadaluarsa tepat saat entry seharusnya sah.
	//
	// Aturan dinamis:
	// - Jika Candle 6 melakukan sweep baru di dalam FVG, DetectHTFIlaSweep sudah
	//   mengembalikan SweepInProgress (WAIT_HTF_CANDLE_CLOSE) sebelum sampai ke sini.
	// - Jika Candle 6 TIDAK melakukan sweep baru, jendela pencarian CISD tetap DIBUKA
	//   selama Candle 6 masih berjalan.
	// - Khusus kasus Candle 6: Stop Loss dikunci pada titik ekstrim High/Low Candle 5
	//   (lebih hemat & presisi).
	// - Kadaluarsa mutlak baru terjadi ketika Candle 6 sudah CLOSED resmi tanpa CISD
	//   (masuk Candle 7, lastHTFIdx > candle5Idx + 1).
	lastHTFIdx := len(htfCandles) - 1
	sweepIdx := sweep.SweepCandleIndex
	candle5Idx := sweepIdx + 1

	// KASUS 2: jendela eksekusi sudah lewat (Candle 6 closed / Candle 7 dan seterusnya)
	if lastHTFIdx > candle5Idx+1 || (lastHTFIdx == candle5Idx+1 && htfCandles[lastHTFIdx].IsClosed) {
		setup.Status = "WAIT_HTF_SWEEP"
		setup.NextAction = fmt.Sprintf("⏳ Setup sweep di %s telah kadaluarsa (melewati batas toleransi Candle 6 tanpa CISD). Menunggu pembentukan sweep baru.", htfName)
		setup.Summary = fmt.Sprintf("Siklus sweep %s hangus. Harga telah melewati jendela Candle 6.", htfName)
		return setup
	}

	// KASUS 1: aktif di Candle 5, atau lanjut di Candle 6 yang masih berjalan.
	// Jendela pencarian CISD di LTF dimulai dari saat candle sweep HTF (Candle 4) resmi CLOSED
	// (yaitu sweep.SweepCandle.Timestamp + htfDur) hingga candle yang sedang berjalan.
	// Stop Loss struktural default adalah ekor ekstrem dari Candle Sweep (Candle 4).
	htfDur := timeframeDuration(htfName)
	entryCandleStart := sweep.SweepCandle.Timestamp.Add(htfDur)
	setup.StopLoss = sweep.StopLoss
	setup.TakeProfit = sweep.TargetDOL

	// Khusus kasus Candle 6 running (lastHTFIdx == candle5Idx + 1):
	// CISD yang dicari justru dibentuk oleh ekstrem Candle 5 (ExtremeSL di
	// DetectDynamicCISD = lowest Low / highest High Candle 5). Karena itu SL dikunci
	// pada High Candle 5 (SELL) / Low Candle 5 (BUY), bukan ekstrem Candle 4.
	// Ini memberi proteksi struktural terhadap manipulasi likuiditas Candle 5.
	isCandle6 := lastHTFIdx == candle5Idx+1
	if isCandle6 && candle5Idx >= 0 && candle5Idx < len(htfCandles) {
		c5 := htfCandles[candle5Idx]
		if sweep.Direction == "BEARISH" && c5.High > 0 {
			setup.StopLoss = c5.High
		} else if sweep.Direction == "BULLISH" && c5.Low > 0 {
			setup.StopLoss = c5.Low
		}
	}

	// Fase WAIT memakai TargetDOL apa adanya. Bila DOL sudah tersentuh lebih dulu
	// (Skenario 3 DOL Breached), TargetDOL berada di belakang harga sehingga panel
	// menampilkan TP yang sudah terlewati. Samakan dengan aturan jalur eksekusi:
	// kunci TP ke batas dasar 1:2.00 R:R diukur dari close Candle 4 (acuan entry
	// pada fase ini) terhadap SL sweep.
	if slDist := math.Abs(sweep.SweepCandle.Close - setup.StopLoss); slDist > 0 {
		dolBreached := false
		if setup.Bias == "BULLISH" && sweep.SweepCandle.High >= setup.TakeProfit {
			dolBreached = true
		} else if setup.Bias == "BEARISH" && sweep.SweepCandle.Low <= setup.TakeProfit {
			dolBreached = true
		}
		if dolBreached {
			if setup.Bias == "BULLISH" {
				setup.TakeProfit = sweep.SweepCandle.Close + (2.00 * slDist)
			} else {
				setup.TakeProfit = sweep.SweepCandle.Close - (2.00 * slDist)
			}
		}
	}

	// 3. Evaluate LTF for CISD murni di dalam jendela waktu setelah sweep HTF closed
	cisd, invalidated := e.DetectDynamicCISD(ltfCandles, sweep.Direction, entryCandleStart, sweep.SweepCandle)
	if invalidated {
		setup.Status = "INVALIDATED"
		setup.NextAction = fmt.Sprintf("🛑 Setup ILA Batal: Harga di Candle 5 menembus ekor Candle 4 (%s).", htfName)
		setup.Summary = fmt.Sprintf("Setup %s ILA batal karena ekor sweep HTF ditembus.", sweep.Direction)
		return setup
	}

	// Jika di Candle 6 dan cisd terbentuk, pastikan ExtremeSL memakai High/Low Candle 5
	if isCandle6 && cisd != nil {
		cisd.ExtremeSL = setup.StopLoss
	}

	if cisd == nil || !cisd.Detected {
		setup.Status = "WAIT_LTF_CISD"
		setup.CISD = cisd
		if cisd != nil {
			// Di Candle 6, SL tetap dikunci pada High/Low Candle 5; ExtremeSL LTF tidak boleh menimpanya.
			if !isCandle6 {
				setup.StopLoss = cisd.ExtremeSL
			}
			digits := 2
			if strings.Contains(symbol, "EUR") || strings.Contains(symbol, "GBP") || strings.Contains(symbol, "AUD") {
				digits = 5
			}
			fStr := fmt.Sprintf("%%.%df", digits)
			cisdStr := fmt.Sprintf(fStr, cisd.CISDLevel)
			slStr := fmt.Sprintf(fStr, setup.StopLoss)
			setup.NextAction = fmt.Sprintf("⏳ Level CISD Valid di %s (SL: %s). Menunggu closed candle %s menembus level tersebut by body.", cisdStr, slStr, ltfName)
			setup.Summary = fmt.Sprintf("HTF %s sweep valid. Menunggu CISD %s di level %s (SL: %s).", htfName, ltfName, cisdStr, slStr)
		} else {
			candleWindowName := "Candle 5"
			if isCandle6 {
				candleWindowName = "Candle 6 (SL: High/Low Candle 5)"
			}
			setup.NextAction = fmt.Sprintf("⏳ HTF %s Sweep FVG resmi CLOSED! Saat ini di %s (SL: %s), menunggu pembentukan CISD di %s.", htfName, candleWindowName, formatILAVal(setup.StopLoss), ltfName)
			setup.Summary = fmt.Sprintf("Internal Liquidity FVG %s sudah disapu (%s). Target: %s. Menunggu CISD di %s.", htfName, sweep.Direction, formatILAVal(sweep.TargetDOL), ltfName)
		}
		return setup
	}

	// 4. Both HTF Sweep and LTF CISD confirmed -> READY TO ENTRY!
	setup.CISD = cisd
	setup.Status = "READY_TO_ENTRY"
	setup.EntryPrice = cisd.EntryPrice
	// Di Candle 6, SL tetap pada High/Low Candle 5 (lihatan struktural jendela ini).
	if !isCandle6 && cisd.ExtremeSL > 0 {
		setup.StopLoss = cisd.ExtremeSL
	}

	// Risk & Reward calculation (Opsi A: DOL-Bounded Clamping antara 1:2.0 s.d. 1:2.3 R:R)
	slDist := math.Abs(setup.EntryPrice - setup.StopLoss)

	// Check if DOL was already breached/passed by the sweep candle or current price
	dolBreached := false
	if setup.Bias == "BULLISH" {
		if setup.EntryPrice >= setup.TakeProfit || (sweep != nil && sweep.SweepCandle.High >= setup.TakeProfit) {
			dolBreached = true
		}
	} else if setup.Bias == "BEARISH" {
		if setup.EntryPrice <= setup.TakeProfit || (sweep != nil && sweep.SweepCandle.Low <= setup.TakeProfit) {
			dolBreached = true
		}
	}

	targetRR := 2.00
	if dolBreached {
		// Rule ILA: Jika DOL/Eksternal sudah jebol duluan, kunci TP ke batas bawah 1:2.00 R:R
		targetRR = 2.00
	} else {
		rawRR := 0.0
		if slDist > 0 {
			rawRR = math.Abs(setup.TakeProfit-setup.EntryPrice) / slDist
		}
		if rawRR >= 2.30 {
			// Skenario 1 (Jarak DOL Jauh / R:R >= 2.3): Cap tepat di 1:2.30 (anti-greed)
			targetRR = 2.30
		} else if rawRR >= 2.00 {
			// Skenario 2 (Jarak DOL di Rentang 2.0 - 2.3): Pakai DOL langsung
			targetRR = math.Round(rawRR*100) / 100
		} else {
			// Skenario 3 (Jarak DOL < 2.0): Kunci di batas dasar 1:2.00
			targetRR = 2.00
		}
	}

	if targetRR == 2.00 || targetRR == 2.30 || dolBreached {
		if setup.Bias == "BULLISH" {
			setup.TakeProfit = setup.EntryPrice + (targetRR * slDist)
		} else {
			setup.TakeProfit = setup.EntryPrice - (targetRR * slDist)
		}
	}

	// Round TakeProfit cleanly sesuai instrumen
	digits := 2
	if strings.Contains(symbol, "EUR") || strings.Contains(symbol, "GBP") || strings.Contains(symbol, "AUD") {
		digits = 5
	}
	pow := math.Pow10(digits)
	setup.TakeProfit = math.Round(setup.TakeProfit*pow) / pow

	setup.RiskReward = targetRR
	setup.PassedMinRR = setup.RiskReward >= 2.00

	actionEmoji := "🟢 BUY"
	if setup.Bias == "BEARISH" {
		actionEmoji = "🔴 SELL"
	}

	fmtPrice := "%.2f"
	if digits == 5 {
		fmtPrice = "%.5f"
	}
	setup.NextAction = fmt.Sprintf("🎯 SIAP EKSEKUSI %s: Masuk di candle pembuka berikutnya!", actionEmoji)
	setup.Summary = fmt.Sprintf("💎 SETUP ILA TERKONFIRMASI LENGKAP (%s ➡️ %s):\n• %s\n• %s\n• Level: Entry="+fmtPrice+", SL="+fmtPrice+", TP="+fmtPrice+" (R:R 1:%.2f)",
		htfName, ltfName, sweep.Details, cisd.Details, setup.EntryPrice, setup.StopLoss, setup.TakeProfit, setup.RiskReward)

	return setup
}
