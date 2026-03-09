package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gogobee/internal/bot"
	"gogobee/internal/db"
	"gogobee/internal/plugin"
	"gogobee/internal/util"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {
	if err := godotenv.Load(); err != nil {
		// Log before structured logger is set up
		fmt.Fprintf(os.Stderr, "WARNING: could not load .env file: %v\n", err)
		fmt.Fprintf(os.Stderr, "  (working directory: %s)\n", mustGetwd())
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	util.InitLogger(logLevel)
	slog.Info("logger initialized", "level", logLevel,
		"ollama_host", os.Getenv("OLLAMA_HOST"),
		"ollama_model", os.Getenv("OLLAMA_MODEL"))

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// Initialize database
	if err := db.Init(dataDir); err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}
	if err := db.SeedSchedulerDefaults(db.Get()); err != nil {
		slog.Warn("seed scheduler defaults failed", "err", err)
	}

	// Create Matrix client
	cfg := bot.Config{
		Homeserver:  os.Getenv("HOMESERVER_URL"),
		UserID:      os.Getenv("BOT_USER_ID"),
		Password:    os.Getenv("BOT_PASSWORD"),
		DataDir:     dataDir,
		DisplayName: envOr("BOT_DISPLAY_NAME", "GogoBee"),
	}

	client, err := bot.NewClient(cfg)
	if err != nil {
		slog.Error("client init failed", "err", err)
		os.Exit(1)
	}

	// Create plugin registry
	registry := bot.NewRegistry()

	// ---- Register plugins in dependency order ----

	// Foundation (passive tracking)
	xpPlugin := plugin.NewXPPlugin(client)
	registry.Register(xpPlugin)

	ratePlugin := plugin.NewRateLimitsPlugin(client)
	registry.Register(ratePlugin)

	registry.Register(plugin.NewReputationPlugin(client, xpPlugin))
	registry.Register(plugin.NewStatsPlugin(client))
	registry.Register(plugin.NewStreaksPlugin(client))

	// Interactive
	registry.Register(plugin.NewTriviaPlugin(client))
	registry.Register(plugin.NewRemindersPlugin(client))
	registry.Register(plugin.NewPresencePlugin(client))
	registry.Register(plugin.NewFunPlugin(client))
	registry.Register(plugin.NewToolsPlugin(client))
	registry.Register(plugin.NewUserPlugin(client))

	// Entertainment / Lookup
	registry.Register(plugin.NewRetroPlugin(client))
	registry.Register(plugin.NewLookupPlugin(client, ratePlugin))
	registry.Register(plugin.NewCountdownPlugin(client))
	registry.Register(plugin.NewStocksPlugin(client))
	registry.Register(plugin.NewConcertsPlugin(client))
	registry.Register(plugin.NewAnimePlugin(client))
	registry.Register(plugin.NewMoviesPlugin(client))

	// Tracking (passive)
	registry.Register(plugin.NewAchievementsPlugin(client, registry))
	registry.Register(plugin.NewReactionsPlugin(client))
	registry.Register(plugin.NewMarkovPlugin(client))
	registry.Register(plugin.NewURLsPlugin(client))

	// LLM-powered (passive)
	registry.Register(plugin.NewLLMPassivePlugin(client, xpPlugin))

	// Scheduled
	wotdPlugin := plugin.NewWOTDPlugin(client)
	registry.Register(wotdPlugin)
	holidaysPlugin := plugin.NewHolidaysPlugin(client)
	registry.Register(holidaysPlugin)
	gamingPlugin := plugin.NewGamingPlugin(client)
	registry.Register(gamingPlugin)
	birthdayPlugin := plugin.NewBirthdayPlugin(client, xpPlugin)
	registry.Register(birthdayPlugin)

	// Utility / Meta
	registry.Register(plugin.NewBotInfoPlugin(client))
	registry.Register(plugin.NewHowAmIPlugin(client))
	registry.Register(plugin.NewVibePlugin(client))
	registry.Register(plugin.NewShadePlugin(client))
	registry.Register(plugin.NewWelcomePlugin(client, xpPlugin, registry))

	// Initialize all plugins
	if err := registry.Init(); err != nil {
		slog.Error("plugin init failed", "err", err)
		os.Exit(1)
	}

	// ---- Set up event handlers ----

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// Auto-join on invite
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		if evt.GetStateKey() == string(client.UserID) {
			mem := evt.Content.AsMember()
			if mem.Membership == event.MembershipInvite {
				_, err := client.JoinRoomByID(ctx, evt.RoomID)
				if err != nil {
					slog.Error("failed to join room", "room", evt.RoomID, "err", err)
				} else {
					slog.Info("joined room", "room", evt.RoomID)
				}
			}
		}
	})

	// Message handler
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {
		// Skip own messages
		if evt.Sender == client.UserID {
			return
		}

		content := evt.Content.AsMessage()
		if content == nil || content.Body == "" {
			return
		}

		body := content.Body
		msgCtx := plugin.MessageContext{
			RoomID:    evt.RoomID,
			EventID:   evt.ID,
			Sender:    evt.Sender,
			Body:      body,
			IsCommand: strings.HasPrefix(strings.TrimSpace(body), "!"),
			Event:     evt,
		}

		registry.DispatchMessage(msgCtx)
	})

	// Reaction handler
	syncer.OnEventType(event.EventReaction, func(ctx context.Context, evt *event.Event) {
		if evt.Sender == client.UserID {
			return
		}

		content := evt.Content.AsReaction()
		if content == nil {
			return
		}

		reactCtx := plugin.ReactionContext{
			RoomID:      evt.RoomID,
			EventID:     evt.ID,
			Sender:      evt.Sender,
			TargetEvent: content.RelatesTo.EventID,
			Emoji:       content.RelatesTo.Key,
			Event:       evt,
		}

		registry.DispatchReaction(reactCtx)
	})

	// ---- Set up cron scheduler ----
	scheduler := cron.New()
	setupScheduledJobs(scheduler, client, wotdPlugin, holidaysPlugin, gamingPlugin, birthdayPlugin)
	scheduler.Start()

	// ---- Start syncing ----
	slog.Info("GogoBee starting sync...")

	ctx, cancel := context.WithCancel(context.Background())

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		scheduler.Stop()
		cancel()
	}()

	if err := client.SyncWithContext(ctx); err != nil {
		slog.Error("sync stopped", "err", err)
	}

	slog.Info("GogoBee stopped")
}

func setupScheduledJobs(
	c *cron.Cron,
	client *mautrix.Client,
	wotd *plugin.WOTDPlugin,
	holidays *plugin.HolidaysPlugin,
	gaming *plugin.GamingPlugin,
	birthday *plugin.BirthdayPlugin,
) {
	rooms := getRooms()

	// WOTD prefetch at 00:05
	c.AddFunc("5 0 * * *", func() {
		slog.Info("scheduler: prefetching WOTD")
		wotd.Prefetch()
	})

	// WOTD post at 08:00
	c.AddFunc("0 8 * * *", func() {
		slog.Info("scheduler: posting WOTD")
		for _, r := range rooms {
			wotd.PostWOTD(r)
		}
	})

	// Holidays at 07:00
	c.AddFunc("0 7 * * *", func() {
		slog.Info("scheduler: posting holidays")
		for _, r := range rooms {
			holidays.PostHolidays(r)
		}
	})

	// Game releases Monday 09:00
	c.AddFunc("0 9 * * 1", func() {
		slog.Info("scheduler: posting game releases")
		for _, r := range rooms {
			gaming.PostReleases(r)
		}
	})

	// Birthday check at 06:00
	c.AddFunc("0 6 * * *", func() {
		slog.Info("scheduler: checking birthdays")
		for _, r := range rooms {
			birthday.CheckAndPost(r)
		}
	})

	// Reminder polling every 30 seconds
	c.AddFunc("@every 30s", func() {
		plugin.FirePendingReminders(client)
	})
}

func getRooms() []id.RoomID {
	roomStr := os.Getenv("BROADCAST_ROOMS")
	if roomStr == "" {
		return nil
	}
	var rooms []id.RoomID
	for _, r := range splitAndTrim(roomStr, ",") {
		if r != "" {
			rooms = append(rooms, id.RoomID(r))
		}
	}
	return rooms
}

func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range splitStr(s, sep) {
		t := trimSpace(p)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return parts
}

func splitStr(s, sep string) []string {
	if s == "" {
		return nil
	}
	result := make([]string, 0)
	for {
		i := indexOf(s, sep)
		if i < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:i])
		s = s[i+len(sep):]
	}
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "(unknown)"
	}
	return wd
}

func envOr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
