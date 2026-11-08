package grpcserver

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	enginev1 "ila-engine/gen/engine/v1"
	"ila-engine/internal/engine"
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
	}
	if !sig.PublishedAt.IsZero() {
		out.PublishedAt = timestamppb.New(sig.PublishedAt)
	}
	if !sig.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(sig.ExpiresAt)
	}
	return out
}
