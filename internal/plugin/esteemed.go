package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"

	_ "golang.org/x/image/webp"

	"gogobee/internal/db"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// esteemedEntry is a single satirical "esteemed community member" entry.
type esteemedEntry struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Category        string   `json:"category"`
	ImageURL        string   `json:"image_url"`
	Characteristics []string `json:"distinguishing_characteristics"`
}

// EsteemPlugin posts satirical milk carton missing posters for fictional "esteemed community members."
type EsteemPlugin struct {
	Base
	entries     []esteemedEntry
	enabled     bool
	room        id.RoomID
	dataDir     string
	regularFont font.Face
	boldFont    font.Face
	smallFont   font.Face
	headerFont  font.Face
	titleFont   font.Face
}

// NewEsteemPlugin creates a new esteemed members plugin.
func NewEsteemPlugin(client *mautrix.Client) *EsteemPlugin {
	enabled := os.Getenv("FEATURE_ESTEEMED") != ""
	roomID := os.Getenv("ESTEEMED_ROOM")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	p := &EsteemPlugin{
		Base:    NewBase(client),
		enabled: enabled,
		room:    id.RoomID(roomID),
		dataDir: dataDir,
	}

	if enabled {
		p.regularFont = loadFont(goregular.TTF, 15)
		p.boldFont = loadFont(gobold.TTF, 16)
		p.smallFont = loadFont(goregular.TTF, 12)
		p.headerFont = loadFont(gobold.TTF, 14)
		p.titleFont = loadFont(gobold.TTF, 20)
	}

	return p
}

func (p *EsteemPlugin) Name() string { return "esteemed" }

func (p *EsteemPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "esteemed", Description: "Preview a random esteemed member carton (admin only)", Usage: "!esteemed [name]", Category: "Admin", AdminOnly: true},
	}
}

func (p *EsteemPlugin) Init() error {
	if !p.enabled {
		return nil
	}

	// Load entries from esteemed.json
	jsonPath := "esteemed.json"
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		slog.Warn("esteemed: could not load esteemed.json", "err", err)
		return nil
	}

	if err := json.Unmarshal(data, &p.entries); err != nil {
		slog.Warn("esteemed: invalid JSON", "err", err)
		return nil
	}

	slog.Info("esteemed: loaded entries", "count", len(p.entries))
	return nil
}

func (p *EsteemPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "esteemed") {
		return nil
	}
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}
	if len(p.entries) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Esteemed members list not loaded.")
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "esteemed"))

	var entry esteemedEntry
	if args == "" {
		// Random entry (ignores dedup — this is a preview)
		entry = p.entries[rand.IntN(len(p.entries))]
	} else {
		// Search by name
		query := strings.ToLower(args)
		found := false
		for _, e := range p.entries {
			if strings.Contains(strings.ToLower(e.Name), query) || e.ID == query {
				entry = e
				found = true
				break
			}
		}
		if !found {
			return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No esteemed member matching \"%s\" found.", args))
		}
	}

	imgData, err := p.renderCarton(entry)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Render failed: %v", err))
	}

	caption := fmt.Sprintf("Yet another one of our esteemed community has gone missing.\nIf found, please return %s to the community immediately.", entry.Name)
	return p.SendImage(ctx.RoomID, imgData, "esteemed.png", caption, cartonWidth, cartonHeight)
}
func (p *EsteemPlugin) OnReaction(_ ReactionContext) error { return nil }

// PostWeekly selects an unposted entry, generates a milk carton, and posts it.
// Called by the scheduler once a week.
func (p *EsteemPlugin) PostWeekly() {
	if !p.enabled || p.room == "" || len(p.entries) == 0 {
		return
	}

	// Pick an entry that hasn't been posted yet
	entry, ok := p.pickEntry()
	if !ok {
		slog.Info("esteemed: all entries have been posted")
		return
	}

	imgData, err := p.renderCarton(entry)
	if err != nil {
		slog.Error("esteemed: render failed", "entry", entry.ID, "err", err)
		return
	}

	caption := fmt.Sprintf("Yet another one of our esteemed community has gone missing.\nIf found, please return %s to the community immediately.", entry.Name)
	if err := p.SendImage(p.room, imgData, "esteemed.png", caption, cartonWidth, cartonHeight); err != nil {
		slog.Error("esteemed: send failed", "entry", entry.ID, "err", err)
		return
	}

	// Mark as posted
	db.MarkJobCompleted("esteemed", entry.ID)
	slog.Info("esteemed: posted", "entry", entry.ID, "name", entry.Name)
}

func (p *EsteemPlugin) pickEntry() (esteemedEntry, bool) {
	// Gather unposted entries
	var candidates []esteemedEntry
	for _, e := range p.entries {
		if !db.JobCompleted("esteemed", e.ID) {
			candidates = append(candidates, e)
		}
	}

	if len(candidates) == 0 {
		return esteemedEntry{}, false
	}

	// Pick a random one
	return candidates[rand.IntN(len(candidates))], true
}

func (p *EsteemPlugin) loadEntryImage(entry esteemedEntry) image.Image {
	imgPath := filepath.Join(p.dataDir, "esteemed", entry.ID+".jpg")
	f, err := os.Open(imgPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		slog.Warn("esteemed: decode image", "entry", entry.ID, "err", err)
		return nil
	}
	return img
}

func (p *EsteemPlugin) renderCarton(entry esteemedEntry) ([]byte, error) {
	dc := gg.NewContext(cartonWidth, cartonHeight)

	// Background — cream/off-white
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

	// Header: "ESTEEMED COMMUNITY MEMBER"
	if p.headerFont != nil {
		dc.SetFontFace(p.headerFont)
	}
	dc.SetColor(color.RGBA{180, 30, 30, 255})
	dc.DrawStringAnchored("ESTEEMED COMMUNITY MEMBER", float64(cartonWidth)/2, 40, 0.5, 0.5)

	// Sub-header: "MISSING"
	if p.titleFont != nil {
		dc.SetFontFace(p.titleFont)
	}
	dc.DrawStringAnchored("MISSING", float64(cartonWidth)/2, 66, 0.5, 0.5)

	// Photo area
	photoX := float64(cartonWidth)/2 - float64(photoSize)/2
	photoY := 85.0

	// Photo border
	dc.SetColor(color.RGBA{160, 50, 50, 255})
	dc.SetLineWidth(2)
	dc.DrawRectangle(photoX-3, photoY-3, float64(photoSize)+6, float64(photoSize)+6)
	dc.Stroke()

	// Photo background
	dc.SetColor(color.RGBA{230, 225, 215, 255})
	dc.DrawRectangle(photoX, photoY, float64(photoSize), float64(photoSize))
	dc.Fill()

	avatar := p.loadEntryImage(entry)
	if avatar != nil {
		avatarResized := resizeImage(avatar, photoSize, photoSize)
		dc.DrawImage(avatarResized, int(photoX), int(photoY))
	} else {
		// Silhouette for missing images
		drawSilhouette(dc, photoX, photoY, float64(photoSize))
	}

	// Name
	yPos := photoY + float64(photoSize) + 25
	if p.boldFont != nil {
		dc.SetFontFace(p.boldFont)
	}
	dc.SetColor(color.RGBA{40, 40, 40, 255})

	name := entry.Name
	if len(name) > 30 {
		name = name[:27] + "..."
	}
	dc.DrawStringAnchored(name, float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Category
	yPos += 20
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.SetColor(color.RGBA{120, 100, 100, 255})
	catDisplay := formatCategory(entry.Category)
	dc.DrawStringAnchored(catDisplay, float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// "Last seen" with a random funny timeframe
	yPos += 24
	if p.regularFont != nil {
		dc.SetFontFace(p.regularFont)
	}
	dc.SetColor(color.RGBA{180, 30, 30, 255})
	lastSeen := randomLastSeen()
	dc.DrawStringAnchored(fmt.Sprintf("Last seen: %s", lastSeen), float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Divider
	yPos += 18
	dc.SetColor(color.RGBA{200, 80, 80, 180})
	dc.SetLineWidth(1)
	dc.DrawLine(40, yPos, float64(cartonWidth)-40, yPos)
	dc.Stroke()

	// Characteristics header
	yPos += 18
	if p.boldFont != nil {
		dc.SetFontFace(p.boldFont)
	}
	dc.SetColor(color.RGBA{60, 60, 60, 255})
	dc.DrawStringAnchored("Distinguishing Characteristics", float64(cartonWidth)/2, yPos, 0.5, 0.5)

	// Characteristics
	yPos += 6
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.SetColor(color.RGBA{80, 80, 80, 255})

	maxItems := 4
	if len(entry.Characteristics) < maxItems {
		maxItems = len(entry.Characteristics)
	}
	maxWidth := float64(cartonWidth) - 60 // padding on each side
	for i := 0; i < maxItems; i++ {
		wrapped := dc.WordWrap(entry.Characteristics[i], maxWidth)
		for _, wline := range wrapped {
			yPos += 16
			if yPos > float64(cartonHeight)-55 {
				break
			}
			dc.DrawStringAnchored(wline, float64(cartonWidth)/2, yPos, 0.5, 0.5)
		}
	}

	// Footer
	if p.smallFont != nil {
		dc.SetFontFace(p.smallFont)
	}
	dc.SetColor(color.RGBA{140, 50, 50, 255})
	dc.DrawStringAnchored("If found, please return to our", float64(cartonWidth)/2, float64(cartonHeight)-48, 0.5, 0.5)
	dc.DrawStringAnchored("community for \"cuddles\"", float64(cartonWidth)/2, float64(cartonHeight)-34, 0.5, 0.5)

	// Encode
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatCategory(cat string) string {
	replacer := strings.NewReplacer(
		"tech_grifters_and_crypto", "Tech & Crypto Division",
		"politicians", "Political Affairs Bureau",
		"corporate_villains", "Corporate Relations Dept.",
		"reality_tv_royalty", "Entertainment Division",
		"internet_infamous", "Digital Community Outreach",
		"fictional_characters", "Literary & Media Wing",
		"historical_figures", "Heritage Society",
	)
	result := replacer.Replace(cat)
	if result == cat {
		// Fallback: replace underscores and capitalize first letter
		s := strings.ReplaceAll(cat, "_", " ")
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return s
	}
	return result
}

func randomLastSeen() string {
	options := []string{
		"fleeing the group chat",
		"updating their LinkedIn bio",
		"somewhere near a microphone",
		"posting without reading the room",
		"starting a new venture",
		"giving unsolicited advice",
		"somewhere on the timeline",
		"rebranding after the incident",
		"in the replies, unfortunately",
		"near a camera crew",
		"drafting a press release",
		"pivoting to their next opportunity",
		"explaining why it was taken out of context",
		"launching a podcast",
		"signing autographs for no one",
	}
	return options[rand.IntN(len(options))]
}

// drawSilhouette draws a generic person silhouette. Extracted as a shared helper.
func drawSilhouette(dc *gg.Context, x, y, size float64) {
	cx := x + size/2
	cy := y + size/2

	dc.SetColor(color.RGBA{170, 165, 155, 255})

	// Head
	headRadius := size * 0.18
	dc.DrawCircle(cx, cy-size*0.12, headRadius)
	dc.Fill()

	// Body
	dc.DrawEllipse(cx, cy+size*0.22, size*0.28, size*0.25)
	dc.Fill()
}
