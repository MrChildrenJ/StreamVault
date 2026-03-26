package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MrChildrenJ/streamvault/internal/bits"
	"github.com/MrChildrenJ/streamvault/internal/config"
	"github.com/MrChildrenJ/streamvault/internal/dashboard"
	"github.com/MrChildrenJ/streamvault/internal/db"
	"github.com/MrChildrenJ/streamvault/internal/donation"
	"github.com/MrChildrenJ/streamvault/internal/event"
	"github.com/MrChildrenJ/streamvault/internal/event/consumer"
	"github.com/MrChildrenJ/streamvault/internal/payout"
	"github.com/MrChildrenJ/streamvault/internal/subscription"
	"github.com/MrChildrenJ/streamvault/internal/transaction"
	"github.com/MrChildrenJ/streamvault/internal/wallet"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied")

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	log.Println("database connected")

	// Shared repositories
	txRepo     := transaction.NewRepository(pool)
	walletRepo := wallet.NewRepository(pool)
	outboxRepo := event.NewOutboxRepository(pool)

	// Kafka producer + outbox relay
	producer := event.NewProducer(cfg.KafkaBroker)
	defer producer.Close()
	relay := event.NewOutboxRelay(pool, producer, 2*time.Second)
	relay.Start(ctx)

	// Services & handlers
	walletSvc := wallet.NewService(pool, walletRepo, txRepo)
	walletH   := wallet.NewHandler(walletSvc)

	bitsRepo := bits.NewRepository(pool)
	bitsSvc  := bits.NewService(pool, bitsRepo, walletRepo, txRepo, outboxRepo)
	bitsH    := bits.NewHandler(bitsSvc)

	subRepo := subscription.NewRepository(pool)
	subSvc  := subscription.NewService(pool, subRepo, walletRepo, txRepo, outboxRepo)
	subH    := subscription.NewHandler(subSvc)
	subSvc.StartExpiryWorker(ctx, 5*time.Minute)

	donationSvc := donation.NewService(pool, walletRepo, txRepo, outboxRepo)
	donationH   := donation.NewHandler(donationSvc)

	payoutSvc := payout.NewService(pool, txRepo)
	payoutH   := payout.NewHandler(payoutSvc)

	dashRepo := dashboard.NewRepository(pool)
	dashH    := dashboard.NewHandler(dashRepo)

	// Revenue aggregator (Kafka consumer)
	aggregator := consumer.NewRevenueAggregator(pool, cfg.KafkaBroker)
	aggregator.Start(ctx)
	defer aggregator.Close()

	// Router
	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	walletH.RegisterRoutes(api)
	bitsH.RegisterRoutes(api)
	subH.RegisterRoutes(api)
	donationH.RegisterRoutes(api)
	payoutH.RegisterRoutes(api)
	dashH.RegisterRoutes(api)

	srv := &http.Server{Addr: ":" + cfg.ServerPort, Handler: r}
	go func() {
		log.Printf("StreamVault listening on :%s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown: %v", err)
	}
	log.Println("stopped")
}
