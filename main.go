package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gogobee/internal/bot"
	"gogobee/internal/db"
	"gogobee/internal/dreamclient"
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

	// Moderation (runs first in dispatch order)
	modPlugin := plugin.NewModerationPlugin(client)
	registry.Register(modPlugin)

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
	registry.Register(plugin.NewFunPlugin(client, ratePlugin))
	registry.Register(plugin.NewToolsPlugin(client))
	registry.Register(plugin.NewUserPlugin(client))

	// Dictionary service
	var dictClient *dreamclient.Client
	if dictURL := os.Getenv("DREAMDICT_URL"); dictURL != "" {
		dictClient = dreamclient.New(dictURL)
		if h, err := dictClient.Health(); err != nil {
			slog.Warn("dreamdict not reachable — dictionary features degraded", "url", dictURL, "err", err)
		} else {
			slog.Info("dreamdict connected", "url", dictURL, "words_en", h.WordCounts["en"])
		}
	} else {
		slog.Warn("DREAMDICT_URL not set — dictionary features disabled")
	}

	// Entertainment / Lookup
	registry.Register(plugin.NewRetroPlugin(client))
	registry.Register(plugin.NewLookupPlugin(client, ratePlugin, dictClient))
	registry.Register(plugin.NewCountdownPlugin(client))
	registry.Register(plugin.NewStocksPlugin(client))
	forexPlugin := plugin.NewForexPlugin(client)
	registry.Register(forexPlugin)
	concertsPlugin := plugin.NewConcertsPlugin(client, ratePlugin)
	registry.Register(concertsPlugin)
	animePlugin := plugin.NewAnimePlugin(client)
	registry.Register(animePlugin)
	moviesPlugin := plugin.NewMoviesPlugin(client)
	registry.Register(moviesPlugin)

	// Games & Economy
	euroPlugin := plugin.NewEuroPlugin(client)
	registry.Register(euroPlugin)
	registry.Register(plugin.NewFlipPlugin(client))
	registry.Register(plugin.NewHangmanPlugin(client, euroPlugin, dictClient))
	registry.Register(plugin.NewBlackjackPlugin(client, euroPlugin))
	registry.Register(plugin.NewUnoPlugin(client, euroPlugin))
	registry.Register(plugin.NewHoldemPlugin(client, euroPlugin))
	adventurePlugin := plugin.NewAdventurePlugin(client, euroPlugin)
	registry.Register(adventurePlugin)
	wordlePlugin := plugin.NewWordlePlugin(client, euroPlugin, dictClient)
	registry.Register(wordlePlugin)

	// Community
	registry.Register(plugin.NewMilkCartonPlugin(client, ratePlugin))
	registry.Register(plugin.NewQuoteWallPlugin(client, ratePlugin))
	registry.Register(plugin.NewTarotPlugin(client, ratePlugin))

	// Tracking (passive)
	achievementsPlugin := plugin.NewAchievementsPlugin(client, registry)
	registry.Register(achievementsPlugin)
	adventurePlugin.SetAchievements(achievementsPlugin)
	registry.Register(plugin.NewReactionsPlugin(client))
	registry.Register(plugin.NewMarkovPlugin(client))
	registry.Register(plugin.NewURLsPlugin(client))

	// Automation
	minifluxPlugin := plugin.NewMinifluxPlugin(client)
	plugin.RegisterMinifluxPlugin(minifluxPlugin)
	registry.Register(minifluxPlugin)

	// LLM-powered (passive)
	registry.Register(plugin.NewLLMPassivePlugin(client, xpPlugin))

	// Scheduled
	wotdPlugin := plugin.NewWOTDPlugin(client, dictClient)
	registry.Register(wotdPlugin)
	holidaysPlugin := plugin.NewHolidaysPlugin(client)
	registry.Register(holidaysPlugin)
	gamingPlugin := plugin.NewGamingPlugin(client)
	registry.Register(gamingPlugin)
	birthdayPlugin := plugin.NewBirthdayPlugin(client, xpPlugin, euroPlugin)
	registry.Register(birthdayPlugin)

	// Satirical
	esteemedPlugin := plugin.NewEsteemPlugin(client)
	registry.Register(esteemedPlugin)

	// Horoscope
	horoscopePlugin := plugin.NewHoroscopePlugin(client)
	registry.Register(horoscopePlugin)

	// Finance — Market overview
	marketPlugin := plugin.NewMarketPlugin(client)
	registry.Register(marketPlugin)

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

	// Initialize space groups (room overlap detection for community-wide leaderboards)
	plugin.InitSpaceGroups(client)

	// ---- Set up event handlers ----

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// Auto-join on invite + moderation member tracking
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("member event handler panic", "panic", rec, "room", evt.RoomID)
			}
		}()

		mem := evt.Content.AsMember()
		if mem == nil {
			return
		}

		// Auto-join invites for the bot
		if evt.GetStateKey() == string(client.UserID) {
			if mem.Membership == event.MembershipInvite {
				_, err := client.JoinRoomByID(ctx, evt.RoomID)
				if err != nil {
					slog.Error("failed to join room", "room", evt.RoomID, "err", err)
				} else {
					slog.Info("joined room", "room", evt.RoomID)
				}
			}
			return
		}

		// Track join/leave/invite for moderation
		targetUser := id.UserID(evt.GetStateKey())
		modPlugin.OnMemberEvent(evt.RoomID, targetUser, mem.Membership)
	})

	// Message handler
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("message event handler panic", "panic", rec,
					"sender", evt.Sender, "room", evt.RoomID)
			}
		}()

		// Skip own messages
		if evt.Sender == client.UserID {
			return
		}

		content := evt.Content.AsMessage()
		if content == nil || content.Body == "" {
			return
		}

		// Ignore edits — they arrive as m.room.message with m.replace relation.
		// Without this check, edits re-trigger URL previews and inflate stats.
		if content.RelatesTo != nil && content.RelatesTo.Type == event.RelReplace {
			return
		}

		body := content.Body
		// Strip Matrix reply fallback: "> <@user:server> ..." lines followed by blank line
		if strings.HasPrefix(body, "> <@") || strings.HasPrefix(body, "> * <@") {
			if idx := strings.Index(body, "\n\n"); idx >= 0 {
				body = strings.TrimSpace(body[idx+2:])
			}
		}
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
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("reaction event handler panic", "panic", rec,
					"sender", evt.Sender, "room", evt.RoomID)
			}
		}()

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
	scheduler := cron.New(cron.WithChain(cron.Recover(cronLogger{})))
	setupScheduledJobs(scheduler, client, wotdPlugin, holidaysPlugin, gamingPlugin, birthdayPlugin, animePlugin, moviesPlugin, concertsPlugin, esteemedPlugin, forexPlugin, minifluxPlugin, marketPlugin)
	scheduler.Start()

	// ---- Initial archetype calculation ----
	go plugin.RefreshAllArchetypes()

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
		client.StopSync()
		cancel()
	}()

syncLoop:
	for {
		err := client.SyncWithContext(ctx)
		if ctx.Err() != nil {
			break // shutdown requested
		}
		if err != nil {
			slog.Error("sync stopped, restarting in 5s", "err", err)
		} else {
			slog.Warn("sync returned without error, restarting in 5s")
		}
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			break syncLoop
		}
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
	anime *plugin.AnimePlugin,
	movies *plugin.MoviesPlugin,
	concerts *plugin.ConcertsPlugin,
	esteemed *plugin.EsteemPlugin,
	forex *plugin.ForexPlugin,
	miniflux *plugin.MinifluxPlugin,
	market *plugin.MarketPlugin,
) {
	rooms := getRooms()

	// Prefetch at 00:05 — grab data ahead of scheduled posts
	c.AddFunc("5 0 * * *", func() {
		slog.Info("scheduler: prefetching daily data")
		wotd.Prefetch()
		holidays.Prefetch()
	})

	// Birthday check at 06:00
	c.AddFunc("0 6 * * *", func() {
		slog.Info("scheduler: checking birthdays")
		for _, r := range rooms {
			birthday.CheckAndPost(r)
		}
	})

	// Holidays at 07:00
	c.AddFunc("0 7 * * *", func() {
		slog.Info("scheduler: posting holidays")
		for _, r := range rooms {
			holidays.PostHolidays(r)
		}
	})

	// WOTD post at 08:00
	c.AddFunc("0 8 * * *", func() {
		slog.Info("scheduler: posting WOTD")
		for _, r := range rooms {
			wotd.PostWOTD(r)
		}
	})

	// Game releases Monday 09:00
	c.AddFunc("0 9 * * 1", func() {
		slog.Info("scheduler: posting game releases")
		for _, r := range rooms {
			gaming.PostReleases(r)
		}
	})

	// Anime releases at 10:00
	c.AddFunc("0 10 * * *", func() {
		slog.Info("scheduler: posting anime releases")
		for _, r := range rooms {
			anime.PostDailyReleases(r)
		}
	})

	// Movie releases at 11:00
	c.AddFunc("0 11 * * *", func() {
		slog.Info("scheduler: posting movie releases")
		for _, r := range rooms {
			movies.PostDailyReleases(r)
		}
	})

	// Concert digest Sunday 12:00
	c.AddFunc("0 12 * * 0", func() {
		slog.Info("scheduler: posting concert digest")
		for _, r := range rooms {
			concerts.PostWeeklyDigest(r)
		}
	})

	// Reminder polling every 30 seconds
	c.AddFunc("@every 30s", func() {
		plugin.FirePendingReminders(client)
	})

	// Esteemed community member — Wednesday & Sunday 13:00
	c.AddFunc("0 13 * * 0,3", func() {
		slog.Info("scheduler: posting esteemed member")
		esteemed.PostWeekly()
	})

	// Forex daily poll at 17:01 UTC (ECB publishes ~16:00 CET)
	c.AddFunc("1 17 * * *", func() {
		slog.Info("scheduler: forex daily poll")
		forex.DailyPoll()
	})

	// Market data daily pull at 23:00 UTC (after all target markets close)
	c.AddFunc("0 23 * * *", func() {
		slog.Info("scheduler: market daily pull")
		market.DailyPull()
	})

	// Space groups refresh every hour
	c.AddFunc("0 * * * *", func() {
		slog.Info("scheduler: refreshing space groups")
		plugin.RefreshSpaceGroups()
	})

	// Markov corpus TTL purge at 03:30 daily
	c.AddFunc("30 3 * * *", func() {
		slog.Info("scheduler: purging expired markov entries")
		plugin.MarkovPurgeExpired()
	})

	// Archetype refresh at 04:30 daily
	c.AddFunc("30 4 * * *", func() {
		slog.Info("scheduler: refreshing archetypes")
		plugin.RefreshAllArchetypes()
	})

	// Miniflux RSS polling
	if miniflux != nil {
		interval := fmt.Sprintf("@every %dm", miniflux.PollInterval())
		c.AddFunc(interval, func() {
			plugin.MinifluxPoll(client)
		})
	}

	// Database maintenance at 03:00 daily
	c.AddFunc("0 3 * * *", func() {
		slog.Info("scheduler: running database maintenance")
		db.RunMaintenance()
		plugin.MinifluxPurgeSeen()
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

// cronLogger adapts slog to the cron.Logger interface for panic recovery logging.
type cronLogger struct{}

func (cronLogger) Info(msg string, keysAndValues ...interface{}) {
	slog.Info(msg, keysAndValues...)
}

func (cronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	args := append([]interface{}{"err", err}, keysAndValues...)
	slog.Error(msg, args...)
}
