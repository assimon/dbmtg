package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/assimon/dbmtg/internal/bot"
	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/server"
	"github.com/assimon/dbmtg/internal/service"
	"github.com/assimon/dbmtg/internal/store"
	"github.com/assimon/dbmtg/internal/telegram"
)

func main() {
	path := flag.String("c", "config.yaml", "配置文件路径")
	flag.Parse()
	if err := run(*path); err != nil {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DB.Path)
	if err != nil {
		return err
	}
	tg, err := telegram.New(cfg.Bot.Token)
	if err != nil {
		return err
	}

	providers := captcha.FromConfig(cfg.Captcha)
	if len(providers) > 0 && cfg.Web.PublicURL == "" {
		slog.Warn("未配置 web.public_url，人机验证不可用")
		providers = nil
	}
	svc := service.New(cfg, st, tg, providers, bot.TopicHeader)
	b := bot.New(cfg, tg, svc)

	var webhook http.Handler
	if cfg.Bot.Mode == config.ModeWebhook {
		webhook = b.WebhookHandler()
	}
	srv := server.New(cfg, svc, bot.VerifyPath, bot.WebhookPath, webhook)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			stop()
		}
	}()
	slog.Info("started", "mode", cfg.Bot.Mode, "listen", cfg.Web.Listen, "captcha", svc.Settings.CaptchaMode())

	if err := b.Run(ctx); err != nil {
		return err
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdown)
	select {
	case err := <-errc:
		return err
	default:
		return nil
	}
}
