package grpcserver

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	enginev1 "ila-engine/gen/engine/v1"
	"ila-engine/internal/engine"
	"ila-engine/pkg/ila"
	"ila-engine/pkg/indicators"
)

// toProto menerjemahkan sinyal engine ke kontrak keluar.
//
// Field harga diteruskan apa adanya dari evaluator. Engine tidak menambahkan
// apa pun yang berbau akun, lot, atau risiko.
func toProto(sig engine.Signal, v engine.Version) *enginev1.Signal {
	out := &enginev1.Signal{
		SignalId:        sig.ID,
		Symbol:          sig.Symbol,
		Htf:             sig.HTF,
		Ltf:             sig.LTF,
		Status:          statusOf(sig.Status),
		Action:          actionOf(sig.Action),
		Direction:       directionOf(sig.Direction),
		EntryPrice:      sig.Entry,
		StopLoss:        sig.StopLoss,
		TakeProfit:      sig.TakeProfit,
		RiskReward:      sig.RiskReward,
		PassedMinRr:     sig.PassedMinRR,
		Summary:         sig.Summary,
		NextAction:      sig.NextAction,
		EngineVersion:   v.Engine,
		StrategyVersion: v.Strategy,
		SetupType:       sig.SetupType,
		SequenceState:   sig.SequenceState,
		CriticalZoneTp:  sig.CriticalZoneTP,
		FinalZoneTp:     sig.FinalZoneTP,
		QuarterInfo:     sig.QuarterInfo,
		SsmtResult:      sig.SSMTResult,
		FeedSource:      sig.FeedSource,
	}
	if !sig.PublishedAt.IsZero() {
		out.PublishedAt = timestamppb.New(sig.PublishedAt)
	}
	if !sig.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(sig.ExpiresAt)
	}
	if sig.HTFSweep != nil {
		out.HtfSweep = htfSweepToProto(sig.HTFSweep)
	}
	if sig.CISD != nil {
		out.Cisd = cisdToProto(sig.CISD)
	}
	return out
}

func htfSweepToProto(h *ila.HTFSweep) *enginev1.HTFSweepInfo {
	if h == nil {
		return nil
	}
	info := &enginev1.HTFSweepInfo{
		Detected:        h.Detected,
		SweepInProgress: h.SweepInProgress,
		Direction:       h.Direction,
		StopLoss:        h.StopLoss,
		TargetDol:       h.TargetDOL,
		Details:         h.Details,
		SweepCandle:     candleToProto(h.SweepCandle),
		SweptCandle:     candleToProto(h.SweptCandle),
	}
	if h.FVG != nil {
		info.Fvg = &enginev1.FairValueGap{
			Type:      string(h.FVG.Type),
			Top:       h.FVG.Top,
			Bottom:    h.FVG.Bottom,
			Mitigated: h.FVG.Mitigated,
		}
		if !h.FVG.Candle3Time.IsZero() {
			info.Fvg.Candle3Time = timestamppb.New(h.FVG.Candle3Time)
		}
	}
	if !h.SweepTime.IsZero() {
		info.SweepTime = timestamppb.New(h.SweepTime)
	}
	return info
}

func cisdToProto(c *ila.CISD) *enginev1.CISDInfo {
	if c == nil {
		return nil
	}
	info := &enginev1.CISDInfo{
		Detected:    c.Detected,
		Direction:   c.Direction,
		CisdLevel:   c.CISDLevel,
		ExtremeSl:   c.ExtremeSL,
		EntryPrice:  c.EntryPrice,
		Details:     c.Details,
		RefCandle:   candleToProto(c.RefCandle),
		BreakCandle: candleToProto(c.BreakCandle),
	}
	if !c.BreakTime.IsZero() {
		info.BreakTime = timestamppb.New(c.BreakTime)
	}
	return info
}

func candleToProto(c indicators.Candle) *enginev1.CandleInfo {
	info := &enginev1.CandleInfo{
		Open:     c.Open,
		High:     c.High,
		Low:      c.Low,
		Close:    c.Close,
		IsClosed: c.IsClosed,
	}
	if !c.Timestamp.IsZero() {
		info.Timestamp = timestamppb.New(c.Timestamp)
	}
	return info
}
