package plugin

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// Embedded Go fonts
	_ "image/jpeg"

	"gogobee/internal/db"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

const (
	cartonWidth  = 400
	cartonHeight = 600
	photoSize    = 140
)

// MilkCartonPlugin generates milk carton style "missing person" images.
type MilkCartonPlugin struct {
	Base
	rateLimiter    *RateLimitsPlugin
	thresholdDays  int
	maxDays        int
	minMessages    int
	excludeUsers   map[string]bool
	regularFont    font.Face
	boldFont       font.Face
	smallFont      font.Face
	headerFont     font.Face
}

// NewMilkCartonPlugin creates a new milk carton plugin.
func NewMilkCartonPlugin(client *mautrix.Client, rateLimiter *RateLimitsPlugin) *MilkCartonPlugin {
	threshold := 14
	if v := os.Getenv("MISSING_THRESHOLD_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			threshold = n
		}
	}

	maxDays := 90
	if v := os.Getenv("MISSING_MAX_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxDays = n
		}
	}

	minMsgs := 10
	if v := os.Getenv("MISSING_MIN_MESSAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minMsgs = n
		}
	}

	exclude := make(map[string]bool)
	if v := os.Getenv("MISSING_EXCLUDE_USERS"); v != "" {
		for _, u := range strings.Split(v, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				exclude[u] = true
			}
		}
	}

	// Load embedded fonts
	regFont := loadFont(goregular.TTF, 16)
	bldFont := loadFont(gobold.TTF, 18)
	smlFont := loadFont(goregular.TTF, 13)
	hdrFont := loadFont(gobold.TTF, 22)

	return &MilkCartonPlugin{
		Base:          NewBase(client),
		rateLimiter:   rateLimiter,
		thresholdDays: threshold,
		maxDays:       maxDays,
		minMessages:   minMsgs,
		excludeUsers:  exclude,
		regularFont:   regFont,
		boldFont:      bldFont,
		smallFont:     smlFont,
		headerFont:    hdrFont,
	}
}

func loadFont(ttfData []byte, size float64) font.Face {
	f, err := opentype.Parse(ttfData)
	if err != nil {
		slog.Error("milkcarton: parse font", "err", err)
		return nil
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		slog.Error("milkcarton: create face", "err", err)
		return nil
	}
	return face
}

func (p *MilkCartonPlugin) Name() string { return "milkcarton" }

func (p *MilkCartonPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "haveyouseenthem", Description: "Generate a milk carton missing poster for a user", Usage: "!haveyouseenthem @user", Category: "Fun & Games"},
		{Name: "missing", Description: "List members who haven't posted recently", Usage: "!missing [post [@user]]", Category: "Fun & Games"},
	}
}

func (p *MilkCartonPlugin) Init() error { return nil }

func (p *MilkCartonPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *MilkCartonPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "haveyouseenthem") {
		return p.handleHaveYouSeenThem(ctx)
	}
	if p.IsCommand(ctx.Body, "missing") {
		return p.handleMissing(ctx)
	}
	return nil
}

func (p *MilkCartonPlugin) handleHaveYouSeenThem(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "haveyouseenthem"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !haveyouseenthem @user")
	}

	targetID, ok := p.ResolveUser(args, ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not find a user matching that name.")
	}

	// Rate limit: 1 carton per room per day
	if p.rateLimiter != nil && !p.rateLimiter.CheckLimit(id.UserID(ctx.RoomID), "milkcarton", 1) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Milk carton limit reached for today. Try again tomorrow.")
	}

	return p.generateAndPost(ctx.RoomID, targetID)
}

func (p *MilkCartonPlugin) handleMissing(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "missing"))

	if strings.HasPrefix(strings.ToLower(args), "post") {
		postArgs := strings.TrimSpace(args[4:])

		// Rate limit: 1 carton per room per day
		if p.rateLimiter != nil && !p.rateLimiter.CheckLimit(id.UserID(ctx.RoomID), "milkcarton", 1) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Milk carton limit reached for today. Try again tomorrow.")
		}

		if postArgs != "" {
			// !missing post @user
			targetID := id.UserID(strings.TrimPrefix(strings.TrimSpace(postArgs), "@"))
			return p.generateAndPost(ctx.RoomID, targetID)
		}

		// !missing post — generate for longest-absent member
		missing := p.getMissingMembers()
		if len(missing) == 0 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "No missing members found. Everyone's been active!")
		}
		return p.generateAndPost(ctx.RoomID, id.UserID(missing[0].userID))
	}

	// !missing — list missing members
	missing := p.getMissingMembers()
	if len(missing) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No missing members found. Everyone's been active!")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Missing members (inactive %d-%d days):\n\n", p.thresholdDays, p.maxDays))

	limit := 15
	if len(missing) < limit {
		limit = len(missing)
	}
	for i := 0; i < limit; i++ {
		m := missing[i]
		sb.WriteString(fmt.Sprintf("  %s — last seen %s\n", m.userID, humanDuration(m.daysSince)))
	}
	if len(missing) > limit {
		sb.WriteString(fmt.Sprintf("\n...and %d more", len(missing)-limit))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

type missingMember struct {
	userID    string
	daysSince int
}

func (p *MilkCartonPlugin) getMissingMembers() []missingMember {
	d := db.Get()
	now := time.Now().UTC()
	minDate := now.AddDate(0, 0, -p.maxDays).Format("2006-01-02")
	maxDate := now.AddDate(0, 0, -p.thresholdDays).Format("2006-01-02")

	rows, err := d.Query(`
		SELECT da.user_id, MAX(da.date) as last_date
		FROM daily_activity da
		JOIN user_stats us ON us.user_id = da.user_id
		WHERE us.total_messages >= ?
		GROUP BY da.user_id
		HAVING last_date >= ? AND last_date <= ?
		ORDER BY last_date ASC`,
		p.minMessages, minDate, maxDate,
	)
	if err != nil {
		slog.Error("milkcarton: query missing", "err", err)
		return nil
	}
	defer rows.Close()

	var result []missingMember
	for rows.Next() {
		var userID, lastDate string
		if err := rows.Scan(&userID, &lastDate); err != nil {
			continue
		}

		// Skip excluded users
		if p.excludeUsers[userID] {
			continue
		}

		// Skip bots (our own user ID)
		if id.UserID(userID) == p.Client.UserID {
			continue
		}

		// Skip users with active away/afk status
		var awayStatus int
		_ = d.QueryRow(`SELECT 1 FROM presence WHERE user_id = ? AND status IN ('away', 'afk')`, userID).Scan(&awayStatus)
		if awayStatus == 1 {
			continue
		}

		lastTime, err := time.Parse("2006-01-02", lastDate)
		if err != nil {
			continue
		}
		days := int(now.Sub(lastTime).Hours() / 24)
		result = append(result, missingMember{userID: userID, daysSince: days})
	}
	return result
}

func (p *MilkCartonPlugin) generateAndPost(roomID id.RoomID, targetID id.UserID) error {
	d := db.Get()
	uid := string(targetID)

	// Get display name
	displayName := uid
	resp, err := p.Client.GetDisplayName(context.Background(), targetID)
	if err == nil && resp.DisplayName != "" {
		displayName = resp.DisplayName
	}

	// Get last seen
	var lastDate string
	err = d.QueryRow(`SELECT MAX(date) FROM daily_activity WHERE user_id = ?`, uid).Scan(&lastDate)
	lastSeen := "Unknown"
	if err == nil && lastDate != "" {
		if t, err := time.Parse("2006-01-02", lastDate); err == nil {
			days := int(time.Since(t).Hours() / 24)
			lastSeen = humanDuration(days)
		}
	}

	// Get level and XP
	var xp, level int
	_ = d.QueryRow(`SELECT xp, level FROM users WHERE user_id = ?`, uid).Scan(&xp, &level)

	// Get characteristics
	characteristics := p.deriveCharacteristics(uid)

	// Get avatar
	avatarImg := p.fetchAvatar(targetID)

	// Generate milk carton image
	imgData, err := p.renderCarton(displayName, uid, lastSeen, level, xp, characteristics, avatarImg)
	if err != nil {
		slog.Error("milkcarton: render", "err", err)
		return p.SendReply(roomID, id.EventID(""), "Failed to generate milk carton image.")
	}

	caption := fmt.Sprintf("🥛 Has anyone seen %s lately?", displayName)
	return p.SendImage(roomID, imgData, "milkcarton.png", caption, cartonWidth, cartonHeight)
}

func (p *MilkCartonPlugin) fetchAvatar(userID id.UserID) image.Image {
	// Try Matrix avatar
	avatarURL, err := p.Client.GetAvatarURL(context.Background(), userID)
	if err == nil && !avatarURL.IsEmpty() {
		data, err := p.Client.DownloadBytes(context.Background(), avatarURL)
		if err == nil {
			img, _, err := image.Decode(bytes.NewReader(data))
			if err == nil {
				return img
			}
		}
	}

	// Try placeholder directory
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	placeholderDir := filepath.Join(dataDir, "placeholders")
	if entries, err := os.ReadDir(placeholderDir); err == nil && len(entries) > 0 {
		var images []string
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") {
				images = append(images, filepath.Join(placeholderDir, e.Name()))
			}
		}
		if len(images) > 0 {
			chosen := images[rand.IntN(len(images))]
			f, err := os.Open(chosen)
			if err == nil {
				defer f.Close()
				img, _, err := image.Decode(f)
				if err == nil {
					return img
				}
			}
		}
	}

	// Fallback: nil — renderCarton will draw a silhouette
	return nil
}

func (p *MilkCartonPlugin) renderCarton(
	displayName, userID, lastSeen string,
	level, xp int,
	characteristics []string,
	avatar image.Image,
) ([]byte, error) {
	dc := gg.NewContext(cartonWidth, cartonHeight)

	// Background — cream/off-white like a milk carton
	dc.SetColor(color.RGBA{255, 253, 245, 255})
	dc.Clear()

	// Border
	dc.SetColor(color.RGBA{180, 60, 60, 255})
	dc.SetLineWidth(4)
	dc.DrawRoundedRectangle(8, 8, float64(cartonWidth-16), float64(cartonHeight-16), 12)
	dc.Stroke()

	// Inner border
	dc.SetColor(color.RGBA{200, 80, 80, 255})
	dc.SetLineWidth(1.5)
	dc.DrawRoundedRectangle(16, 16, float64(cartonWidth-32), float64(cartonHeight-32), 8)
	dc.Stroke()

	// Header: "HAVE YOU SEEN THIS PERSON?"
	if p.headerFont != nil {
		dc.SetFontFace(p.headerFont)
	}
	dc.SetColor(color.RGBA{180, 30, 30, 255})
	dc.DrawStringAnchored("HAVE YOU SEEN", float64(cartonWidth)/2, 48, 0.5, 0.5)
	dc.DrawStringAnchored("THIS PERSON?", float64(cartonWidth)/2, 74, 0.5, 0.5)

	// Photo area
	photoX := float64(cartonWidth)/2 - float64(photoSize)/2
	photoY := 95.0

	// Photo border
	dc.SetColor(color.RGBA{160, 50, 50, 255})
	dc.SetLineWidth(2)
	dc.DrawRectangle(photoX-3, photoY-3, float64(photoSize)+6, float64(photoSize)+6)
	dc.Stroke()

	// Photo background
	dc.SetColor(color.RGBA{230, 225, 215, 255})
	dc.DrawRectangle(photoX, photoY, float64(photoSize), float64(photoSize))
	dc.Fill()

	if avatar != nil {
		// Resize and crop avatar to fit photo area
		avatarResized := resizeImage(avatar, photoSize, photoSize)
		dc.DrawImage(avatarResized, int(photoX), int(photoY))
	} else {
		// Draw silhouette
		drawSilhouette(dc, photoX, photoY, float64(photoSize))
	}

	// Name
	yPos := photoY + float64(photoSize) + 25
	if p.boldFont != nil {
		dc.SetFontFace(p.boldFont)
	}
	dc.SetColor(color.RGBA{40, 40, 40, 255})

	// Truncate display name if too long
	name := displayName
	if len(name) > 28 {
		name = name[:25] + "..."
	}
	dc.DrawStringAnchored(name, float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// User ID
	yPos += 22
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.SetColor(color.RGBA{100, 100, 100, 255})
	shortID := userID
	if len(shortID) > 35 {
		shortID = shortID[:32] + "..."
	}
	dc.DrawStringAnchored(shortID, float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Last seen
	yPos += 28
	if p.regularFont != nil {
		dc.SetFontFace(p.regularFont)
	}
	dc.SetColor(color.RGBA{180, 30, 30, 255})
	dc.DrawStringAnchored(fmt.Sprintf("Last seen: %s", lastSeen), float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Level / XP
	yPos += 22
	dc.SetColor(color.RGBA{80, 80, 80, 255})
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.DrawStringAnchored(fmt.Sprintf("Level %d | %d XP", level, xp), float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Divider
	yPos += 18
	dc.SetColor(color.RGBA{200, 80, 80, 180})
	dc.SetLineWidth(1)
	dc.DrawLine(40, yPos, float64(cartonWidth)-40, yPos)
	dc.Stroke()

	// Characteristics header
	yPos += 20
	if p.boldFont != nil {
		dc.SetFontFace(p.boldFont)
	}
	dc.SetColor(color.RGBA{60, 60, 60, 255})
	dc.DrawStringAnchored("Distinguishing Characteristics", float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Characteristics
	yPos += 8
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.SetColor(color.RGBA{80, 80, 80, 255})
	for _, c := range characteristics {
		yPos += 18
		dc.DrawStringAnchored(fmt.Sprintf("• %s", c), float64(cartonWidth)/2, yPos, 0.5, 0.5)
	}

	// Footer
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.SetColor(color.RGBA{140, 50, 50, 255})
	dc.DrawStringAnchored("If found, please return to the community", float64(cartonWidth)/2, float64(cartonHeight)-42, 0.5, 0.5)

	dc.SetColor(color.RGBA{180, 180, 180, 255})
	dc.DrawStringAnchored("🥛 milk carton community alert 🥛", float64(cartonWidth)/2, float64(cartonHeight)-24, 0.5, 0.5)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// deriveCharacteristics generates flavor text from user stats.
func (p *MilkCartonPlugin) deriveCharacteristics(userID string) []string {
	d := db.Get()
	var chars []string

	var totalMsgs, totalWords, totalEmojis, totalQuestions, totalLinks int
	err := d.QueryRow(
		`SELECT COALESCE(total_messages,0), COALESCE(total_words,0), COALESCE(total_emojis,0),
		        COALESCE(total_questions,0), COALESCE(total_links,0)
		 FROM user_stats WHERE user_id = ?`, userID,
	).Scan(&totalMsgs, &totalWords, &totalEmojis, &totalQuestions, &totalLinks)
	if err != nil {
		return []string{"Whereabouts unknown", "Considered a person of interest"}
	}

	avgWords := 0
	if totalMsgs > 0 {
		avgWords = totalWords / totalMsgs
	}

	// Sentiment data
	var positive, negative, sarcastic, humorous int
	_ = d.QueryRow(
		`SELECT COALESCE(positive,0), COALESCE(negative,0), COALESCE(sarcastic,0), COALESCE(humorous,0)
		 FROM sentiment_stats WHERE user_id = ?`, userID,
	).Scan(&positive, &negative, &sarcastic, &humorous)

	// Profanity
	var profanityCount int
	_ = d.QueryRow(`SELECT COALESCE(count,0) FROM potty_mouth WHERE user_id = ?`, userID).Scan(&profanityCount)

	// Build characteristics based on stat thresholds
	type trait struct {
		condition bool
		text      string
	}

	traits := []trait{
		{totalMsgs > 1000, "Known to be extremely chatty"},
		{totalMsgs > 500 && totalMsgs <= 1000, "Considered a regular contributor"},
		{totalMsgs < 50 && totalMsgs > 0, "Frequently lurks, rarely commits"},
		{avgWords > 12, "Known to post novellas"},
		{avgWords > 0 && avgWords <= 3, "A person of few words"},
		{totalQuestions > totalMsgs/4 && totalQuestions > 20, "Asks questions compulsively"},
		{totalEmojis > totalMsgs/3 && totalEmojis > 30, "Communicates primarily in emoji"},
		{totalLinks > totalMsgs/8 && totalLinks > 20, "Prone to sharing unsolicited links"},
		{profanityCount > 100, "Has a mouth that could strip paint"},
		{profanityCount > 30, "Known to use colorful language"},
		{sarcastic > positive && sarcastic > 10, "Armed with weapons-grade sarcasm"},
		{humorous > 20, "Considered dangerously funny"},
		{negative > positive && negative > 15, "Last seen expressing strong opinions"},
		{positive > 50, "Generally considered a ray of sunshine"},
	}

	for _, t := range traits {
		if t.condition {
			chars = append(chars, t.text)
		}
		if len(chars) >= 3 {
			break
		}
	}

	// Fallbacks
	fallbacks := []string{
		"Considered armed with strong opinions",
		"May be found near a keyboard",
		"Last seen staring at a screen",
		"Approach with memes",
	}
	for len(chars) < 2 {
		chars = append(chars, fallbacks[rand.IntN(len(fallbacks))])
	}

	return chars
}

// resizeImage scales an image to fit within the target dimensions.
func resizeImage(src image.Image, targetW, targetH int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// Calculate scale to fill target (crop to fit)
	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)
	scale := math.Max(scaleX, scaleY)

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	dc := gg.NewContext(targetW, targetH)
	dc.DrawImage(src, (targetW-newW)/2, (targetH-newH)/2)

	// Use gg's built-in scaling — anchor to top so heads aren't cropped
	dcScaled := gg.NewContext(targetW, targetH)
	dcScaled.Scale(scale, scale)
	offsetX := -float64(srcW)/2 + float64(targetW)/(2*scale)
	offsetY := 0.0 // top-anchored crop
	dcScaled.DrawImage(src, int(offsetX), int(offsetY))

	return dcScaled.Image()
}

func humanDuration(days int) string {
	if days == 0 {
		return "today"
	}
	if days == 1 {
		return "yesterday"
	}
	if days < 7 {
		return fmt.Sprintf("%d days ago", days)
	}
	weeks := days / 7
	if weeks == 1 {
		return "1 week ago"
	}
	if days < 30 {
		return fmt.Sprintf("%d weeks ago", weeks)
	}
	months := days / 30
	if months == 1 {
		return "1 month ago"
	}
	return fmt.Sprintf("%d months ago", months)
}
