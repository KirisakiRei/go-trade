package grpcserver

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/metadata"
)

// KunciTrace adalah metadata gRPC yang membawa nomor rangkaian log dari Runner.
const KunciTrace = "x-ila-trace"

type kunciTrace struct{}

// loggerDenganTrace mengembalikan logger yang selalu membawa kunci "trace".
//
// Runner mengirim trace pada setiap panggilan, dan Engine memakainya untuk
// menulis log. Hasilnya satu sinyal bisa ditelusuri di dua VPS dengan nomor
// yang sama.
func loggerDenganTrace(ctx context.Context, dasar *slog.Logger) *slog.Logger {
	if dasar == nil {
		dasar = slog.Default()
	}

	trace := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(KunciTrace); len(v) > 0 {
			trace = v[0]
		}
	}
	if trace == "" {
		if v, ok := ctx.Value(kunciTrace{}).(string); ok {
			trace = v
		}
	}
	if trace == "" {
		return dasar
	}
	return dasar.With("trace", trace)
}

// DenganTrace menempelkan trace ke konteks, dipakai pada pengujian.
func DenganTrace(ctx context.Context, trace string) context.Context {
	return context.WithValue(ctx, kunciTrace{}, trace)
}
