package ila

import (
	"time"

	"ila-engine/pkg/indicators"
)

// C adalah ringkasan OHLC satu candle untuk fixture test.
type C struct{ o, h, l, c float64 }

func series(base time.Time, step time.Duration, cs ...C) []indicators.Candle {
	out := make([]indicators.Candle, len(cs))
	for i, c := range cs {
		out[i] = indicators.Candle{Timestamp: base.Add(time.Duration(i) * step),
			Open: c.o, High: c.h, Low: c.l, Close: c.c, IsClosed: true}
	}
	return out
}

var testBase = time.Date(2025, 6, 9, 18, 0, 0, 0, nyLocation)

// mirror200 mencerminkan harga (200-x, high<->low) untuk menguji sisi bullish / swing high.
func mirror200(cs []indicators.Candle) []indicators.Candle {
	out := make([]indicators.Candle, len(cs))
	for i, c := range cs {
		out[i] = c
		out[i].Open, out[i].High, out[i].Low, out[i].Close = 200-c.Open, 200-c.Low, 200-c.High, 200-c.Close
	}
	return out
}
