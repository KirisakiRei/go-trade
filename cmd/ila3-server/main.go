package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"ila-engine/pkg/indicators"
	"ila-engine/pkg/marketfeed"
	"ila-engine/pkg/quarterly"
	"ila-engine/pkg/sequence"
	"ila-engine/pkg/ssmt"
	"ila-engine/pkg/stdev"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting ILA 3.0 Research Server on port 8082...")

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	feed := marketfeed.NewMultiFeedProvider()
	eval := sequence.NewEvaluator()
	qtCalc := quarterly.NewCalculator()
	stdevProj := stdev.NewProjector()
	ssmtDet := ssmt.NewDetector()

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"app":     "ila3-engine",
			"version": "3.0.0-research",
			"port":    8082,
		})
	})

	api := r.Group("/api/v1/ila3")
	{
		// 1. Full Sequence Evaluation
		api.GET("/analyze", func(c *gin.Context) {
			sym := strings.ToUpper(c.DefaultQuery("symbol", "EURUSD"))
			htf := c.DefaultQuery("htf", "4H")
			mtf := c.DefaultQuery("mtf", "1H")
			ltf := c.DefaultQuery("ltf", "5m")
			corr := c.Query("corr")
			if corr == "" {
				corr = ssmt.DefaultCorrelationPairs[sym]
			}

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			hCandles, _ := feed.FetchCandles(ctx, sym, htf, 40)
			mCandles, _ := feed.FetchCandles(ctx, sym, mtf, 40)
			lCandles, _ := feed.FetchCandles(ctx, sym, ltf, 40)
			var corrCandles []indicators.Candle
			if corr != "" {
				corrCandles, _ = feed.FetchCandles(ctx, corr, mtf, 40)
			}

			setup := eval.EvaluateSequence(hCandles, mCandles, lCandles, corrCandles, htf, mtf, ltf, sym, corr)
			c.JSON(200, gin.H{"success": true, "data": setup})
		})

		// 2. Quarterly Theory Info
		api.GET("/quarterly", func(c *gin.Context) {
			now := time.Now().UTC()
			qInfo := qtCalc.GetDailyQuarter(now)
			subQ := qtCalc.Get90MQuarter(now)
			c.JSON(200, gin.H{
				"success": true,
				"data": gin.H{
					"daily_quarter": qInfo,
					"sub_quarter":   subQ,
					"pairings":      quarterly.CyclePairings,
				},
			})
		})

		// 3. ICT Standard Deviation Calculator
		api.GET("/stdev", func(c *gin.Context) {
			dirStr := strings.ToUpper(c.DefaultQuery("direction", "BULLISH"))
			var p1, p0 float64
			_, _ = fmt.Sscanf(c.DefaultQuery("point1", "100"), "%f", &p1)
			_, _ = fmt.Sscanf(c.DefaultQuery("point0", "110"), "%f", &p0)

			dir := stdev.DirectionBullish
			if dirStr == "BEARISH" {
				dir = stdev.DirectionBearish
			}
			proj := stdevProj.CalculateProjection(dir, p1, p0)
			c.JSON(200, gin.H{"success": true, "data": proj})
		})

		// 4. SSMT Correlation Evaluator
		api.GET("/ssmt", func(c *gin.Context) {
			pSym := strings.ToUpper(c.DefaultQuery("primary", "EURUSD"))
			cSym := strings.ToUpper(c.DefaultQuery("correlated", "GBPUSD"))
			tf := c.DefaultQuery("tf", "15m")
			dir := strings.ToUpper(c.DefaultQuery("direction", "BULLISH"))

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			pCandles, _ := feed.FetchCandles(ctx, pSym, tf, 20)
			cCandles, _ := feed.FetchCandles(ctx, cSym, tf, 20)

			res := ssmtDet.EvaluateSSMT(pSym, cSym, pCandles, cCandles, dir)
			c.JSON(200, gin.H{"success": true, "data": res})
		})
	}

	srv := &http.Server{
		Addr:    ":8082",
		Handler: r,
	}

	go func() {
		slog.Info("ILA 3.0 Server listening on port 8082")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("ILA 3.0 Server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down ILA 3.0 server...")
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(ctxShutdown)
	slog.Info("ILA 3.0 Server stopped cleanly")
}
