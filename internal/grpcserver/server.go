// Package grpcserver menyajikan sinyal engine lewat kontrak gRPC di plans/08.
//
// Server ini tidak menyimpan apa pun selain sinyal yang masih hidup. Tidak ada
// akun, kredensial, atau posisi di sini: engine memang tidak boleh tahu.
package grpcserver

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	enginev1 "ila-engine/gen/engine/v1"
	"ila-engine/internal/engine"
	"ila-engine/internal/market"
	"ila-engine/internal/signal"
)

// Server mengimplementasikan enginev1.EngineServiceServer.
type Server struct {
	enginev1.UnimplementedEngineServiceServer

	reg     *engine.Registry
	pub     *engine.Publisher
	version engine.Version
	market  func() market.State
	log     *slog.Logger

	mu     sync.Mutex
	stream map[string]chan *enginev1.EngineEvent
	acked  map[string]time.Time
}

// Deps adalah bahan yang dibutuhkan server.
type Deps struct {
	Registry  *engine.Registry
	Publisher *engine.Publisher
	Version   engine.Version
	// MarketState dibaca saat klien meminta snapshot.
	MarketState func() market.State
	Log         *slog.Logger
}

// New membuat server gRPC.
func New(d Deps) *Server {
	return &Server{
		reg:     d.Registry,
		pub:     d.Publisher,
		version: d.Version,
		market:  d.MarketState,
		log:     d.Log,
		stream:  make(map[string]chan *enginev1.EngineEvent),
		acked:   make(map[string]time.Time),
	}
}

// Subscribe mengganti daftar simbol satu pelanggan.
// Symbol yang tidak dikenali tetap diterima: daftar simbol yang sah ada di
// konfigurasi engine, bukan di sini.
func (s *Server) Subscribe(ctx context.Context, req *enginev1.SubscribeRequest) (*enginev1.SubscribeResponse, error) {
	log := loggerDenganTrace(ctx, s.log)
	if req.GetSubscriberId() == "" {
		return nil, status.Error(codes.InvalidArgument, "subscriber_id wajib diisi")
	}

	symbols := sanitize(req.GetSymbols())
	s.reg.Add(engine.Subscriber{
		ID:         req.GetSubscriberId(),
		Symbols:    toSet(symbols),
		WantStatus: req.GetWantStatusUpdates(),
		Send:       s.senderFor(req.GetSubscriberId()),
	})

	log.Info("pelanggan berlangganan",
		"evt", "subscribe.ok",
		"subscriber", req.GetSubscriberId(), "symbols", symbols)

	return &enginev1.SubscribeResponse{
		Ok:              true,
		AcceptedSymbols: symbols,
		Message:         "langganan diperbarui",
	}, nil
}

// Unsubscribe mencabut sebagian simbol pelanggan.
func (s *Server) Unsubscribe(ctx context.Context, req *enginev1.UnsubscribeRequest) (*enginev1.SubscribeResponse, error) {
	log := loggerDenganTrace(ctx, s.log)
	id := req.GetSubscriberId()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "subscriber_id wajib diisi")
	}

	if len(req.GetSymbols()) == 0 {
		s.reg.Remove(id)
	} else {
		s.reg.RemoveSymbols(id, sanitize(req.GetSymbols()))
	}

	log.Info("pelanggan mencabut langganan", "evt", "unsubscribe.ok", "subscriber", id, "symbols", req.GetSymbols())
	return &enginev1.SubscribeResponse{Ok: true, Message: "langganan dicabut"}, nil
}

// StreamEvents mengalirkan sinyal dan status pasar sampai klien terputus.
func (s *Server) StreamEvents(req *enginev1.SubscribeRequest, stream enginev1.EngineService_StreamEventsServer) error {
	log := loggerDenganTrace(stream.Context(), s.log)
	id := req.GetSubscriberId()
	if id == "" {
		return status.Error(codes.InvalidArgument, "subscriber_id wajib diisi")
	}

	ch := make(chan *enginev1.EngineEvent, 64)
	s.mu.Lock()
	if old, ok := s.stream[id]; ok {
		close(old)
	}
	s.stream[id] = ch
	s.mu.Unlock()

	// Simbol dari permintaan stream ikut didaftarkan agar klien boleh
	// langsung meminta tanpa memanggil Subscribe lebih dulu.
	symbols := sanitize(req.GetSymbols())
	s.reg.Add(engine.Subscriber{
		ID: id, Symbols: toSet(symbols),
		WantStatus: req.GetWantStatusUpdates(), Send: s.senderFor(id),
	})

	defer func() {
		s.mu.Lock()
		if cur, ok := s.stream[id]; ok && cur == ch {
			delete(s.stream, id)
			close(ch)
		}
		s.mu.Unlock()
		s.reg.Remove(id)
		log.Info("stream pelanggan berakhir", "evt", "stream.end", "subscriber", id)
	}()

	// Kirim status pasar lebih dulu agar Runner tidak salah menahan sinyal.
	if s.market != nil {
		if err := stream.Send(marketEvent(s.market())); err != nil {
			return err
		}
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}

// AckSignal mencatat bahwa Runner sudah menerima sinyal.
// ACK hanya untuk pelacakan: tidak mengubah status sinyal.
func (s *Server) AckSignal(_ context.Context, req *enginev1.AckRequest) (*enginev1.AckResponse, error) {
	if req.GetSignalId() == "" {
		return nil, status.Error(codes.InvalidArgument, "signal_id wajib diisi")
	}

	s.mu.Lock()
	s.acked[req.GetSignalId()] = time.Now().UTC()
	s.mu.Unlock()

	return &enginev1.AckResponse{Ok: true, SignalId: req.GetSignalId(), Message: "diterima"}, nil
}

// Snapshot mengembalikan sinyal yang masih berlaku untuk pelanggan baru.
func (s *Server) Snapshot(_ context.Context, req *enginev1.SnapshotRequest) (*enginev1.SnapshotResponse, error) {
	signals := s.pub.Snapshot(sanitize(req.GetSymbols()))

	out := make([]*enginev1.Signal, 0, len(signals))
	for _, sig := range signals {
		out = append(out, toProto(sig, s.version))
	}

	resp := &enginev1.SnapshotResponse{
		Signals:       out,
		TakenAt:       timestamppb.Now(),
		EngineVersion: s.version.Engine,
	}
	if s.market != nil {
		resp.MarketState = marketStateEvent(s.market())
	}
	return resp, nil
}

// senderFor mengembalikan fungsi pengirim yang menaruh event ke antrean stream
// pelanggan. Antrean penuh dibuang agar engine tidak pernah terblokir oleh
// Runner yang lambat.
func (s *Server) senderFor(id string) func(engine.Signal) {
	return func(sig engine.Signal) {
		s.mu.Lock()
		ch, ok := s.stream[id]
		s.mu.Unlock()
		if !ok {
			return
		}
		ev := &enginev1.EngineEvent{
			Payload: &enginev1.EngineEvent_Signal{Signal: toProto(sig, s.version)},
		}
		select {
		case ch <- ev:
		default:
			s.log.Warn("antrean pelanggan penuh, event dibuang",
				"subscriber", id, "signal_id", sig.ID)
		}
	}
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, it := range items {
		out[it] = struct{}{}
	}
	return out
}

func sanitize(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it != "" {
			out = append(out, it)
		}
	}
	return out
}

func statusOf(s signal.Status) enginev1.SignalStatus {
	switch s {
	case signal.Ready:
		return enginev1.SignalStatus_READY_TO_ENTRY
	case signal.WaitHTFSweep:
		return enginev1.SignalStatus_WAIT_HTF_SWEEP
	case signal.WaitHTFCandleClose:
		return enginev1.SignalStatus_WAIT_HTF_CANDLE_CLOSE
	case signal.WaitLTFCISD:
		return enginev1.SignalStatus_WAIT_LTF_CISD
	case signal.Invalidated:
		return enginev1.SignalStatus_INVALIDATED
	default:
		return enginev1.SignalStatus_SIGNAL_STATUS_UNSPECIFIED
	}
}

func actionOf(a signal.Action) enginev1.SignalAction {
	switch a {
	case signal.ActionPublish:
		return enginev1.SignalAction_SIGNAL_ACTION_PUBLISH
	case signal.ActionCancel:
		return enginev1.SignalAction_SIGNAL_ACTION_CANCEL
	case signal.ActionExpire:
		return enginev1.SignalAction_SIGNAL_ACTION_EXPIRE
	case signal.ActionSupersede:
		return enginev1.SignalAction_SIGNAL_ACTION_SUPERSEDE
	default:
		return enginev1.SignalAction_SIGNAL_ACTION_UNSPECIFIED
	}
}

func directionOf(d string) enginev1.Direction {
	switch d {
	case "BUY":
		return enginev1.Direction_BUY
	case "SELL":
		return enginev1.Direction_SELL
	default:
		return enginev1.Direction_DIRECTION_UNSPECIFIED
	}
}

func marketStateEvent(st market.State) *enginev1.MarketStateEvent {
	var ms enginev1.MarketState
	switch st {
	case market.Open:
		ms = enginev1.MarketState_MARKET_OPEN
	case market.RolloverBlackout:
		ms = enginev1.MarketState_MARKET_ROLLOVER_BLACKOUT
	default:
		ms = enginev1.MarketState_MARKET_CLOSED
	}
	return &enginev1.MarketStateEvent{State: ms, At: timestamppb.Now(), Detail: string(st)}
}

func marketEvent(st market.State) *enginev1.EngineEvent {
	return &enginev1.EngineEvent{
		Payload: &enginev1.EngineEvent_MarketState{MarketState: marketStateEvent(st)},
	}
}
