package main

import (
	"back/config"
	"back/router"
	"back/utils"
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/sirupsen/logrus"
)

func main() {
	config.InitConfig()
	defer func() {
		if err := config.CloseRabbitMQ(); err != nil {
			logrus.Errorf("close rabbitmq failed: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	utils.InitLikeCacheInfrastructure()
	utils.StartLikeOutboxRelay(ctx)
	utils.StartLikeWorker(ctx)
	utils.StartLikeStatsSyncWorker(ctx)
	utils.StartRAGIndexWorker(ctx)

	utils.InitBloomFilter()
	utils.StartRAGSyncWorker()

	r := router.SetupRouter()
	port := config.AppConfig.App.Port
	if port == "" {
		port = ":8080"
	}
	srv := &http.Server{
		Addr:    port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	logrus.Info("Shutdown Server ...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logrus.Fatal("Server Shutdown:", err)
	}
	logrus.Info("Server exiting")
}
